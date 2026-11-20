// Package outlookids moves an Outlook account's cache from Graph's default
// message IDs to its immutable ones.
//
// Graph's default ID changes whenever a message moves to another folder, so
// every archive or filing orphaned what the cache keeps by ID: the replied
// flag, the spam override, the cached body. An immutable ID survives a move.
// Asking for immutable IDs is one request header, but a cache keyed by the old
// IDs no longer matches anything the server returns, so the cache is rebuilt
// once, here.
//
// The rebuild keeps what cannot be fetched again and lets the rest be
// re-derived. Each cached body is copied under its new ID with the user's
// override, the scan's evidence and any LLM summary; the read, replied and
// pending-read state, the training ledgers, the saved result sets and the
// watcher's seen IDs are renamed. The folder indexes are left to the next
// listing. The new cache is built beside the old one and swapped in by rename,
// and the old one is kept, renamed, as the fallback.
package outlookids

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/sarielhp/feral/cache"
	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/cache/dirlock"
	"github.com/sarielhp/feral/cache/meta"
	"github.com/sarielhp/feral/cache/msg"
	"github.com/sarielhp/feral/cache/virtual"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/train"
	"github.com/sarielhp/feral/watcher"
)

// Translator maps default Graph IDs to immutable ones. An ID it cannot
// translate -- the message is gone -- is left out of the result; an error
// means the translation itself failed, and nothing may be changed.
type Translator func(ctx context.Context, ids []email.ID) (map[email.ID]email.ID, error)

// Options names the account to migrate.
type Options struct {
	Account     cachedir.Account // the account's cache
	AccountName string           // the account, as the watcher keys it
	CacheRoot   string           // the download directory, where the watcher keeps its state
	Translate   Translator
	// Verify proves on the account that Translate gives the IDs Graph
	// itself lists, before anything is built from it. Optional.
	Verify func(context.Context) error
	Now    func() time.Time
	Build  string // this build's version, recorded in the manifest
}

// Count is a number before and after the migration.
type Count struct{ Before, After int }

// Report says what Migrate did.
type Report struct {
	AlreadyMigrated bool // the cache was keyed by immutable IDs already
	Empty           bool // the cache held no messages, and was only marked
	IDs             int  // distinct IDs found in the cache
	Translated      int  // of those, how many the server translated
	Messages        msg.RekeyReport
	FlagsCarried    int
	FlagsDropped    int
	SpamLedger      Count
	HamLedger       Count
	SavedSets       int
	WatcherErr      error  // the watcher's seen IDs could not be renamed
	Aside           string // where the old cache was kept
}

// ErrSpliceInFlight refuses a migration while a cross-account move is half
// done: its journal names messages by the old IDs, and losing it would leave
// a duplicate behind.
var ErrSpliceInFlight = errors.New("an interrupted splice is recorded for this account; finish it (rerun the splice) before migrating")

const (
	stagingSuffix = ".immutable-staging"
	asideSuffix   = ".pre-immutable-"
)

// Migrate rebuilds the account's cache keyed by immutable IDs. It refuses
// while a watcher runs, and changes nothing when the translation fails. A run
// interrupted after the new cache was complete is finished by the next one.
func Migrate(ctx context.Context, opts Options) (Report, error) {
	var rep Report
	live := opts.Account
	if live.IsZero() {
		return rep, errors.New("no account cache")
	}
	staging := live.Path() + stagingSuffix
	if done, err := resumeSwap(live.Path(), staging); done || err != nil {
		rep.AlreadyMigrated = done
		return rep, err
	}
	if meta.ImmutableIDs(live) {
		rep.AlreadyMigrated = true
		return rep, nil
	}
	if !meta.HoldsMessages(live) {
		rep.Empty = true
		return rep, meta.MarkImmutableIDs(live, opts.Build)
	}
	if spliceInFlight(live) {
		return rep, ErrSpliceInFlight
	}
	releaseWatch, err := watcher.HoldWatchLock(opts.CacheRoot)
	if err != nil {
		return rep, fmt.Errorf("close every feral TUI and `feral watch` first: %w", err)
	}
	defer releaseWatch()
	release, err := dirlock.Guard(live.Path())
	if release != nil {
		defer release()
	}
	if err != nil {
		return rep, err
	}
	return migrateLocked(ctx, opts, staging, rep)
}

