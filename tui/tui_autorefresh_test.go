package tui

import (
	"errors"
	"testing"
	"time"

	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/mailclient"

	tea "github.com/charmbracelet/bubbletea"
)

func newTestAutoRefreshModel(tempDir string) *tuiModel {
	cfg := &cfg_g.Config{
		Paths:        cfg_g.Paths{CacheRoot: tempDir},
		AccountCache: cachedir.At(tempDir),
	}
	cl := &mailclient.MockMailClient{Cfg: cfg}
	m := newTuiModel(cl, "INBOX", nil)
	m.height = 20
	m.width = 80
	m.currentF = "INBOX"
	return m
}

func TestAutoRefreshPeriodicCheck(t *testing.T) {
	tempDir := t.TempDir()
	m := newTestAutoRefreshModel(tempDir)

	// Less than 60s since last check: should NOT trigger check.
	m.lastCheckTime = time.Now().Add(-20 * time.Second)
	m.lastActivityTime = time.Now()
	res, _ := m.Update(timeTickMsg{})
	m = res.(*tuiModel)
	if m.bgCheckingMail {
		t.Fatal("expected bgCheckingMail to remain false when < 60s since last check")
	}

	// At least 60s since last check: SHOULD trigger check.
	m.lastCheckTime = time.Now().Add(-65 * time.Second)
	res, cmd := m.Update(timeTickMsg{})
	m = res.(*tuiModel)
	if !m.bgCheckingMail {
		t.Fatal("expected bgCheckingMail to be true when >= 60s since last check")
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd batch when background check is triggered")
	}
	if time.Since(m.lastCheckTime) > 2*time.Second {
		t.Fatal("expected lastCheckTime to be updated to now")
	}
}

func TestAutoRefreshInactivityKeyCheck(t *testing.T) {
	tempDir := t.TempDir()
	m := newTestAutoRefreshModel(tempDir)

	// 1. User active recently (< 30s): key press does not trigger check.
	m.lastActivityTime = time.Now().Add(-10 * time.Second)
	m.lastCheckTime = time.Now().Add(-40 * time.Second)
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = res.(*tuiModel)
	if m.bgCheckingMail {
		t.Fatal("expected no background check when activity was < 30s ago")
	}

	// 2. User inactive > 30s, and last check was > 30s ago: key press triggers check.
	m.lastActivityTime = time.Now().Add(-35 * time.Second)
	m.lastCheckTime = time.Now().Add(-40 * time.Second)
	res, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = res.(*tuiModel)
	if !m.bgCheckingMail {
		t.Fatal("expected bgCheckingMail to be true when key pressed after > 30s inactivity")
	}
	if cmd == nil {
		t.Fatal("expected cmd to include background check")
	}

	// 3. User inactive > 30s, but check ran very recently (< 30s ago): throttled.
	m.bgCheckingMail = false
	m.lastActivityTime = time.Now().Add(-35 * time.Second)
	m.lastCheckTime = time.Now().Add(-10 * time.Second)
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = res.(*tuiModel)
	if m.bgCheckingMail {
		t.Fatal("expected background check to be throttled when lastCheckTime < 30s ago")
	}
}

func TestAutoRefreshCanCheckGuards(t *testing.T) {
	tempDir := t.TempDir()
	m := newTestAutoRefreshModel(tempDir)

	m.lastCheckTime = time.Now().Add(-65 * time.Second)

	// Guard 1: In compose mode.
	m.mode = modeCompose
	if m.canCheckNewMail() {
		t.Error("canCheckNewMail should be false in compose mode")
	}
	m.mode = modeIndex

	// Guard 2: In search view.
	m.currentF = searchFolderPrefix + "test"
	if m.canCheckNewMail() {
		t.Error("canCheckNewMail should be false in search view")
	}
	m.currentF = "INBOX"

	// Guard 3: In standalone compose.
	m.standaloneCompose = true
	if m.canCheckNewMail() {
		t.Error("canCheckNewMail should be false in standalone compose")
	}
	m.standaloneCompose = false

	// Guard 4: Already checking.
	m.bgCheckingMail = true
	if m.canCheckNewMail() {
		t.Error("canCheckNewMail should be false when bgCheckingMail is true")
	}
	m.bgCheckingMail = false

	// Guard 5: showLoad is non-empty.
	m.showLoad = "Uploading draft..."
	if m.canCheckNewMail() {
		t.Error("canCheckNewMail should be false when showLoad is non-empty")
	}
	m.showLoad = ""

	// Guard 6: Tree or overlays open.
	m.treeOpen = true
	if m.canCheckNewMail() {
		t.Error("canCheckNewMail should be false when treeOpen is true")
	}
	m.treeOpen = false

	m.menuOpen = true
	if m.canCheckNewMail() {
		t.Error("canCheckNewMail should be false when menuOpen is true")
	}
	m.menuOpen = false

	m.spamMenuOpen = true
	if m.canCheckNewMail() {
		t.Error("canCheckNewMail should be false when spamMenuOpen is true")
	}
	m.spamMenuOpen = false

	if !m.canCheckNewMail() {
		t.Error("canCheckNewMail should be true when all conditions are met")
	}

	// loadEmailsCmd updates lastCheckTime
	m.lastCheckTime = time.Now().Add(-60 * time.Second)
	_ = m.loadEmailsCmd("INBOX")
	if time.Since(m.lastCheckTime) > 2*time.Second {
		t.Error("loadEmailsCmd should update lastCheckTime to now")
	}
}

func TestAutoRefreshEmailsLoadedHandling(t *testing.T) {
	tempDir := t.TempDir()
	m := newTestAutoRefreshModel(tempDir)
	m.bgCheckingMail = true

	e0 := email.Email{ID: "e0", Subject: "Existing"}
	e1 := email.Email{ID: "e1", Subject: "New Arrival"}
	m.rawEmails = []email.Email{e0}
	m.rebuildVisibleEmails()
	m.selectedID = "e0"

	// Incoming emails for another folder should be discarded.
	res, _ := m.Update(emailsLoadedMsg{folder: "Archive", emails: []email.Email{e1}})
	m = res.(*tuiModel)
	if len(m.emails) != 1 || m.emails[0].ID != "e0" {
		t.Errorf("expected emails to be untouched when folder does not match, got %v", m.emails)
	}
	if m.bgCheckingMail {
		t.Error("bgCheckingMail should be reset on emailsLoadedMsg")
	}

	// Incoming emails for current folder should update the list.
	m.bgCheckingMail = true
	res, _ = m.Update(emailsLoadedMsg{folder: "INBOX", emails: []email.Email{e1, e0}})
	m = res.(*tuiModel)
	if len(m.emails) != 2 {
		t.Fatalf("expected 2 emails after refresh, got %d", len(m.emails))
	}
	if m.selectedID != "e0" {
		t.Errorf("expected selectedID 'e0' to be preserved, got %q", m.selectedID)
	}
	if m.bgCheckingMail {
		t.Error("bgCheckingMail should be cleared")
	}
}

func TestAutoRefreshBgCheckErrorSilent(t *testing.T) {
	tempDir := t.TempDir()
	m := newTestAutoRefreshModel(tempDir)
	m.bgCheckingMail = true

	res, _ := m.Update(bgCheckDoneMsg{folder: "INBOX", err: errors.New("network timeout")})
	m = res.(*tuiModel)

	if m.bgCheckingMail {
		t.Error("bgCheckingMail should be reset to false on bgCheckDoneMsg")
	}
	if m.showError {
		t.Error("background check error should not set showError = true")
	}
}
