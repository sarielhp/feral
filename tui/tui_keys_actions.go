package tui

import (
	"fmt"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/label"
	"log/slog"
	"strings"
	"time"

	"github.com/sarielhp/feral/cache"
	"github.com/sarielhp/feral/cache/folderindex"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *tuiModel) kIndexOpenDetail(_ tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.eIdx >= 0 && m.eIdx < len(m.emails) {
		em := m.emails[m.eIdx]
		if em.ThreadHasReplies && !m.expandedThreads[threadKeyOf(&em)] {
			m.expandRecursively(em.ID)
			m.rebuildVisibleEmails()
			for i, e := range m.emails {
				if e.ID == m.selectedID {
					m.eIdx = i
					break
				}
			}
			return m, nil
		}
		m.mode = modeDetail
		m.selectedID = em.ID
		m.detailSplitOpen = false
		m.rightPaneFocus = false
		var cmds []tea.Cmd
		if !em.IsRead {
			m.emails[m.eIdx].IsRead = true
			m.noteRead(em.ID, true)
			if cmd := m.markAsReadCmd(em.ID); cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		h := 20
		if m.height > 0 {
			h = max(5, m.height-3)
		}
		if m.width > 0 {
			m.vp.Width = max(20, m.width-2)
			m.vp.Height = h
		}
		m.vp.SetContent("")
		m.detailH = ""
		m.detailCrypto = nil
		m.detail = "(Loading message...)"
		cmds = append(cmds, m.loadDetailCmd(m.selectedID))
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

func (m *tuiModel) markAsReadCmd(msgID email.ID) tea.Cmd {
	if m.isReadOnly() || m.client == nil || m.client.Config() == nil {
		return nil
	}
	plan := m.planAction([]email.ID{msgID}, m.currentF)
	return func() tea.Msg {
		client, err := m.targetClient(plan, msgID)
		if err != nil {
			slog.Error("markAsReadCmd: cannot resolve account", slog.String("msgID", string(msgID)), slog.Any("error", err))
			return nil
		}
		account := client.Config().AccountCache
		_ = cache.MarkIDsRead(account, []email.ID{msgID}, nil)
		if err := client.MarkAsRead([]email.ID{msgID}); err != nil {
			// Left pending: the next folder load retries it, and no sync
			// undoes it meanwhile.
			slog.Error("markAsReadCmd: client.MarkAsRead failed", slog.String("msgID", string(msgID)), slog.Any("error", err))
			return nil
		}
		_ = cache.ConfirmReads(account, []email.ID{msgID})
		return nil
	}
}

// markAsUnreadCmd clears the message's read state on the server and in the
// local cache, so it reads as new again.
func (m *tuiModel) markAsUnreadCmd(msgID email.ID) tea.Cmd {
	if m.isReadOnly() || m.client == nil || m.client.Config() == nil {
		return nil
	}
	plan := m.planAction([]email.ID{msgID}, m.currentF)
	return func() tea.Msg {
		client, err := m.targetClient(plan, msgID)
		if err != nil {
			slog.Error("markAsUnreadCmd: cannot resolve account", slog.String("msgID", string(msgID)), slog.Any("error", err))
			return nil
		}
		account := client.Config().AccountCache
		_ = cache.MarkIDsUnread(account, []email.ID{msgID})
		if err := client.MarkAsUnread([]email.ID{msgID}); err != nil {
			slog.Error("markAsUnreadCmd: client.MarkAsUnread failed", slog.String("msgID", string(msgID)), slog.Any("error", err))
			return nil
		}
		_ = cache.ConfirmReads(account, []email.ID{msgID})
		return nil
	}
}

// toggleReadCurrentEmail flips the current message between read and unread.
func (m *tuiModel) toggleReadCurrentEmail() (tea.Model, tea.Cmd) {
	targetID := m.currentEmailID()
	if targetID == "" {
		return m, nil
	}
	if m.isReadOnly() {
		m.err = fmt.Errorf("🔒 Read-Only Mode: Action simulated (no server changes)")
		m.showError = true
		return m, nil
	}
	wasRead := m.isRead[targetID]
	for i := range m.emails {
		if m.emails[i].ID == targetID {
			wasRead = m.emails[i].IsRead
			m.emails[i].IsRead = !wasRead
			break
		}
	}
	m.noteRead(targetID, !wasRead)
	if wasRead {
		return m, m.markAsUnreadCmd(targetID)
	}
	return m, m.markAsReadCmd(targetID)
}

// markFolderRead marks every unread message in the current view read, on the
// server and in the local read cache. The messages that are already read are
// left out, so the count is what changed and no backend is asked to redo work.
func (m *tuiModel) markFolderRead() (tea.Model, tea.Cmd) {
	if m.isReadOnly() {
		m.err = fmt.Errorf("🔒 Read-Only Mode: Action simulated (no server changes)")
		m.showError = true
		return m, nil
	}
	ids := make([]email.ID, 0, len(m.emails))
	for i := range m.emails {
		if m.emails[i].IsRead {
			continue
		}
		m.emails[i].IsRead = true
		m.noteRead(m.emails[i].ID, true)
		ids = append(ids, m.emails[i].ID)
	}
	if len(ids) == 0 {
		m.saveStatus = "Every message in this view is already read."
		m.saveStatusTime = time.Now()
		return m, nil
	}
	plan := m.planAction(ids, m.currentF)
	return m, func() tea.Msg {
		o := m.dispatchBulkMarkRead(plan, ids, true)
		return bulkMarkReadDoneMsg{ids: o.done, read: true, err: o.err}
	}
}

// kIndexActions runs the list action a key is bound to (tui_keymap.go). Any
// Alt chord opens the menu: the letters are free for the actions.
func (m *tuiModel) kIndexActions(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if k.Alt || k.String() == "alt" {
		return m.openMenu()
	}
	return m.runKeyAction(groupList, k)
}

// openMenu opens the menu on its first item.
func (m *tuiModel) openMenu() (tea.Model, tea.Cmd) {
	m.menuOpen = true
	m.menuCursor = 0
	return m, nil
}

// openPipePrompt asks for a command to pipe the message to.
func (m *tuiModel) openPipePrompt() (tea.Model, tea.Cmd) {
	m.inPipePrompt = true
	m.pipeCommand = ""
	return m, nil
}

// openRemoteSearch asks for a query to run on the server.
func (m *tuiModel) openRemoteSearch() (tea.Model, tea.Cmd) {
	m.inRemoteSearch = true
	m.remoteSearchQuery = ""
	return m, nil
}

// openTagPatternPrompt asks for a pattern to tag the messages it matches.
func (m *tuiModel) openTagPatternPrompt() (tea.Model, tea.Cmd) {
	m.inTagPatternPrompt = true
	m.tagPatternQuery = ""
	return m, nil
}

// openTagPrefix waits for the key that says what to do with the tagged set.
func (m *tuiModel) openTagPrefix() (tea.Model, tea.Cmd) {
	m.inTagPrefix = true
	return m, nil
}

// openSavedSearches opens the saved result sets.
func (m *tuiModel) openSavedSearches() (tea.Model, tea.Cmd) {
	m.openVirtualBrowser()
	return m, nil
}

// composeNew starts a new message.
func (m *tuiModel) composeNew() (tea.Model, tea.Cmd) {
	return m, m.composeCmd("")
}

// startListSearch opens the list's filter, ending one already in place.
func (m *tuiModel) startListSearch() (tea.Model, tea.Cmd) {
	m.endListFilter()
	m.inSearch = true
	return m, nil
}

func (m *tuiModel) startFuzzyAll() (tea.Model, tea.Cmd) {
	m.startFuzzySearch(true)
	return m, nil
}

func (m *tuiModel) startFuzzyFolder() (tea.Model, tea.Cmd) {
	m.startFuzzySearch(false)
	return m, nil
}

// loadMoreKey fetches the next page of a server search; elsewhere it does
// nothing.
func (m *tuiModel) loadMoreKey() (tea.Model, tea.Cmd) {
	if !isSearchView(m.currentF) {
		return m, nil
	}
	return m.loadMoreSearchResults()
}

// listTarget is the message under the list's cursor, if there is one.
func (m *tuiModel) listTarget() (email.ID, bool) {
	if m.eIdx < 0 || m.eIdx >= len(m.emails) {
		return "", false
	}
	return m.emails[m.eIdx].ID, true
}

func (m *tuiModel) replyFromList() (tea.Model, tea.Cmd)     { return m.replyToListTarget(replyToSender) }
func (m *tuiModel) replyAllFromList() (tea.Model, tea.Cmd)  { return m.replyToListTarget(replyToAll) }
func (m *tuiModel) replyListFromList() (tea.Model, tea.Cmd) { return m.replyToListTarget(replyToList) }

func (m *tuiModel) replyToListTarget(mode replyMode) (tea.Model, tea.Cmd) {
	id, ok := m.listTarget()
	if !ok {
		return m, nil
	}
	return m, m.replyCmd(id, mode)
}

func (m *tuiModel) forwardFromList() (tea.Model, tea.Cmd) {
	id, ok := m.listTarget()
	if !ok {
		return m, nil
	}
	return m, m.forwardCmd(id)
}

func (m *tuiModel) openAliasesFromList() (tea.Model, tea.Cmd) {
	return m.openAliasBrowser(false, "", "")
}

// startFuzzySearch opens the in-list fuzzy filter over the folder on screen, or
// over the whole cache when global is set. It is the shared body of the "~"
// and "!" keys, which differ only in that scope. A filter already in place is
// ended first, so a second "~" does not save the whole cache as the folder.
func (m *tuiModel) startFuzzySearch(global bool) {
	m.endListFilter()
	if global {
		m.loadAllCachedEmails()
	}
	m.fuzzySearch = true
	m.fuzzyGlobal = global
	m.inSearch = true
	m.searchQuery = ""
	m.rebuildVisibleEmails()
	if len(m.emails) > 0 {
		m.eIdx = 0
		m.selectedID = m.emails[0].ID
	}
}

func (m *tuiModel) currentEmailID() email.ID {
	if m.mode == modeIndex && m.eIdx >= 0 && m.eIdx < len(m.emails) {
		return m.emails[m.eIdx].ID
	}
	return m.selectedID
}

// beginEmailAction resolves the current email and refuses to act in read-only
// mode, reporting the simulated refusal. ok is false when there is nothing to
// act on or the session forbids it.
func (m *tuiModel) beginEmailAction() (email.ID, bool) {
	targetID := m.currentEmailID()
	if targetID == "" {
		return "", false
	}
	if m.refuseReadOnly() {
		return "", false
	}
	return targetID, true
}

// refuseReadOnly says so and reports true under --read-only, for an action
// that would change the server or record a change that did not happen.
func (m *tuiModel) refuseReadOnly() bool {
	if !m.isReadOnly() {
		return false
	}
	m.err = fmt.Errorf("🔒 Read-Only Mode: Action simulated (no server changes)")
	m.showError = true
	return true
}

// stageEmailRemoval ghosts a message for an action that moves it out of the
// folder: it expands the thread, drops the row and marks it pending.
func (m *tuiModel) stageEmailRemoval(targetID email.ID, pending pendingOpType) {
	m.expandWholeThreadForEmail(targetID)
	m.removeEmailByID(targetID)
	m.addPending(targetID, pending)
	m.showList()
}

// removeCurrentEmail carries out an action that moves the current email out of
// the folder: it resolves the target, refuses in read-only mode, ghosts the row
// and returns the command that performs the move.
func (m *tuiModel) removeCurrentEmail(pending pendingOpType, cmd func(email.ID, actionPlan) tea.Cmd) (tea.Model, tea.Cmd) {
	if m.refuseInSearchView() {
		return m, nil
	}
	targetID, ok := m.beginEmailAction()
	if !ok {
		return m, nil
	}
	// The plan is taken before the row goes: afterwards nothing says which
	// account a unified-inbox message belongs to.
	plan := m.planAction([]email.ID{targetID}, m.currentF)
	m.stageEmailRemoval(targetID, pending)
	return m, cmd(targetID, plan)
}

func (m *tuiModel) archiveCurrentEmail() (tea.Model, tea.Cmd) {
	// m.currentF is captured here, on the Update goroutine, not inside the command.
	return m.removeCurrentEmail(pendingArchive, m.cmdArchive)
}

func (m *tuiModel) spamCurrentEmail() (tea.Model, tea.Cmd) {
	targetID, ok := m.beginEmailAction()
	if !ok {
		return m, nil
	}
	m.lastSpammedID = targetID
	m.lastSpammedTime = time.Now()
	m.undoSpamBanner = "Marked as spam. Undo spam marking by pressing '" + m.keyHint(groupList, actUnspam) + "' (3s)"

	plan := m.planAction([]email.ID{targetID}, m.currentF)
	m.stageEmailRemoval(targetID, pendingSpam)

	var cmds []tea.Cmd
	cmds = append(cmds, m.cmdSpam(targetID, plan))
	cmds = append(cmds, tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
		return clearSpamUndoMsg{time: t}
	}))
	return m, tea.Batch(cmds...)
}

func (m *tuiModel) unspamCurrentEmail() (tea.Model, tea.Cmd) {
	if m.undoSpamBanner != "" && time.Since(m.lastSpammedTime) < 3*time.Second && m.lastSpammedID != "" {
		targetID := m.lastSpammedID
		m.undoSpamBanner = ""
		m.lastSpammedID = ""
		return m.unspamEmailByID(targetID)
	}
	targetID := m.currentEmailID()
	return m.unspamEmailByID(targetID)
}

func (m *tuiModel) unspamEmailByID(targetID email.ID) (tea.Model, tea.Cmd) {
	if targetID == "" {
		return m, nil
	}
	if m.isReadOnly() {
		m.err = fmt.Errorf("🔒 Read-Only Mode: Action simulated (no server changes)")
		m.showError = true
		return m, nil
	}
	if !strings.EqualFold(m.currentF, "inbox") {
		m.expandWholeThreadForEmail(targetID)
		m.removeEmailByID(targetID)
	}
	m.addPending(targetID, pendingUnspam)
	// Unspam moves it to the inbox: an earlier move of it no longer knows where it is.
	m.forgetUndo([]email.ID{targetID})
	m.showList()
	m.showLoad = loadUnspamming
	m.loadEscNoted = false
	return m, m.cmdUnspam(targetID)
}

func (m *tuiModel) deleteCurrentEmail() (tea.Model, tea.Cmd) {
	return m.removeCurrentEmail(pendingDelete, m.cmdDelete)
}

// kDetail gives a key to the open message: the attachment pane or drawer
// when one has the focus, else the fixed navigation, else the message action
// the key is bound to (tui_keymap.go).
func (m *tuiModel) kDetail(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.rightPaneFocus {
		return m.kRightPane(k)
	}
	if m.attachmentDrawer {
		return m.kAttachmentDrawer(k)
	}
	s := k.String()
	a := m.keyAction(groupMessage, k)
	switch {
	case s == "tab" || a == actFocusPane:
		if m.resolveViewableAttachment() != nil && m.width >= 100 {
			m.rightPaneFocus = true
		}
	case a == actHidePane:
		if m.resolveViewableAttachment() != nil || m.hideRightPane {
			m.hideRightPane = !m.hideRightPane
			m.rightPaneFocus = false
			m.detailVpDirty = true
		}
	case m.scrollDetail(k, a):
		m.updateAttachmentFocusOnScroll()
	case k.Type == tea.KeyLeft || s == "left":
		return m.navigateDetailEmail(-1)
	case k.Type == tea.KeyRight || s == "right":
		return m.navigateDetailEmail(1)
	case k.Alt || s == "alt":
		return m.openMenu()
	default:
		return m.runKeyAction(groupMessage, k)
	}
	return m, nil
}

// scrollDetail scrolls the message for a scrolling key, a the action it is
// bound to, and reports whether it was one.
func (m *tuiModel) scrollDetail(k tea.KeyMsg, a string) bool {
	s := k.String()
	switch {
	case k.Type == tea.KeyUp || a == actScrollUp:
		m.vp.ScrollUp(1)
	case k.Type == tea.KeyDown || a == actScrollDown:
		m.vp.ScrollDown(1)
	case s == "pageup" || s == "pgup":
		m.vp.ScrollUp(m.vp.Height)
	case s == "pagedown" || s == "pgdown" || s == "pgdn":
		m.vp.ScrollDown(m.vp.Height)
	case s == "home":
		m.vp.GotoTop()
	case s == "end":
		m.vp.GotoBottom()
	default:
		return false
	}
	return true
}

func (m *tuiModel) startDetailSearch() (tea.Model, tea.Cmd) {
	m.inDetailSearch = true
	m.detailSearch = ""
	return m, nil
}

func (m *tuiModel) replyFromMessage() (tea.Model, tea.Cmd) {
	return m, m.replyCmd(m.selectedID, replyToSender)
}

func (m *tuiModel) replyAllFromMessage() (tea.Model, tea.Cmd) {
	return m, m.replyCmd(m.selectedID, replyToAll)
}

func (m *tuiModel) replyListFromMessage() (tea.Model, tea.Cmd) {
	return m, m.replyCmd(m.selectedID, replyToList)
}

func (m *tuiModel) forwardFromMessage() (tea.Model, tea.Cmd) {
	return m, m.forwardCmd(m.selectedID)
}

func (m *tuiModel) openAliasesFromMessage() (tea.Model, tea.Cmd) {
	addr, name := m.currentSender()
	return m.openAliasBrowser(false, addr, name)
}

func (m *tuiModel) navigateDetailEmail(delta int) (tea.Model, tea.Cmd) {
	if len(m.emails) == 0 {
		return m, nil
	}
	m.eIdx += delta
	if m.eIdx < 0 {
		m.eIdx = 0
	}
	if m.eIdx >= len(m.emails) {
		m.eIdx = len(m.emails) - 1
	}
	m.selectedID = m.emails[m.eIdx].ID
	var cmds []tea.Cmd
	if !m.emails[m.eIdx].IsRead {
		m.emails[m.eIdx].IsRead = true
		m.noteRead(m.selectedID, true)
		if cmd := m.markAsReadCmd(m.selectedID); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	m.vp.GotoTop()
	m.vp.SetContent("")
	m.detailH = ""
	m.detailCrypto = nil
	m.detail = "(Loading message...)"
	cmds = append(cmds, m.loadDetailCmd(m.selectedID))
	return m, tea.Batch(cmds...)
}

func (m *tuiModel) updateAttachmentFocusOnScroll() {
	if len(m.attachments) == 0 || m.attachmentLineOffset <= 0 {
		return
	}
	middleLine := m.vp.YOffset + (m.vp.Height / 2)
	attStart := m.attachmentLineOffset
	attEnd := m.attachmentLineOffset + len(m.attachments)
	if middleLine >= attStart && middleLine < attEnd {
		idx := middleLine - attStart
		m.detailSplitOpen = true
		if !m.attachmentDrawer || m.attCursor != idx {
			m.attachmentDrawer = true
			m.attCursor = idx
			m.detailVpDirty = true
		}
	} else if m.attachmentDrawer {
		m.attachmentDrawer = false
		m.detailVpDirty = true
	}
}

func (m *tuiModel) toggleFolderLock() (tea.Model, tea.Cmd) {
	if m.lockedFolders == nil {
		m.lockedFolders = make(map[string]bool)
	}
	key := strings.ToLower(m.currentF)
	m.lockedFolders[key] = !m.lockedFolders[key]
	if m.lockedFolders[key] {
		m.err = fmt.Errorf("🔒 Folder %q locked as read-only", m.currentF)
		m.showError = true
		return m, nil
	}
	m.err = fmt.Errorf("🔓 Folder %q unlocked", m.currentF)
	m.showError = true
	return m, nil
}

type saveEmailDoneMsg struct {
	id     email.ID
	folder string
	err    error
	// moved is the ids a bulk move filed elsewhere, which a saved set must stop
	// listing. A copy leaves it empty: the message stays where it was.
	moved []email.ID
	// undo is what the move took where, to move it back.
	undo []undoBatch
}

func (m *tuiModel) promptSaveMessageToFolder() (tea.Model, tea.Cmd) {
	return m.promptSaveMessageToFolderWithID(m.currentEmailID())
}

func (m *tuiModel) isMessageInFolder(targetID email.ID, folder string) bool {
	cfg := m.Config()
	if cfg == nil {
		cfg = m.cfg
	}
	if cfg == nil || cfg.AccountCache.IsZero() {
		return false
	}
	ids, err := folderindex.IDs(cfg.AccountCache, folder)
	if err != nil {
		return false
	}
	for _, id := range ids {
		if id == targetID {
			return true
		}
	}
	return false
}

func (m *tuiModel) saveEmailToFolder(targetID email.ID, srcFolder, destFolder string) (tea.Model, tea.Cmd) {
	if targetID == "" || destFolder == "" {
		return m, nil
	}
	if m.isReadOnly() {
		m.err = fmt.Errorf("🔒 Read-Only Mode: Action simulated (no server changes)")
		m.showError = true
		return m, nil
	}
	if m.client == nil {
		m.err = fmt.Errorf("mail client not configured")
		m.showError = true
		return m, nil
	}
	if m.isMessageInFolder(targetID, destFolder) {
		m.saveStatus = fmt.Sprintf("Message already in %s", destFolder)
		m.saveStatusTime = time.Now()
		return m, nil
	}

	m.saveStatus = fmt.Sprintf("Saving to %s...", destFolder)
	m.saveStatusTime = time.Now()

	client := m.client
	return m, func() tea.Msg {
		err := client.CopyEmail([]email.ID{targetID}, label.FromServer(srcFolder), label.FromServer(destFolder))
		return saveEmailDoneMsg{id: targetID, folder: destFolder, err: err}
	}
}
