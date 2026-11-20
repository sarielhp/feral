package outlookids

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/feral/cache"
	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/cache/meta"
	"github.com/sarielhp/feral/cache/msg"
	"github.com/sarielhp/feral/cache/virtual"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/train"
	"github.com/sarielhp/feral/watcher"
)

// fixture is an Outlook cache keyed by default IDs a, b, c and gone, with
// something of every kind the migration carries.
type fixture struct {
	root    string
	live    cachedir.Account
	when    time.Time
	renamed map[email.ID]email.ID
	calls   int
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	f := &fixture{
		root: root,
		live: cachedir.At(filepath.Join(root, "work")),
		when: time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC),
		renamed: map[email.ID]email.ID{
			"a": "A-immutable", "b": "B-immutable", "c": "C-immutable",
		},
	}
	for _, id := range []email.ID{"a", "b", "c", "gone"} {
		must(t, msg.Store(f.live, id, []byte("Subject: "+string(id)+"\r\n\r\nbody "+string(id)), f.when))
	}
	must(t, msg.SetEvidence(f.live, "a", msg.Evidence{Checks: msg.CheckBogofilter, BogoScore: 0.9, Stamp: 7}))
	must(t, msg.SetOverride(f.live, "b", msg.OverrideHam))
	must(t, msg.StoreSummary(f.live, "a", "an LLM summary"))
	must(t, cache.MarkIDsReplied(f.live, []email.ID{"a", "gone"}, nil))
	must(t, cache.MarkIDsRead(f.live, []email.ID{"c"}, nil))
	must(t, train.OpenOrEmpty(train.SpamUIDPath(f.live)).AddAll([]string{"a"}))
	must(t, train.OpenOrEmpty(train.HamPath(f.live)).AddAll([]string{"b", "gone"}))
	must(t, virtual.Save(f.live, &virtual.Mailbox{
		Name: "last", MessageIDs: []email.ID{"a", "gone"},
		FolderMap: map[email.ID]string{"a": "Inbox", "gone": "Inbox"},
	}))
	must(t, os.WriteFile(filepath.Join(f.live.Path(), "labels_cache.json"), []byte(`["Inbox"]`), 0600))
	state, err := watcher.LoadState(root)
	must(t, err)
	state.MarkSeen("work", []email.ID{"a", "c"}, f.when)
	must(t, os.MkdirAll(filepath.Join(root, "watcher"), 0700))
	must(t, state.Save(root))
	return f
}

