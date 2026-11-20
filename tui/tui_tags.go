package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/label"
	"github.com/sarielhp/feral/mailclient"
)

// The bulk done messages carry the ids the servers accepted and the joined
// error of everything they did not, so a partial failure is neither hidden
// nor counted as done.
type bulkArchiveDoneMsg struct {
	ids  []email.ID
	err  error
	undo []undoBatch
}
type bulkDeleteDoneMsg struct {
	ids  []email.ID
	err  error
	undo []undoBatch
}
type bulkMarkReadDoneMsg struct {
	ids  []email.ID
	read bool
	err  error
}
type bulkSpamDoneMsg struct {
	ids  []email.ID
	err  error
	undo []undoBatch
}

func (m *tuiModel) isTagged(id email.ID) bool {
	return m.taggedIDs != nil && m.taggedIDs[id]
}

func (m *tuiModel) countTagged() int {
	return len(m.taggedIDs)
}

func (m *tuiModel) getTaggedIDs() []email.ID {
	if len(m.taggedIDs) == 0 {
		return nil
	}
	ids := make([]email.ID, 0, len(m.taggedIDs))
	for id, tagged := range m.taggedIDs {
		if tagged {
			ids = append(ids, id)
		}
	}
	return ids
}

func (m *tuiModel) toggleTag(id email.ID) {
	if id == "" {
		return
	}
	if m.taggedIDs == nil {
		m.taggedIDs = make(map[email.ID]bool)
	}
	if m.taggedIDs[id] {
		delete(m.taggedIDs, id)
		return
	}
	m.taggedIDs[id] = true
}

func (m *tuiModel) toggleTagAndAdvance() (tea.Model, tea.Cmd) {
	id := m.currentEmailID()
	if id == "" {
		return m, nil
	}
	m.toggleTag(id)
	if m.eIdx < len(m.emails)-1 {
		m.eIdx++
		m.selectedID = m.emails[m.eIdx].ID
	}
	return m, nil
}

func (m *tuiModel) tagAllVisible() (tea.Model, tea.Cmd) {
	if m.taggedIDs == nil {
		m.taggedIDs = make(map[email.ID]bool)
	}
	for _, em := range m.emails {
		m.taggedIDs[em.ID] = true
	}
	return m, nil
}

func (m *tuiModel) clearTags() (tea.Model, tea.Cmd) {
	m.taggedIDs = nil
	m.inTagPrefix = false
	return m, nil
}

func (m *tuiModel) tagByPattern(pattern string) {
	if pattern == "" {
		return
	}
	lower := strings.ToLower(pattern)
	if m.taggedIDs == nil {
		m.taggedIDs = make(map[email.ID]bool)
	}
	for _, em := range m.emails {
		if strings.Contains(strings.ToLower(em.Subject), lower) ||
			strings.Contains(strings.ToLower(em.FromRaw), lower) ||
			strings.Contains(strings.ToLower(em.FromEmail), lower) {
			m.taggedIDs[em.ID] = true
		}
	}
}

func (m *tuiModel) kTagPattern(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if query, ok := m.kPromptLine(k, &m.tagPatternQuery, &m.inTagPatternPrompt); ok {
		m.tagByPattern(query)
	}
	return m, nil
}

func (m *tuiModel) kTagPrefix(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.inTagPrefix = false
	s := k.String()
	switch s {
	case "E":
		return m.bulkArchiveTagged()
	case "d":
		return m.bulkDeleteTagged()
	case "s":
		return m.promptBulkMoveToFolder()
	case "a":
		return m.beginAccountMove()
	case "m":
		return m.bulkMarkReadTagged(true)
	case "M":
		return m.bulkMarkReadTagged(false)
	case "S":
		return m.bulkSpamTagged()
	case "v":
		return m.beginSaveName(saveNameVirtualFolder)
	case "x":
		return m.beginSaveName(saveNameMbox)
	case "esc":
		return m, nil
	default:
		return m, nil
	}
}

// beginBulkTagged resolves the tagged ids, refuses in read-only mode, plans
// where each one lives, ghosts each message with pending and clears the
// selection. The plan is taken before the rows go: afterwards there is nothing
// left to say which account a unified-inbox message belongs to. ok is false
// when there is nothing to act on.
func (m *tuiModel) beginBulkTagged(pending pendingOpType) ([]email.ID, actionPlan, bool) {
	ids := m.getTaggedIDs()
	if len(ids) == 0 {
		return nil, actionPlan{}, false
	}
	if m.isReadOnly() {
		m.err = fmt.Errorf("🔒 Read-Only Mode: Action simulated (no server changes)")
		m.showError = true
		return nil, actionPlan{}, false
	}
	if m.refuseInSearchView() {
		return nil, actionPlan{}, false
	}
	plan := m.planAction(ids, m.currentF)
	for _, id := range ids {
		m.removeEmailByID(id)
		m.addPending(id, pending)
	}
	m.taggedIDs = nil
	return ids, plan, true
}

// bulkDispatch performs a bulk action against the servers the plan names.
type bulkDispatch func(actionPlan, []email.ID) bulkOutcome

