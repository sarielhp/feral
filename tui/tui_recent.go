package tui

import (
	"fmt"
	"log/slog"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/sarielhp/feral/cache"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/mailclient"
	"github.com/sarielhp/feral/tui/render"
)

// keyRecentFolders opens the recent-folders picker in the message list, the
// reader and the folder tree.
const keyRecentFolders = "f8"

// accountKey names the client's account by its cache directory. A client is
// often a wrapper built afresh, so its pointer says nothing about which
// account it is.
func accountKey(c mailclient.MailClient) string {
	if c == nil || c.Config() == nil {
		return ""
	}
	return c.Config().AccountCache.Path()
}

func (m *tuiModel) positionKey(folder string) string {
	return accountKey(m.client) + "\x00" + folder
}

// rememberPosition records the message selected in the folder on screen, so
// that coming back to the folder this session selects it again.
func (m *tuiModel) rememberPosition() {
	if m.currentF == "" || m.selectedID == "" {
		return
	}
	if m.folderPositions == nil {
		m.folderPositions = make(map[string]email.ID)
	}
	m.folderPositions[m.positionKey(m.currentF)] = m.selectedID
}

// openingIndex is the row the folder on screen opens on: the message it had
// selected when it was last left this session, if that message is still in
// it, and otherwise the newest, which is the last row.
func (m *tuiModel) openingIndex() int {
	if id, ok := m.folderPositions[m.positionKey(m.currentF)]; ok {
		for i, e := range m.emails {
			if e.ID == id {
				return i
			}
		}
	}
	return len(m.emails) - 1
}

// recordsVisit reports whether folder belongs in the recent list: a folder,
// not a search's results or a saved set, which Ctrl+f and F7 reopen.
func recordsVisit(folder string) bool {
	return folder != "" && !isSearchView(folder) && !isVirtualView(folder)
}

// recordVisit adds the folders left and entered to the account's recent list,
// the entered one newest. The folder left is recorded too, so the folder the
// session started in is listed once it is left. A failure only costs the
// list an entry, so it is logged rather than put in front of the user.
func (m *tuiModel) recordVisit(from, to string) {
	folders := slices.DeleteFunc([]string{from, to}, func(f string) bool { return !recordsVisit(f) })
	if len(folders) == 0 || m.client == nil || m.client.Config() == nil {
		return
	}
	if _, err := cache.RecordRecentFolders(m.client.Config().AccountCache, folders...); err != nil {
		slog.Warn("cannot record recent folders", slog.Any("error", err))
	}
}

// enterFolder opens folder as the message list. It is the one way the folder
// tree and the recent-folders picker change folder, and Esc comes back from
// it to the view on screen now.
func (m *tuiModel) enterFolder(folder string) (tea.Model, tea.Cmd) {
	m.endListFilter()
	m.pushHistory(folder)
	return m.reopenFolder(folder)
}

// reopenFolder opens folder as the message list without recording the move
// to it: the position of the folder left is kept, the visit is recorded in
// the recent list, and the selection starts over.
func (m *tuiModel) reopenFolder(folder string) (tea.Model, tea.Cmd) {
	// A filter belongs to the view it was typed in; carried over, it hid
	// messages of the new folder and "~" later restored the old folder.
	m.endListFilter()
	m.endDetailFilter()
	m.rememberPosition()
	m.recordVisit(m.currentF, folder)
	m.currentF = folder
	// The selection belongs to the folder being left. Kept, the cache load
	// took the switch for a reload of the same folder, found no such message,
	// and kept the old row number -- an unrelated message in the new folder.
	m.selectedID = ""
	m.closeTree()
	m.inFolderSearch = false
	m.folderSearch = ""
	m.recentOpen = false
	m.showList()
	m.loadCachedEmailsForFolder(folder)
	m.showLoad = ""
	if len(m.emails) == 0 {
		m.showLoad = "Downloading..."
	}
	return m, m.loadEmailsCmd(folder)
}

