package watcher

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRenameAccountMovesWatermark(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "watcher"), 0o700); err != nil {
		t.Fatal(err)
	}
	state := &State{Accounts: map[string]Account{
		"old":   {LastChecked: time.Unix(5, 0).UTC(), LastSeenIDs: []string{"a", "b"}},
		"other": {LastSeenIDs: []string{"c"}},
	}}
	if err := state.Save(root); err != nil {
		t.Fatal(err)
	}
	for range 2 { // idempotent
		if err := RenameAccount(root, "old", "new"); err != nil {
			t.Fatal(err)
		}
	}
	got, err := LoadState(root)
	if err != nil {
		t.Fatal(err)
	}
	if got.Known("old") || len(got.Accounts["new"].LastSeenIDs) != 2 || len(got.Accounts["other"].LastSeenIDs) != 1 {
		t.Errorf("state after rename = %+v", got.Accounts)
	}
}
