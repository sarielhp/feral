package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/sarielhp/feral/email"
)

// The back and forward stacks, as in a browser: Esc on the message list,
// Shift+← and the mouse's back button return to the view left before; Shift+→
// and the forward button undo that. Opening a new view forgets what was ahead.
// With nothing to go back to, Esc asks before quitting instead of quitting.

const (
	keyBack    = "shift+left"
	keyForward = "shift+right"
)

// historyLimit is how many views back (or forward) the stacks reach.
const historyLimit = 20

// quitConfirmGrace is how long the quit dialog ignores Esc after it opens:
// Esc pressed in a burst to back out of screens, or held down and repeating,
// must not confirm a quit the user never saw. q is never ignored.
const quitConfirmGrace = 500 * time.Millisecond

// sentToastFor is how long "Message sent" stays over the list. It does not
// take keys, so it never swallows the keystroke after a send.
const sentToastFor = 2 * time.Second

// historyEntry is a view left for another: the folder and the message it had
// selected. A search's results or a saved set cannot be reloaded by name
// alone, so their messages are kept with them; a folder reloads from cache.
type historyEntry struct {
	folder   string
	selected email.ID
	raw      []email.Email
}

// currentEntry is the view on screen, as the stacks keep it.
func (m *tuiModel) currentEntry() historyEntry {
	entry := historyEntry{folder: m.currentF, selected: m.selectedID}
	if isSearchView(m.currentF) || isVirtualView(m.currentF) {
		entry.raw = m.rawEmails
		if m.fuzzyGlobal && m.savedRawEmails != nil {
			// "~" has the whole cache on screen; the view's own rows are kept aside.
			entry.raw = m.savedRawEmails
		}
	}
	return entry
}

// pushHistory records the view on screen before a new one replaces it, and
// forgets the views ahead. Leaving for the same view -- a refresh, or loading
// more results -- is no move.
func (m *tuiModel) pushHistory(next string) {
	if m.currentF == "" || m.currentF == next {
		return
	}
	m.history = pushCapped(m.history, m.currentEntry())
	m.forward = nil
}

// pushCapped appends entry, dropping the oldest past historyLimit.
func pushCapped(stack []historyEntry, entry historyEntry) []historyEntry {
	stack = append(stack, entry)
	if len(stack) > historyLimit {
		stack = stack[len(stack)-historyLimit:]
	}
	return stack
}

// clearHistory forgets both stacks: their views belong to the account left.
func (m *tuiModel) clearHistory() {
	m.history, m.forward = nil, nil
}

// goBack reopens the view left last, keeping the one on screen for Forward.
func (m *tuiModel) goBack() (tea.Model, tea.Cmd) {
	return m.moveInHistory(&m.history, &m.forward, "Nothing to go back to", "Back to ")
}

// goForward reopens the view Back left, keeping the one on screen for Back.
func (m *tuiModel) goForward() (tea.Model, tea.Cmd) {
	return m.moveInHistory(&m.forward, &m.history, "Nothing to go forward to", "Forward to ")
}

// moveInHistory pops the view to reopen from one stack and pushes the view on
// screen onto the other.
func (m *tuiModel) moveInHistory(from, to *[]historyEntry, empty, verb string) (tea.Model, tea.Cmd) {
	if len(*from) == 0 {
		return m.noMove(empty)
	}
	entry := (*from)[len(*from)-1]
	*from = (*from)[:len(*from)-1]
	*to = pushCapped(*to, m.currentEntry())
	return m.restoreEntry(entry, verb)
}

// noMove says that a stack is empty.
func (m *tuiModel) noMove(status string) (tea.Model, tea.Cmd) {
	m.saveStatus = status
	m.saveStatusTime = time.Now()
	return m, nil
}

// restoreEntry puts a kept view back on screen as the message list, on the
// message it had selected.
func (m *tuiModel) restoreEntry(entry historyEntry, verb string) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	if entry.raw == nil {
		_, cmd = m.reopenFolder(entry.folder)
	} else {
		m.endListFilter()
		m.endDetailFilter()
		m.rememberPosition()
		m.rawEmails = entry.raw
		m.currentF = entry.folder
		m.showList()
		m.rebuildVisibleEmails()
		m.selectSearchRow(entry.selected)
	}
	m.saveStatus = verb + viewTitle(entry.folder)
	m.saveStatusTime = time.Now()
	return m, cmd
}