// openRecentFolders shows the account's recently opened folders, newest
// first. The folder on screen is left out, so the first row is the folder
// before it and Enter goes back there.
func (m *tuiModel) openRecentFolders() (tea.Model, tea.Cmd) {
	if m.client == nil || m.client.Config() == nil || m.client.Config().AccountCache.IsZero() {
		m.err = fmt.Errorf("no account cache to read recent folders from")
		m.showError = true
		return m, nil
	}
	folders, err := cache.LoadRecentFolders(m.client.Config().AccountCache)
	if err != nil {
		m.err = fmt.Errorf("cannot read recent folders: %w", err)
		m.showError = true
		return m, nil
	}
	m.recentFolders = slices.DeleteFunc(folders, func(f string) bool { return f == m.currentF })
	m.recentCursor = 0
	m.recentOpen = true
	m.closeTree()
	m.closeHelp()
	m.showError = false
	return m, nil
}

func (m *tuiModel) kRecentFolders(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := k.String()
	n := len(m.recentFolders)
	switch {
	case s == "esc" || s == "q" || s == keyRecentFolders:
		m.recentOpen = false
	case (s == "up" || s == "k") && n > 0:
		m.recentCursor = (m.recentCursor - 1 + n) % n
	case (s == "down" || s == "j" || s == "tab") && n > 0:
		m.recentCursor = (m.recentCursor + 1) % n
	case s == "enter" || s == " ":
		return m.openRecentAt(m.recentCursor)
	case len(s) == 1 && s[0] >= '0' && s[0] <= '9':
		return m.openRecentAt(recentDigitIndex(s[0]))
	}
	return m, nil
}

// recentDigitIndex is the row a digit opens: 1 to 9 are the first nine rows
// and 0 the tenth, as on the keyboard.
func recentDigitIndex(digit byte) int {
	if digit == '0' {
		return 9
	}
	return int(digit - '1')
}

func recentDigitLabel(i int) string {
	if i == 9 {
		return "0"
	}
	return fmt.Sprint(i + 1)
}

func (m *tuiModel) openRecentAt(i int) (tea.Model, tea.Cmd) {
	if i < 0 || i >= len(m.recentFolders) {
		return m, nil
	}
	return m.enterFolder(m.recentFolders[i])
}

// renderRecentFolders draws the F8 dialog over a dimmed screen.
func renderRecentFolders(m *tuiModel) string {
	t := m.theme.Theme()
	head := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(t.Get(render.ColorHighlightFg)))
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color(t.Get(render.ColorDate)))
	fg := lipgloss.NewStyle().Foreground(lipgloss.Color(t.Get(render.ColorFg)))

	lines := []string{head.Render("       RECENT FOLDERS"), ""}
	if len(m.recentFolders) == 0 {
		lines = append(lines, dim.Render("  No other folder opened yet. Folders you open"))
		lines = append(lines, dim.Render("  are listed here, newest first."))
	}
	for i, folder := range m.recentFolders {
		prefix, style := "  ", fg
		if i == m.recentCursor {
			prefix, style = "> ", head
		}
		row := fmt.Sprintf("%s%s  %s", prefix, recentDigitLabel(i), truncateTo(folder, 60))
		lines = append(lines, rowZone(frameRecent, i, 0, style.Render(row)))
	}
	lines = append(lines, "", dim.Render(markHints(
		"Enter: Open  │  1-9,0: Jump  │  j/k: Nav  │  Esc: Close",
		hint{"Enter: Open", "enter"}, hint{"Esc: Close", "esc"})))

	dialog := lipgloss.NewStyle().
		Width(min(72, max(20, m.width-4))).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(t.Get(render.ColorBorder))).
		Padding(1, 2).
		Render(strings.Join(lines, "\n"))
	return lipgloss.NewStyle().
		Width(m.width).
		Height(max(10, m.height)).
		Align(lipgloss.Center, lipgloss.Center).
		Render(dialog)
}
