package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/feral/app"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/outbox"
	"github.com/sarielhp/feral/when"
)

// --at reaches the service as a due time; a past or unreadable time is a
// usage error and nothing is sent.
func TestRunSendAt(t *testing.T) {
	flagSendBody, flagSendSubject = "body", "Later"
	defer func() { flagSendBody, flagSendSubject, flagSendAt = "", "", "" }()

	var sent *SendOptions
	session := &Session{
		Config: &cfg_g.Config{SelectedAccount: "work"},
		Deps: Deps{Send: stubSend{SendFn: func(_ *cfg_g.Config, opts SendOptions) error {
			sent = &opts
			return nil
		}}},
	}

	flagSendAt = "+2h"
	before := time.Now()
	if err := runSend(session, []string{"to@example.com"}); err != nil {
		t.Fatal(err)
	}
	if sent == nil || sent.SendAfter.Before(before.Add(2*time.Hour)) || sent.SendAfter.After(time.Now().Add(2*time.Hour)) {
		t.Fatalf("--at +2h sent %+v; want a due time two hours on", sent)
	}

	for in, want := range map[string]error{"2020-01-01 08:00": when.ErrPast, "soonish": when.ErrUnknownForm} {
		sent = nil
		flagSendAt = in
		err := runSend(session, []string{"to@example.com"})
		if !errors.Is(err, app.ErrUsage) || !errors.Is(err, want) || sent != nil {
			t.Errorf("--at %q: err = %v, sent = %v; want a usage error wrapping %v and nothing sent", in, err, sent, want)
		}
	}
}

// --at through the real command line: a value starting with "+" or holding a
// space parses as one flag value; a past time and a blank one are usage
// errors and nothing is sent -- a blank one must not mean "now".
func TestSendAtThroughTheCommandLine(t *testing.T) {
	defer func() { flagSendBody, flagSendSubject, flagSendAt = "", "", "" }()
	var sent *SendOptions
	session := &Session{
		Config: &cfg_g.Config{SelectedAccount: "work"},
		Deps: Deps{Send: stubSend{SendFn: func(_ *cfg_g.Config, opts SendOptions) error {
			sent = &opts
			return nil
		}}},
	}
	cliApp := InitCLI(session)
	cliApp.Stdout = &bytes.Buffer{}
	cliApp.Stderr = &bytes.Buffer{}
	run := func(at string) error {
		sent, flagSendAt = nil, ""
		return cliApp.Execute([]string{"msg", "send", "--at", at, "-s", "S", "--body", "b", "to@example.com"})
	}
	for _, at := range []string{"+2h", "tomorrow 8am", "2099-01-01 08:00"} {
		if err := run(at); err != nil || sent == nil || sent.SendAfter.IsZero() {
			t.Errorf("--at %q: err = %v, sent = %+v; want a due time", at, err, sent)
		}
	}
	for _, at := range []string{"2020-01-01 08:00", "", "  "} {
		if err := run(at); !errors.Is(err, app.ErrUsage) || sent != nil {
			t.Errorf("--at %q: err = %v, sent = %+v; want a usage error and nothing sent", at, err, sent)
		}
	}
}

func scheduledOutboxSession(buf *bytes.Buffer, entries []outbox.Entry, flushed []outbox.FlushResult) *Session {
	return &Session{
		Config: &cfg_g.Config{},
		Out:    buf,
		Deps: Deps{Outbox: stubOutbox{
			ListFn:  func(*cfg_g.Config) ([]outbox.Entry, error) { return entries, nil },
			FlushFn: func(*cfg_g.Config) ([]outbox.FlushResult, error) { return flushed, nil },
		}},
	}
}

// ls shows when each entry is sent; a flush that found nothing due says what
// is waiting rather than that the outbox is empty, and does not fail.
func TestOutboxShowsAndWaitsForDueTimes(t *testing.T) {
	due := time.Now().Add(3 * time.Hour)
	entries := []outbox.Entry{
		{ID: "now-1", Account: "work", Subject: "Now"},
		{ID: "later-1", Account: "work", Subject: "Later", Format: 2, SendAfter: due},
		{ID: "newer-1", Format: 3},
	}
	var buf bytes.Buffer
	session := scheduledOutboxSession(&buf, entries, nil)

	if err := runOutboxList(session); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"now  ", due.Format("2006-01-02 15:04"), "format 3 (newer)"} {
		if !strings.Contains(out, want) {
			t.Errorf("ls output lacks %q:\n%s", want, out)
		}
	}

	buf.Reset()
	if err := runOutboxFlush(session); err != nil {
		t.Fatalf("a flush with only a scheduled entry waiting must not fail: %v", err)
	}
	if out := buf.String(); strings.Contains(out, "empty") || !strings.Contains(out, "1 message(s) scheduled for later") {
		t.Errorf("flush output = %q", out)
	}
}

func TestOutboxFlushDryRunNamesWhatIsNotDue(t *testing.T) {
	due := time.Now().Add(time.Hour)
	var buf bytes.Buffer
	session := scheduledOutboxSession(&buf, []outbox.Entry{
		{ID: "a", Account: "work", Subject: "Now"},
		{ID: "b", Account: "work", Subject: "Later", Format: 2, SendAfter: due},
	}, nil)
	session.Config.DryRun = true
	if err := runOutboxFlush(session); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "Would send [work] Now (a)") || strings.Contains(out, "Would send [work] Later") ||
		!strings.Contains(out, "Not due until "+due.Format("2006-01-02 15:04")+": [work] Later (b)") {
		t.Errorf("dry-run flush output = %q", out)
	}
}
