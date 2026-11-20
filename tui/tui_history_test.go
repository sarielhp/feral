package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sarielhp/feral/email"
)

var (
	escKey     = tea.KeyMsg{Type: tea.KeyEsc}
	backKey    = tea.KeyMsg{Type: tea.KeyShiftLeft}
	forwardKey = tea.KeyMsg{Type: tea.KeyShiftRight}
)

// visit puts a view with one message on screen as a move from the current
// one, the way a search or a saved set arrives.
func visit(m *tuiModel, view, id string) {
	m.pushHistory(view)
	m.currentF = view
	m.rawEmails = []email.Email{{ID: email.ID(id), Subject: id}}
	m.rebuildVisibleEmails()
	m.selectSearchRow(email.ID(id))
}

// historyModel is on a saved set, having come from a search, which came
// from INBOX.
func historyModel(t *testing.T) *tuiModel {
	t.Helper()
	m := aliasModel(t)
	m.currentF = "INBOX"
	visit(m, searchFolderPrefix+"q", "s1")
	visit(m, virtualFolderPrefix+"set", "v1")
	return m
}

// Esc and Shift+← walk back, Shift+→ forward again, each view on the message
// it had selected.
func TestHistory_BackAndForward(t *testing.T) {
	m := historyModel(t)
	m.Update(escKey)
	if m.currentF != searchFolderPrefix+"q" || m.selectedID != "s1" {
		t.Fatalf("after Esc: %q on %q", m.currentF, m.selectedID)
	}
	m.Update(forwardKey)
	if m.currentF != virtualFolderPrefix+"set" || m.selectedID != "v1" {
		t.Fatalf("after Shift+→: %q on %q", m.currentF, m.selectedID)
	}
	m.Update(backKey)
	if m.currentF != searchFolderPrefix+"q" {
		t.Fatalf("after Shift+←: %q", m.currentF)
	}
	if !strings.Contains(m.saveStatus, "Back to search results") {
		t.Errorf("status = %q", m.saveStatus)
	}
}

// Opening a new view forgets the views ahead, as a browser does.
func TestHistory_NewViewClearsForward(t *testing.T) {
	m := historyModel(t)
	m.Update(backKey)
	visit(m, virtualFolderPrefix+"other", "o1")
	m.Update(forwardKey)
	if m.currentF != virtualFolderPrefix+"other" || !strings.Contains(m.saveStatus, "Nothing to go forward to") {
		t.Fatalf("forward after a new view: %q, status %q", m.currentF, m.saveStatus)
	}
}

// The mouse's back and forward buttons are Shift+← and Shift+→.
func TestHistory_MouseButtons(t *testing.T) {
	m := historyModel(t)
	m.Update(tea.MouseMsg{Button: tea.MouseButtonBackward, Action: tea.MouseActionPress})
	if m.currentF != searchFolderPrefix+"q" {
		t.Fatalf("mouse back: %q", m.currentF)
	}
	m.Update(tea.MouseMsg{Button: tea.MouseButtonForward, Action: tea.MouseActionPress})
	if m.currentF != virtualFolderPrefix+"set" {
		t.Fatalf("mouse forward: %q", m.currentF)
	}
}

// Shift+← does not leave a screen with something open over it.
func TestHistory_NotFromAnOverlay(t *testing.T) {
	m := historyModel(t)
	m.openHelp()
	m.Update(backKey)
	if m.currentF != virtualFolderPrefix+"set" {
		t.Fatalf("Shift+← under help moved to %q", m.currentF)
	}
}

// An Esc burst that runs past the last view opens the quit dialog but does
// not confirm it; another key keeps the program running, and q quits.
func TestQuitDialog_BurstAndKeys(t *testing.T) {
	m := aliasModel(t)
	m.mode = modeIndex
	m.Update(escKey)
	m.Update(escKey) // same burst: inside the grace
	if m.done || !m.quitConfirmOpen {
		t.Fatalf("burst: done=%v dialog=%v", m.done, m.quitConfirmOpen)
	}
	press(m, "j")
	if m.done || m.quitConfirmOpen {
		t.Fatalf("other key: done=%v dialog=%v", m.done, m.quitConfirmOpen)
	}
	m.Update(escKey)
	if _, cmd := m.Update(key("q")); !m.done || cmd == nil {
		t.Fatal("q in the dialog did not quit")
	}
}

// A send that went through shows "Message sent" over the list until its time
// is up, without taking the keys.
func TestSentToast(t *testing.T) {
	m := aliasModel(t)
	m.mode = modeIndex
	_ = m.showSentToast()
	if !strings.Contains(m.View(), "Message sent") {
		t.Fatal("toast not drawn")
	}
	m.sentToastUntil = time.Now().Add(-time.Millisecond)
	if strings.Contains(m.View(), "Message sent") {
		t.Fatal("toast drawn after its time")
	}
}

// Back to a real folder reloads it, and the tree's folder change is a move
// Back undoes.
func TestHistory_BackToAFolder(t *testing.T) {
	m := historyModel(t)
	m.Update(escKey)
	m.Update(escKey)
	if m.currentF != "INBOX" || m.mode != modeIndex {
		t.Fatalf("back to the folder: %q mode %v", m.currentF, m.mode)
	}
	m.enterFolder("Work")
	m.Update(escKey)
	if m.currentF != "INBOX" {
		t.Fatalf("Esc after the tree's move: %q", m.currentF)
	}
}
