package tui

import (
	"os"
	"strings"
	"testing"

	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/mailclient"

	tea "github.com/charmbracelet/bubbletea"
)

func TestBrowser_OpenInDetailMode(t *testing.T) {
	openedBrowserFilesMu.Lock()
	openedBrowserFiles = nil
	openedBrowserFilesMu.Unlock()

	emails := []email.Email{{ID: "msg-1", Subject: "HTML Test"}}
	m := newDetailModel(t, 80, 24, "INBOX", emails)
	m.selectedID = "msg-1"
	mock := m.client.(*mailclient.MockMailClient)
	mock.RawMessages = map[email.ID][]byte{
		"msg-1": []byte("From: sender@example.com\r\nSubject: HTML Test\r\nContent-Type: text/html; charset=utf-8\r\n\r\n<p>HTML Body</p>"),
	}

	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	tm := model.(*tuiModel)
	if cmd == nil {
		t.Fatal("expected non-nil cmd on 'w' in modeDetail")
	}

	msg := cmd()
	openedMsg, ok := msg.(browserOpenedMsg)
	if !ok {
		t.Fatalf("expected browserOpenedMsg, got %T", msg)
	}
	if openedMsg.err != nil {
		t.Fatalf("browserOpenedMsg returned error: %v", openedMsg.err)
	}

	openedBrowserFilesMu.Lock()
	count := len(openedBrowserFiles)
	var openedPath string
	if count > 0 {
		openedPath = openedBrowserFiles[0]
	}
	openedBrowserFilesMu.Unlock()

	if count != 1 {
		t.Fatalf("expected 1 opened file, got %d", count)
	}

	content, err := os.ReadFile(openedPath)
	if err != nil {
		t.Fatalf("reading opened file %s: %v", openedPath, err)
	}
	if !strings.Contains(string(content), "<p>HTML Body</p>") {
		t.Errorf("expected HTML body in file, got: %s", string(content))
	}

	// Delivering the message updates status
	model, _ = tm.Update(openedMsg)
	tm = model.(*tuiModel)
	if tm.saveStatus != "Opened message in browser" {
		t.Errorf("expected 'Opened message in browser', got %q", tm.saveStatus)
	}
}

func TestBrowser_OpenInIndexMode(t *testing.T) {
	openedBrowserFilesMu.Lock()
	openedBrowserFiles = nil
	openedBrowserFilesMu.Unlock()

	emails := []email.Email{{ID: "msg-idx", Subject: "Index Test"}}
	m := newDetailModel(t, 80, 24, "INBOX", emails)
	m.mode = modeIndex
	m.eIdx = 0
	mock := m.client.(*mailclient.MockMailClient)
	mock.RawMessages = map[email.ID][]byte{
		"msg-idx": []byte("From: sender@example.com\r\nSubject: Index Test\r\nContent-Type: text/html\r\n\r\n<b>Index HTML</b>"),
	}

	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	if cmd == nil {
		t.Fatal("expected non-nil cmd on 'w' in modeIndex")
	}

	msg := cmd()
	openedMsg, ok := msg.(browserOpenedMsg)
	if !ok || openedMsg.err != nil {
		t.Fatalf("unexpected browserOpenedMsg: %+v", msg)
	}

	openedBrowserFilesMu.Lock()
	count := len(openedBrowserFiles)
	var path string
	if count > 0 {
		path = openedBrowserFiles[0]
	}
	openedBrowserFilesMu.Unlock()

	if count != 1 {
		t.Fatalf("expected 1 opened file, got %d", count)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "<b>Index HTML</b>") {
		t.Errorf("expected HTML content in file, got: %s", string(data))
	}
	_ = model
}

func TestBrowser_PlainFallback(t *testing.T) {
	openedBrowserFilesMu.Lock()
	openedBrowserFiles = nil
	openedBrowserFilesMu.Unlock()

	emails := []email.Email{{ID: "msg-plain", Subject: "Plain Test"}}
	m := newDetailModel(t, 80, 24, "INBOX", emails)
	m.selectedID = "msg-plain"
	mock := m.client.(*mailclient.MockMailClient)
	mock.RawMessages = map[email.ID][]byte{
		"msg-plain": []byte("From: a@b.c\r\nContent-Type: text/plain\r\n\r\nPlain <text> & symbols"),
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	if cmd == nil {
		t.Fatal("expected cmd on 'w'")
	}
	msg := cmd()
	openedMsg := msg.(browserOpenedMsg)
	if openedMsg.err != nil {
		t.Fatalf("unexpected error: %v", openedMsg.err)
	}

	openedBrowserFilesMu.Lock()
	path := openedBrowserFiles[0]
	openedBrowserFilesMu.Unlock()

	data, _ := os.ReadFile(path)
	body := string(data)
	if !strings.Contains(body, "&lt;text&gt; &amp; symbols") {
		t.Errorf("expected escaped text in file, got %s", body)
	}
	if !strings.Contains(body, "<pre style=") {
		t.Errorf("expected <pre> tag, got %s", body)
	}
}

func TestBrowser_NoEmailSelected(t *testing.T) {
	m := newDetailModel(t, 80, 24, "INBOX", nil)
	m.selectedID = ""
	m.emails = nil

	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	tm := model.(*tuiModel)
	if cmd != nil {
		t.Errorf("expected nil cmd when no email selected")
	}
	if !strings.Contains(tm.saveStatus, "No email selected") {
		t.Errorf("expected 'No email selected' in saveStatus, got %q", tm.saveStatus)
	}
}

func TestBrowser_PreferredBrowser(t *testing.T) {
	orig := openBrowserFile
	defer func() { openBrowserFile = orig }()

	var receivedCmd string
	openBrowserFile = func(_, preferred string) error {
		receivedCmd = preferred
		return nil
	}

	emails := []email.Email{{ID: "msg-pref", Subject: "Pref Test"}}
	m := newDetailModel(t, 80, 24, "INBOX", emails)
	m.selectedID = "msg-pref"
	m.cfg = &cfg_g.Config{Prefs: cfg_g.Prefs{Browser: "custom-chrome"}}
	mock := m.client.(*mailclient.MockMailClient)
	mock.Cfg = m.cfg
	mock.RawMessages = map[email.ID][]byte{
		"msg-pref": []byte("From: a@b.c\r\nContent-Type: text/html\r\n\r\n<p>Pref</p>"),
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	if cmd == nil {
		t.Fatal("expected cmd")
	}
	_ = cmd()

	if receivedCmd != "custom-chrome" {
		t.Errorf("expected preferredCmd 'custom-chrome', got %q", receivedCmd)
	}
}
