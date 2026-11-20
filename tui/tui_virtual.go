package tui

import (
	"errors"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/sarielhp/feral/cache/virtual"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/tui/render"
)

// virtualFolderPrefix names the TUI's view of a saved result set. Like the
// search view it is not a server folder.
const virtualFolderPrefix = "%virtual:"

func isVirtualView(folder string) bool { return strings.HasPrefix(folder, virtualFolderPrefix) }

// errNoSourceFolder refuses an action on a saved set's message that nothing
// places in a folder. A move names one source folder, and naming the wrong one
// takes mail out of a folder it was never in.
var errNoSourceFolder = errors.New("this saved search does not record a folder for that message; open the message's folder to act on it")

// refreshCurrentView reloads whatever the current view shows. A server folder
// is re-downloaded, but neither result-set view names one: a saved set is
// re-read from the cache, and a remote search's query is re-run at its limit.
// Sending "%virtual:<name>" or "%search:<q>" to a backend used to be what R
// did, which is a request for a folder that does not exist.
func (m *tuiModel) refreshCurrentView() (tea.Model, tea.Cmd) {
	m.showError = false
	m.err = nil
	switch {
	case isVirtualView(m.currentF):
		return m.openVirtualMailbox(strings.TrimPrefix(m.currentF, virtualFolderPrefix))
	case isSearchView(m.currentF):
		return m.refreshSearchResults()
	}
	m.showLoad = "Downloading..."
	m.lastCheckTime = time.Now()
	return m, m.loadEmailsCmd(m.currentF)
}

// refuseInSearchView reports, and returns true, when the current view is a
// remote search's results, where a move would have to name a source folder the
// view does not carry: the query is re-run, so the rows span folders and
// nothing records which. A saved set does record one per message, which is what
// makes it actionable, so only the search view refuses.
func (m *tuiModel) refuseInSearchView() bool {
	if !isSearchView(m.currentF) {
		return false
	}
	m.err = errSearchViewAction
	m.showError = true
	return true
}

// savedSet is the saved set the current view shows, or nil when the view is not
// that set.
func (m *tuiModel) savedSet() *virtual.Mailbox { return m.savedSetFor(m.currentF) }

// savedSetFor is the loaded set when folder is the view of it, or nil. The
// field outlives a folder change, so it is checked against the folder it claims
// to be rather than cleared at every switch.
func (m *tuiModel) savedSetFor(folder string) *virtual.Mailbox {
	if m.virtualBox == nil || folder != virtualFolderPrefix+m.virtualBox.Name {
		return nil
	}
	return m.virtualBox
}

// savedSetFolders is where each message of the saved set named by folder lives,
// which is the source folder an action on it must name: the set's rows come
// from several folders, so the folder on screen is not one of them.
//
// An id the set does not place is looked up in the mailboxes that do, and one
// nothing knows is left out of the result, so the action refuses that id by
// name rather than moving it out of a folder it was never in.
func (m *tuiModel) savedSetFolders(folder string, ids []email.ID) map[email.ID]string {
	box := m.savedSetFor(folder)
	out := make(map[email.ID]string, len(ids))
	for _, id := range ids {
		if box != nil && box.FolderMap[id] != "" {
			out[id] = box.FolderMap[id]
			continue
		}
		if f := virtual.FindMessageFolder(setCache(m.client), id); f != "" {
			out[id] = f
		}
	}
	return out
}

// sourceFolderOf names where a message lives, for an action that must name a
// folder without going through an actionPlan: copying one out, or blocking its
// sender. A saved set is the one view whose rows do not answer with the folder
// on screen; an id the set does not place returns "", which the caller refuses.
func (m *tuiModel) sourceFolderOf(id email.ID) string {
	if !isVirtualView(m.currentF) {
		return m.currentF
	}
	return m.savedSetFolders(m.currentF, []email.ID{id})[id]
}

