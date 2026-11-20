package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// screen names one state of the TUI as a person meets it: what is in front,
// and what the next key goes to. The model holds that state as some forty
// independent flags -- menuOpen, treeOpen, inSearch, inDetailSearch and the
// rest -- and the bugs they breed are combinations nobody meant: a message
// search left set behind the message list, so every message opened after it
// showed the prompt instead of its body; an Esc spent on a state the screen no
// longer showed.
//
// The screens are one ordered table, screen_layers.go: the top open layer is
// what view draws and what handleKey gives the next key to. keyScreen and
// drawnScreen name it; stateProblems checks them and the invariants single
// flags must keep, for the tests that drive the model with random keys from
// every screen (tui_fsm_test.go).
type screen string

const (
	scrNone           screen = "none" // the program has finished
	scrList           screen = "list"
	scrMessage        screen = "message"
	scrListSearch     screen = "list-search"
	scrMessageSearch  screen = "message-search"
	scrRightPane      screen = "right-pane"
	scrFolderTree     screen = "folder-tree"
	scrFolderSearch   screen = "folder-search"
	scrPipePrompt     screen = "pipe-prompt"
	scrTagPattern     screen = "tag-pattern"
	scrSaveName       screen = "save-name"
	scrMovePrompt     screen = "move-prompt"
	scrSnoozePrompt   screen = "snooze-prompt"
	scrTagPrefix      screen = "tag-prefix"
	scrRemoteSearch   screen = "remote-search"
	scrExiting        screen = "exiting"
	scrQuitConfirm    screen = "quit-confirm"
	scrHelp           screen = "help"
	scrHelpSearch     screen = "help-search"
	scrCompose        screen = "compose"
	scrContacts       screen = "contacts"
	scrSaveToFolder   screen = "save-to-folder"
	scrSaveToDisk     screen = "save-to-disk"
	scrSaveHelp       screen = "save-help"
	scrFileBrowser    screen = "file-browser"
	scrAttachmentList screen = "attachment-list"
	scrICS            screen = "ics"
	scrAttachment     screen = "attachment"
	scrAttachmentHelp screen = "attachment-help"
	scrAttachmentBar  screen = "attachment-drawer"
	scrMenu           screen = "menu"
	scrSpamMenu       screen = "spam-menu"
	scrDiag           screen = "diag"
	scrSummary        screen = "summary"
	scrDeadlines      screen = "deadlines"
	scrBlock          screen = "block"
	scrUnsubscribe    screen = "unsubscribe"
	scrAliases        screen = "aliases"
	scrAccounts       screen = "accounts"
	scrSort           screen = "sort"
	scrSavedSearches  screen = "saved-searches"
	scrRecent         screen = "recent"
	scrLoading        screen = "loading"
)

// keyScreen is the screen the next key goes to: the top open layer.
func (m *tuiModel) keyScreen() screen {
	if m.done {
		return scrNone
	}
	_, top := m.topLayer(0)
	return top.id
}

// drawnScreen is the screen view draws: the top open layer that draws itself.
func (m *tuiModel) drawnScreen() screen {
	if m.done {
		return scrNone
	}
	return m.drawnLayer().id
}

// stateProblems lists what is wrong with the model's state: the screen drawn
// is not the one taking keys, or a flag outlived the state it belongs to.
// The two screens are one table's top layer, so they disagree only where the
// table lets them: the wait on the way out draws what is beneath it.
func (m *tuiModel) stateProblems() []string {
	var out []string
	if key, drawn := m.keyScreen(), m.drawnScreen(); key != drawn && key != scrExiting {
		out = append(out, fmt.Sprintf("keys go to %s while %s is drawn", key, drawn))
	}
	return append(out, m.flagProblems()...)
}

