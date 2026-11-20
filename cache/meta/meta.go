// Package meta keeps each account cache's manifest, cache_meta.json: which
// layout the cache has, which backend wrote it, and what kind of message ID
// it is keyed by.
//
// The manifest exists so a binary can tell whether it understands a cache
// before it writes into one. Five cache migrations came before it, each
// probing for its own clue on every start, and none could stop an older build
// from writing what it knew into a cache a newer one had reshaped -- an older
// build would have written Outlook's default IDs into a cache migrated to
// immutable ones, splitting every message in two, and said nothing. Open
// refuses a cache whose version is newer than this build's, whose ID scheme
// it does not know, or that another backend wrote; Check does the same
// without writing, for code that only reads.
//
// A cache with no manifest is version 0. Open upgrades it once: it runs the
// layout probes that predate the manifest and infers the ID scheme from what
// the cache holds, then records both. Nothing is written under --dry-run: the
// manifest is worked out in memory and the next normal run records it.
package meta

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/cache/dirlock"
)

// FileName is the manifest's name inside an account cache.
const FileName = "cache_meta.json"

// CurrentVersion is the cache layout this build writes and understands. A
// change to the layout raises it and adds a step to upgrade.
const CurrentVersion = 1

// IDScheme is what a message ID in a cache is.
type IDScheme string

// The schemes. Only Outlook has two; mbox derives its IDs from the file, so a
// change to that derivation is a scheme change too.
const (
	SchemeGmail          IDScheme = "gmail"
	SchemeJMAP           IDScheme = "jmap"
	SchemeGraphDefault   IDScheme = "graph-default"   // changes when the message moves
	SchemeGraphImmutable IDScheme = "graph-immutable" // survives a move
	SchemeMbox           IDScheme = "mbox-message-id-v1"
)

var knownSchemes = map[IDScheme]string{
	SchemeGmail: "gmail", SchemeJMAP: "jmap",
	SchemeGraphDefault: "outlook", SchemeGraphImmutable: "outlook",
	SchemeMbox: "mbox",
}

// Manifest is an account cache's description of itself.
type Manifest struct {
	Version   int      `json:"version"`
	Backend   string   `json:"backend"`
	IDScheme  IDScheme `json:"id_scheme"`
	WrittenBy string   `json:"written_by,omitempty"` // the build that last wrote it, for messages only
}

// TooNewError refuses a cache a newer build has reshaped.
type TooNewError struct {
	Path      string
	Version   int
	WrittenBy string
}

func (e *TooNewError) Error() string {
	by := ""
	if e.WrittenBy != "" {
		by = " by feral " + e.WrittenBy
	}
	return fmt.Sprintf("%s was written%s (cache version %d); this build understands version %d. Install a newer feral",
		e.Path, by, e.Version, CurrentVersion)
}

// ErrDamaged refuses a manifest that does not parse. `utils cache repair`
// rebuilds it from what the cache holds.
var ErrDamaged = errors.New("the cache manifest is damaged; run `feral utils cache repair` for this account")

// Options is how Open may change a cache.
type Options struct {
	// DryRun works the manifest out without writing it or running any step.
	DryRun bool
	// Build is this build's version, recorded as WrittenBy.
	Build string
	// LegacyLayout runs the layout migrations that predate the manifest.
	// Version 1 is recorded only when it succeeds, so a failure is retried
	// on the next start, as it always was.
	LegacyLayout func() error
}

func path(acct cachedir.Account) string { return filepath.Join(acct.Path(), FileName) }

