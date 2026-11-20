package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/feral/app"
	"github.com/sarielhp/feral/cfg_g"
)

// accountsSession is a session over a config.json holding three accounts, the
// first of them named "default" as configurations from before the name was
// reserved are.
func accountsSession(t *testing.T) (*Session, string, *bytes.Buffer) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"accounts": [
  {"name": "default", "type": "jmap"}, {"name": "fastmail", "type": "jmap"}, {"name": "work", "type": "jmap"}
]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	session := &Session{Config: &cfg_g.Config{Paths: cfg_g.Paths{ConfigDir: dir}}, Out: &buf, Err: &buf}
	return session, path, &buf
}

func execAccounts(t *testing.T, session *Session, args ...string) error {
	t.Helper()
	app.FlagReadOnly, app.FlagDryRun = false, false
	cliApp := InitCLI(session)
	cliApp.Stdout, cliApp.Stderr = session.Out, session.Err
	return cliApp.Execute(args)
}

func TestAccountDefaultSetShowAndRemove(t *testing.T) {
	session, path, buf := accountsSession(t)
	if err := execAccounts(t, session, "account", "default"); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(buf.String(), "default\n") || !strings.Contains(buf.String(), "first account is used") {
		t.Errorf("with none set, the first account is the default and the output says so:\n%s", buf)
	}
	if err := execAccounts(t, session, "account", "default", "Work"); err != nil {
		t.Fatal(err)
	}
	fc, err := cfg_g.LoadConfigFile(path)
	if err != nil || fc.DefaultAccount != "work" {
		t.Fatalf("default_account = %q, %v; want the canonical name \"work\"", fc.DefaultAccount, err)
	}
	if err = execAccounts(t, session, "account", "default", "nosuch"); err == nil {
		t.Error("an unknown account was made the default")
	}

	buf.Reset()
	if err = execAccounts(t, session, "account", "list"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "work (jmap, type: regular) (default)") {
		t.Errorf("account list does not mark the default:\n%s", buf)
	}
	if !strings.Contains(buf.String(), `"default" is reserved`) || !strings.Contains(buf.String(), "account rename default") {
		t.Errorf("account list does not warn about the account named \"default\":\n%s", buf)
	}

	if err = execAccounts(t, session, "account", "rm", "work", "--yes"); err != nil {
		t.Fatal(err)
	}
	fc, err = cfg_g.LoadConfigFile(path)
	if err != nil || fc.DefaultAccount != "" {
		t.Errorf("removing the default account left default_account = %q (%v)", fc.DefaultAccount, err)
	}
}

func TestAccountAddRefusesReservedName(t *testing.T) {
	session, _, _ := accountsSession(t)
	for _, name := range []string{"Default", "all", "has space"} {
		err := execAccounts(t, session, "account", "add", "mbox", name, "--path", "/dev/null")
		if err == nil || !(strings.Contains(err.Error(), "reserved") || strings.Contains(err.Error(), "may hold only")) {
			t.Errorf("account add %q = %v, want a refusal of the name", name, err)
		}
	}
}

func TestAccountRenameReportsPlan(t *testing.T) {
	session, _, buf := accountsSession(t)
	var gotIdent, gotTo string
	session.Deps.Accounts = stubAccounts{RenameFn: func(_ *cfg_g.Config, ident, to string) (cfg_g.AccountRename, error) {
		gotIdent, gotTo = ident, to
		return cfg_g.AccountRename{From: "default", To: to, Moves: []cfg_g.RenameMove{{From: "/a/default", To: "/a/personal"}}}, nil
	}}
	session.Config.DryRun = true
	if err := execAccounts(t, session, "--dry-run", "account", "rename", "default", "personal"); err != nil {
		t.Fatal(err)
	}
	if gotIdent != "default" || gotTo != "personal" {
		t.Errorf("rename called with %q, %q", gotIdent, gotTo)
	}
	if !strings.Contains(buf.String(), "Would rename account \"default\" to \"personal\"") || !strings.Contains(buf.String(), "/a/personal") {
		t.Errorf("dry run did not list the plan:\n%s", buf)
	}
}

// Only `account rename` may load a configuration whose rename is unfinished;
// cliBeforeRun tells the bootstrap which command is running.
func TestOnlyRenameFinishesARename(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want bool
	}{
		{[]string{"account", "rename", "a", "b"}, true},
		{[]string{"account", "list"}, false},
	} {
		session, _, _ := accountsSession(t)
		var got StartupOptions
		cfg := session.Config
		session.Bootstrap = func(_ context.Context, opts StartupOptions) (*cfg_g.Config, error) {
			got = opts
			return cfg, nil
		}
		_ = execAccounts(t, session, tc.args...)
		if got.FinishRename != tc.want {
			t.Errorf("%v: FinishRename = %v, want %v", tc.args, got.FinishRename, tc.want)
		}
	}
}
