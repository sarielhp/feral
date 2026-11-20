package meta

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/sarielhp/feral/cache/cachedir"
)

func touch(t *testing.T, acct cachedir.Account, name string) {
	t.Helper()
	if err := os.MkdirAll(acct.Path(), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(acct.Path(), name), []byte("[]"), 0600); err != nil {
		t.Fatal(err)
	}
}

func newAcct(t *testing.T) cachedir.Account { return cachedir.At(filepath.Join(t.TempDir(), "acct")) }

// A cache with no manifest is given one inferred from its backend and what it
// holds -- the four real cases among them.
func TestOpenInfersEveryBackend(t *testing.T) {
	cases := []struct {
		name, backend string
		holds         []string
		want          IDScheme
	}{
		{"gmail", "gmail", []string{"message_info.bin"}, SchemeGmail},
		{"jmap", "JMAP", []string{"message_info.bin"}, SchemeJMAP},
		{"mbox view", "mbox", nil, SchemeMbox},
		{"outlook with messages", "outlook", []string{"message_info.bin"}, SchemeGraphDefault},
		{"outlook with only a ledger", "outlook", []string{"trained_uids_ham.json"}, SchemeGraphDefault},
		{"outlook migrated before the manifest", "outlook", []string{"message_info.bin", legacyImmutableMarker}, SchemeGraphImmutable},
		{"new outlook account", "outlook", nil, SchemeGraphImmutable},
	}
	for _, c := range cases {
		acct := newAcct(t)
		for _, name := range c.holds {
			touch(t, acct, name)
		}
		m, err := Open(acct, c.backend, Options{Build: "1.0"})
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if m.IDScheme != c.want || m.Version != CurrentVersion || m.Backend != strings.ToLower(c.backend) || m.WrittenBy != "1.0" {
			t.Errorf("%s: manifest %+v, want scheme %s", c.name, m, c.want)
		}
		got, ok, err := Read(acct)
		if err != nil || !ok || got != m {
			t.Errorf("%s: recorded %+v (%v, %v), want %+v", c.name, got, ok, err, m)
		}
		if _, err := os.Stat(filepath.Join(acct.Path(), legacyImmutableMarker)); err == nil {
			t.Errorf("%s: the old marker outlived the manifest that replaces it", c.name)
		}
	}
}

// A second open finds the manifest and changes nothing.
func TestOpenTwiceChangesNothing(t *testing.T) {
	acct := newAcct(t)
	touch(t, acct, "message_info.bin")
	first, err := Open(acct, "outlook", Options{Build: "1.0"})
	if err != nil {
		t.Fatal(err)
	}
	touch(t, acct, legacyImmutableMarker) // must not be read again
	calls := 0
	second, err := Open(acct, "outlook", Options{Build: "2.0", LegacyLayout: func() error { calls++; return nil }})
	if err != nil || second != first || calls != 0 {
		t.Errorf("second open: %+v, %v, legacy steps run %d time(s); want %+v unchanged", second, err, calls, first)
	}
}

// The forward guard: a newer layout, an unknown ID scheme or another
// backend's cache is refused, by Open and by Check alike.
func TestOpenRefusesWhatItDoesNotUnderstand(t *testing.T) {
	cases := []struct {
		name    string
		m       Manifest
		backend string
		want    string
	}{
		{"newer layout", Manifest{Version: CurrentVersion + 1, Backend: "gmail", IDScheme: SchemeGmail, WrittenBy: "9.9"}, "gmail", "Install a newer feral"},
		{"unknown scheme", Manifest{Version: CurrentVersion, Backend: "outlook", IDScheme: "graph-quantum"}, "outlook", "does not know"},
		{"other backend", Manifest{Version: CurrentVersion, Backend: "gmail", IDScheme: SchemeGmail}, "jmap", "re-created"},
		{"scheme of another backend", Manifest{Version: CurrentVersion, Backend: "gmail", IDScheme: SchemeJMAP}, "gmail", "cannot hold"},
	}
	for _, c := range cases {
		acct := newAcct(t)
		if err := Write(acct, c.m); err != nil {
			t.Fatal(err)
		}
		if _, err := Open(acct, c.backend, Options{}); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: Open = %v, want an error mentioning %q", c.name, err, c.want)
		}
		if err := Check(acct, c.backend); err == nil {
			t.Errorf("%s: Check passed", c.name)
		}
	}
	acct := newAcct(t)
	_ = Write(acct, Manifest{Version: CurrentVersion + 1, Backend: "gmail", IDScheme: SchemeGmail})
	var tooNew *TooNewError
	if _, err := Open(acct, "gmail", Options{}); !errors.As(err, &tooNew) {
		t.Errorf("a newer layout must be a TooNewError, got %v", err)
	}
}