// Read returns the cache's manifest, and false when it has none. It writes
// nothing and validates nothing; a manifest that does not parse is ErrDamaged.
func Read(acct cachedir.Account) (Manifest, bool, error) {
	if acct.IsZero() {
		return Manifest{}, false, nil
	}
	data, err := os.ReadFile(path(acct))
	if errors.Is(err, os.ErrNotExist) {
		return Manifest{}, false, nil
	}
	if err != nil {
		return Manifest{}, false, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil || m.Version <= 0 {
		return Manifest{}, false, fmt.Errorf("%s: %w", path(acct), ErrDamaged)
	}
	return m, true, nil
}

// Write records m as the cache's manifest.
func Write(acct cachedir.Account, m Manifest) error {
	if acct.IsZero() {
		return errors.New("no account cache")
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return dirlock.AtomicWrite(path(acct), append(data, '\n'), 0600)
}

// validate refuses a manifest this build must not write under.
func validate(acct cachedir.Account, m Manifest, backend string) error {
	if m.Version > CurrentVersion {
		return &TooNewError{Path: acct.Path(), Version: m.Version, WrittenBy: m.WrittenBy}
	}
	owner, known := knownSchemes[m.IDScheme]
	if !known {
		return fmt.Errorf("%s is keyed by message IDs of kind %q, which this build does not know. Install a newer feral", acct.Path(), m.IDScheme)
	}
	if backend != "" && !strings.EqualFold(m.Backend, backend) {
		return fmt.Errorf("%s was written by a %s account, not %s: the account was re-created with another type; move its cache aside", acct.Path(), m.Backend, backend)
	}
	if !strings.EqualFold(owner, m.Backend) {
		return fmt.Errorf("%s: a %s cache cannot hold %s message IDs; run `feral utils cache repair` for this account", acct.Path(), m.Backend, m.IDScheme)
	}
	return nil
}

// Check refuses a cache this build must not use, without writing anything:
// for code that reads a cache without building a client. A cache with no
// manifest passes -- it is older, and the next client built for it upgrades
// it. backend may be empty when the caller does not know it.
func Check(acct cachedir.Account, backend string) error {
	m, ok, err := Read(acct)
	if err != nil || !ok {
		return err
	}
	return validate(acct, m, backend)
}

// Open returns the cache's manifest, upgrading a cache that has none, or the
// refusal. It is what every backend's client is built behind.
func Open(acct cachedir.Account, backend string, opts Options) (Manifest, error) {
	backend = strings.ToLower(backend)
	if acct.IsZero() {
		return Manifest{}, nil
	}
	m, ok, err := Read(acct)
	if err != nil {
		return m, err
	}
	if ok {
		return m, validate(acct, m, backend)
	}
	if opts.DryRun {
		return infer(acct, backend, opts.Build)
	}
	return upgrade(acct, backend, opts)
}

// upgrade records the manifest of a cache that has none, under the account's
// lock -- re-reading it there, so two processes opening the same cache do not
// both run the legacy steps.
func upgrade(acct cachedir.Account, backend string, opts Options) (Manifest, error) {
	if err := os.MkdirAll(acct.Path(), 0700); err != nil {
		return Manifest{}, err
	}
	release, err := dirlock.Guard(acct.Path())
	if release != nil {
		defer release()
	}
	if err != nil {
		return Manifest{}, err
	}
	m, ok, err := Read(acct)
	if err != nil {
		return m, err
	}
	if ok {
		return m, validate(acct, m, backend)
	}
	m, err = infer(acct, backend, opts.Build)
	if err != nil {
		return m, err
	}
	if opts.LegacyLayout != nil {
		if err := opts.LegacyLayout(); err != nil {
			// Not recorded: the legacy steps retry on the next start.
			return m, nil
		}
	}
	if err := Write(acct, m); err != nil {
		return m, err
	}
	// The marker the immutable-ID migration used before the manifest; the
	// manifest now says the same, so it goes only after that is written.
	_ = os.Remove(filepath.Join(acct.Path(), legacyImmutableMarker))
	return m, nil
}

// legacyImmutableMarker is the file that marked an Outlook cache keyed by
// immutable IDs before the manifest existed.
const legacyImmutableMarker = "immutable_ids"

// infer works out the manifest of a cache that has none, from what it holds.
func infer(acct cachedir.Account, backend, build string) (Manifest, error) {
	scheme, err := inferScheme(acct, backend)
	if err != nil {
		return Manifest{}, err
	}
	return Manifest{Version: CurrentVersion, Backend: backend, IDScheme: scheme, WrittenBy: build}, nil
}

func inferScheme(acct cachedir.Account, backend string) (IDScheme, error) {
	switch backend {
	case "gmail":
		return SchemeGmail, nil
	case "jmap":
		return SchemeJMAP, nil
	case "mbox":
		return SchemeMbox, nil
	case "outlook":
		if _, err := os.Stat(filepath.Join(acct.Path(), legacyImmutableMarker)); err == nil {
			return SchemeGraphImmutable, nil
		}
		if HoldsMessages(acct) {
			return SchemeGraphDefault, nil
		}
		return SchemeGraphImmutable, nil
	default:
		return "", fmt.Errorf("no cache layout for account type %q", backend)
	}
}

// HoldsMessages reports whether the account's cache records any message ID: the
// index, the messages, the read, replied and pending marks, the training
// ledgers, saved result sets or a splice journal. A cache that holds none has
// no IDs whose scheme matters; one holding only a ledger still does.
func HoldsMessages(acct cachedir.Account) bool {
	if acct.IsZero() {
		return false
	}
	for _, name := range []string{
		"message_info.bin", "messages", "seen_cache.json", "replied_cache.json", "pending_reads.json",
		"trained_uids.json", "trained_uids_ham.json", "virtual", "splice_journal.json",
	} {
		if _, err := os.Stat(filepath.Join(acct.Path(), name)); err == nil {
			return true
		}
	}
	return false
}

// Rebuild infers the manifest from what the cache holds and writes it,
// replacing a damaged or missing one. It is what `utils cache repair` runs.
// An Outlook cache holding messages is taken as keyed by default IDs unless
// its messages say otherwise; see SchemeOf.
func Rebuild(acct cachedir.Account, backend string, build string, sample []string) (Manifest, error) {
	backend = strings.ToLower(backend)
	m, err := infer(acct, backend, build)
	if err != nil {
		return m, err
	}
	if backend == "outlook" && len(sample) > 0 {
		m.IDScheme = SchemeOf(sample)
	}
	return m, Write(acct, m)
}

// SchemeOf tells Outlook's two ID schemes apart from a sample of IDs. A
// default ID is a REST ID of an Exchange item -- base64 that begins "AQMk" or
// "AAMk" -- while an immutable one begins "AAkA" (checked against a live
// account). Used only to repair a manifest, never to decide a migration.
func SchemeOf(sample []string) IDScheme {
	immutable, legacy := 0, 0
	for _, id := range sample {
		switch {
		case strings.HasPrefix(id, "AAkA"):
			immutable++
		case strings.HasPrefix(id, "AQMk"), strings.HasPrefix(id, "AAMk"):
			legacy++
		}
	}
	if immutable > legacy {
		return SchemeGraphImmutable
	}
	return SchemeGraphDefault
}

// ImmutableIDs reports whether the cache is keyed by Outlook's immutable IDs.
func ImmutableIDs(acct cachedir.Account) bool {
	m, ok, err := Read(acct)
	return err == nil && ok && m.IDScheme == SchemeGraphImmutable
}

// IDsChangeOnMove reports whether the cache's message IDs are ones the server
// replaces when a message moves to another folder (Outlook's default IDs), so
// an ID recorded before a move names nothing after it. A cache with no
// manifest, or a damaged one, answers false; the client factory refuses those
// before anything moves.
func IDsChangeOnMove(acct cachedir.Account) bool {
	m, ok, err := Read(acct)
	return err == nil && ok && m.IDScheme == SchemeGraphDefault
}

// MarkImmutableIDs records that an Outlook cache is keyed by immutable IDs.
func MarkImmutableIDs(acct cachedir.Account, build string) error {
	return Write(acct, Manifest{Version: CurrentVersion, Backend: "outlook", IDScheme: SchemeGraphImmutable, WrittenBy: build})
}

// The download directory and the configuration directory hold files of their
// own -- drafts, the watcher's state, saved contacts; the white and black
// lists, rules, the global training ledger -- so each has a manifest too:
// a version, under the same forward guard, and no ID scheme.
const (
	// DirVersion is the layout of the two directories this build understands.
	DirVersion = 1
	// DownloadDirFile is the download directory's manifest.
	DownloadDirFile = "download_dir_meta.json"
	// ConfigDirFile is the configuration directory's manifest.
	ConfigDirFile = "config_meta.json"
)

// DirManifest is a directory's description of itself.
type DirManifest struct {
	Version   int    `json:"version"`
	WrittenBy string `json:"written_by,omitempty"`
}

// OpenDir refuses dir when its manifest is damaged or newer than this build
// understands, and records one when it has none and write is set. A missing
// directory is left alone: nothing in it to protect.
func OpenDir(dir, file, build string, write bool) error {
	if dir == "" {
		return nil
	}
	if _, err := os.Stat(dir); err != nil {
		return nil
	}
	p := filepath.Join(dir, file)
	data, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		if !write {
			return nil
		}
		out, _ := json.MarshalIndent(DirManifest{Version: DirVersion, WrittenBy: build}, "", "  ")
		return dirlock.AtomicWrite(p, append(out, '\n'), 0600)
	}
	if err != nil {
		return err
	}
	var m DirManifest
	if err := json.Unmarshal(data, &m); err != nil || m.Version <= 0 {
		return fmt.Errorf("%s is damaged: move it aside and run again, and it is recreated", p)
	}
	if m.Version > DirVersion {
		by := ""
		if m.WrittenBy != "" {
			by = " by feral " + m.WrittenBy
		}
		return fmt.Errorf("%s was written%s (version %d); this build understands version %d. Install a newer feral", dir, by, m.Version, DirVersion)
	}
	return nil
}
