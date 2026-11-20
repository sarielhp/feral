package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sarielhp/clihelp"
	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
)

// configWrites records what the config layer was asked to write.
type configWrites struct {
	set   []string // "key=value"
	reset []string
}

func emailsSession(t *testing.T, myEmails ...string) (*Session, *bytes.Buffer, *bytes.Buffer, *configWrites) {
	t.Helper()
	var out, status bytes.Buffer
	writes := &configWrites{}
	cfg := &cfg_g.Config{
		Accounts: []cfg_acc.AccountConfig{
			{Name: "work", Username: "me@work.example", UserEmail: "Me@Work.example"},
			{Name: "home", Username: "homelogin", UserEmail: "me@home.example"},
		},
	}
	cfg.MyEmails = myEmails
	session := &Session{
		Config: cfg,
		Out:    &out,
		Err:    &status,
		Deps: Deps{Config: stubConfig{
			SetFn: func(_ *cfg_g.Config, k, v string) error {
				writes.set = append(writes.set, k+"="+v)
				return nil
			},
			ResetFn: func(_ *cfg_g.Config, k string) error {
				writes.reset = append(writes.reset, k)
				return nil
			},
			GetFn: func(*cfg_g.Config, string) error { return nil },
		}},
	}
	return session, &out, &status, writes
}

// config emails lists each address once with every place it is set, and the
// commands that change the list.
func TestConfigEmails_ListsSourcesAndCommands(t *testing.T) {
	session, out, _, _ := emailsSession(t, "old@uni.example", "me@home.example")
	if err := newConfigEmailsCmd(session).Run(nil); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{
		"old@uni.example",
		"me@home.example  my_emails, account home (user_email)",
		"me@work.example  account work (user_email), account work (username)",
		"feral config add-email <address>...",
		"feral config remove-email <address>...",
		"feral account user-email <account> <email>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("listing lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "homelogin") {
		t.Errorf("a login that is not an address was listed:\n%s", got)
	}
}

func TestConfigRemoveEmail(t *testing.T) {
	t.Run("removes, case-insensitively", func(t *testing.T) {
		session, _, _, w := emailsSession(t, "a@x.example", "b@x.example")
		if err := runRemoveEmails(session, []string{"A@X.example"}); err != nil {
			t.Fatal(err)
		}
		if len(w.set) != 1 || w.set[0] != "my_emails=b@x.example" {
			t.Fatalf("set = %v", w.set)
		}
	})
	t.Run("the last one clears the list", func(t *testing.T) {
		session, _, _, w := emailsSession(t, "a@x.example")
		if err := runRemoveEmails(session, []string{"a@x.example"}); err != nil {
			t.Fatal(err)
		}
		if len(w.set) != 0 || len(w.reset) != 1 || w.reset[0] != "my_emails" {
			t.Fatalf("set %v reset %v, want my_emails reset", w.set, w.reset)
		}
	})
	t.Run("one bad address refuses them all", func(t *testing.T) {
		session, _, _, w := emailsSession(t, "a@x.example")
		err := runRemoveEmails(session, []string{"a@x.example", "me@home.example", "me@work.example", "nobody@x.example"})
		if err == nil || len(w.set)+len(w.reset) != 0 {
			t.Fatalf("err %v, writes %+v; want a refusal and no write", err, w)
		}
		for _, want := range []string{
			"me@home.example is account home's user_email; change it with `feral account user-email home <email>`",
			"me@work.example is account work's user_email",
			"nobody@x.example is not in my_emails",
		} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error lacks %q: %v", want, err)
			}
		}
	})
}

// An empty my_emails says so on stderr; stdout stays the value, for scripts.
func TestConfigGetMyEmails_EmptySaysSoOnStderr(t *testing.T) {
	session, out, status, _ := emailsSession(t)
	get := findSubcommand(t, ConfigCmd(session), "get")
	if err := get.Run(contextWithArgs("my_emails")); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Errorf("stdout = %q, want only the value (empty)", out.String())
	}
	if !strings.Contains(status.String(), "(none)") {
		t.Errorf("stderr = %q, want the (none) note", status.String())
	}

	session, _, status, _ = emailsSession(t, "a@x.example")
	get = findSubcommand(t, ConfigCmd(session), "get")
	if err := get.Run(contextWithArgs("my_emails")); err != nil {
		t.Fatal(err)
	}
	if status.Len() != 0 {
		t.Errorf("a non-empty list printed a note: %q", status.String())
	}
}

// findSubcommand is cmd's subcommand called name.
func findSubcommand(t *testing.T, cmd clihelp.Command, name string) clihelp.Command {
	t.Helper()
	for _, sub := range cmd.Subcommands {
		if sub.Name == name {
			return sub
		}
	}
	t.Fatalf("%s has no subcommand %s", cmd.Name, name)
	return clihelp.Command{}
}

func contextWithArgs(args ...string) *clihelp.Context {
	return &clihelp.Context{Args: args}
}