func migrateLocked(ctx context.Context, opts Options, staging string, rep Report) (Report, error) {
	live := opts.Account
	ids, err := collectIDs(live, opts)
	if err != nil {
		return rep, err
	}
	rep.IDs = len(ids)
	if opts.Verify != nil {
		if verifyErr := opts.Verify(ctx); verifyErr != nil {
			return rep, fmt.Errorf("nothing was changed: %w", verifyErr)
		}
	}
	rename, err := opts.Translate(ctx, ids)
	if err != nil {
		return rep, fmt.Errorf("translating message IDs: %w", err)
	}
	rep.Translated = len(rename)
	if len(ids) > 0 && len(rename) == 0 {
		return rep, errors.New("the server translated none of the cached message IDs; nothing was changed")
	}
	if err := os.RemoveAll(staging); err != nil {
		return rep, err
	}
	if err := buildStaging(live, cachedir.At(staging), rename, opts.Build, &rep); err != nil {
		_ = os.RemoveAll(staging)
		return rep, err
	}
	now := time.Now
	if opts.Now != nil {
		now = opts.Now
	}
	rep.Aside = live.Path() + asideSuffix + now().Format("20060102-150405")
	if err := swapIn(live.Path(), staging, rep.Aside); err != nil {
		return rep, err
	}
	rep.WatcherErr = watcher.RekeySeen(opts.CacheRoot, opts.AccountName, rename)
	return rep, nil
}

// resumeSwap finishes a migration interrupted between its two renames -- the
// complete new cache is staged and marked, and no live cache exists -- and
// discards an incomplete staging directory otherwise.
func resumeSwap(live, staging string) (bool, error) {
	if _, err := os.Stat(staging); err != nil {
		return false, nil
	}
	_, liveErr := os.Stat(live)
	if meta.ImmutableIDs(cachedir.At(staging)) && errors.Is(liveErr, os.ErrNotExist) {
		return true, os.Rename(staging, live)
	}
	return false, os.RemoveAll(staging)
}

// swapIn sets the old cache aside and moves the staged one into its place,
// restoring the old one if the second rename fails.
func swapIn(live, staging, aside string) error {
	if err := os.Rename(live, aside); err != nil {
		return fmt.Errorf("setting the old cache aside: %w", err)
	}
	if err := os.Rename(staging, live); err != nil {
		if restoreErr := os.Rename(aside, live); restoreErr != nil {
			return fmt.Errorf("moving the new cache in: %w (and restoring the old one: %v; it is at %s)", err, restoreErr, aside)
		}
		return fmt.Errorf("moving the new cache in: %w", err)
	}
	return nil
}

func spliceInFlight(live cachedir.Account) bool {
	data, err := os.ReadFile(filepath.Join(live.Path(), "splice_journal.json"))
	if err != nil {
		return false
	}
	var j struct {
		Entries map[string]json.RawMessage `json:"entries"`
	}
	// An unreadable journal is treated as in flight: it is the splice's to
	// judge, not this migration's to discard.
	return json.Unmarshal(data, &j) != nil || len(j.Entries) > 0
}

