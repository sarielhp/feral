package contactsui

import (
	"fmt"
	"strings"

	"github.com/sarielhp/feral/contacts"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// managerHeaderLines is how many lines come before the first list row.
const managerHeaderLines = 4

// View draws the current screen.
func (m *Manager) View() string {
	width := max(30, m.width)
	var body []string
	switch m.mode {
	case modeHelp:
		body = m.helpLines()
	case modeForm:
		body = m.formLines(width)
	case modeDetail:
		body = m.detailLines(width)
	default:
		body = m.listLines(width)
	}
	body = append(body, "", m.footer(width))
	return lipgloss.NewStyle().MaxWidth(width).Render(strings.Join(body, "\n"))
}

// footer is the status line, or the question waiting for an answer.
func (m *Manager) footer(width int) string {
	if m.confirm != nil {
		return m.st.warn.Render(Truncate(m.confirm.prompt, width))
	}
	if m.status != "" {
		return m.st.label.Render(Truncate(m.status, width))
	}
	return m.st.dim.Render(Truncate(m.keyHint(), width))
}

func (m *Manager) keyHint() string {
	switch m.mode {
	case modeForm:
		return "Tab/↑/↓: field  Enter: next (saves on the last)  Ctrl-S: save  Esc: cancel"
	case modeDetail:
		return "e: edit  r: revert to source  d: delete/hide  u: unhide  Esc: back"
	case modeSearch:
		return "type: search  ↑/↓: move  Enter: keep  Esc: clear"
	}
	return "/: search  Enter: open  a: add  N: group  e: edit  d: delete  Space: mark  m: merge  Tab: list  ?: help  q: quit"
}

func (m *Manager) listLines(width int) []string {
	tabs := make([]string, 0, viewCount)
	for v := range viewCount {
		name := v.String()
		if v == m.view {
			name = m.st.sel.Render("[" + name + "]")
		}
		tabs = append(tabs, name)
	}
	count := fmt.Sprintf("%d shown", len(m.list.Results))
	if n := m.list.MarkCount(); n > 0 {
		count += fmt.Sprintf(", %d marked", n)
	}
	head := m.st.title.Render("CONTACTS") + "  " + strings.Join(tabs, " ") + m.st.dim.Render("  "+count+m.sortLabel())
	search := "Search: " + m.list.Query
	if m.mode == modeSearch {
		search += "▏"
	}
	lines := []string{head, "", search, ""}
	return append(lines, m.rowLines(width)...)
}

// sortLabel names the order of the Contacts list; the others have one order.
func (m *Manager) sortLabel() string {
	if m.view != viewContacts {
		return ""
	}
	return "  sort: " + m.sortBy.String()
}

func (m *Manager) rowLines(width int) []string {
	l := m.list
	if len(l.Results) == 0 {
		return []string{m.st.dim.Render("  Nothing to show." + m.emptyHint())}
	}
	start, end := l.Window(m.rows())
	out := make([]string, 0, end-start+1)
	for i := start; i < end; i++ {
		sc := l.Results[i]
		mark := "  "
		if l.IsMarked(sc) {
			mark = "✓ "
		}
		row := Truncate(mark+m.rowText(sc), width-2)
		if i == l.Cursor {
			out = append(out, m.st.sel.Render("> "+row))
			continue
		}
		out = append(out, m.st.fg.Render("  "+row))
	}
	if len(l.Results) > m.rows() {
		out = append(out, m.st.dim.Render(fmtRange(start, end, len(l.Results))))
	}
	return out
}

func (m *Manager) emptyHint() string {
	switch {
	case m.list.Query != "":
		return " Esc clears the search."
	case m.view == viewHistory:
		return " Run `feral contacts harvest` to collect addresses from mail."
	case m.view == viewContacts:
		return " a adds a contact; `feral contacts sync` imports Google's."
	}
	return ""
}

// rowText is one list row: name, address, alias, where it is from.
func (m *Manager) rowText(sc contacts.ScoredContact) string {
	addr := sc.PrimaryEmail()
	if sc.IsGroup() {
		addr = fmt.Sprintf("group of %d", len(sc.Emails))
	} else if n := len(sc.Emails); n > 1 {
		addr += fmt.Sprintf(" +%d", n-1)
	}
	source := sc.Source
	if sc.IsEdited() {
		source += "*"
	}
	extra := ""
	if sc.Frequency > 0 {
		extra = fmt.Sprintf("%d×", sc.Frequency)
	}
	return fmt.Sprintf("%-26s %-34s %-12s %-14s %s", Truncate(sc.Label(), 26), Truncate(addr, 34),
		Truncate(sc.DisplayAlias(), 12), Truncate(source, 14), extra)
}

func (m *Manager) detailLines(width int) []string {
	c, ok := m.detailContact()
	if !ok {
		return []string{m.st.dim.Render("That contact is gone.")}
	}
	kind := "CONTACT"
	if c.IsGroup() {
		kind = "GROUP"
	}
	lines := []string{m.st.title.Render(kind + "  " + c.Label()), ""}
	src := c.SourceView()
	field := func(label, value, source string) {
		lines = append(lines, m.st.label.Render(fmt.Sprintf("%-10s", label))+" "+Truncate(value, width-12))
		if c.IsEdited() && source != value {
			lines = append(lines, m.st.dim.Render(fmt.Sprintf("%-10s %s says: %s", "", c.Source, Truncate(source, width-30))))
		}
	}
	emailsLabel := "Emails"
	if c.IsGroup() {
		emailsLabel = "Members"
	}
	field("Name", c.Name, src.Name)
	field(emailsLabel, strings.Join(c.Emails, ", "), strings.Join(src.Emails, ", "))
	field("Aliases", strings.Join(c.Aliases, ", "), strings.Join(c.Aliases, ", "))
	if len(c.Nicknames) > 0 {
		nicks := strings.Join(c.Nicknames, ", ") + " (from " + c.Source + ")"
		field("Nicknames", nicks, nicks)
	}
	field("Groups", strings.Join(c.Groups, ", "), strings.Join(src.Groups, ", "))
	field("Notes", c.Notes, c.Notes)
	field("Source", c.Source, c.Source)
	field("ID", c.ID, c.ID)
	if c.IsEdited() {
		lines = append(lines, "", m.st.dim.Render("Edited here; your edits are kept across syncs. r reverts to the source."))
	}
	return lines
}

func (m *Manager) formLines(width int) []string {
	f := m.form
	lines := []string{m.st.title.Render(f.title()), ""}
	for i := range fieldCount {
		label := fmt.Sprintf("%-8s", f.label(i))
		value := f.inputs[i].view(i == f.focus)
		row := m.st.label.Render(label) + " " + Truncate(value, width-10)
		if i == f.focus {
			row = m.st.sel.Render("›") + row
		} else {
			row = " " + row
		}
		lines = append(lines, row)
		if hint := f.sourceHint(i); hint != "" {
			lines = append(lines, m.st.dim.Render(fmt.Sprintf("          source: %s", Truncate(hint, width-20))))
		}
	}
	lines = append(lines, "", m.st.dim.Render("Lists are comma-separated. The first address is the primary one."))
	if f.source != nil {
		lines = append(lines, m.st.dim.Render("This contact is imported: your changes are kept beside the source's and survive every sync."))
	}
	return lines
}

func (m *Manager) helpLines() []string {
	keys := [][2]string{
		{"/", "search (Enter keeps it, Esc clears it)"},
		{"↑ ↓ j k PgUp PgDn g G", "move"},
		{"Enter", "open the contact (in Harvested: make it a contact)"},
		{"Tab / Shift-Tab", "switch list: Contacts, Harvested, Hidden"},
		{"a", "add a contact"},
		{"N", "new group of the marked contacts"},
		{"e", "edit"},
		{"d", "delete a manual contact, hide an imported one"},
		{"u", "bring back a hidden contact (Hidden list)"},
		{"r", "revert an edited imported contact to its source"},
		{"Space", "mark; x clears the marks"},
		{"m", "merge the one marked contact into the one under the cursor"},
		{"p", "make the harvested address a contact (Harvested list)"},
		{"s", "sort by name, use, or source"},
		{"Ctrl-R", "reload from disk"},
		{"q / Esc", "quit"},
	}
	lines := []string{m.st.title.Render("CONTACTS — KEYS"), ""}
	for _, k := range keys {
		lines = append(lines, m.st.label.Render(fmt.Sprintf("%-24s", k[0]))+" "+k[1])
	}
	lines = append(lines, "", m.st.dim.Render("A * after the source marks an imported contact you edited. Any key closes this."))
	return lines
}

func (m *Manager) mouse(msg tea.MouseMsg) {
	if m.mode != modeList && m.mode != modeSearch {
		return
	}
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		m.list.Move(-3)
		return
	case tea.MouseButtonWheelDown:
		m.list.Move(3)
		return
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return
	}
	start, end := m.list.Window(m.rows())
	i := start + msg.Y - managerHeaderLines
	if i < start || i >= end {
		return
	}
	if i == m.list.Cursor {
		m.open()
		return
	}
	m.list.Cursor = i
}
