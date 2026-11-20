package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/mailclient"
)

// relayModel is a TUI on the "main" account with "relay" configured as a
// relay whose client is relayClient.
func relayModel(t *testing.T, relayClient *mailclient.MockMailClient) *tuiModel {
	t.Helper()
	cfg := &cfg_g.Config{
		Paths:        cfg_g.Paths{CacheRoot: t.TempDir()},
		AccountCache: cachedir.At(t.TempDir()),
		Accounts: []cfg_acc.AccountConfig{
			{Name: "main", Type: "gmail"},
			{Name: "relay", Type: "gmail", InboxMaxAge: "15m"},
		},
		SelectedAccount: "main",
	}
	m := newTuiModel(&mailclient.MockMailClient{Cfg: cfg}, "INBOX", nil)
	m.width, m.height = 120, 30
	m.bk = &Backend{Clients: stubClients{
		ForAccountFn: func(_ *cfg_g.Config, acc cfg_acc.AccountConfig) (mailclient.MailClient, error) {
			if acc.Name != "relay" {
				t.Errorf("checked %q, which is not a relay", acc.Name)
			}
			return relayClient, nil
		},
	}}
	return m
}

// runRelayCheck starts a check the way a tick does and feeds its result back.
func runRelayCheck(t *testing.T, m *tuiModel, now time.Time) {
	t.Helper()
	if !m.relayCheckDue(now) {
		t.Fatal("a relay check is not due")
	}
	msg := m.startRelayCheck(now)()
	if m.relayCheckDue(now) {
		t.Error("a second check must not start while one is in flight")
	}
	m.Update(msg)
}

func TestRelayBannerShowsAStaleInboxAndClears(t *testing.T) {
	relay := &mailclient.MockMailClient{InboxEntries: []mailclient.InboxEntry{
		{ID: "x", Received: time.Now().Add(-time.Hour), Subject: "stuck message"},
	}}
	m := relayModel(t, relay)
	lines := m.indexLines()

	runRelayCheck(t, m, time.Now())
	view := m.View()
	if !strings.Contains(view, "relay: inbox has mail") {
		t.Fatalf("no relay banner on screen:\n%s", view)
	}
	if got := m.indexLines(); got != lines-1 {
		t.Errorf("the banner must take a row from the list: %d rows, was %d", got, lines)
	}
	if n := strings.Count(view, "\n") + 1; n > m.height {
		t.Errorf("the view is %d lines on a %d-line screen; the top bar would scroll off", n, m.height)
	}

	if m.relayCheckDue(time.Now().Add(time.Minute)) {
		t.Error("a check is due a minute after the last one")
	}
	relay.InboxEntries = nil
	runRelayCheck(t, m, time.Now().Add(relayCheckInterval))
	if strings.Contains(m.View(), "inbox has mail") || m.bannerLines() != 0 {
		t.Error("the banner outlived a drained inbox")
	}
}

func TestRelayBannerReportsAnUncheckableRelay(t *testing.T) {
	relay := &mailclient.MockMailClient{FailOn: map[string]error{
		"InboxMessageBefore": mailclient.ErrInteractiveAuthRequired,
	}}
	m := relayModel(t, relay)
	runRelayCheck(t, m, time.Now())
	if !strings.Contains(m.View(), "relay: cannot check") {
		t.Errorf("an uncheckable relay must be shown, not taken as drained:\n%s", m.View())
	}
}

func TestNoRelayNoCheck(t *testing.T) {
	m := relayModel(t, &mailclient.MockMailClient{})
	m.Config().Accounts[1].InboxMaxAge = ""
	if m.relayCheckDue(time.Now()) {
		t.Error("a check is due with no relay configured")
	}
}

// With the relay banner and the error banner both up, every layout still
// fits the screen, and the relay banner appears once.
func TestRelayAndErrorBannersFitTheScreen(t *testing.T) {
	m := relayModel(t, &mailclient.MockMailClient{})
	m.relay.alerts = []string{"relay: inbox has mail at least 40m old"}
	m.err, m.showError = errors.New("connection refused"), true
	for i := range 40 {
		m.emails = append(m.emails, email.Email{ID: email.ID(fmt.Sprint(i)), Subject: fmt.Sprint("message ", i)})
	}
	for _, split := range []bool{false, true} {
		m.splitPreview = split
		view := m.view()
		if n := strings.Count(view, "\n") + 1; n > m.height {
			t.Errorf("split=%v: %d lines on a %d-line screen", split, n, m.height)
		}
		if c := strings.Count(view, "inbox has mail"); c != 1 {
			t.Errorf("split=%v: relay banner drawn %d times", split, c)
		}
		if !strings.Contains(view, "connection refused") {
			t.Errorf("split=%v: the error banner is missing", split)
		}
	}
	m.splitPreview, m.mode, m.eIdx = false, modeDetail, 0
	view := m.view()
	if n := strings.Count(view, "\n") + 1; n > m.height || strings.Count(view, "inbox has mail") != 1 {
		t.Errorf("message view: %d lines on %d, banner %d time(s)", n, m.height, strings.Count(view, "inbox has mail"))
	}
}
