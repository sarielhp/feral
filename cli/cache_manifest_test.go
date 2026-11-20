package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/cache/meta"
	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
)

func manifestSession(t *testing.T, typ string) (*Session, cachedir.Account, *bytes.Buffer) {
	t.Helper()
	cfg := &cfg_g.Config{
		Paths:    cfg_g.Paths{CacheRoot: t.TempDir()},
		Accounts: []cfg_acc.AccountConfig{{Name: "a", Type: typ}}, SelectedAccount: "a",
	}
	acct, err := cfg.SelectedAccountCache()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(acct.Path(), 0o700); err != nil {
		t.Fatal(err)
	}
	out := &bytes.Buffer{}
	return &Session{Config: cfg, Out: out}, acct, out
}

// Commands that work on a cache without building a client still refuse one a
// newer build reshaped: prune rewrites its index, and the label lookups read it.
func TestDirectCacheUsersRefuseNewerCache(t *testing.T) {
	session, acct, _ := manifestSession(t, "gmail")
	if err := meta.Write(acct, meta.Manifest{Version: meta.CurrentVersion + 1, Backend: "gmail", IDScheme: meta.SchemeGmail}); err != nil {
		t.Fatal(err)
	}
	if err := runCachePrune(session, []string{"30"}, false); err == nil || !strings.Contains(err.Error(), "newer") {
		t.Errorf("prune = %v, want a refusal", err)
	}
	if _, err := ResolveCacheDir(session.Config); err == nil {
		t.Error("ResolveCacheDir accepted a newer cache")
	}
}

// The guard sends a damaged manifest to `utils cache repair`, which rebuilds
// it from what the cache holds -- only when allowed to write.
func TestCacheRepairRebuildsDamagedManifest(t *testing.T) {
	session, acct, out := manifestSession(t, "outlook")
	if err := os.WriteFile(filepath.Join(acct.Path(), meta.FileName), []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	session.Config.DryRun = true
	if err := runCacheRepair(session); err != nil {
		t.Fatal(err)
	}
	if _, _, err := meta.Read(acct); err == nil {
		t.Fatal("--dry-run rebuilt the manifest")
	}
	session.Config.DryRun = false
	if err := runCacheRepair(session); err != nil {
		t.Fatal(err)
	}
	m, ok, err := meta.Read(acct)
	if err != nil || !ok || m.Backend != "outlook" {
		t.Errorf("after repair: %+v (%v, %v)", m, ok, err)
	}
	if !strings.Contains(out.String(), "rebuilt the cache manifest") {
		t.Errorf("repair did not say so: %q", out.String())
	}
}
