package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sarielhp/feral/app"
	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
)

// linksSession is a session over a config.json with three accounts -- the
// second with a display name, the third with an alias -- and a directory for
// the symlinks. The test binary is a go-build file, so every link command
// here passes --dir.
func linksSession(t *testing.T, extra string) (session *Session, configDir, binDir string, buf *bytes.Buffer) {
	t.Helper()
	configDir, binDir = t.TempDir(), t.TempDir()
	body := `{"accounts": [{"name": "personal", "type": "jmap"}, {"name": "fastmail", "type": "jmap"},
		{"name": "work", "type": "jmap"}]` + extra + `}`
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	writeAccountJSON(t, configDir, "fastmail", `{"name": "fastmail", "type": "jmap", "display_name": "Fast Mail"}`)
	writeAccountJSON(t, configDir, "work", `{"name": "work", "type": "jmap", "aliases": ["job"]}`)
	buf = &bytes.Buffer{}
	session = &Session{Config: &cfg_g.Config{Paths: cfg_g.Paths{ConfigDir: configDir}}, Out: buf, Err: buf}
	t.Cleanup(func() {
		cfg_g.ForbidConfigWrites(false)
		app.FlagDryRun, app.FlagReadOnly = false, false
	})
	return session, configDir, binDir, buf
}

func writeAccountJSON(t *testing.T, configDir, name, body string) {
	t.Helper()
	dir := cfg_g.AccountDir(configDir, name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "account.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readLinks(t *testing.T, configDir string) (map[string]cfg_g.Link, bool) {
	t.Helper()
	links, present, err := cfg_g.ReadLinks(configDir)
	if err != nil {
		t.Fatal(err)
	}
	return links, present
}

func TestLinkAddStoresWhatWillRun(t *testing.T) {
	for _, tt := range []struct {
		name   string
		preset []string
		want   cfg_g.Link
	}{
		{"account and command", []string{"--", "-A", "fastmail", "tui"}, cfg_g.Link{Account: "fastmail", Args: []string{"tui"}}},
		{"index", []string{"--", "-2", "tui"}, cfg_g.Link{Account: "fastmail", Args: []string{"tui"}}},
		{"display name", []string{"--", "-A", "Fast Mail"}, cfg_g.Link{Account: "fastmail"}},
		{"alias, long form", []string{"--", "--account=job", "scan"}, cfg_g.Link{Account: "work", Args: []string{"scan"}}},
		{"last account wins", []string{"--", "-A", "work", "-1"}, cfg_g.Link{Account: "personal"}},
		{"no flags, no --", []string{"tui"}, cfg_g.Link{Args: []string{"tui"}}},
		{"pure rename", nil, cfg_g.Link{}},
		{"other flags kept", []string{"--", "--read-only", "tui"}, cfg_g.Link{Args: []string{"--read-only", "tui"}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			session, configDir, binDir, buf := linksSession(t, "")
			args := append([]string{"link", "add", "mt", "--dir", binDir}, tt.preset...)
			if err := execAccounts(t, session, args...); err != nil {
				t.Fatalf("%v\n%s", err, buf)
			}
			links, _ := readLinks(t, configDir)
			if got := links["mt"]; !reflect.DeepEqual(got, tt.want) {
				t.Errorf("stored %#v, want %#v", got, tt.want)
			}
			exe, _ := runningBinary()
			if !sameFile(filepath.Join(binDir, "mt"), exe) {
				t.Errorf("%s/mt is not a symlink to the running binary", binDir)
			}
		})
	}
}

func TestLinkAddRefuses(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
		says string
	}{
		{"abbreviated command", []string{"mt", "--", "-A", "work", "tu"}, "feral link add mt -- -A work tui"},
		{"unknown account", []string{"mt", "--", "-A", "nosuch", "tui"}, "nosuch"},
		{"index out of range", []string{"mt", "--", "-7"}, "out of range"},
		{"unknown command", []string{"mt", "--", "bogus"}, "bogus"},
		{"unknown flag", []string{"mt", "--", "tui", "--bogus"}, "bogus"},
		{"bad name", []string{"m/t", "tui"}, "letters"},
		{"own name", []string{"feral", "tui"}, "own name"},
		{"cluster", []string{"mt", "--", "-vA", "work"}, "apart"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			session, configDir, binDir, buf := linksSession(t, "")
			before, _ := os.ReadFile(filepath.Join(configDir, "config.json"))
			args := append([]string{"link", "add", "--dir", binDir}, tt.args...)
			err := execAccounts(t, session, args...)
			if err == nil || !strings.Contains(err.Error(), tt.says) {
				t.Fatalf("err = %v, want one saying %q\n%s", err, tt.says, buf)
			}
			after, _ := os.ReadFile(filepath.Join(configDir, "config.json"))
			if !bytes.Equal(before, after) {
				t.Error("a refused link changed config.json")
			}
			if entries, _ := os.ReadDir(binDir); len(entries) != 0 {
				t.Errorf("a refused link left %d file(s) in the link directory", len(entries))
			}
		})
	}
}

