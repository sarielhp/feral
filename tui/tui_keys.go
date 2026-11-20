package tui

import (
	"errors"
	"strings"
	"time"

	"github.com/sarielhp/feral/mailclient"

	tea "github.com/charmbracelet/bubbletea"
)

// quitKey leaves the program from anywhere in it.
const quitKey = "ctrl+q"

// handleKey gives a key to the screen on top: the first open layer of the
// table in screen_layers.go, which is also what view draws. A few keys act
// whatever is open beneath a given depth, as the table's groups say: Ctrl+Q
// and Ctrl+C quit, F9 lets the mouse go, l signs in after an expired sign-in,
// F1 opens help and Ctrl+D the diagnostics.
func (m *tuiModel) handleKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := k.String()
	from := 0
	if s == quitKey {
		// Ctrl+q quits from every screen and prompt, the way Ctrl+c does
		// outside them: a message being written is kept as a draft.
		s, from = "ctrl+c", firstLayerAfter(groupPrompt)
	}
	i, top := m.topLayer(from)
	switch {
	case top.group == groupPrompt:
		return top.key(m, k)
	case top.group == groupExiting:
		return m.handleKeyWhileExiting(s)
	case s == "ctrl+c":
		if m.mode == modeCompose && m.compose != nil {
			return m.ctrlCInCompose()
		}
		m.done = true
		return m, tea.Quit
	case top.group == groupQuit:
		return m.kQuitConfirm(s)
	case s == mouseToggleKey:
		return m.toggleMouse()
	case top.group == groupHelp:
		return m.kHelp(k)
	}
	if model, cmd, handled := m.handleAuthErrorLogin(s); handled {
		return model, cmd
	}
	return m.dispatchLayer(i, k)
}

// dispatchLayer gives the key to layer i or, past a busy notice that lets
// it through, to the one beneath; F1 and Ctrl+D act from the depth the
// table gives them.
func (m *tuiModel) dispatchLayer(i int, k tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := k.String()
	for {
		var top *layer
		i, top = m.topLayer(i)
		if top.id == scrLoading {
			if m.busyBlocks(s) {
				return m, nil
			}
			i++
			continue
		}
		if top.group >= groupOverlay && s == "f1" {
			m.openHelp()
			m.showDiag = false
			m.menuOpen = false
			m.spamMenuOpen = false
			m.showError = false
			return m, nil
		}
		if top.group >= groupBase && s == "ctrl+d" {
			m.showDiag = true
			m.closeHelp()
			m.closeTree()
			m.accountOverlayOpen = false
			m.diagVp.GotoBottom()
			return m, nil
		}
		return top.key(m, k)
	}
}

// firstLayerAfter is the index of the first layer below group g.
func firstLayerAfter(g layerGroup) int {
	for i, l := range screenLayers {
		if l.group > g {
			return i
		}
	}
	return len(screenLayers) - 1
}

// handleKeyWhileExiting runs while the program waits for a draft upload
// before exiting. Every key is swallowed: a second q or Esc used to quit at
// once and drop the upload. Two Ctrl+C in a row abandon the wait.
func (m *tuiModel) handleKeyWhileExiting(s string) (tea.Model, tea.Cmd) {
	if s != "ctrl+c" {
		m.abandonArmed = false
		return m, nil
	}
	if !m.abandonArmed {
		m.abandonArmed = true
		m.showLoad = "Uploading draft... (Ctrl+C again to abandon)"
		return m, nil
	}
	m.saveStatus = joinStatus(m.saveStatus, "server upload abandoned")
	return m, m.quitNow()
}

