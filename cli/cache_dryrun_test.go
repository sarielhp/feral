package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
)

// Under --dry-run cache clear and prune say what they would delete and keep
// it: the account cache holds spam overrides, training ledgers and read state,
// which cannot be downloaded again.
func TestCacheResetAndPruneKeepEverythingUnderDryRun(t *testing.T) {
	root := t.TempDir()
	cfg := &cfg_g.Config{
		DryRun: true, ReadOnly: true, Paths: cfg_g.Paths{CacheRoot: root},
		Accounts: []cfg_acc.AccountConfig{{Name: "a", Type: "gmail"}}, SelectedAccount: "a",
	}
	dir, err := cfg.SelectedAccountCache()
	if err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(dir.Path(), "message_info.bin")
	if err := os.MkdirAll(dir.Path(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keep, []byte("overrides"), 0o600); err != nil {
		t.Fatal(err)
	}
	session := &Session{Config: cfg, Out: &bytes.Buffer{}}
	if err := runCacheReset(session); err != nil {
		t.Fatal(err)
	}
	if err := runCachePrune(session, nil, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Errorf("--dry-run deleted the account cache: %v", err)
	}
}
