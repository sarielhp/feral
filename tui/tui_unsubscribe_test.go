package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/mailclient"
)

// unsubModel is a list holding one message with the given unsubscribe
// headers, in an account that sends as Me <me@work.example>, with the
// one-click POST recorded instead of sent.
func unsubModel(t *testing.T, headers string) (*tuiModel, *mailclient.MockMailClient, *[]string) {
	t.Helper()
	raw := "From: News <news@list.example>\r\nTo: me@work.example\r\nSubject: Weekly\r\n" + headers +
		"Content-Type: text/plain\r\n\r\nNews.\r\n"
	client := &mailclient.MockMailClient{
		RawMessages: map[email.ID][]byte{"l1": []byte(raw)},
		Cfg: &cfg_g.Config{
			Paths:           cfg_g.Paths{ConfigDir: t.TempDir()},
			SelectedAccount: "work",
			Accounts:        []cfg_acc.AccountConfig{{Name: "work", Username: "me@work.example", UserName: "Me"}},
		},
	}
	posted := &[]string{}
	previous := unsubscribePost
	unsubscribePost = func(u string) error { *posted = append(*posted, u); return nil }
	t.Cleanup(func() { unsubscribePost = previous })

	m := newTuiModel(client, "INBOX", nil)
	m.mode = modeIndex
	m.currentF = "INBOX"
	m.rawEmails = []email.Email{{ID: "l1", Subject: "Weekly", FromRaw: "News <news@list.example>", EmailDate: time.Now()}}
	m.rebuildVisibleEmails()
	m.eIdx, m.selectedID = 0, "l1"
	m.width, m.height = 120, 40
	return m, client, posted
}

// press delivers a key and every message the commands it starts produce.
func unsubPress(t *testing.T, m *tuiModel, key string) *tuiModel {
	t.Helper()
	k := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	if key == "esc" {
		k = tea.KeyMsg{Type: tea.KeyEsc}
	}
	res, cmd := m.Update(k)
	m = res.(*tuiModel)
	for i := 0; cmd != nil && i < 5; i++ {
		msg := cmd()
		if msg == nil {
			break
		}
		res, cmd = m.Update(msg)
		m = res.(*tuiModel)
	}
	return m
}

const unsubOneClick = "List-Unsubscribe: <mailto:leave@list.example>, <https://list.example/u?t=1>\r\n" +
	"List-Unsubscribe-Post: List-Unsubscribe=One-Click\r\n"

func TestUnsubscribeKeyAsksThenPosts(t *testing.T) {
	m, _, posted := unsubModel(t, unsubOneClick)
	m = unsubPress(t, m, "X")
	if m.keyScreen() != scrUnsubscribe {
		t.Fatalf("X opened %s, want the unsubscribe dialog", m.keyScreen())
	}
	if len(*posted) != 0 {
		t.Fatal("posted before the answer")
	}
	if view := m.View(); !strings.Contains(view, "POST to https://list.example/u?t=1") {
		t.Errorf("the dialog does not show the plan:\n%s", view)
	}
	m = unsubPress(t, m, "y")
	if len(*posted) != 1 || (*posted)[0] != "https://list.example/u?t=1" {
		t.Errorf("posted %q", *posted)
	}
	if m.unsubDialog != nil || !strings.Contains(m.saveStatus, "Unsubscribed from news@list.example") {
		t.Errorf("after y: dialog %v, status %q", m.unsubDialog != nil, m.saveStatus)
	}
	if _, err := os.Stat(filepath.Join(m.client.Config().ConfigDir, "unsubscriptions.json")); err != nil {
		t.Errorf("the attempt was not logged: %v", err)
	}
}

func TestUnsubscribeKeyCancels(t *testing.T) {
	for _, key := range []string{"n", "esc"} {
		m, client, posted := unsubModel(t, unsubOneClick)
		m = unsubPress(t, m, "X")
		m = unsubPress(t, m, key)
		if m.unsubDialog != nil || len(*posted) != 0 || len(client.SentRawEmails) != 0 {
			t.Errorf("%s: dialog %v, posted %q, sent %d", key, m.unsubDialog != nil, *posted, len(client.SentRawEmails))
		}
	}
}

// Read-only shows the plan and reports it on y; nothing is sent or logged.
func TestUnsubscribeKeyReadOnlyReports(t *testing.T) {
	m, client, posted := unsubModel(t, unsubOneClick)
	client.Cfg.ReadOnly = true
	m = unsubPress(t, m, "X")
	if !strings.Contains(m.View(), "Read-only") {
		t.Error("the dialog does not say it is read-only")
	}
	m = unsubPress(t, m, "y")
	if len(*posted) != 0 || !strings.HasPrefix(m.saveStatus, "[RO] Would") {
		t.Errorf("posted %q, status %q", *posted, m.saveStatus)
	}
	if _, err := os.Stat(filepath.Join(client.Cfg.ConfigDir, "unsubscriptions.json")); err == nil {
		t.Error("a simulated unsubscribe was logged")
	}
}

