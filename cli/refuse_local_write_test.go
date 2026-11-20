package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/label"
)

// Commands that write local records refuse before touching anything: under
// --dry-run all of them, under --read-only the ones whose record describes
// the simulated server change. They used to train the filter, write the
// ledgers and reset them under both flags.
func TestLocalWritesRefusedUnderReadOnlyAndDryRun(t *testing.T) {
	calls := 0
	acted := func() error { calls++; return nil }
	recording := stubSpam{
		MarkByIDFn:      func(*cfg_g.Config, string) error { return acted() },
		UnspamByIDFn:    func(*cfg_g.Config, string) error { return acted() },
		ResetLearningFn: func(*cfg_g.Config) error { return acted() },
		LearnHamFn:      func(*cfg_g.Config, label.Folder, bool) error { return acted() },
	}
	for _, tc := range []struct {
		args     []string
		readOnly bool // refused under --read-only too, not only --dry-run
	}{
		{[]string{"spam", "mark", "abc123de"}, true},
		{[]string{"spam", "unmark", "abc123de"}, false},
		{[]string{"spam", "reset"}, false},
	} {
		for _, mode := range []string{"dry-run", "read-only"} {
			calls = 0
			session := &Session{Config: &cfg_g.Config{}, Deps: Deps{Spam: recording}}
			args := append([]string{"--" + mode}, tc.args...)
			_, err := captureOutput(func() error { return InitCLI(session).Execute(args) })
			wantRefused := mode == "dry-run" || tc.readOnly
			if refused := err != nil && strings.Contains(err.Error(), "refused under --"+mode); refused != wantRefused {
				t.Errorf("%v under --%s: err = %v, want refused=%v", tc.args, mode, err, wantRefused)
			}
			if acted := calls > 0; acted == wantRefused {
				t.Errorf("%v under --%s: acted=%v, want acted=%v", tc.args, mode, acted, !wantRefused)
			}
		}
	}
}

// --dry-run writes no configuration: some thirty commands save config.json
// (rules, lists, config set, accounts), and each used to write it. The CLI
// turns the guard in SaveConfigFile on for a --dry-run run and off for any
// other.
func TestDryRunForbidsConfigWrites(t *testing.T) {
	t.Cleanup(func() { cfg_g.ForbidConfigWrites(false) })
	cfgDir := t.TempDir()
	path := cfgDir + "/config.json"
	accounts := []cfg_acc.AccountConfig{{Name: "a", Type: "gmail", Username: "a@example.com"}}
	if err := cfg_g.SaveConfigFile(path, &cfg_g.FileConfig{Accounts: &accounts}); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		session := &Session{Config: &cfg_g.Config{Paths: cfg_g.Paths{ConfigDir: cfgDir}, Accounts: accounts}}
		if _, err := captureOutput(func() error { return InitCLI(session).Execute(args) }); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}

	run("--dry-run", "account", "list")
	if err := cfg_g.SaveConfigFile(path, &cfg_g.FileConfig{Accounts: &accounts}); !errors.Is(err, cfg_g.ErrConfigWriteForbidden) {
		t.Errorf("after a --dry-run run, SaveConfigFile: err = %v, want ErrConfigWriteForbidden", err)
	}
	run("account", "list")
	if err := cfg_g.SaveConfigFile(path, &cfg_g.FileConfig{Accounts: &accounts}); err != nil {
		t.Errorf("after an ordinary run, SaveConfigFile: %v", err)
	}
}
