package runner

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/mailops"
	"github.com/sarielhp/feral/mute"
	"github.com/sarielhp/feral/outbox"
	"github.com/sarielhp/feral/snooze"
	"github.com/sarielhp/feral/watcher"
)

// renameFixture is a configuration with a gmail account named "default" --
// the name a rename exists to retire -- and an outlook account, each with the
// files a real one has: settings, token, cache, watcher state, a queued message.
type renameFixture struct {
	config    *cfg_g.Config
	configDir string
	cacheRoot string
}

func writeFixtureFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func newRenameFixture(t *testing.T) renameFixture {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	f := renameFixture{configDir: t.TempDir(), cacheRoot: t.TempDir()}
	writeFixtureFile(t, filepath.Join(f.configDir, "config.json"), `{
  "accounts": [{"name": "default", "type": "gmail"}, {"name": "work", "type": "outlook"}],
  "default_account": "default",
  "default_send_account": "default",
  "links": {"default": {"account": "default", "args": ["tui"], "x_future": 1}, "mt": {"account": "work"}}
}`)
	writeFixtureFile(t, filepath.Join(cfg_g.AccountDir(f.configDir, "default"), "account.json"),
		`{"name": "default", "type": "gmail", "display_name": "GMail", "calendar_manager": true}`)
	writeFixtureFile(t, filepath.Join(cfg_g.AccountDir(f.configDir, "default"), "rules.json"), `[]`)
	writeFixtureFile(t, cfg_g.SignaturePath(f.configDir, "default"), "Bob\n")
	writeFixtureFile(t, filepath.Join(cfg_g.AccountDir(f.configDir, "default"), mute.FileName),
		`{"format": 1, "threads": [{"key": "<root@example.com>", "scheme": "header-root"}]}`)
	writeFixtureFile(t, mailops.TokenFiles(f.configDir, cfg_acc.AccountConfig{Name: "default", Type: "gmail"})[0], `{"token":"gmail"}`)
	writeFixtureFile(t, mailops.TokenFiles(f.configDir, cfg_acc.AccountConfig{Name: "work", Type: "outlook"})[0], `{"token":"work"}`)
	writeFixtureFile(t, filepath.Join(cachedir.ForAccount(f.cacheRoot, "default").Path(), "cache_meta.json"), `{"version":1}`)
	state := &watcher.State{Accounts: map[string]watcher.Account{
		"default": {LastChecked: time.Unix(100, 0).UTC(), LastSeenIDs: []string{"m1"}},
	}}
	if err := os.MkdirAll(filepath.Join(f.cacheRoot, "watcher"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := state.Save(f.cacheRoot); err != nil {
		t.Fatal(err)
	}
	if _, err := outbox.Enqueue(outbox.Dir(f.cacheRoot), "default", []string{"x@example.com"}, "queued", []byte("Subject: queued\r\n\r\nhi")); err != nil {
		t.Fatal(err)
	}
	for _, acct := range []string{"default", "work"} {
		if _, err := snooze.Put(f.cacheRoot, snooze.Entry{Account: acct, ID: email.ID("s-" + acct),
			Until: time.Unix(200, 0).UTC(), Folder: snooze.FolderName}, false); err != nil {
			t.Fatal(err)
		}
	}
	f.config = &cfg_g.Config{Paths: cfg_g.Paths{ConfigDir: f.configDir, CacheRoot: f.cacheRoot}}
	return f
}

func (f renameFixture) assertRenamed(t *testing.T, from, to string) {
	t.Helper()
	fc, err := cfg_g.LoadConfigFile(filepath.Join(f.configDir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	acc, err := cfg_g.Lookup(*fc.Accounts, to)
	if err != nil {
		t.Fatalf("renamed account not found: %v", err)
	}
	if acc.DisplayName != "GMail" || !acc.CalendarManager {
		t.Errorf("the account's own settings did not follow it: %+v", acc)
	}
	if fc.DefaultAccount != to || fc.DefaultSendAccount != to || !acc.IsDefault {
		t.Errorf("default_account %q / default_send_account %q did not follow the rename", fc.DefaultAccount, fc.DefaultSendAccount)
	}
	// A link follows its account; its own name, the user's command, does not.
	if fc.Links == nil || (*fc.Links)["default"].Account != to || (*fc.Links)["mt"].Account != "work" {
		t.Errorf("links did not follow the rename: %+v", fc.Links)
	}
	if links, _, _ := cfg_g.ReadLinks(f.configDir); len(links["default"].Args) != 1 {
		t.Errorf("a link lost its arguments in the rename: %+v", links)
	}
	if data, _ := os.ReadFile(filepath.Join(f.configDir, "config.json")); !strings.Contains(string(data), "x_future") {
		t.Error("the rename dropped a link field this build does not know")
	}
	token := mailops.TokenFiles(f.configDir, cfg_acc.AccountConfig{Name: to, Type: "gmail"})[0]
	if _, statErr := os.Stat(token); statErr != nil {
		t.Errorf("token not moved: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(cachedir.ForAccount(f.cacheRoot, to).Path(), "cache_meta.json")); statErr != nil {
		t.Errorf("cache not moved: %v", statErr)
	}
	if sig, set, sigErr := cfg_g.ReadSignature(f.configDir, to); sigErr != nil || !set || sig != "Bob\n" {
		t.Errorf("signature not moved: %q (set %v), %v", sig, set, sigErr)
	}
	if mutePath, pathErr := mute.Path(f.configDir, to); pathErr != nil {
		t.Error(pathErr)
	} else if muted, loadErr := mute.Load(mutePath); loadErr != nil || len(muted.Threads) != 1 {
		t.Errorf("muted threads did not follow the rename: %+v, %v", muted, loadErr)
	}
	for _, gone := range append(cfg_g.AccountFiles(f.configDir, from), cachedir.ForAccount(f.cacheRoot, from).Path()) {
		if _, statErr := os.Lstat(gone); !errors.Is(statErr, os.ErrNotExist) {
			t.Errorf("%s still exists", gone)
		}
	}
	state, err := watcher.LoadState(f.cacheRoot)
	if err != nil {
		t.Fatal(err)
	}
	if _, old := state.Accounts[from]; old || len(state.Accounts[to].LastSeenIDs) != 1 {
		t.Errorf("watcher state not rekeyed: %+v", state.Accounts)
	}
	entries, err := outbox.List(outbox.Dir(f.cacheRoot))
	if err != nil || len(entries) != 1 || entries[0].Account != to {
		t.Errorf("queued message not relabelled: %+v, %v", entries, err)
	}
	snoozed, err := snooze.Load(f.cacheRoot)
	if err != nil {
		t.Fatal(err)
	}
	accounts := map[email.ID]string{}
	for _, e := range snoozed.Entries {
		accounts[e.ID] = e.Account
	}
	if accounts["s-default"] != to || accounts["s-work"] != "work" || len(accounts) != 2 {
		t.Errorf("snooze record not relabelled: %+v", snoozed.Entries)
	}
	if m, err := cfg_g.PendingRename(f.configDir); m != nil || err != nil {
		t.Errorf("marker left behind: %+v, %v", m, err)
	}
}

func TestRenameAccountMovesEverything(t *testing.T) {
	f := newRenameFixture(t)
	plan, err := RenameAccount(f.config, "GMail", "personal") // named by its display name
	if err != nil {
		t.Fatal(err)
	}
	if plan.From != "default" || plan.To != "personal" || plan.Requeued != 1 || plan.Resumed {
		t.Errorf("unexpected report: %+v", plan)
	}
	f.assertRenamed(t, "default", "personal")
	work := mailops.TokenFiles(f.configDir, cfg_acc.AccountConfig{Name: "work", Type: "outlook"})[0]
	if _, err := os.Stat(work); err != nil {
		t.Errorf("another account's token was touched: %v", err)
	}
}

func TestRenameAccountDryRunChangesNothing(t *testing.T) {
	f := newRenameFixture(t)
	f.config.DryRun = true
	before, _ := os.ReadFile(filepath.Join(f.configDir, "config.json"))
	plan, err := RenameAccount(f.config, "default", "personal")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Moves) != 3 { // settings directory, token, cache
		t.Errorf("plan lists %d moves, want 3: %+v", len(plan.Moves), plan.Moves)
	}
	after, _ := os.ReadFile(filepath.Join(f.configDir, "config.json"))
	if string(before) != string(after) {
		t.Error("a dry run rewrote config.json")
	}
	for _, m := range plan.Moves {
		if _, err := os.Stat(m.From); err != nil {
			t.Errorf("a dry run moved %s", m.From)
		}
	}
	if m, _ := cfg_g.PendingRename(f.configDir); m != nil {
		t.Error("a dry run left a marker")
	}
}

// A rename cut short leaves the marker and part of the moves; running it again
// finishes it, and nothing else may run in between.
func TestRenameAccountResumes(t *testing.T) {
	f := newRenameFixture(t)
	if err := cfg_g.BeginRename(f.configDir, "default", "personal"); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(cfg_g.AccountDir(f.configDir, "default"), cfg_g.AccountDir(f.configDir, "personal")); err != nil {
		t.Fatal(err)
	}
	if _, err := RenameAccount(f.config, "default", "other"); err == nil {
		t.Fatal("a different rename was allowed to start over an unfinished one")
	}
	plan, err := RenameAccount(f.config, "default", "personal")
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Resumed {
		t.Error("the rerun did not report that it finished an interrupted rename")
	}
	f.assertRenamed(t, "default", "personal")
}

func TestRenameAccountRefuses(t *testing.T) {
	f := newRenameFixture(t)
	for _, to := range []string{"default", "all", "work", "bad name", "9lives"} {
		if _, err := RenameAccount(f.config, "default", to); err == nil {
			t.Errorf("rename to %q was accepted", to)
		}
	}
	// A file already under the new name belongs to something else: refuse
	// before anything moves.
	writeFixtureFile(t, mailops.TokenFiles(f.configDir, cfg_acc.AccountConfig{Name: "personal", Type: "gmail"})[0], "stray")
	if _, err := RenameAccount(f.config, "default", "personal"); err == nil || !strings.Contains(err.Error(), "both") {
		t.Errorf("rename over an existing file = %v, want a refusal", err)
	}
	if _, err := os.Stat(cfg_g.AccountDir(f.configDir, "default")); err != nil {
		t.Error("a refused rename moved the settings directory")
	}
	// Nor while a watcher runs: it would write the old name's state back.
	f2 := newRenameFixture(t)
	release, err := watcher.HoldWatchLock(f2.cacheRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err := RenameAccount(f2.config, "default", "personal"); !errors.Is(err, watcher.ErrAlreadyWatching) {
		t.Errorf("rename beside a watcher = %v, want ErrAlreadyWatching", err)
	}
}

// A crash after config.json was committed but before the marker was removed:
// config.json already names the new account, and the rerun finds it there.
func TestRenameAccountResumesAfterCommit(t *testing.T) {
	f := newRenameFixture(t)
	if err := cfg_g.BeginRename(f.configDir, "default", "personal"); err != nil {
		t.Fatal(err)
	}
	if _, err := applyRename(f.cacheRoot, mustPlan(t, f, "default", "personal")); err != nil {
		t.Fatal(err)
	}
	if err := cfg_g.CommitAccountRename(filepath.Join(f.configDir, "config.json"), "default", "personal"); err != nil {
		t.Fatal(err)
	}
	plan, err := RenameAccount(f.config, "default", "personal")
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Resumed || len(plan.Moves) != 0 {
		t.Errorf("rerun after commit = %+v, want a resumed rename with nothing left to move", plan)
	}
	f.assertRenamed(t, "default", "personal")
}

func mustPlan(t *testing.T, f renameFixture, from, to string) AccountRename {
	t.Helper()
	moves, err := renameMoves(f.configDir, f.cacheRoot, "gmail", from, to)
	if err != nil {
		t.Fatal(err)
	}
	return AccountRename{From: from, To: to, Moves: moves}
}

// Outlook names its token file with a hash of the account name, so the token
// must be found by the backend's own path under the new name -- and a legacy
// unhashed token must move too, or the fallback that migrates it is lost.
func TestRenameOutlookAccountKeepsItsToken(t *testing.T) {
	f := newRenameFixture(t)
	legacy := mailops.TokenFiles(f.configDir, cfg_acc.AccountConfig{Name: "work", Type: "outlook"})[4]
	writeFixtureFile(t, legacy, `{"token":"legacy"}`)
	if _, err := RenameAccount(f.config, "work", "office"); err != nil {
		t.Fatal(err)
	}
	path, err := mailops.TokenPath(f.configDir, cfg_acc.AccountConfig{Name: "office", Type: "outlook"})
	if err != nil {
		t.Fatal(err)
	}
	if data, readErr := os.ReadFile(path); readErr != nil || string(data) != `{"token":"work"}` {
		t.Errorf("TokenPath for the new name = %s: %q, %v", path, data, readErr)
	}
	newLegacy := mailops.TokenFiles(f.configDir, cfg_acc.AccountConfig{Name: "office", Type: "outlook"})[4]
	if _, statErr := os.Stat(newLegacy); statErr != nil {
		t.Errorf("legacy token not moved: %v", statErr)
	}
	for _, old := range mailops.TokenFiles(f.configDir, cfg_acc.AccountConfig{Name: "work", Type: "outlook"}) {
		if _, statErr := os.Lstat(old); !errors.Is(statErr, os.ErrNotExist) {
			t.Errorf("%s left behind", old)
		}
	}
}
