package mailops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/cache/meta"
	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/report"
)

func manifestFixture(t *testing.T, typ string) (cfg_acc.AccountConfig, *cfg_g.Config, cachedir.Account) {
	t.Helper()
	base := t.TempDir()
	acc := cfg_acc.AccountConfig{Name: "acct", Type: typ, SessionURL: "https://jmap.example.com"}
	cfg := &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: base}}
	acct := cfg.AccountCacheFor(acc)
	if err := os.MkdirAll(acct.Path(), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(acct.Path(), "message_info.bin"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	return acc, cfg, acct
}

// Building any backend's client records the cache's manifest, from the
// account's type and what the cache holds.
func TestNewMailClientRecordsManifest(t *testing.T) {
	for typ, want := range map[string]meta.IDScheme{
		"gmail": meta.SchemeGmail, "jmap": meta.SchemeJMAP, "Outlook": meta.SchemeGraphDefault,
	} {
		acc, cfg, acct := manifestFixture(t, typ)
		if _, err := NewMailClient(acc, cfg, report.Discard()); err != nil {
			t.Fatalf("%s: %v", typ, err)
		}
		m, ok, err := meta.Read(acct)
		if err != nil || !ok || m.IDScheme != want || m.Backend != strings.ToLower(typ) {
			t.Errorf("%s: manifest %+v (%v, %v), want scheme %s", typ, m, ok, err, want)
		}
	}
}

// The forward guard, at the one place every client is built: a cache a newer
// build reshaped is refused, under --read-only too.
func TestNewMailClientRefusesNewerCache(t *testing.T) {
	for _, typ := range []string{"gmail", "jmap", "outlook"} {
		acc, cfg, acct := manifestFixture(t, typ)
		scheme := map[string]meta.IDScheme{"gmail": meta.SchemeGmail, "jmap": meta.SchemeJMAP, "outlook": meta.SchemeGraphImmutable}[typ]
		if err := meta.Write(acct, meta.Manifest{Version: meta.CurrentVersion + 1, Backend: typ, IDScheme: scheme, WrittenBy: "9.9.9"}); err != nil {
			t.Fatal(err)
		}
		cfg.ReadOnly = true
		if _, err := NewMailClient(acc, cfg, report.Discard()); err == nil || !strings.Contains(err.Error(), "9.9.9") {
			t.Errorf("%s: NewMailClient = %v, want a refusal naming the newer build", typ, err)
		}
	}
}

// --dry-run records nothing.
func TestNewMailClientDryRunRecordsNothing(t *testing.T) {
	acc, cfg, acct := manifestFixture(t, "gmail")
	cfg.DryRun = true
	if _, err := NewMailClient(acc, cfg, report.Discard()); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := meta.Read(acct); ok {
		t.Error("a manifest was written under --dry-run")
	}
}

// An mbox view's cache gets a manifest too.
func TestOpenMboxRecordsManifest(t *testing.T) {
	dir := t.TempDir()
	box := filepath.Join(dir, "in.mbox")
	if err := os.WriteFile(box, []byte("From a@b Mon Jan  1 00:00:00 2024\nSubject: x\n\nbody\n"), 0600); err != nil {
		t.Fatal(err)
	}
	acct := cachedir.At(filepath.Join(dir, "view"))
	cfg := &cfg_g.Config{Accounts: []cfg_acc.AccountConfig{{Name: "mbox", Type: "mbox"}}}
	if _, err := OpenMbox(cfg, box, acct); err != nil {
		t.Fatal(err)
	}
	if m, ok, err := meta.Read(acct); err != nil || !ok || m.IDScheme != meta.SchemeMbox {
		t.Errorf("mbox view manifest: %+v (%v, %v)", m, ok, err)
	}
}