// flagProblems checks flags that belong to one state against the state.
func (m *tuiModel) flagProblems() []string {
	var out []string
	add := func(bad bool, what string) {
		if bad {
			out = append(out, what)
		}
	}
	add((m.inDetailSearch || m.detailSearch != "") && m.mode != modeDetail,
		fmt.Sprintf("a message search is set outside a message (typing %v, query set %v, mode %v)", m.inDetailSearch, m.detailSearch != "", m.mode))
	add(m.rightPaneFocus && m.mode != modeDetail, "the right pane has focus outside a message")
	add(m.inFolderSearch && !m.treeOpen, "a folder search is open without the folder tree")
	add(m.helpFind.active() && !m.showHelp && !(m.saveHelpOpen && m.saving()) && !m.attachmentHelpOpen,
		"a help search is set with no help open")
	add(m.fuzzyGlobal && !m.fuzzySearch, "a whole-cache search is marked without a fuzzy search")
	add(m.snoozeTarget != nil && !m.inSnoozePrompt, "a snooze target is kept with no snooze prompt open")
	add(m.savedRawEmails != nil && !m.fuzzyGlobal, "a folder's rows are kept aside with no whole-cache search")
	add(len(m.emails) > 0 && (m.eIdx < 0 || m.eIdx >= len(m.emails)),
		fmt.Sprintf("the selection %d is outside the list of %d", m.eIdx, len(m.emails)))
	add(len(m.undoStack) > undoStackDepth, fmt.Sprintf("the undo stack holds %d actions", len(m.undoStack)))
	return out
}

// runProgram hands the terminal to an external program -- the editor, a pipe
// command, a sign-in -- and reports how it ended. It is tea.ExecProcess; the
// state-machine tests replace it to record the program instead of running it,
// so a random walk can pass through compose without starting an editor.
var runProgram = tea.ExecProcess

// closeTree closes the folder tree and the search typed into it. The search
// belongs to the tree: closed some ways -- Ctrl+D, a folder chosen elsewhere --
// it used to stay set, and the next "f" opened the tree already filtering by
// the old query.
func (m *tuiModel) closeTree() {
	m.treeOpen = false
	m.inFolderSearch = false
	if m.folderSearch != "" {
		m.folderSearch = ""
		m.buildEntries()
	}
}

// showList puts the message list on screen. A message-view search belongs to
// the message it was typed in, so it ends here: seventeen places switched to
// the list by assigning the mode, and the ones that forgot the search carried
// it -- prompt and all -- into the next message opened.
func (m *tuiModel) showList() {
	m.endDetailFilter()
	m.mode = modeIndex
}

// busyOnTop reports whether a busy notice -- "Downloading...", "Unspamming...",
// "Summarizing..." -- is what the screen shows. Compose keeps its own busy
// states (sending, uploading the draft) and is not covered here.
func (m *tuiModel) busyOnTop() bool {
	return m.mode != modeCompose && m.drawnScreen() == scrLoading
}

// busyBlocks reports whether key s is to be dropped because a busy notice
// covers the screen. A screen that hides the list must not let keys act on
// it: they used to pass through, so a "d" pressed under "Unspamming..."
// deleted whichever message the hidden cursor was on.
//
// Esc, q and Ctrl+C always work. While content is on its way -- a folder
// downloading, a server search, a saved set opening -- there is nothing
// underneath to act on, so the keys that go somewhere else stay live too: f
// for another folder, and help.
func (m *tuiModel) busyBlocks(s string) bool {
	if !m.busyOnTop() {
		return false
	}
	switch s {
	case "esc", "q", "ctrl+c", mouseToggleKey:
		return false // q, like Esc, dismisses -- or says a change cannot be recalled
	case "f", "F", "f1", "h", "?":
		return !m.busyFetching()
	}
	return true
}

// busyFetching reports whether the busy notice is content on its way, as
// opposed to a change in flight or a summary being written.
func (m *tuiModel) busyFetching() bool {
	return m.showLoad == "Downloading..." || m.showLoad == "Searching server..." ||
		strings.HasPrefix(m.showLoad, "Opening ")
}