func TestLinkAddReplacesOnlyWithForce(t *testing.T) {
	session, configDir, binDir, buf := linksSession(t, `, "links": {"mt": {"args": ["scan"]}}`)
	if err := execAccounts(t, session, "link", "add", "mt", "--dir", binDir, "tui"); err == nil {
		t.Fatal("an existing link was replaced without --force")
	}
	if err := execAccounts(t, session, "link", "add", "mt", "--dir", binDir, "--force", "--description", "the TUI", "tui"); err != nil {
		t.Fatalf("%v\n%s", err, buf)
	}
	links, _ := readLinks(t, configDir)
	if want := (cfg_g.Link{Args: []string{"tui"}, Description: "the TUI"}); !reflect.DeepEqual(links["mt"], want) {
		t.Errorf("links[mt] = %#v, want %#v", links["mt"], want)
	}
}

func TestLinkAddDryRunWritesNothing(t *testing.T) {
	session, configDir, binDir, buf := linksSession(t, "")
	before, _ := os.ReadFile(filepath.Join(configDir, "config.json"))
	if err := execAccounts(t, session, "--dry-run", "link", "add", "mt", "--dir", binDir, "--", "-A", "work", "tui"); err != nil {
		t.Fatalf("%v\n%s", err, buf)
	}
	if !strings.Contains(buf.String(), "Would add link") {
		t.Errorf("--dry-run did not say what it would do:\n%s", buf)
	}
	after, _ := os.ReadFile(filepath.Join(configDir, "config.json"))
	if !bytes.Equal(before, after) {
		t.Error("--dry-run changed config.json")
	}
	if entries, _ := os.ReadDir(binDir); len(entries) != 0 {
		t.Error("--dry-run made a symlink")
	}
}

// A config.json the aliases migration has not reached is migrated before the
// first link is added: the key must never appear without the aliases' links.
func TestLinkAddMigratesAliasesFirst(t *testing.T) {
	session, configDir, binDir, buf := linksSession(t, "")
	if err := execAccounts(t, session, "link", "add", "mt", "--dir", binDir, "tui"); err != nil {
		t.Fatalf("%v\n%s", err, buf)
	}
	links, _ := readLinks(t, configDir)
	if want := (cfg_g.Link{Account: "work", Description: cfg_g.MigratedLinkDescription}); !reflect.DeepEqual(links["job"], want) {
		t.Errorf("the alias job did not become a link: %#v", links)
	}
}

