package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// ctrlQ is the key message a terminal sends for Ctrl+q.
var ctrlQ = tea.KeyMsg{Type: tea.KeyCtrlQ}

// Ctrl+q quits from the list, and from inside a prompt that owns every
// other key, Ctrl+c included.
func TestCtrlQ_QuitsFromAnywhere(t *testing.T) {
	for _, setup := range []struct {
		name string
		open func(*tuiModel)
	}{
		{"list", func(*tuiModel) {}},
		{"pipe prompt", func(m *tuiModel) { m.inPipePrompt = true }},
		{"help", func(m *tuiModel) { m.openHelp() }},
		{"aliases", func(m *tuiModel) { press(m, "f6") }},
	} {
		m := aliasModel(t)
		setup.open(m)
		_, cmd := m.Update(ctrlQ)
		if !m.done || cmd == nil {
			t.Errorf("%s: Ctrl+q did not quit", setup.name)
		}
	}
}

// In compose, Ctrl+q keeps the message as a draft, the field being typed
// included, the way Ctrl+c does.
func TestCtrlQ_ComposeKeepsTheDraft(t *testing.T) {
	m, mock := composeWithTrash(t)
	m.standaloneCompose = true
	m.compose.FieldEditing = fieldSubject
	m.compose.FieldInput = "Typed before Ctrl+q"

	_, cmd := m.Update(ctrlQ)
	drain(t, m, cmd)
	if len(mock.UploadedEmails) != 1 || !strings.Contains(string(mock.UploadedEmails[0].RawBytes), "Typed before Ctrl+q") {
		t.Fatal("Ctrl+q did not save the draft")
	}
}