// ctrlCInCompose leaves a compose session keeping the draft: the field being
// typed is committed, overlays close, and the draft is saved and uploaded
// before the program exits. During a send it waits for the send instead: a
// draft saved then would be a copy of a message that went out.
func (m *tuiModel) ctrlCInCompose() (tea.Model, tea.Cmd) {
	if m.compose.sending {
		m.compose.quitAfterSend = true
		m.showLoad = "Sending email... (exits when sent)"
		return m, nil
	}
	m.contactPick = nil
	m.aliasView = nil
	m.commitComposeField()
	m.fileBrowserOpen = false
	m.attModalOpen = false
	m.accountOverlayOpen = false
	m.compose.DiscardConfirm = false
	m.compose.WarnEmptySend = false
	return m.saveDraftAndLeave(true)
}

func (m *tuiModel) key(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	now := time.Now()
	wasIdle := !m.lastActivityTime.IsZero() && now.Sub(m.lastActivityTime) >= idleActivityThreshold
	m.lastActivityTime = now

	model, cmd := m.handleKey(k)
	tm, ok := model.(*tuiModel)
	if !ok || !wasIdle || !tm.canCheckNewMail() {
		return model, cmd
	}
	if time.Since(tm.lastCheckTime) < minRefreshInterval {
		return model, cmd
	}
	tm.lastCheckTime = now
	tm.bgCheckingMail = true
	bgCmd := tm.bgCheckNewMailCmd(tm.currentF)
	if cmd == nil {
		return model, bgCmd
	}
	return model, tea.Batch(cmd, bgCmd)
}

func (m *tuiModel) kDiag(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := k.String()
	switch s {
	case "ctrl+d", "esc", "q":
		m.showDiag = false
	case "up", "k":
		m.diagVp.ScrollUp(1)
	case "down", "j":
		m.diagVp.ScrollDown(1)
	case "pageup", "pgup":
		m.diagVp.ScrollUp(m.diagVp.Height)
	case "pagedown", "pgdn", "pgdown":
		m.diagVp.ScrollDown(m.diagVp.Height)
	case "home":
		m.diagVp.GotoTop()
	case "end":
		m.diagVp.GotoBottom()
	}
	return m, nil
}

func (m *tuiModel) handleGlobalQuit() (tea.Model, tea.Cmd) {
	if m.showLoad != "" {
		m.dismissLoad()
		return m, nil
	}
	if m.treeOpen {
		m.closeTree()
		return m, nil
	}
	if m.mode == modeDetail {
		m.showList()
		return m, nil
	}
	if m.mode == modeIndex && m.splitPreview {
		m.splitPreview = false
		return m, nil
	}
	m.done = true
	return m, tea.Quit
}

func (m *tuiModel) handleDetailEsc() (tea.Model, tea.Cmd) {
	if m.rightPaneFocus {
		m.rightPaneFocus = false
		return m, nil
	}
	if m.detailSplitOpen {
		m.detailSplitOpen = false
		m.attachmentDrawer = false
		m.detailVpDirty = true
		return m, nil
	}
	if m.attachmentDrawer {
		m.attachmentDrawer = false
		m.detailVpDirty = true
		return m, nil
	}
	if m.detailSearch != "" {
		m.endDetailFilter()
		return m, nil
	}
	m.showList()
	return m, nil
}

func (m *tuiModel) handleIndexEsc() (tea.Model, tea.Cmd) {
	if m.searchQuery != "" || m.fuzzySearch {
		m.endListFilter()
		return m, nil
	}
	if m.splitPreview {
		m.splitPreview = false
		return m, nil
	}
	return m.escBack()
}

func (m *tuiModel) handleGlobalEsc() (tea.Model, tea.Cmd) {
	if m.attachmentListOpen {
		if len(m.attachmentDeleted) > 0 {
			m.applyAttachmentDeletions()
		}
		m.attachmentListOpen = false
		m.attachmentViewerOpen = false
		return m, nil
	}
	if m.showSummary {
		m.showSummary = false
		return m, nil
	}
	if m.showError {
		m.showError = false
		m.err = nil
		return m, nil
	}
	if m.showHelp {
		m.closeHelp()
		return m, nil
	}
	if m.blockModalOpen {
		m.blockModalOpen = false
		return m, nil
	}
	if m.accountOverlayOpen {
		m.accountOverlayOpen = false
		return m, nil
	}
	if m.showLoad != "" {
		m.dismissLoad()
		return m, nil
	}
	if m.treeOpen {
		m.closeTree()
		m.savingMessageToFolder = false
		m.bulkMovingToFolder = false
		m.savingMessageID = ""
		m.savingMessageSourceFolder = ""
		return m, nil
	}
	if m.mode == modeDetail {
		return m.handleDetailEsc()
	}
	if m.mode == modeIndex {
		return m.handleIndexEsc()
	}
	return m, nil
}

