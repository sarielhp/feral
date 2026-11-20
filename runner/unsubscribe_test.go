package runner

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/cli"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/report"
)

// unsubFixture is an account holding one list message with the given
// List-Unsubscribe headers, and a service whose POST and browser record
// instead of acting.
type unsubFixture struct {
	cfg     *cfg_g.Config
	svc     UnsubscribeService
	out     *bytes.Buffer
	posted  []string
	opened  []string
	asked   []string
	confirm func(string) error
}

func newUnsubFixture(t *testing.T, headers string) *unsubFixture {
	t.Helper()
	dir := t.TempDir()
	raw := "From: News <news@list.example>\r\nTo: Worker <user@work.example>\r\nSubject: Weekly\r\n" +
		"Message-ID: <w1@list.example>\r\nDate: Sun, 20 Sep 2026 10:00:00 +0000\r\n" + headers +
		"Content-Type: text/plain\r\n\r\nNews.\r\n"
	setupTestCachedMessage(t, dir, "work", "list001", []byte(raw))
	f := &unsubFixture{
		cfg: &cfg_g.Config{
			Paths:    cfg_g.Paths{ConfigDir: dir, CacheRoot: dir},
			Accounts: []cfg_acc.AccountConfig{{Name: "work", Username: "user@work.example", UserName: "Worker", Type: "jmap"}},
		},
		out:     &bytes.Buffer{},
		confirm: func(string) error { return nil },
	}
	f.svc = UnsubscribeService{
		Rep:          report.Discard(),
		Out:          f.out,
		PostOneClick: func(u string) error { f.posted = append(f.posted, u); return nil },
		OpenBrowser:  func(_ *cfg_g.Config, u string) error { f.opened = append(f.opened, u); return nil },
	}
	return f
}

func (f *unsubFixture) run() error {
	return f.svc.Unsubscribe(f.cfg, cli.UnsubscribeOptions{
		MessageID: "list001",
		Confirm: func(action string) error {
			f.asked = append(f.asked, action)
			return f.confirm(action)
		},
	})
}

func (f *unsubFixture) logEntries(t *testing.T) int {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(f.cfg.ConfigDir, "unsubscriptions.json"))
	if errors.Is(err, os.ErrNotExist) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(data), `"method"`)
}

const oneClickHeaders = "List-Unsubscribe: <mailto:leave@list.example>, <https://list.example/u?t=1>\r\n" +
	"List-Unsubscribe-Post: List-Unsubscribe=One-Click\r\n"

func TestUnsubscribeOneClickAsksThenPosts(t *testing.T) {
	f := newUnsubFixture(t, oneClickHeaders)
	if err := f.run(); err != nil {
		t.Fatalf("Unsubscribe: %v", err)
	}
	if len(f.asked) != 1 || !strings.Contains(f.asked[0], "news@list.example") {
		t.Errorf("asked %q, want one confirmation naming the list", f.asked)
	}
	if len(f.posted) != 1 || f.posted[0] != "https://list.example/u?t=1" {
		t.Errorf("posted %q", f.posted)
	}
	if !strings.Contains(f.out.String(), "POST to https://list.example/u?t=1") {
		t.Errorf("preview = %q", f.out.String())
	}
	if n := f.logEntries(t); n != 1 {
		t.Errorf("log has %d entries, want 1", n)
	}
}

func TestUnsubscribeDeclinedDoesNothing(t *testing.T) {
	f := newUnsubFixture(t, oneClickHeaders)
	f.confirm = func(string) error { return cli.ErrDeclined }
	if err := f.run(); !errors.Is(err, cli.ErrDeclined) {
		t.Fatalf("err = %v, want ErrDeclined", err)
	}
	if len(f.posted)+len(f.opened) != 0 || f.logEntries(t) != 0 {
		t.Errorf("a declined unsubscribe acted: posted %q, opened %q", f.posted, f.opened)
	}
}

// --read-only and --dry-run show the plan and do nothing: no question, no
// request, no browser, no log entry.
func TestUnsubscribeReportsUnderReadOnlyAndDryRun(t *testing.T) {
	for _, mode := range []string{"read-only", "dry-run"} {
		t.Run(mode, func(t *testing.T) {
			f := newUnsubFixture(t, oneClickHeaders)
			f.cfg.ReadOnly = mode == "read-only"
			f.cfg.DryRun = mode == "dry-run"
			if err := f.run(); err != nil {
				t.Fatalf("Unsubscribe: %v", err)
			}
			if len(f.asked)+len(f.posted)+len(f.opened) != 0 || f.logEntries(t) != 0 {
				t.Errorf("acted under --%s: asked %q, posted %q, opened %q", mode, f.asked, f.posted, f.opened)
			}
			if !strings.Contains(f.out.String(), "POST to https://list.example/u") {
				t.Errorf("--%s did not show the plan: %q", mode, f.out.String())
			}
		})
	}
}

// The mailto is shown in full, from the account the message arrived in.
func TestUnsubscribeMailtoPreview(t *testing.T) {
	f := newUnsubFixture(t, "List-Unsubscribe: <mailto:leave@list.example?subject=remove>\r\n")
	f.cfg.DryRun = true
	if err := f.run(); err != nil {
		t.Fatalf("Unsubscribe: %v", err)
	}
	out := f.out.String()
	for _, want := range []string{"From:    Worker <user@work.example>", "To:      leave@list.example", "Subject: remove"} {
		if !strings.Contains(out, want) {
			t.Errorf("preview lacks %q:\n%s", want, out)
		}
	}
}

func TestUnsubscribeBrowserOpensNeverFetches(t *testing.T) {
	f := newUnsubFixture(t, "List-Unsubscribe: <https://list.example/manage>\r\n")
	if err := f.run(); err != nil {
		t.Fatalf("Unsubscribe: %v", err)
	}
	if len(f.posted) != 0 || len(f.opened) != 1 || f.opened[0] != "https://list.example/manage" {
		t.Errorf("posted %q, opened %q; want only the browser", f.posted, f.opened)
	}
}

func TestUnsubscribeWithoutHeaderSaysSo(t *testing.T) {
	f := newUnsubFixture(t, "")
	err := f.run()
	if !errors.Is(err, email.ErrNoListUnsubscribe) {
		t.Fatalf("err = %v, want ErrNoListUnsubscribe", err)
	}
	if len(f.asked) != 0 {
		t.Errorf("asked %q with nothing to do", f.asked)
	}
}