// bulkActTagged runs dispatch on the tagged messages and reports its outcome
// as the done message of the pending operation.
func (m *tuiModel) bulkActTagged(pending pendingOpType, dispatch bulkDispatch) (tea.Model, tea.Cmd) {
	ids, plan, ok := m.beginBulkTagged(pending)
	if !ok {
		return m, nil
	}
	return m, func() tea.Msg {
		return bulkDoneMsg(pending, dispatch(plan, ids))
	}
}

// bulkDoneMsg reports a bulk move's outcome as its operation's message.
func bulkDoneMsg(pending pendingOpType, o bulkOutcome) tea.Msg {
	switch pending {
	case pendingArchive:
		return bulkArchiveDoneMsg{ids: o.done, err: o.err, undo: o.undo}
	case pendingDelete:
		return bulkDeleteDoneMsg{ids: o.done, err: o.err, undo: o.undo}
	default:
		return bulkSpamDoneMsg{ids: o.done, err: o.err, undo: o.undo}
	}
}

func (m *tuiModel) bulkArchiveTagged() (tea.Model, tea.Cmd) {
	return m.bulkActTagged(pendingArchive, m.dispatchBulkArchive)
}

func (m *tuiModel) bulkDeleteTagged() (tea.Model, tea.Cmd) {
	return m.bulkActTagged(pendingDelete, m.dispatchBulkDelete)
}

// bulkMarkReadTagged marks the tagged messages read or unread, on their
// servers and in the local read cache.
func (m *tuiModel) bulkMarkReadTagged(read bool) (tea.Model, tea.Cmd) {
	ids := m.getTaggedIDs()
	if len(ids) == 0 {
		return m, nil
	}

	// Refused as the single-message mark is: it used to simulate and report
	// "Marked N message(s)", a change that never reached the server or disk.
	if m.isReadOnly() {
		m.err = fmt.Errorf("🔒 Read-Only Mode: Action simulated (no server changes)")
		m.showError = true
		return m, nil
	}
	for _, id := range ids {
		for i := range m.emails {
			if m.emails[i].ID == id {
				m.emails[i].IsRead = read
				break
			}
		}
		m.noteRead(id, read)
	}
	plan := m.planAction(ids, m.currentF)
	return m, func() tea.Msg {
		var o bulkOutcome
		if read {
			o = m.dispatchBulkMarkRead(plan, ids, true)
		} else {
			o = m.dispatchBulkMarkUnread(plan, ids, true)
		}
		return bulkMarkReadDoneMsg{ids: o.done, read: read, err: o.err}
	}
}

func (m *tuiModel) bulkSpamTagged() (tea.Model, tea.Cmd) {
	return m.bulkActTagged(pendingSpam, m.dispatchBulkSpam)
}

func (m *tuiModel) promptBulkMoveToFolder() (tea.Model, tea.Cmd) {
	if m.countTagged() == 0 {
		return m, nil
	}
	m.bulkMovingToFolder = true
	m.treeOpen = true
	m.folderSearch = ""
	m.inFolderSearch = false
	return m, nil
}

func (m *tuiModel) executeBulkMoveToFolder(targetFolder label.Folder) (tea.Model, tea.Cmd) {
	ids := m.getTaggedIDs()
	m.bulkMovingToFolder = false
	if len(ids) == 0 || m.refuseInSearchView() || m.refuseReadOnly() {
		return m, nil
	}
	m.taggedIDs = nil

	plan := m.planAction(ids, m.currentF)
	for _, id := range ids {
		m.removeEmailByID(id)
	}

	return m, func() tea.Msg {
		o := m.dispatchBulkMove(plan, ids, func(mailclient.MailClient) (label.Folder, error) {
			return targetFolder, nil
		})
		if o.err != nil {
			return errMsg{err: fmt.Errorf("move to %s: %w", targetFolder, o.err)}
		}
		return saveEmailDoneMsg{folder: targetFolder.String(), moved: o.done, undo: o.undo}
	}
}

// handleBulkDone reports a bulk action: the count is of the ids the servers
// accepted, and any failure is shown as an error beside it.
func (m *tuiModel) handleBulkDone(msg tea.Msg) (tea.Model, tea.Cmd) {
	var err error
	switch msg := msg.(type) {
	case bulkArchiveDoneMsg:
		m.dropFromSavedSet(msg.ids)
		m.saveStatus = fmt.Sprintf("Archived %d message(s)", len(msg.ids)) + m.recordUndo(undoEntry{verb: "Archived", batches: msg.undo})
		err = msg.err
	case bulkDeleteDoneMsg:
		m.dropFromSavedSet(msg.ids)
		m.saveStatus = fmt.Sprintf("Deleted %d message(s)", len(msg.ids)) + m.recordUndo(undoEntry{verb: "Deleted", batches: msg.undo})
		err = msg.err
	case bulkMarkReadDoneMsg:
		state := "read"
		if !msg.read {
			state = "unread"
		}
		m.saveStatus = fmt.Sprintf("Marked %d message(s) as %s", len(msg.ids), state)
		err = msg.err
	case bulkSpamDoneMsg:
		m.dropFromSavedSet(msg.ids)
		m.saveStatus = fmt.Sprintf("Reported %d message(s) as spam", len(msg.ids)) + m.recordUndo(undoEntry{verb: "Marked as spam", batches: msg.undo})
		err = msg.err
	}
	m.saveStatusTime = time.Now()
	if err != nil {
		m.err = fmt.Errorf("bulk action failed: %w", err)
		m.showError = true
	}
	return m, nil
}