func (m *tuiModel) handleGlobalTab(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.treeOpen {
		return m.kFolder(k)
	}
	if m.mode == modeDetail {
		if len(m.attachments) > 0 && m.width >= 100 {
			m.rightPaneFocus = !m.rightPaneFocus
			return m, nil
		}
		m.showList()
		return m, nil
	}
	if m.mode == modeIndex {
		if m.eIdx >= 0 && m.eIdx < len(m.emails) {
			em := m.emails[m.eIdx]
			if em.ThreadHasReplies || em.ThreadDepth > 0 {
				m.cycleEmailThreadSubtreeExpansion(em.ID)
				return m, nil
			}
		}
		if m.searchQuery != "" && !m.inSearch {
			m.inSearch = true
			return m, nil
		}
	}
	return m, nil
}

func (m *tuiModel) kGlobalKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := k.String()
	switch s {
	case "q":
		if m.standaloneCompose {
			return m, m.quitNow()
		}
		return m.handleGlobalQuit()
	case "f3", "F3":
		m.openAccountOverlay()
		return m, nil
	case "f4", "F4":
		return m.summarizeCurrentEmail()
	case keyRecentFolders, "F8":
		// Leaving the folder would abandon a message being written.
		if m.mode == modeCompose {
			return m.kCompose(k)
		}
		return m.openRecentFolders()
	case "f2", "F2":
		return m.startSaveMessage()
	case "f", "F":
		if m.currentF != "" {
			if m.treeOpen {
				m.closeTree()
			} else {
				m.treeOpen = true
				m.showGlobalTree = true
				m.refreshUnifiedCounts()
				m.buildEntries()
			}
		}
		return m, nil
	case "h", "?":
		return m.toggleHelp()
	case "tab":
		return m.handleGlobalTab(k)
	case "esc":
		return m.handleGlobalEsc()
	case keyBack, keyForward:
		return m.historyKey(s)
	default:
		if m.showHelp {
			return m, nil
		}
		if m.treeOpen {
			return m.kFolder(k)
		}
		switch m.mode {
		case modeIndex:
			return m.kIndex(k)
		case modeDetail:
			return m.kDetail(k)
		case modeCompose:
			return m.kCompose(k)
		}
		return m, nil
	}
}

func (m *tuiModel) pageStep() int {
	if m.height > 2 {
		return m.height - 2
	}
	return 10
}

func (m *tuiModel) moveIndexSelection(delta int) {
	if len(m.emails) == 0 {
		return
	}
	m.eIdx = max(0, min(len(m.emails)-1, m.eIdx+delta))
	m.selectedID = m.emails[m.eIdx].ID
}

