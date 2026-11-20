package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/snooze"
)

// typeLine types text into whatever prompt is open, spaces included.
func typeLine(m *tuiModel, text string) {
	for _, r := range text {
		if r == ' ' {
			m.Update(tea.KeyMsg{Type: tea.KeySpace})
			continue
		}
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

func snoozeRecord(t *testing.T, root string) []snooze.Entry {
	t.Helper()
	f, err := snooze.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	return f.Entries
}

// H (hide until) asks until when, and Enter moves the message to Snoozed and records it
// where `msg snooze` and the watcher read. A time it cannot read leaves the
// message where it is; Esc leaves nothing behind.
func TestSnoozeKeyMovesAndRecords(t *testing.T) {
	m, mock, root := setupTestComposeModel(t, false)
	const id = email.ID("snz-1")
	m.emails = []email.Email{{ID: id, Subject: "Later", FromRaw: "Alice <alice@example.com>"}}
	m.eIdx, m.selectedID, m.mode = 0, id, modeIndex

	press(m, "H")
	if !m.inSnoozePrompt || m.keyScreen() != scrSnoozePrompt {
		t.Fatalf("H did not open the snooze prompt (screen %s)", m.keyScreen())
	}
	press(m, "esc")
	if m.inSnoozePrompt || m.snoozeTarget != nil {
		t.Fatal("Esc left the snooze prompt or its target behind")
	}

	press(m, "H")
	typeLine(m, "next blursday")
	if cmd := press(m, "enter"); cmd != nil {
		t.Fatal("an unreadable time started a snooze")
	}
	if !m.showError || len(m.emails) != 1 || len(mock.MovedEmails) != 0 || len(snoozeRecord(t, root)) != 0 {
		t.Fatalf("a bad time: error shown %v, rows %d, moves %v", m.showError, len(m.emails), mock.MovedEmails)
	}
	m.showError, m.err = false, nil

	press(m, "H")
	typeLine(m, "+2h")
	cmd := press(m, "enter")
	if cmd == nil || m.showLoad != loadSnoozing {
		t.Fatalf("Enter did not start the snooze (load %q)", m.showLoad)
	}
	done, ok := cmd().(snoozeDoneMsg)
	if !ok || done.err != nil {
		t.Fatalf("snooze command returned %#v", done)
	}
	m.Update(done)

	if len(m.emails) != 0 || m.showLoad != "" {
		t.Errorf("the snoozed row is still listed (%d rows, load %q)", len(m.emails), m.showLoad)
	}
	if fmt.Sprint(mock.MovedEmails) != "[{[snz-1] INBOX Snoozed}]" {
		t.Errorf("moves = %+v; want snz-1 from INBOX to Snoozed", mock.MovedEmails)
	}
	got := snoozeRecord(t, root)
	if len(got) != 1 || got[0].ID != id || got[0].Account != "Personal" || got[0].FromFolder != "INBOX" ||
		got[0].Subject != "Later" || !got[0].Until.After(got[0].SnoozedAt) {
		t.Errorf("record = %+v", got)
	}
}

// Under --read-only H refuses before asking: the record would describe a move
// the decorator only simulated.
func TestSnoozeKeyRefusedReadOnly(t *testing.T) {
	m, mock, root := setupTestComposeModel(t, true)
	m.emails = []email.Email{{ID: "snz-ro", Subject: "Later"}}
	m.eIdx, m.selectedID, m.mode = 0, "snz-ro", modeDetail

	if cmd := press(m, "H"); cmd != nil || m.inSnoozePrompt {
		t.Fatal("H opened the snooze prompt under --read-only")
	}
	if !m.showError || len(mock.MovedEmails) != 0 || len(snoozeRecord(t, root)) != 0 {
		t.Errorf("read-only H: error shown %v, moves %v", m.showError, mock.MovedEmails)
	}
}

// H on a message already in Snoozed changes its time and moves nothing, so
// its row stays.
func TestSnoozeKeyInSnoozedKeepsRow(t *testing.T) {
	m, mock, root := setupTestComposeModel(t, false)
	const id = email.ID("snz-again")
	m.currentF = snooze.FolderName
	m.emails = []email.Email{{ID: id, Subject: "Again"}}
	m.eIdx, m.selectedID, m.mode = 0, id, modeIndex

	press(m, "H")
	typeLine(m, "+3h")
	cmd := press(m, "enter")
	if cmd == nil {
		t.Fatal("Enter did not start the snooze")
	}
	m.Update(cmd())
	if len(m.emails) != 1 || len(mock.MovedEmails) != 0 || m.showError {
		t.Errorf("rows %d, moves %+v, error %v; want the row kept and nothing moved", len(m.emails), mock.MovedEmails, m.err)
	}
	if got := snoozeRecord(t, root); len(got) != 1 || got[0].ID != id {
		t.Errorf("record = %+v", got)
	}
}

// Under --dry-run H says what it would do instead of asking.
func TestSnoozeKeyDryRunReports(t *testing.T) {
	m, mock, root := setupTestComposeModel(t, false)
	m.cfg.DryRun, m.cfg.ReadOnly = true, true
	m.emails = []email.Email{{ID: "snz-dry", Subject: "Later"}}
	m.eIdx, m.selectedID, m.mode = 0, "snz-dry", modeIndex

	press(m, "H")
	if m.inSnoozePrompt || !m.showError || m.err == nil || !strings.Contains(m.err.Error(), "dry-run") {
		t.Fatalf("prompt %v, error %v; want a dry-run report", m.inSnoozePrompt, m.err)
	}
	if len(mock.MovedEmails) != 0 || len(snoozeRecord(t, root)) != 0 {
		t.Errorf("dry-run moved %+v", mock.MovedEmails)
	}
}