// dropFromSavedSet takes ids out of the saved set on screen and writes it back.
// A message that left the view has been filed elsewhere, so it is no longer
// where the set says it is: the set keeps what is still there, and a refresh
// does not resurrect the row the user just moved. The mirror in the folder
// index follows, because virtual.Save rewrites it.
func (m *tuiModel) dropFromSavedSet(ids []email.ID) {
	box := m.savedSet()
	if box == nil || len(ids) == 0 {
		return
	}
	gone := make(map[email.ID]bool, len(ids))
	for _, id := range ids {
		gone[id] = true
	}
	kept := make([]email.ID, 0, len(box.MessageIDs))
	for _, id := range box.MessageIDs {
		if gone[id] {
			delete(box.FolderMap, id)
			continue
		}
		kept = append(kept, id)
	}
	if len(kept) == len(box.MessageIDs) {
		return
	}
	box.MessageIDs = kept
	if err := virtual.Save(setCache(m.client), box); err != nil {
		m.err = fmt.Errorf("cannot update saved search %q: %w", box.Name, err)
		m.showError = true
	}
}

// openVirtualBrowser lists the account's saved result sets and shows them. It
// is the read side of virtual.Save, which every search, `last` and a contact's
// latest mail write to.
func (m *tuiModel) openVirtualBrowser() {
	if m.client == nil || m.client.Config() == nil || m.client.Config().AccountCache.IsZero() {
		m.err = fmt.Errorf("no account cache to read saved searches from")
		m.showError = true
		return
	}
	mailboxes, err := virtual.List(m.client.Config().AccountCache)
	if err != nil {
		m.err = fmt.Errorf("cannot list saved searches: %w", err)
		m.showError = true
		return
	}
	m.virtualMailboxes = mailboxes
	m.virtualCursor = 0
	m.virtualBrowserOpen = true
	m.closeTree()
	m.closeHelp()
	m.showError = false
}

func (m *tuiModel) kVirtualBrowser(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := k.String()
	switch {
	case s == "esc" || s == "q" || s == "f7":
		m.virtualBrowserOpen = false
		return m, nil
	case s == "up" || s == "k":
		if len(m.virtualMailboxes) > 0 {
			m.virtualCursor--
			if m.virtualCursor < 0 {
				m.virtualCursor = len(m.virtualMailboxes) - 1
			}
		}
	case s == "down" || s == "j" || s == "tab":
		if len(m.virtualMailboxes) > 0 {
			m.virtualCursor++
			if m.virtualCursor >= len(m.virtualMailboxes) {
				m.virtualCursor = 0
			}
		}
	case s == "enter" || s == " ":
		if m.virtualCursor >= 0 && m.virtualCursor < len(m.virtualMailboxes) {
			return m.openVirtualMailbox(m.virtualMailboxes[m.virtualCursor].Name)
		}
	case s == "d":
		return m.deleteVirtualMailbox()
	}
	return m, nil
}

// deleteVirtualMailbox removes the selected saved result set.
func (m *tuiModel) deleteVirtualMailbox() (tea.Model, tea.Cmd) {
	if m.virtualCursor < 0 || m.virtualCursor >= len(m.virtualMailboxes) {
		return m, nil
	}
	name := m.virtualMailboxes[m.virtualCursor].Name
	if err := virtual.Delete(m.client.Config().AccountCache, name); err != nil {
		m.err = fmt.Errorf("cannot delete saved search %q: %w", name, err)
		m.showError = true
		return m, nil
	}
	m.virtualMailboxes = append(m.virtualMailboxes[:m.virtualCursor], m.virtualMailboxes[m.virtualCursor+1:]...)
	if m.virtualCursor >= len(m.virtualMailboxes) {
		m.virtualCursor = max(0, len(m.virtualMailboxes)-1)
	}
	m.saveStatus = fmt.Sprintf("Deleted saved search %q", name)
	m.saveStatusTime = time.Now()
	return m, nil
}

type virtualMailboxLoadedMsg struct {
	name   string
	emails []*email.Email
	err    error
}

