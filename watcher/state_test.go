package watcher

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/feral/email"
)

func TestStateMissingFileIsEmptyNotDamaged(t *testing.T) {
	dir := t.TempDir()
	state, err := LoadState(dir)
	if err != nil {
		t.Fatalf("LoadState on empty dir: %v", err)
	}
	if state.Known("work") {
		t.Fatal("an account is known before anything was recorded")
	}
	if got := state.Unseen("work", []email.ID{"a"}); len(got) != 1 {
		t.Fatalf("Unseen on an unknown account = %v, want the id back", got)
	}
}

func TestStateDamagedFileIsAnError(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "watcher"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stateFile(dir), []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadState(dir); err == nil {
		t.Fatal("a damaged state file must be an error, not an empty state")
	}
}

func TestStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	at := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)

	state, err := LoadState(dir)
	if err != nil {
		t.Fatal(err)
	}
	state.MarkSeen("work", []email.ID{"a", "b"}, at)
	if saveErr := state.Save(dir); saveErr != nil {
		t.Fatalf("Save: %v", saveErr)
	}

	reloaded, err := LoadState(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reloaded.Known("work") {
		t.Fatal("account is not known after a round trip")
	}
	if got := reloaded.Accounts["work"].LastChecked; !got.Equal(at) {
		t.Fatalf("last checked = %v, want %v", got, at)
	}
	if got := reloaded.Unseen("work", []email.ID{"a", "b", "c"}); len(got) != 1 || got[0] != "c" {
		t.Fatalf("Unseen = %v, want only the new id c", got)
	}
}

func TestStateMarkSeenKeepsNewestFirstAndDeduplicates(t *testing.T) {
	state := &State{Accounts: map[string]Account{}}
	state.MarkSeen("work", []email.ID{"a", "b"}, time.Now())
	state.MarkSeen("work", []email.ID{"c", "a"}, time.Now())

	got := state.Accounts["work"].LastSeenIDs
	want := []string{"c", "a", "b"}
	if len(got) != len(want) {
		t.Fatalf("LastSeenIDs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("LastSeenIDs = %v, want %v", got, want)
		}
	}
}

func TestStateMarkSeenCapsTheList(t *testing.T) {
	state := &State{Accounts: map[string]Account{}}
	ids := make([]email.ID, 0, maxSeen+50)
	for i := range maxSeen + 50 {
		ids = append(ids, email.ID(string(rune('a'+i%26))+string(rune('0'+i%10))+string(rune(i))))
	}
	state.MarkSeen("work", ids, time.Now())
	if got := len(state.Accounts["work"].LastSeenIDs); got != maxSeen {
		t.Fatalf("LastSeenIDs length = %d, want the cap %d", got, maxSeen)
	}
}

// Renaming seen IDs keeps those the rename maps; when none survive, the
// account is forgotten, so the next poll records its inbox silently rather
// than announcing all of it.
func TestRekeySeen(t *testing.T) {
	dir := t.TempDir()
	state := &State{Accounts: map[string]Account{}}
	state.MarkSeen("kept", []email.ID{"a", "gone"}, time.Now())
	state.MarkSeen("emptied", []email.ID{"gone2"}, time.Now())
	if err := os.MkdirAll(filepath.Join(dir, "watcher"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := state.Save(dir); err != nil {
		t.Fatal(err)
	}
	rename := map[email.ID]email.ID{"a": "A"}
	for _, name := range []string{"kept", "emptied"} {
		if err := RekeySeen(dir, name, rename); err != nil {
			t.Fatal(err)
		}
	}
	got, err := LoadState(dir)
	if err != nil {
		t.Fatal(err)
	}
	if ids := got.Accounts["kept"].LastSeenIDs; len(ids) != 1 || ids[0] != "A" {
		t.Errorf("kept: %v, want [A]", ids)
	}
	if got.Known("emptied") {
		t.Error("an account whose seen IDs all vanished must be forgotten, or its inbox is announced as new")
	}
}

// The watermark is written with its format; one a newer build wrote is refused,
// never read as "nothing seen", which would announce every inbox.
func TestStateFormat(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "watcher"), 0700); err != nil {
		t.Fatal(err)
	}
	s := &State{Accounts: map[string]Account{}}
	if err := s.Save(dir); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(stateFile(dir)); !strings.Contains(string(data), `"format": 1`) {
		t.Errorf("not stamped: %s", data)
	}
	if err := os.WriteFile(stateFile(dir), []byte(`{"format":2,"accounts":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadState(dir); err == nil {
		t.Error("LoadState accepted a newer format")
	}
}
