package tui

import (
	"errors"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/mute"
)

// Mailing-list mode: "=" narrows the list to the selected message's list and
// "G" replies to the list (replyModeForKey); "M" mutes or unmutes the
// selected thread (package mute).

// listFilterPrefix starts a list filter: "list:<text>" keeps the rows whose
// List-Id contains text. It is an ordinary list filter -- searchQuery, typed
// after "/" or set by "=" -- so Esc, Tab and endListFilter treat it as they
// treat any other.
const listFilterPrefix = "list:"

// matchesListFilter reports whether lowerQuery, already lower-cased, is a list
// filter, and if so whether em came through a matching list. A list filter
// matches nothing but List-Id: a subject that mentions the list is not on it.
func matchesListFilter(em *email.Email, lowerQuery string) (matched, isListFilter bool) {
	rest, ok := strings.CutPrefix(lowerQuery, listFilterPrefix)
	if !ok {
		return false, false
	}
	want := strings.TrimSpace(rest)
	return em.ListID != "" && strings.Contains(em.ListID, want), true
}

// filterBySelectedList narrows the list to the selected message's list, or
// clears that filter when it is already the one applied.
func (m *tuiModel) filterBySelectedList() (tea.Model, tea.Cmd) {
	if m.eIdx < 0 || m.eIdx >= len(m.emails) {
		return m, nil
	}
	listID := m.emails[m.eIdx].ListID
	if listID == "" {
		m.saveStatus = "This message did not come through a mailing list."
		m.saveStatusTime = time.Now()
		return m, nil
	}
	query := listFilterPrefix + listID
	applied := m.searchQuery == query && !m.fuzzySearch
	m.endListFilter()
	if applied {
		return m, nil
	}
	m.setListFilter(query)
	return m, nil
}

// toggleMuteSelectedThread mutes the selected message's thread in the
// account the message belongs to, or unmutes it when it is muted. Under
// --read-only a mute is refused: it asks the watcher to archive, which
// --read-only cannot do. Under --dry-run it says what it would do.
func (m *tuiModel) toggleMuteSelectedThread() (tea.Model, tea.Cmd) {
	em := m.findEmailByID(m.currentEmailID())
	if em == nil {
		return m, nil
	}
	cfg := m.Config()
	if cfg == nil {
		cfg = m.cfg
	}
	if cfg == nil {
		return m, nil
	}
	account := em.Account
	if account == "" {
		acc, _ := m.resolveActiveAccount()
		account = acc.Name
	}
	notice, err := toggleMute(cfg.ConfigDir, account, em, muteMode{readOnly: cfg.ReadOnly, dryRun: cfg.DryRun})
	if err != nil {
		m.err = err
		m.showError = true
		return m, nil
	}
	m.saveStatus = notice
	m.saveStatusTime = time.Now()
	return m, nil
}

// muteMode is what the session allows a mute to do.
type muteMode struct{ readOnly, dryRun bool }

// errMuteReadOnly refuses a mute under --read-only.
var errMuteReadOnly = errors.New("🔒 Read-Only Mode: muting refused -- a muted thread is archived on arrival, which --read-only cannot do")

// toggleMute flips em's thread in account's muted list and says what it did.
func toggleMute(configDir, account string, em *email.Email, mode muteMode) (string, error) {
	path, err := mute.Path(configDir, account)
	if err != nil {
		return "", err
	}
	list, err := mute.Load(path)
	if err != nil {
		return "", err
	}
	thread, err := mute.ThreadOf(em, time.Now())
	if err != nil {
		return "", err
	}
	if current, muted := list.Match(em); muted {
		if mode.dryRun {
			return "dry-run: would unmute this thread", nil
		}
		list.Remove(current.Key)
		return "Thread unmuted: its new messages reach the inbox again.", list.Save(path)
	}
	if mode.dryRun {
		return "dry-run: would mute this thread", nil
	}
	if mode.readOnly {
		return "", errMuteReadOnly
	}
	list.Add(thread)
	return fmt.Sprintf("Thread muted in %s: new messages are archived on arrival (M again to unmute).", account), list.Save(path)
}
