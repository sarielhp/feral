package tui

import (
	"errors"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/snooze"
	"github.com/sarielhp/feral/when"
)

// H (hide until) snoozes the message under the cursor, or the open one: a prompt asks
// until when, read as `msg snooze --until` reads it, and the message moves to
// its account's Snoozed folder under a busy notice, through the same
// snooze.Snooze the command line uses. The watcher this TUI runs brings it
// back to the inbox, unread, when the time comes.

const loadSnoozing = "Snoozing..."

// snoozeTarget is what H captured when the prompt opened: the message, the
// plan that says which account and folder it is in, and what the record
// shows of it. The list can change under an open prompt, so none of this is
// read again at Enter.
type snoozeTarget struct {
	id      email.ID
	plan    actionPlan
	subject string
	from    string
}

// snoozeDoneMsg reports a snooze from the prompt. stayed is true when the
// message was already in the Snoozed folder, so only its time changed and its
// row stays where it is.
type snoozeDoneMsg struct {
	id     email.ID
	until  time.Time
	stayed bool
	err    error
}

// beginSnooze opens the prompt for the current message. --dry-run is checked
// first: it implies --read-only, whose refusal would not say what a dry run
// would have done.
func (m *tuiModel) beginSnooze() (tea.Model, tea.Cmd) {
	if m.refuseInSearchView() {
		return m, nil
	}
	if m.isDryRun() && m.currentEmailID() != "" {
		m.err = errors.New("--dry-run: would ask until when and move this message to Snoozed; nothing moved")
		m.showError = true
		return m, nil
	}
	id, ok := m.beginEmailAction()
	if !ok {
		return m, nil
	}
	target := &snoozeTarget{id: id, plan: m.planAction([]email.ID{id}, m.currentF)}
	if em := m.findEmailByID(id); em != nil {
		target.subject, target.from = em.Subject, email.FormatSenderVerbose(em.FromRaw)
	}
	m.snoozeTarget = target
	m.snoozeQuery = ""
	m.inSnoozePrompt = true
	return m, nil
}

func (m *tuiModel) kSnoozePrompt(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	text, ok := m.kPromptLine(k, &m.snoozeQuery, &m.inSnoozePrompt)
	if !m.inSnoozePrompt && !ok {
		m.snoozeTarget = nil
	}
	if !ok {
		return m, nil
	}
	return m.executeSnooze(text)
}

// executeSnooze reads the time first, so a mistyped one leaves the message
// where it is, then moves it under a busy notice.
func (m *tuiModel) executeSnooze(text string) (tea.Model, tea.Cmd) {
	target := m.snoozeTarget
	m.snoozeTarget = nil
	if target == nil {
		return m, nil
	}
	now := time.Now()
	until, err := when.Future(text, now)
	if err != nil {
		m.err = fmt.Errorf("snooze: %w", err)
		m.showError = true
		return m, nil
	}
	root := m.snoozeRoot()
	m.showList()
	m.showLoad = loadSnoozing
	m.loadEscNoted = false
	return m, func() tea.Msg { return m.runSnooze(target, root, until, now) }
}

// snoozeRoot is the download directory snooze.json lives in: the root
// config's, as the drafts directory is, so the record is the one `msg snooze`
// and the watcher read whichever account the message is in.
func (m *tuiModel) snoozeRoot() string {
	if m.cfg != nil && m.cfg.CacheRoot != "" {
		return m.cfg.CacheRoot
	}
	if c := m.Config(); c != nil {
		return c.CacheRoot
	}
	return ""
}

// runSnooze snoozes the target on its own account. Off the Update goroutine.
func (m *tuiModel) runSnooze(t *snoozeTarget, root string, until, now time.Time) snoozeDoneMsg {
	done := snoozeDoneMsg{id: t.id, until: until}
	client, folder, err := m.target(t.plan, t.id)
	if err != nil {
		done.err = err
		return done
	}
	account := ""
	if client.Config() != nil {
		account = string(client.Config().SelectedAccount)
	}
	entries, err := snooze.Snooze(client, snooze.Request{Root: root, Account: account, Until: until, Now: now},
		[]snooze.Item{{ID: t.id, Folder: folder, Subject: t.subject, From: t.from}})
	done.err = err
	// Already in Snoozed: snooze.Snooze changed the time and moved nothing.
	done.stayed = err == nil && len(entries) == 1 && strings.EqualFold(folder, entries[0].Folder)
	return done
}

func (m *tuiModel) handleSnoozeDone(msg snoozeDoneMsg) (tea.Model, tea.Cmd) {
	m.showLoad = ""
	m.loadEscNoted = false
	if msg.err != nil {
		m.reportActionOutcome(msg.err, "snooze", "")
		return m, nil
	}
	if !msg.stayed {
		m.removeEmailFromList(msg.id)
	}
	m.reportActionOutcome(nil, "", "Snoozed until "+msg.until.Local().Format("Mon Jan 2 15:04"))
	return m, nil
}
