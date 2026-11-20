package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/feral/app"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/mailclient"
	"github.com/sarielhp/feral/snooze"
)

type snoozeHarness struct {
	session *Session
	out     *bytes.Buffer
	calls   []string
	until   time.Time
	run     func(args ...string) error
}

// newSnoozeHarness runs `msg snooze` through the real command line, with
// global flags given as flags: the run's own setup decides what --read-only
// and --dry-run mean.
func newSnoozeHarness(t *testing.T, cfg *cfg_g.Config, entries ...snooze.Entry) *snoozeHarness {
	t.Helper()
	t.Cleanup(func() {
		app.FlagReadOnly, app.FlagDryRun = false, false
		cfg_g.ForbidConfigWrites(false)
	})
	h := &snoozeHarness{out: &bytes.Buffer{}}
	h.session = &Session{
		Config: cfg,
		Deps: Deps{
			Snooze: stubSnooze{entries: entries, calls: &h.calls, until: &h.until},
			Clients: stubClients{ForSelectedFn: func(*cfg_g.Config) (mailclient.MailClient, error) {
				return &mailclient.MockMailClient{}, nil
			}},
		},
	}
	cliApp := InitCLI(h.session)
	cliApp.Stdout = h.out
	cliApp.Stderr = &bytes.Buffer{}
	h.run = func(args ...string) error {
		h.out.Reset()
		h.calls = nil
		app.FlagReadOnly, app.FlagDryRun = false, false
		return cliApp.Execute(append([]string{"msg", "snooze"}, args...))
	}
	return h
}

// --until reaches the service as a future time; a blank, past or missing
// time is a usage error and nothing is snoozed.
func TestSnoozeUntilThroughTheCommandLine(t *testing.T) {
	h := newSnoozeHarness(t, &cfg_g.Config{SelectedAccount: "work"})
	before := time.Now()
	if err := h.run("--until", "+2h", "abc"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(h.calls, ",") != "snooze" || h.until.Before(before.Add(2*time.Hour)) {
		t.Fatalf("calls %v, until %v; want one snooze two hours on", h.calls, h.until)
	}
	if !strings.Contains(h.out.String(), "Snoozed") {
		t.Errorf("output %q", h.out.String())
	}
	for _, args := range [][]string{
		{"--until", "", "abc"},
		{"--until", "  ", "abc"},
		{"--until", "2020-01-01 08:00", "abc"},
		{"--until", "+2h"},
		{"abc"},
		{"--list", "abc"},
		{"--forget"},
	} {
		if err := h.run(args...); !errors.Is(err, app.ErrUsage) || len(h.calls) != 0 {
			t.Errorf("%q: err = %v, calls %v; want a usage error and nothing done", args, err, h.calls)
		}
	}
}

// --read-only refuses to snooze or wake, since the record would describe a
// move that never happened; --dry-run says what would happen and does none
// of it.
func TestSnoozeReadOnlyRefusesDryRunReports(t *testing.T) {
	entry := snooze.Entry{Account: "work", ID: "m1", Subject: "Lunch", Folder: snooze.FolderName, Until: time.Now()}

	ro := newSnoozeHarness(t, &cfg_g.Config{SelectedAccount: "work"}, entry)
	for _, args := range [][]string{{"--until", "+1h", "m1"}, {"--wake", "m1"}, {"--wake"}} {
		err := ro.run(append([]string{"--read-only"}, args...)...)
		if err == nil || !strings.Contains(err.Error(), "read-only") || len(ro.calls) != 0 {
			t.Errorf("--read-only %q: err = %v, calls %v", args, err, ro.calls)
		}
	}

	dry := newSnoozeHarness(t, &cfg_g.Config{SelectedAccount: "work"}, entry)
	for args, want := range map[string]string{
		"--until=+1h m1": "[DRY-RUN] Would snooze",
		"--wake m1":      "[DRY-RUN] Would return",
		"--wake":         "[DRY-RUN] Would return",
		"--forget m1":    "[DRY-RUN] Would forget",
	} {
		err := dry.run(append([]string{"--dry-run"}, strings.Fields(args)...)...)
		if err != nil || len(dry.calls) != 0 || !strings.Contains(dry.out.String(), want) {
			t.Errorf("--dry-run %s: err = %v, calls %v, output %q", args, err, dry.calls, dry.out.String())
		}
	}
}

func TestSnoozeListWakeForget(t *testing.T) {
	entries := []snooze.Entry{
		{Account: "work", ID: "m1", Subject: "Lunch", FromFolder: "INBOX", Folder: snooze.FolderName, Until: time.Now().Add(-time.Minute)},
		{Account: "home", ID: "m2", Subject: "Later", FromFolder: "Lists", Folder: snooze.FolderName, Until: time.Now().Add(time.Hour)},
	}
	h := newSnoozeHarness(t, &cfg_g.Config{SelectedAccount: "work"}, entries...)
	for _, args := range [][]string{nil, {"--list"}} {
		if err := h.run(args...); err != nil {
			t.Fatal(err)
		}
		out := h.out.String()
		if !strings.Contains(out, "due ") || !strings.Contains(out, "Lunch") || !strings.Contains(out, "Later") {
			t.Errorf("list %q: %q", args, out)
		}
	}
	if err := h.run("--wake", "m1"); err != nil || strings.Join(h.calls, ",") != "wake" || !strings.Contains(h.out.String(), "Back in the inbox") {
		t.Errorf("wake: %v, %v, %q", err, h.calls, h.out.String())
	}
	// Forgetting under --read-only is allowed: it records nothing false.
	if err := h.run("--read-only", "--forget", "m2"); err != nil || strings.Join(h.calls, ",") != "forget" || !strings.Contains(h.out.String(), "stays in") {
		t.Errorf("forget: %v, %v, %q", err, h.calls, h.out.String())
	}
	empty := newSnoozeHarness(t, &cfg_g.Config{})
	if err := empty.run(); err != nil || !strings.Contains(empty.out.String(), "Nothing is snoozed") {
		t.Errorf("empty list: %v, %q", err, empty.out.String())
	}
}