// A damaged manifest is refused, and repair rebuilds it.
func TestDamagedManifestRefusedAndRebuilt(t *testing.T) {
	acct := newAcct(t)
	touch(t, acct, "message_info.bin")
	touch(t, acct, FileName)
	if err := os.WriteFile(filepath.Join(acct.Path(), FileName), []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(acct, "outlook", Options{}); !errors.Is(err, ErrDamaged) {
		t.Fatalf("Open = %v, want ErrDamaged", err)
	}
	m, err := Rebuild(acct, "outlook", "1.0", []string{"AAkALgAAAA1", "AAkALgAAAA2", "AQMkADAw3"})
	if err != nil || m.IDScheme != SchemeGraphImmutable {
		t.Fatalf("Rebuild: %+v, %v", m, err)
	}
	if _, err := Open(acct, "outlook", Options{}); err != nil {
		t.Errorf("after repair: %v", err)
	}
}

// --dry-run works the manifest out but writes nothing and runs no step.
func TestOpenDryRunWritesNothing(t *testing.T) {
	acct := newAcct(t)
	touch(t, acct, "message_info.bin")
	m, err := Open(acct, "gmail", Options{DryRun: true, LegacyLayout: func() error {
		t.Error("a legacy step ran under --dry-run")
		return nil
	}})
	if err != nil || m.IDScheme != SchemeGmail {
		t.Errorf("dry run: %+v, %v", m, err)
	}
	if _, ok, _ := Read(acct); ok {
		t.Error("a manifest was written under --dry-run")
	}
}

// A failed legacy step leaves the cache unrecorded, so it is retried.
func TestOpenRetriesAFailedLegacyStep(t *testing.T) {
	acct := newAcct(t)
	touch(t, acct, "message_info.bin")
	fail := errors.New("label cache unreadable")
	if _, err := Open(acct, "gmail", Options{LegacyLayout: func() error { return fail }}); err != nil {
		t.Fatalf("a best-effort step's failure is not the caller's: %v", err)
	}
	if _, ok, _ := Read(acct); ok {
		t.Fatal("version 1 was recorded although the legacy step failed")
	}
	calls := 0
	if _, err := Open(acct, "gmail", Options{LegacyLayout: func() error { calls++; return nil }}); err != nil || calls != 1 {
		t.Errorf("retry: %v, ran %d time(s)", err, calls)
	}
	if _, ok, _ := Read(acct); !ok {
		t.Error("the successful retry was not recorded")
	}
}

// Two openers of one unrecorded cache run the legacy steps once between them.
func TestOpenConcurrentUpgradeRunsOnce(t *testing.T) {
	acct := newAcct(t)
	touch(t, acct, "message_info.bin")
	var mu sync.Mutex
	calls := 0
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = Open(acct, "gmail", Options{LegacyLayout: func() error {
				mu.Lock()
				calls++
				mu.Unlock()
				return nil
			}})
		}()
	}
	wg.Wait()
	if calls != 1 {
		t.Errorf("the legacy steps ran %d times", calls)
	}
}

// Check passes a cache with no manifest -- it is older, not unknown -- and
// writes nothing.
func TestCheckLeavesAnOldCacheAlone(t *testing.T) {
	acct := newAcct(t)
	touch(t, acct, "message_info.bin")
	if err := Check(acct, "outlook"); err != nil {
		t.Errorf("Check on an unrecorded cache: %v", err)
	}
	if _, ok, _ := Read(acct); ok {
		t.Error("Check wrote a manifest")
	}
}

// A cache counts as holding messages when any store keyed by message ID has
// something in it -- a training ledger alone included.
func TestHoldsMessages(t *testing.T) {
	empty := newAcct(t)
	touch(t, empty, "labels_cache.json")
	touch(t, empty, FileName)
	if HoldsMessages(empty) {
		t.Error("a folder listing and a manifest are no message IDs")
	}
	for _, name := range []string{"trained_uids_ham.json", "trained_uids.json", "seen_cache.json", "splice_journal.json"} {
		acct := newAcct(t)
		touch(t, acct, name)
		if !HoldsMessages(acct) {
			t.Errorf("a cache holding only %s must not count as empty", name)
		}
	}
}

// The download and configuration directories' manifests: recorded when
// allowed, never otherwise, and a newer or damaged one refuses the run.
func TestOpenDir(t *testing.T) {
	dir := t.TempDir()
	if err := OpenDir(dir, ConfigDirFile, "1.0", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ConfigDirFile)); err == nil {
		t.Error("a manifest was written although writing was not allowed")
	}
	if err := OpenDir(dir, ConfigDirFile, "1.0", true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ConfigDirFile)); err != nil {
		t.Errorf("no manifest recorded: %v", err)
	}
	if err := OpenDir(dir, ConfigDirFile, "2.0", true); err != nil {
		t.Errorf("a second run: %v", err)
	}
	for content, want := range map[string]string{`{"version":2,"written_by":"9.9"}`: "9.9", `{oops`: "damaged"} {
		if err := os.WriteFile(filepath.Join(dir, DownloadDirFile), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		if err := OpenDir(dir, DownloadDirFile, "1.0", true); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: OpenDir = %v, want %q", content, err, want)
		}
	}
	if err := OpenDir(filepath.Join(dir, "missing"), DownloadDirFile, "1.0", true); err != nil {
		t.Errorf("a missing directory: %v", err)
	}
}