func (f *fixture) options() Options {
	return Options{
		Account: f.live, AccountName: "work", CacheRoot: f.root,
		Translate: func(_ context.Context, ids []email.ID) (map[email.ID]email.ID, error) {
			f.calls++
			out := map[email.ID]email.ID{}
			for _, id := range ids {
				if n, ok := f.renamed[id]; ok {
					out[id] = n
				}
			}
			return out, nil
		},
		Now: func() time.Time { return time.Date(2026, 11, 19, 14, 0, 0, 0, time.UTC) },
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// Everything that cannot be fetched again comes across under the new IDs,
// and the old cache is kept beside it.
func TestMigrateCarriesEverything(t *testing.T) {
	f := newFixture(t)
	rep, migErr := Migrate(context.Background(), f.options())
	must(t, migErr)

	if !meta.ImmutableIDs(f.live) {
		t.Fatal("the new cache is not marked")
	}
	if rep.Messages.Carried != 3 || rep.Messages.Dropped != 1 || rep.Messages.Decided != 1 {
		t.Errorf("messages: %+v", rep.Messages)
	}
	if body, err := msg.Read(f.live, "A-immutable"); err != nil || !strings.Contains(string(body), "body a") {
		t.Errorf("body of a: %q, %v", body, err)
	}
	if info, err := msg.GetInfo(f.live, "B-immutable"); err != nil || info.Override != msg.OverrideHam {
		t.Errorf("b lost its override: %+v, %v", info, err)
	}
	if info, err := msg.GetInfo(f.live, "A-immutable"); err != nil || !info.Scanned || info.Evidence.Stamp != 7 || !info.Date.Equal(f.when) {
		t.Errorf("a lost its evidence or date: %+v, %v", info, err)
	}
	if s, err := msg.ReadSummary(f.live, "A-immutable"); err != nil || s != "an LLM summary" {
		t.Errorf("a lost its LLM summary: %q, %v", s, err)
	}
	if !cache.LoadRepliedState(f.live)["A-immutable"] || !cache.LoadReadState(f.live)["C-immutable"] {
		t.Error("read or replied marks were not carried")
	}
	if _, ok := cache.LoadPendingReads(f.live)["C-immutable"]; !ok {
		t.Error("the pending read mark was not carried")
	}
	if rep.SpamLedger != (Count{1, 1}) || rep.HamLedger != (Count{2, 1}) {
		t.Errorf("ledgers: spam %+v ham %+v", rep.SpamLedger, rep.HamLedger)
	}
	if ham, err := train.Open(train.HamPath(f.live)); err != nil || !ham.Has("B-immutable") {
		t.Errorf("ham ledger: %v", err)
	}
	mb, err := virtual.Load(f.live, "last")
	if err != nil || len(mb.MessageIDs) != 1 || mb.MessageIDs[0] != "A-immutable" || mb.FolderMap["A-immutable"] != "Inbox" {
		t.Errorf("saved set: %+v, %v", mb, err)
	}
	if _, statErr := os.Stat(filepath.Join(f.live.Path(), "labels_cache.json")); statErr != nil {
		t.Error("the folder listing was not carried")
	}
	seen, err := watcher.SeenIDs(f.root, "work")
	if err != nil || len(seen) != 2 || seen[0] != "A-immutable" || seen[1] != "C-immutable" {
		t.Errorf("watcher seen: %v, %v", seen, err)
	}
	if want := f.live.Path() + ".pre-immutable-20261119-140000"; rep.Aside != want {
		t.Errorf("aside %q, want %q", rep.Aside, want)
	}
	if body, err := msg.Read(cachedir.At(rep.Aside), "a"); err != nil || !strings.Contains(string(body), "body a") {
		t.Error("the old cache was not kept intact")
	}
}

// A second run is a no-op, and needs no translation.
func TestMigrateTwiceIsANoOp(t *testing.T) {
	f := newFixture(t)
	_, err := Migrate(context.Background(), f.options())
	must(t, err)
	calls := f.calls
	rep, err := Migrate(context.Background(), f.options())
	if err != nil || !rep.AlreadyMigrated || f.calls != calls {
		t.Errorf("second run: %+v, %v, translated again: %v", rep, err, f.calls != calls)
	}
}

// assertUntouched checks that a refused migration left the old cache as it was.
func assertUntouched(t *testing.T, f *fixture) {
	t.Helper()
	if meta.ImmutableIDs(f.live) {
		t.Error("the cache was marked although the migration was refused")
	}
	if _, err := msg.Read(f.live, "a"); err != nil {
		t.Errorf("the old cache was changed: %v", err)
	}
	if _, err := os.Stat(f.live.Path() + stagingSuffix); !os.IsNotExist(err) {
		t.Error("a staging directory was left behind")
	}
}

// Nothing changes when the translation fails, or translates nothing.
func TestMigrateRefusesWithoutTranslation(t *testing.T) {
	for name, translate := range map[string]Translator{
		"error": func(context.Context, []email.ID) (map[email.ID]email.ID, error) {
			return nil, errors.New("403 forbidden")
		},
		"nothing": func(context.Context, []email.ID) (map[email.ID]email.ID, error) {
			return map[email.ID]email.ID{}, nil
		},
	} {
		f := newFixture(t)
		opts := f.options()
		opts.Translate = translate
		if _, err := Migrate(context.Background(), opts); err == nil {
			t.Errorf("%s: expected a refusal", name)
		}
		assertUntouched(t, f)
	}
}

// A running watcher -- a TUI or `feral watch` -- would record old IDs again.
func TestMigrateRefusesBesideAWatcher(t *testing.T) {
	f := newFixture(t)
	release, err := watcher.HoldWatchLock(f.root)
	must(t, err)
	defer release()
	if _, err := Migrate(context.Background(), f.options()); !errors.Is(err, watcher.ErrAlreadyWatching) {
		t.Errorf("expected ErrAlreadyWatching, got %v", err)
	}
	assertUntouched(t, f)
}

// A half-done splice names messages by the old IDs; losing its journal would
// leave a duplicate behind.
func TestMigrateRefusesDuringASplice(t *testing.T) {
	f := newFixture(t)
	must(t, os.WriteFile(filepath.Join(f.live.Path(), "splice_journal.json"),
		[]byte(`{"source_folder":"Inbox","entries":{"a":{"dest_id":"x"}}}`), 0600))
	if _, err := Migrate(context.Background(), f.options()); !errors.Is(err, ErrSpliceInFlight) {
		t.Errorf("expected ErrSpliceInFlight, got %v", err)
	}
	assertUntouched(t, f)
}

// A cache with nothing in it is only marked.
func TestMigrateEmptyCacheIsMarked(t *testing.T) {
	root := t.TempDir()
	live := cachedir.At(filepath.Join(root, "new"))
	rep, err := Migrate(context.Background(), Options{Account: live, AccountName: "new", CacheRoot: root,
		Translate: func(context.Context, []email.ID) (map[email.ID]email.ID, error) {
			t.Error("an empty cache needs no translation")
			return nil, nil
		}})
	if err != nil || !rep.Empty || !meta.ImmutableIDs(live) {
		t.Errorf("empty cache: %+v, %v, marked %v", rep, err, meta.ImmutableIDs(live))
	}
}

// A run interrupted between its two renames is finished by the next one; an
// incomplete staging directory is discarded and rebuilt.
func TestMigrateResumesAnInterruptedSwap(t *testing.T) {
	f := newFixture(t)
	staging := f.live.Path() + stagingSuffix
	must(t, os.MkdirAll(staging, 0700))
	must(t, meta.MarkImmutableIDs(cachedir.At(staging), "test"))
	must(t, os.Rename(f.live.Path(), f.live.Path()+asideSuffix+"x"))

	rep, err := Migrate(context.Background(), f.options())
	if err != nil || !rep.AlreadyMigrated || !meta.ImmutableIDs(f.live) {
		t.Errorf("resume: %+v, %v", rep, err)
	}

	g := newFixture(t)
	must(t, os.MkdirAll(g.live.Path()+stagingSuffix, 0700)) // unmarked: incomplete
	if _, err := Migrate(context.Background(), g.options()); err != nil {
		t.Fatal(err)
	}
	if _, err := msg.Read(g.live, "A-immutable"); err != nil {
		t.Errorf("an incomplete staging directory was not rebuilt: %v", err)
	}
}

// A failed verification -- translation that disagrees with Graph's own
// listing -- changes nothing, and nothing is translated.
func TestMigrateRefusesWhenVerificationFails(t *testing.T) {
	f := newFixture(t)
	opts := f.options()
	opts.Verify = func(context.Context) error { return errors.New("a translates to x but Graph lists y") }
	if _, err := Migrate(context.Background(), opts); err == nil || !strings.Contains(err.Error(), "nothing was changed") {
		t.Errorf("expected a refusal, got %v", err)
	}
	if f.calls != 0 {
		t.Error("the cache was translated although verification failed")
	}
	assertUntouched(t, f)
}