// collectIDs is every message ID the account's cache and the watcher keep.
func collectIDs(live cachedir.Account, opts Options) ([]email.ID, error) {
	set := make(map[email.ID]bool)
	msg.ForEachID(live, func(id email.ID) bool {
		set[id] = true
		return false
	})
	for _, state := range []map[email.ID]bool{cache.LoadReadState(live), cache.LoadRepliedState(live)} {
		for id := range state {
			set[id] = true
		}
	}
	for id := range cache.LoadPendingReads(live) {
		set[id] = true
	}
	for _, path := range []string{train.SpamUIDPath(live), train.HamPath(live)} {
		ledger, err := train.Open(path)
		if err != nil {
			return nil, fmt.Errorf("a damaged training ledger would be lost: %w", err)
		}
		for _, id := range ledger.IDs() {
			set[email.ID(id)] = true
		}
	}
	sets, err := virtual.List(live)
	if err != nil {
		return nil, fmt.Errorf("reading saved result sets: %w", err)
	}
	for _, mb := range sets {
		for _, id := range mb.MessageIDs {
			set[id] = true
		}
	}
	seen, err := watcher.SeenIDs(opts.CacheRoot, opts.AccountName)
	if err != nil {
		return nil, err
	}
	for _, id := range seen {
		set[id] = true
	}
	ids := make([]email.ID, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	return ids, nil
}

// buildStaging writes the new cache: messages, flags, ledgers, saved sets and
// the folder listing, and the marker last, so an incomplete one is never
// taken for done.
func buildStaging(live, staging cachedir.Account, rename map[email.ID]email.ID, build string, rep *Report) error {
	var err error
	if rep.Messages, err = msg.Rekey(live, staging, rename); err != nil {
		return err
	}
	if rep.FlagsCarried, rep.FlagsDropped, err = cache.RekeyFlagState(live, staging, rename); err != nil {
		return err
	}
	if rep.SpamLedger, err = carryLedger(train.SpamUIDPath(live), train.SpamUIDPath(staging), rename); err != nil {
		return err
	}
	if rep.HamLedger, err = carryLedger(train.HamPath(live), train.HamPath(staging), rename); err != nil {
		return err
	}
	if rep.SavedSets, err = carrySavedSets(live, staging, rename); err != nil {
		return err
	}
	if err := copyFolderListing(live, staging); err != nil {
		return err
	}
	return meta.MarkImmutableIDs(staging, build)
}

func carryLedger(from, to string, rename map[email.ID]email.ID) (Count, error) {
	old, err := train.Open(from)
	if err != nil {
		return Count{}, err
	}
	count := Count{Before: old.Len()}
	if count.Before == 0 {
		return count, nil
	}
	var ids []string
	for _, id := range old.IDs() {
		if newID, ok := rename[email.ID(id)]; ok {
			ids = append(ids, string(newID))
		}
	}
	ledger := train.OpenOrEmpty(to)
	if err := ledger.AddAll(ids); err != nil {
		return count, err
	}
	count.After = ledger.Len()
	return count, nil
}

func carrySavedSets(live, staging cachedir.Account, rename map[email.ID]email.ID) (int, error) {
	sets, err := virtual.List(live)
	if err != nil {
		return 0, err
	}
	for i := range sets {
		mb := sets[i]
		renamed := virtual.Mailbox{Name: mb.Name, Description: mb.Description, FolderMap: map[email.ID]string{}}
		for _, id := range mb.MessageIDs {
			newID, ok := rename[id]
			if !ok {
				continue
			}
			renamed.MessageIDs = append(renamed.MessageIDs, newID)
			renamed.FolderMap[newID] = mb.FolderMap[id]
		}
		if err := virtual.Save(staging, &renamed); err != nil {
			return i, err
		}
	}
	return len(sets), nil
}

// copyFolderListing carries the cached folder listing: folder IDs are the
// same in both schemes.
func copyFolderListing(live, staging cachedir.Account) error {
	for _, name := range []string{"labels_cache.json", "recent_folders.json"} {
		data, err := os.ReadFile(filepath.Join(live.Path(), name))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if err := dirlock.AtomicWrite(filepath.Join(staging.Path(), name), data, 0600); err != nil {
			return err
		}
	}
	return nil
}