// openVirtualMailbox shows a saved result set as the message list. The set
// already holds its message IDs, so only their summaries are needed.
func (m *tuiModel) openVirtualMailbox(name string) (tea.Model, tea.Cmd) {
	if m.client == nil || m.client.Config() == nil {
		return m, nil
	}
	mb, err := virtual.Load(m.client.Config().AccountCache, name)
	if err != nil {
		m.err = fmt.Errorf("cannot open saved search %q: %w", name, err)
		m.showError = true
		return m, nil
	}
	m.virtualBrowserOpen = false
	m.virtualBox = mb
	m.showLoad = fmt.Sprintf("Opening %s...", name)
	client := m.client
	ids := mb.MessageIDs
	return m, func() tea.Msg {
		if len(ids) == 0 {
			return virtualMailboxLoadedMsg{name: name, emails: []*email.Email{}}
		}
		summaries, err := client.GetMessageSummariesBatch(ids)
		if err != nil {
			return virtualMailboxLoadedMsg{name: name, err: err}
		}
		return virtualMailboxLoadedMsg{name: name, emails: summaries}
	}
}

func (m *tuiModel) handleVirtualMailboxLoaded(msg virtualMailboxLoadedMsg) (tea.Model, tea.Cmd) {
	m.showLoad = ""
	if msg.err != nil {
		m.err = fmt.Errorf("cannot open saved search %q: %w", msg.name, msg.err)
		m.showError = true
		return m, nil
	}
	rawList := make([]email.Email, 0, len(msg.emails))
	for _, em := range msg.emails {
		if em != nil {
			rawList = append(rawList, *em)
		}
	}
	m.endListFilter()
	m.pushHistory(virtualFolderPrefix + msg.name)
	m.rawEmails = rawList
	m.currentF = virtualFolderPrefix + msg.name
	m.rebuildVisibleEmails()
	m.eIdx = 0
	if len(m.emails) > 0 {
		m.selectedID = m.emails[0].ID
	} else {
		m.selectedID = ""
	}
	m.saveStatus = fmt.Sprintf("%s: %d message(s)", msg.name, len(rawList))
	m.saveStatusTime = time.Now()
	return m, nil
}

// renderVirtualBrowser draws the F7 dialog over a dimmed screen.
func renderVirtualBrowser(m *tuiModel) string {
	t := m.theme.Theme()
	head := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(t.Get(render.ColorHighlightFg)))
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color(t.Get(render.ColorDate)))
	fg := lipgloss.NewStyle().Foreground(lipgloss.Color(t.Get(render.ColorFg)))

	lines := []string{head.Render("       SAVED SEARCHES"), ""}
	if len(m.virtualMailboxes) == 0 {
		lines = append(lines, dim.Render("  Nothing saved yet. A search, `last`, or a"))
		lines = append(lines, dim.Render("  contact's latest mail saves its result set here."))
	} else {
		for i, mb := range m.virtualMailboxes {
			prefix, style := "  ", fg
			if i == m.virtualCursor {
				prefix, style = "> ", head
			}
			desc := mb.Description
			if desc == "" {
				desc = "saved result set"
			}
			row := fmt.Sprintf("%s%-28s %s (%d)", prefix, truncateTo(mb.Name, 28), desc, len(mb.MessageIDs))
			lines = append(lines, rowZone(frameVirtual, i, 0, style.Render(row)))
		}
	}

	lines = append(lines, "")
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(t.Get(render.ColorDate)))
	lines = append(lines, helpStyle.Render(markHints(
		"Enter: Open  │  d: Delete  │  j/k: Nav  │  Esc: Close",
		hint{"Enter: Open", "enter"}, hint{"d: Delete", "d"}, hint{"Esc: Close", "esc"})))

	dialogWidth := min(72, max(20, m.width-4))
	if m.width > 0 && dialogWidth > m.width {
		dialogWidth = m.width
	}
	dialog := lipgloss.NewStyle().
		Width(dialogWidth).
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