// A mailto is sent from the message's account, as the dialog showed it.
func TestUnsubscribeKeyMailtoSends(t *testing.T) {
	m, client, _ := unsubModel(t, "List-Unsubscribe: <mailto:leave@list.example?subject=remove>\r\n")
	m = unsubPress(t, m, "X")
	if view := m.View(); !strings.Contains(view, "To:      leave@list.example") || !strings.Contains(view, "Me <me@work.example>") {
		t.Errorf("the dialog does not show the message:\n%s", view)
	}
	m = unsubPress(t, m, "y")
	if len(client.SentRawEmails) != 1 {
		t.Fatalf("sent %d messages, want 1 (status %q, err %v)", len(client.SentRawEmails), m.saveStatus, m.err)
	}
	sent := string(client.SentRawEmails[0])
	for _, want := range []string{"To: leave@list.example", "Subject: remove", "From: Me <me@work.example>"} {
		if !strings.Contains(sent, want) {
			t.Errorf("sent message lacks %q:\n%s", want, sent)
		}
	}
}

func TestUnsubscribeKeyWithoutHeader(t *testing.T) {
	m, _, _ := unsubModel(t, "")
	m = unsubPress(t, m, "X")
	if m.unsubDialog != nil || !strings.Contains(m.saveStatus, "No List-Unsubscribe header") {
		t.Errorf("dialog %v, status %q", m.unsubDialog != nil, m.saveStatus)
	}
}

// The header arrives after the user moved on -- into help, or to another
// message: no dialog opens behind or over what they are doing now.
func TestUnsubscribeLateHeaderIsDropped(t *testing.T) {
	moves := map[string]func(m *tuiModel){
		"help opened":     func(m *tuiModel) { m.showHelp = true },
		"another message": func(m *tuiModel) { m.selectedID, m.eIdx = "other", -1; m.emails = nil },
	}
	for name, move := range moves {
		m, _, _ := unsubModel(t, unsubOneClick)
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("X")})
		prepared := cmd()
		move(m)
		res, _ := m.Update(prepared)
		m = res.(*tuiModel)
		if m.unsubDialog != nil || !strings.Contains(m.saveStatus, "dropped") {
			t.Errorf("%s: dialog %v, status %q", name, m.unsubDialog != nil, m.saveStatus)
		}
	}
}

// The mailto is from the account the message's client belongs to, not the
// default one.
func TestUnsubscribeKeyMailtoFromTheMessagesAccount(t *testing.T) {
	m, client, _ := unsubModel(t, "List-Unsubscribe: <mailto:leave@list.example>\r\n")
	client.Cfg.Accounts = append([]cfg_acc.AccountConfig{{Name: "home", Username: "me@home.example", UserName: "Home Me"}},
		client.Cfg.Accounts...)
	client.Cfg.DefaultAccount = "home"
	m = unsubPress(t, m, "X")
	m = unsubPress(t, m, "y")
	if len(client.SentRawEmails) != 1 || !strings.Contains(string(client.SentRawEmails[0]), "From: Me <me@work.example>") {
		t.Errorf("sent %q, want one message from the work account (status %q)", client.SentRawEmails, m.saveStatus)
	}
}

// A plain link goes to the browser; feral never fetches it.
func TestUnsubscribeKeyBrowserOpensNeverPosts(t *testing.T) {
	m, _, posted := unsubModel(t, "List-Unsubscribe: <https://list.example/manage>\r\n")
	var opened []string
	previous := openBrowserFile
	openBrowserFile = func(target, _ string) error { opened = append(opened, target); return nil }
	t.Cleanup(func() { openBrowserFile = previous })
	m = unsubPress(t, m, "X")
	m = unsubPress(t, m, "y")
	if len(*posted) != 0 || len(opened) != 1 || opened[0] != "https://list.example/manage" {
		t.Errorf("posted %q, opened %q; want only the browser", *posted, opened)
	}
	if !strings.Contains(m.saveStatus, "finish there") {
		t.Errorf("status %q", m.saveStatus)
	}
}

// A link longer than the dialog is broken across lines, never cut short:
// the user confirms what they can read in full.
func TestWrapUnsubPreviewKeepsTheWholeLink(t *testing.T) {
	link := "https://list.example/" + strings.Repeat("a", 150) + "?t=END"
	lines := wrapUnsubPreview("  by: POST to "+link+"\n", 40)
	joined := ""
	for i, l := range lines {
		if len([]rune(l)) > 40 {
			t.Errorf("line %d is %d wide", i, len([]rune(l)))
		}
		joined += strings.TrimPrefix(l, "    ")
	}
	if !strings.Contains(joined, link) {
		t.Errorf("the link did not survive wrapping: %q", lines)
	}
}