// D7: what may stand where the symlink goes.
func TestLinkSymlinkPlacement(t *testing.T) {
	exe, err := runningBinary()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	place, err := linkPlacement(dir, "mt")
	if err != nil {
		t.Fatal(err)
	}
	if place.target != exe {
		t.Errorf("a symlink outside the binary's directory points at %q, want the full path %q", place.target, exe)
	}
	if _, err := linkPlacement("", "mt"); err == nil {
		t.Error("a link to a go-build binary was placed without --dir")
	}

	if err := place.create(); err != nil || !place.isThisProgram() {
		t.Fatalf("create: %v", err)
	}
	if err := place.check(false); err != nil {
		t.Errorf("an existing symlink to this feral was refused: %v", err)
	}

	other := filepath.Join(dir, "other")
	if err := os.WriteFile(other, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, setup := range []func(path string) error{
		func(path string) error { return os.WriteFile(path, []byte("#!/bin/sh\nexec mail_cli -2 tui\n"), 0o755) },
		func(path string) error { return os.Symlink(other, path) },
	} {
		_ = os.Remove(place.path)
		if err := setup(place.path); err != nil {
			t.Fatal(err)
		}
		if err := place.check(false); err == nil {
			t.Errorf("%s, which is not this feral, was accepted without --force", place.path)
		}
		if err := place.check(true); err != nil {
			t.Errorf("--force refused %s: %v", place.path, err)
		}
		if err := place.create(); err != nil || !place.isThisProgram() {
			t.Errorf("--force did not replace %s: %v", place.path, err)
		}
	}

	_ = os.Remove(place.path)
	if err := os.Mkdir(place.path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := place.check(true); err == nil {
		t.Error("a directory was accepted, with --force, as a link's place")
	}
}

func TestLinkRmLeavesAForeignFile(t *testing.T) {
	session, configDir, binDir, buf := linksSession(t, `, "links": {"mt": {"args": ["tui"]}, "gm": {"account": "work"}}`)
	exe, _ := runningBinary()
	if err := os.Symlink(exe, filepath.Join(binDir, "mt")); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(binDir, "gm")
	if err := os.WriteFile(foreign, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"mt", "gm"} {
		if err := execAccounts(t, session, "link", "rm", name, "--dir", binDir); err != nil {
			t.Fatalf("link rm %s: %v\n%s", name, err, buf)
		}
	}
	if _, err := os.Lstat(filepath.Join(binDir, "mt")); !os.IsNotExist(err) {
		t.Error("link rm left the symlink to this feral")
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Errorf("link rm removed a file that is not this feral: %v", err)
	}
	if !strings.Contains(buf.String(), "was left") {
		t.Errorf("link rm did not report the file it left:\n%s", buf)
	}
	links, present := readLinks(t, configDir)
	if !present || len(links) != 0 {
		t.Errorf("links after rm = %#v", links)
	}
	if err := execAccounts(t, session, "link", "rm", "mt", "--dir", binDir); err == nil {
		t.Error("removing a link that does not exist succeeded")
	}
}

func TestLinkListStates(t *testing.T) {
	exe, err := runningBinary()
	if err != nil {
		t.Fatal(err)
	}
	pathDir, besideDir := t.TempDir(), t.TempDir()
	t.Setenv("PATH", pathDir)
	if err := os.Symlink(exe, filepath.Join(pathDir, "onpath")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(exe, filepath.Join(besideDir, "offpath")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(besideDir, "foreign"), nil, 0o755); err != nil {
		t.Fatal(err)
	}
	accounts := []cfg_acc.AccountConfig{{Name: "work"}}
	for name, want := range map[string]string{
		"onpath": "ok", "offpath": "not on PATH", "foreign": "symlink elsewhere", "absent": "no symlink",
	} {
		if got := symlinkState(name, exe, besideDir); got != want {
			t.Errorf("symlinkState(%q) = %q, want %q", name, got, want)
		}
	}
	if got := linkState("onpath", cfg_g.Link{Account: "gone"}, accounts, exe); got != "account missing" {
		t.Errorf("a link to a removed account is %q", got)
	}
	if got := linkState("onpath", cfg_g.Link{Account: "WORK"}, accounts, exe); got != "ok" {
		t.Errorf("a link to an existing account is %q", got)
	}

	var buf bytes.Buffer
	session := &Session{Config: &cfg_g.Config{Accounts: accounts, Links: map[string]cfg_g.Link{
		"onpath": {Account: "work", Args: []string{"tui"}},
		"gone":   {Account: "removed"},
	}}, Out: &buf, Err: &buf}
	if err := runLinkList(session); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"onpath  ok", "feral -A work tui", "gone    account missing"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("link list lacks %q:\n%s", want, buf.String())
		}
	}
}

// A preset that names a link command walks into options link add itself
// has: the walk binds them, which resets their variables, so link add reads
// its flags before it walks. --dir used to be lost here, and the symlink went
// beside the binary.
func TestLinkAddKeepsItsFlagsWhenThePresetIsALinkCommand(t *testing.T) {
	session, configDir, binDir, buf := linksSession(t, "")
	if err := execAccounts(t, session, "link", "add", "lr", "--dir", binDir, "--force", "--description", "remove",
		"--", "link", "rm"); err != nil {
		t.Fatalf("%v\n%s", err, buf)
	}
	if _, err := os.Lstat(filepath.Join(binDir, "lr")); err != nil {
		t.Errorf("the symlink is not in --dir: %v", err)
	}
	links, _ := readLinks(t, configDir)
	if want := (cfg_g.Link{Args: []string{"link", "rm"}, Description: "remove"}); !reflect.DeepEqual(links["lr"], want) {
		t.Errorf("links[lr] = %#v, want %#v", links["lr"], want)
	}
}

// Without --dir the symlink goes beside the binary and points at it by its
// base name, as tools/install's mail_cli -> feral does.
func TestLinkDefaultPlacementBesideTheBinary(t *testing.T) {
	binDir := t.TempDir()
	exe := filepath.Join(binDir, "feral")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	place, err := linkPlacementFor(exe, "", "mt")
	if err != nil {
		t.Fatal(err)
	}
	if place.path != filepath.Join(binDir, "mt") || place.target != "feral" {
		t.Fatalf("placement = %+v, want %s/mt -> feral", place, binDir)
	}
	if err := place.create(); err != nil {
		t.Fatal(err)
	}
	if target, err := os.Readlink(place.path); err != nil || target != "feral" {
		t.Errorf("symlink holds %q (%v), want the relative feral", target, err)
	}
	if !place.isThisProgram() {
		t.Error("the symlink does not resolve to the binary")
	}
	if same, err := linkPlacementFor(exe, binDir, "mt"); err != nil || same.target != "feral" {
		t.Errorf("--dir naming the binary's own directory: %+v, %v", same, err)
	}
	if _, err := linkPlacementFor("/tmp/go-build123/b001/exe/feral", "", "mt"); err == nil {
		t.Error("a go-build binary was linked to without --dir")
	}
}

// -A became a clihelp.Var; its values still complete.
func TestAccountFlagValuesStillComplete(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	dir := filepath.Join(home, ".config", app.AppName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"accounts": [{"name": "fastmail", "type": "jmap"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cliApp := InitCLI(&Session{Config: &cfg_g.Config{}})
	var out bytes.Buffer
	cliApp.Stdout = &out
	if err := cliApp.Execute([]string{"__complete", "-A", ""}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "fastmail") {
		t.Errorf("-A <Tab> does not offer the accounts:\n%s", out.String())
	}
}

// D8: removing an account keeps the links that name it -- a new account of
// that name makes them work again -- and says which they are.
func TestAccountRmKeepsItsLinksAndWarns(t *testing.T) {
	session, configDir, _, buf := linksSession(t, `, "links": {"mt": {"account": "work", "args": ["tui"]},
		"gm": {"account": "personal"}}`)
	if err := execAccounts(t, session, "account", "rm", "work", "--yes"); err != nil {
		t.Fatalf("%v\n%s", err, buf)
	}
	links, _ := readLinks(t, configDir)
	if links["mt"].Account != "work" || links["gm"].Account != "personal" {
		t.Errorf("links after account rm = %#v", links)
	}
	if !strings.Contains(buf.String(), "Links still naming it: mt") {
		t.Errorf("account rm did not warn about the links naming it:\n%s", buf)
	}
}

func TestLinkCommandLineQuotes(t *testing.T) {
	got := linkCommandLine(cfg_g.Link{Account: "work", Args: []string{"scan", "%inbox", "$HOME/x", "it's"}})
	if want := `feral -A work scan %inbox '$HOME/x' 'it'\''s'`; got != want {
		t.Errorf("linkCommandLine = %s, want %s", got, want)
	}
}