// escBack is Esc on the bare list: back a view, or, with none left, the
// dialog that asks before quitting.
func (m *tuiModel) escBack() (tea.Model, tea.Cmd) {
	if len(m.history) > 0 {
		return m.goBack()
	}
	m.quitConfirmOpen = true
	m.quitConfirmAt = time.Now()
	return m, nil
}

// kQuitConfirm owns the keys while the quit dialog is open: q quits, Esc
// quits once the dialog has been up past its grace, any other key keeps
// the program running.
func (m *tuiModel) kQuitConfirm(s string) (tea.Model, tea.Cmd) {
	switch {
	case s == "q" || s == "Q":
	case s == "esc" && time.Since(m.quitConfirmAt) >= quitConfirmGrace:
	case s == "esc":
		return m, nil // part of the burst that opened the dialog
	default:
		m.quitConfirmOpen = false
		return m, nil
	}
	m.quitConfirmOpen = false
	m.done = true
	return m, tea.Quit
}

// quitConfirmBox is the quit dialog, drawn over the dimmed screen.
func quitConfirmBox() string {
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#4a9e6e")).Padding(1, 3).Bold(true).
		Foreground(lipgloss.Color("#ffdd44")).Background(lipgloss.Color("#2b2b2b")).
		Render(strings.Join([]string{
			" Quit feral? ",
			"",
			"q or Esc: quit     any other key: stay",
		}, "\n"))
}

// sentToastExpiredMsg redraws the screen when the toast's time is up.
type sentToastExpiredMsg struct{}

// showSentToast puts "Message sent" over the screen for sentToastFor.
func (m *tuiModel) showSentToast() tea.Cmd {
	m.sentToastUntil = time.Now().Add(sentToastFor)
	return tea.Tick(sentToastFor, func(time.Time) tea.Msg { return sentToastExpiredMsg{} })
}

// sentToastShowing reports whether the toast is still due on screen.
func (m *tuiModel) sentToastShowing() bool {
	return time.Now().Before(m.sentToastUntil)
}

// withSentToast draws the toast over the top-right of a rendered screen,
// leaving the rest of it visible.
func (m *tuiModel) withSentToast(screen string) string {
	if !m.sentToastShowing() {
		return screen
	}
	toast := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#4a9e6e")).Foreground(lipgloss.Color("#4a9e6e")).
		Bold(true).Padding(0, 2).Render("✓ Message sent")
	lines := strings.Split(screen, "\n")
	w := lipgloss.Width(toast)
	left := max(0, m.width-w-2)
	for i, l := range strings.Split(toast, "\n") {
		if row := 1 + i; row < len(lines) {
			lines[row] = spliceDialogRow(lines[row], l, left, w)
		}
	}
	return strings.Join(lines, "\n")
}

// viewTitle is how a status line names a view: a search as such, a saved
// set by its name, a folder by itself.
func viewTitle(view string) string {
	switch {
	case isSearchView(view):
		return "search results"
	case isVirtualView(view):
		return strings.TrimPrefix(view, virtualFolderPrefix)
	}
	return view
}

// historyKey is Shift+← or Shift+→ on the list or the reader. Compose and
// the overlays keep their own keys; there a move would abandon what is open.
func (m *tuiModel) historyKey(s string) (tea.Model, tea.Cmd) {
	if !m.canMoveInHistory() {
		return m, nil
	}
	if s == keyForward {
		return m.goForward()
	}
	return m.goBack()
}

// historyMouse is the mouse's back or forward button, as the keys.
func (m *tuiModel) historyMouse(b tea.MouseButton) (tea.Model, tea.Cmd) {
	if b == tea.MouseButtonForward {
		return m.historyKey(keyForward)
	}
	return m.historyKey(keyBack)
}

// canMoveInHistory reports whether the screen is one Back may leave: the
// list or the reader, with nothing open over it.
func (m *tuiModel) canMoveInHistory() bool {
	if m.mode != modeIndex && m.mode != modeDetail {
		return false
	}
	return !m.showHelp && !m.treeOpen && !m.inSearch && m.aliasView == nil &&
		!m.quitConfirmOpen && !m.recentOpen && !m.menuOpen && m.showLoad == ""
}
