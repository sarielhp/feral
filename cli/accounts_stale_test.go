package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/feral/app"
	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/mailclient"
)

// staleSession is a session with "main" and the relay "relay", whose
// client is relay; it records which account indexes were asked for. The
// buffer is for the CLI app's Stdout, which becomes the session's Out.
func staleSession(relay *mailclient.MockMailClient, asked *[]int) (*Session, *bytes.Buffer) {
	out := &bytes.Buffer{}
	return &Session{
		Config: &cfg_g.Config{Accounts: []cfg_acc.AccountConfig{
			{Name: "main", Type: "gmail"},
			{Name: "relay", Type: "outlook", InboxMaxAge: "15m"},
		}},
		Deps: Deps{Clients: stubClients{
			ForAccountIndexFn: func(_ *cfg_g.Config, i int) (mailclient.MailClient, error) {
				*asked = append(*asked, i)
				return relay, nil
			},
		}},
	}, out
}

// account stale is meant for cron: a stale or uncheckable relay is a
// non-zero exit with the reason on stdout, a drained one is a clean exit,
// and only relays are asked.
func TestAccountStaleExitStatus(t *testing.T) {
	stuck := mailclient.InboxEntry{ID: "x", Received: time.Now().Add(-time.Hour), Subject: "stuck"}
	cases := []struct {
		name    string
		client  *mailclient.MockMailClient
		args    []string
		wantErr error
		wantOut string
	}{
		{"drained", &mailclient.MockMailClient{}, nil, nil, "inbox drained"},
		{"stale", &mailclient.MockMailClient{InboxEntries: []mailclient.InboxEntry{stuck}}, nil, errRelaysStale, "at least 1h00m old"},
		{"cannot check", &mailclient.MockMailClient{Err: mailclient.ErrInteractiveAuthRequired}, nil, errRelaysStale, "cannot check"},
		{"named", &mailclient.MockMailClient{}, []string{"relay"}, nil, "relay: inbox drained"},
		{"named, no limit", &mailclient.MockMailClient{}, []string{"main"}, app.ErrUsage, ""},
		{"unknown", &mailclient.MockMailClient{}, []string{"nope"}, errors.New("any"), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var asked []int
			session, out := staleSession(tc.client, &asked)
			cliApp := InitCLI(session)
			cliApp.Stdout = out
			err := cliApp.Execute(append([]string{"account", "stale"}, tc.args...))
			switch {
			case tc.wantErr == nil && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tc.wantErr != nil && err == nil:
				t.Fatalf("want an error (%v), got none; output %q", tc.wantErr, out)
			case tc.wantErr == errRelaysStale || tc.wantErr == app.ErrUsage:
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("error %v is not %v", err, tc.wantErr)
				}
			}
			if !strings.Contains(out.String(), tc.wantOut) {
				t.Errorf("output %q lacks %q", out, tc.wantOut)
			}
			for _, i := range asked {
				if i != 2 {
					t.Errorf("asked for account %d; only the relay (2) should be checked", i)
				}
			}
		})
	}
}

func TestAccountInboxMaxAgeSetsAndClears(t *testing.T) {
	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "config.json")
	data, err := json.Marshal(cfg_g.FileConfig{Accounts: &[]cfg_acc.AccountConfig{{Name: "relay", Type: "outlook"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	session := &Session{Config: &cfg_g.Config{Paths: cfg_g.Paths{ConfigDir: configDir, CacheRoot: t.TempDir()}}}
	stored := func() string {
		fc, err := cfg_g.LoadConfigFile(configPath)
		if err != nil {
			t.Fatal(err)
		}
		return (*fc.Accounts)[0].InboxMaxAge
	}

	if err := InitCLI(session).Execute([]string{"account", "inbox-max-age", "relay", "30s"}); !errors.Is(err, app.ErrUsage) {
		t.Errorf("30s: want a usage error, got %v", err)
	}
	if err := InitCLI(session).Execute([]string{"account", "inbox-max-age", "relay", "15m"}); err != nil {
		t.Fatalf("15m: %v", err)
	}
	if got := stored(); got != "15m" {
		t.Errorf("stored %q, want 15m", got)
	}
	if err := InitCLI(session).Execute([]string{"account", "inbox-max-age", "relay", "off"}); err != nil {
		t.Fatalf("off: %v", err)
	}
	if got := stored(); got != "" {
		t.Errorf("off stored %q, want it cleared", got)
	}
}
