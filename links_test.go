package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sarielhp/clihelp"

	"github.com/sarielhp/feral/app"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/cli"
)

// linkTestHome points HOME and the XDG directories at a fresh directory, so
// that nothing a test runs reads or writes the developer's own files, and
// writes config.json there when body is not empty. It returns the home.
func linkTestHome(t *testing.T, body string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	t.Setenv("CLIHELP_NO_AUTO_COMPLETION", "1")
	if body == "" {
		return home
	}
	dir := filepath.Join(home, ".config", app.AppName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return home
}

// fixedInvocations is an App.Invocations that declares one link.
func fixedInvocations(name string, args ...string) func() ([]clihelp.Invocation, error) {
	return func() ([]clihelp.Invocation, error) {
		return []clihelp.Invocation{{Name: name, Args: args}}, nil
	}
}

func TestNormalizeInvokedArgs(t *testing.T) {
	for _, tt := range []struct {
		name   string
		preset []string
		args   []string
		want   []string
	}{
		{"index after preset", []string{"-A", "work", "tui"}, []string{"-2"}, []string{"--account-index=2"}},
		// The preset ends in a flag that takes a value, so the typed -2 is
		// that value: walking the typed words alone would have made it an
		// account.
		{"preset ends in a value flag", []string{"-A", "work", "scan", "--pattern"}, []string{"-2"}, []string{"-2"}},
		{"completion", []string{"-A", "work", "tui"}, []string{"__complete", "-2", ""}, []string{"__complete", "--account-index=2", ""}},
		{"completion after a value flag", []string{"scan", "--pattern"}, []string{"__complete", "-2"}, []string{"__complete", "-2"}},
		{"explanation is one word", []string{"tui"}, []string{"__explain", "mt -2"}, []string{"__explain", "mt -2"}},
		{"setup is never expanded", []string{"tui"}, []string{"__clihelp", "x", "-2"}, []string{"__clihelp", "x", "-2"}},
		{"empty preset", nil, []string{"-2", "tui"}, []string{"--account-index=2", "tui"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cliApp := cli.InitCLI(&cli.Session{Config: &cfg_g.Config{}})
			cliApp.InvokedAs = "mt"
			cliApp.Invocations = fixedInvocations("mt", tt.preset...)
			got, err := normalizeInvokedArgs(cliApp, tt.args)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("normalized = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLinkInvocationsReadConfig(t *testing.T) {
	linkTestHome(t, `{"links": {
		"mt": {"account": "fastmail", "args": ["tui"], "description": "mail"},
		"two": {"args": ["-2", "scan"]},
		"plain": {}}}`)
	cliApp := cli.InitCLI(&cli.Session{Config: &cfg_g.Config{}})
	invs, err := linkInvocations(cliApp)()
	if err != nil {
		t.Fatal(err)
	}
	want := []clihelp.Invocation{
		{Name: "mt", Args: []string{"-A", "fastmail", "tui"}, Description: "mail"},
		{Name: "plain", Args: nil},
		// A -N written by hand is stored canonical when it is read, so that
		// normalizeInvokedArgs finds the preset as clihelp will run it.
		{Name: "two", Args: []string{"--account-index=2", "scan"}},
	}
	if len(invs) != len(want) {
		t.Fatalf("invocations = %#v", invs)
	}
	for i := range want {
		if invs[i].Name != want[i].Name || invs[i].Description != want[i].Description ||
			strings.Join(invs[i].Args, " ") != strings.Join(want[i].Args, " ") {
			t.Errorf("invocation %d = %#v, want %#v", i, invs[i], want[i])
		}
	}
}

func TestLinkInvocationsWithoutConfigDoNotCreateIt(t *testing.T) {
	home := linkTestHome(t, "")
	invs, err := linkInvocations(cli.InitCLI(&cli.Session{Config: &cfg_g.Config{}}))()
	if err != nil || len(invs) != 0 {
		t.Fatalf("invocations=%v err=%v", invs, err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", app.AppName)); !os.IsNotExist(err) {
		t.Fatalf("reading the links created the configuration directory: %v", err)
	}
}

// A config.json that cannot be read refuses a run under a link -- run
// unexpanded, `gmail delete` would act on the default account -- but not a run
// as feral, which needs no links.
func TestDamagedConfigRefusesALinkOnly(t *testing.T) {
	linkTestHome(t, `{"links": {`)
	for _, tt := range []struct {
		invoked string
		wantErr bool
	}{
		{"mt", true},
		{app.AppName, false},
	} {
		cliApp := buildCLI()
		cliApp.InvokedAs = tt.invoked
		var out bytes.Buffer
		cliApp.Stdout, cliApp.Stderr = &out, &out
		err := cliApp.Execute([]string{"--help"})
		if (err != nil) != tt.wantErr {
			t.Errorf("as %s: err = %v, want error %v", tt.invoked, err, tt.wantErr)
		}
	}
}

func TestLinkCompletesAndExplainsItsPreset(t *testing.T) {
	linkTestHome(t, `{"links": {"mt": {"account": "work", "args": ["tui"]}}}`)
	cliApp := buildCLI()
	cliApp.InvokedAs = "mt"
	var out bytes.Buffer
	cliApp.Stdout = &out
	args, err := normalizeInvokedArgs(cliApp, []string{"__complete", ""})
	if err == nil {
		err = cliApp.Execute(args)
	}
	if err != nil {
		t.Fatalf("__complete: %v", err)
	}
	words := map[string]bool{}
	for _, line := range strings.Split(out.String(), "\n") {
		words[strings.SplitN(line, "\t", 2)[0]] = true
	}
	if !words["write"] || words["scan"] {
		t.Errorf("completion under mt is not tui's:\n%s", out.String())
	}

	out.Reset()
	if err := cliApp.Execute([]string{"__explain", "mt "}); err != nil {
		t.Fatalf("__explain: %v", err)
	}
	first, rest, _ := strings.Cut(out.String(), "\n")
	if first != "mt " || !strings.Contains(rest, "tui") {
		t.Errorf("explanation under mt:\n%s", out.String())
	}

	out.Reset()
	if err := cliApp.Execute([]string{"--help"}); err != nil {
		t.Fatalf("--help: %v", err)
	}
	if !strings.HasPrefix(out.String(), "mt runs: feral -A work tui") {
		t.Errorf("help under mt does not say what it runs:\n%s", out.String())
	}
}

// Run end to end through buildCLI: a link selects its account through its
// preset, a typed -A beats it, and a stray name that is an account's is
// refused once config.json has links.
func TestLinkSelectsItsAccountAndStrayNamesAreRefused(t *testing.T) {
	linkTestHome(t, `{"accounts": [{"name": "gmail", "type": "jmap"}, {"name": "work", "type": "jmap"}],
		"links": {"mt": {"account": "work", "args": ["--read-only"]}}}`)
	for _, tt := range []struct {
		invoked string
		args    []string
		want    string
		refused bool
	}{
		{invoked: "mt", want: "work"},
		{invoked: "mt", args: []string{"-A", "gmail"}, want: "gmail"},
		{invoked: "mt", args: []string{"-1"}, want: "gmail"},
		{invoked: app.AppName},
		{invoked: "gmail", refused: true},
		{invoked: "mail_cli"},
	} {
		// buildCLI, with the session kept to read the selection back.
		session := buildSession(&cfg_g.Config{})
		session.Bootstrap = initializeRuntime
		cliApp := cli.InitCLI(session)
		cliApp.Invocations = linkInvocations(cliApp)
		cliApp.InvokedAs = tt.invoked
		cliApp.Run = func(*clihelp.Context) error { return nil }
		args, err := normalizeInvokedArgs(cliApp, tt.args)
		if err == nil {
			err = cliApp.Execute(args)
		}
		app.CloseLogger()
		if (err != nil) != tt.refused {
			t.Errorf("as %s %v: err = %v, want refused %v", tt.invoked, tt.args, err, tt.refused)
			continue
		}
		if selected := string(session.Config.SelectedAccount); !tt.refused && selected != tt.want {
			t.Errorf("as %s %v: selected %q, want %q", tt.invoked, tt.args, selected, tt.want)
		}
	}
}