func (m *tuiModel) openPreviewCurrentEmail() tea.Cmd {
	if m.eIdx < 0 || m.eIdx >= len(m.emails) {
		return nil
	}
	m.selectedID = m.emails[m.eIdx].ID
	m.detailVpDirty = true
	m.vp.GotoTop()
	var cmds []tea.Cmd
	if !m.emails[m.eIdx].IsRead {
		m.emails[m.eIdx].IsRead = true
		m.noteRead(m.emails[m.eIdx].ID, true)
		if cmd := m.markAsReadCmd(m.emails[m.eIdx].ID); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	m.detailH = ""
	m.detailCrypto = nil
	m.detail = "(Loading preview...)"
	cmds = append(cmds, m.loadDetailCmd(m.selectedID))
	return tea.Batch(cmds...)
}

func (m *tuiModel) navigateIndexPreview(delta int) (tea.Model, tea.Cmd) {
	next := m.eIdx + delta
	if next < 0 || next >= len(m.emails) {
		return m, nil
	}
	m.eIdx = next
	m.adjustScrollOffset()
	cmd := m.openPreviewCurrentEmail()
	return m, cmd
}

func (m *tuiModel) toggleSplitPreview() (tea.Model, tea.Cmd) {
	if len(m.emails) == 0 {
		return m, nil
	}
	m.splitPreview = !m.splitPreview
	if !m.splitPreview {
		return m, nil
	}
	cmd := m.openPreviewCurrentEmail()
	return m, cmd
}

func (m *tuiModel) kIndex(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.folderJustChanged = false
	s := k.String()
	a := m.keyAction(groupList, k)
	switch {
	case m.splitPreview && (k.Type == tea.KeyUp || s == "up"):
		m.vp.ScrollUp(1)
	case m.splitPreview && (k.Type == tea.KeyDown || s == "down"):
		m.vp.ScrollDown(1)
	case m.splitPreview && (s == "pageup" || s == "pgup"):
		m.vp.ScrollUp(m.vp.Height)
	case m.splitPreview && (s == "pagedown" || s == "pgdown" || s == "pgdn"):
		m.vp.ScrollDown(m.vp.Height)
	case m.splitPreview && (k.Type == tea.KeyLeft || s == "left" || a == actUp):
		return m.navigateIndexPreview(-1)
	case m.splitPreview && (k.Type == tea.KeyRight || s == "right" || a == actDown):
		return m.navigateIndexPreview(1)
	case !m.splitPreview && (k.Type == tea.KeyUp || a == actUp || s == "up"):
		m.moveIndexSelection(-1)
	case !m.splitPreview && (k.Type == tea.KeyDown || a == actDown || s == "down"):
		m.moveIndexSelection(1)
	case !m.splitPreview && (s == "pageup" || s == "pgup"):
		m.moveIndexSelection(-m.pageStep())
	case !m.splitPreview && (s == "pagedown" || s == "pgdown" || s == "pgdn"):
		m.moveIndexSelection(m.pageStep())
	case s == "home":
		m.eIdx = 0
		if len(m.emails) > 0 {
			m.selectedID = m.emails[0].ID
		}
		if m.splitPreview {
			m.vp.GotoTop()
		}
	case s == "end":
		if len(m.emails) > 0 {
			m.eIdx = len(m.emails) - 1
			m.selectedID = m.emails[m.eIdx].ID
		}
		if m.splitPreview {
			m.vp.GotoBottom()
		}
	case k.Type == tea.KeyEnter || a == actOpen:
		return m.kIndexOpenDetail(k)
	case a == actSplit:
		return m.toggleSplitPreview()
	case a == actSort:
		return m.openSortMenu(false)
	case a == actSortReverse:
		return m.openSortMenu(true)
	default:
		return m.kIndexActions(k)
	}
	return m, nil
}

// isAuthError reports whether err means the account must sign in again,
// which 'L' on the error banner does without leaving the TUI.
func isAuthError(err error) bool {
	return errors.Is(err, mailclient.ErrInteractiveAuthRequired) ||
		(err != nil && strings.Contains(err.Error(), "needs re-authentication"))
}

func (m *tuiModel) handleAuthErrorLogin(s string) (tea.Model, tea.Cmd, bool) {
	if !m.showError || (s != "l" && s != "L") {
		return m, nil, false
	}
	if !isAuthError(m.err) {
		return m, nil, false
	}
	m.showError = false
	m.err = nil
	accounts := m.getAccounts()
	idx := m.currentAccountIndex()
	if idx >= 0 && idx < len(accounts) {
		return m, m.loginAccountCmd(accounts[idx]), true
	}
	return m, nil, true
}
