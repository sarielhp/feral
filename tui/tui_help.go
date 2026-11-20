package tui

import (
	"fmt"
	"strings"

	"github.com/sarielhp/feral/tui/render"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// The help overlay: a tree, not a wall. The top level is the section menu;
// Enter (or the section's number) opens a page of keys, and opening help on a
// screen goes straight to the page that describes that screen. Pages scroll
// when a section has more keys than the terminal has rows, so the overlay is
// never taller than the screen.

// openHelp opens help on the page the current screen calls for.
func (m *tuiModel) openHelp() {
	m.showHelp = true
	m.helpDirect = true
	m.helpCursor = 0
	m.helpScroll = 0
	m.helpSection = contextualHelpSection(m)
	m.helpFind = helpQuery{}
}

// closeHelp closes help and the search typed into it.
func (m *tuiModel) closeHelp() {
	m.showHelp = false
	m.helpFind = helpQuery{}
}

// toggleHelp opens help, or closes it.
func (m *tuiModel) toggleHelp() (tea.Model, tea.Cmd) {
	if m.showHelp {
		m.closeHelp()
		return m, nil
	}
	m.openHelp()
	return m, nil
}

// openHelpAt lands on a chosen section from the menu.
func (m *tuiModel) openHelpAt(section int) {
	m.helpSection = section
	m.helpCursor = 0
	m.helpScroll = 0
	m.helpDirect = false
	m.helpFind = helpQuery{}
}

// kHelp owns every key while help is open, page or menu. While a search is
// being typed every key but F1 goes to it; otherwise F1, h, ?, and q close
// help from anywhere — the toggles it had as one page — and "/" starts a
// search: of the page, or, from the menu, of every page.
func (m *tuiModel) kHelp(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := k.String()
	if s != "f1" && m.helpFind.key(k) {
		m.helpScroll = 0
		if m.helpSection < 0 && !m.helpFind.active() {
			m.followHelpCursor()
		}
		return m, nil
	}
	switch s {
	case "f1", "h", "q", "?":
		m.closeHelp()
		return m, nil
	}
	if m.helpSection < 0 && !m.helpFind.active() {
		return m.kHelpMenu(s)
	}
	return m.kHelpPage(s)
}

// kHelpMenu is the section list: digits jump, Enter opens, ← and Esc leave.
func (m *tuiModel) kHelpMenu(s string) (tea.Model, tea.Cmd) {
	m.helpCursor = max(0, min(len(helpSections)-1, m.helpCursor))
	switch {
	case s == "esc" || s == "left":
		m.closeHelp()
		return m, nil
	case s == "up" || s == "k":
		m.helpCursor--
		if m.helpCursor < 0 {
			m.helpCursor = len(helpSections) - 1
		}
	case s == "down" || s == "j":
		m.helpCursor = (m.helpCursor + 1) % len(helpSections)
	case s == "home":
		m.helpCursor = 0
	case s == "end":
		m.helpCursor = len(helpSections) - 1
	case s == "enter" || s == "right":
		m.openHelpAt(m.helpCursor)
	default:
		if n, ok := helpDigit(s); ok && n < len(helpSections) {
			m.openHelpAt(n)
		}
	}
	if m.helpSection < 0 {
		m.followHelpCursor()
	}
	return m, nil
}

// followHelpCursor scrolls the section menu so its cursor is on screen: on
// a short terminal the menu is windowed like a page.
func (m *tuiModel) followHelpCursor() {
	rows := m.helpBodyRows()
	switch {
	case m.helpCursor < m.helpScroll:
		m.helpScroll = m.helpCursor
	case m.helpCursor >= m.helpScroll+rows:
		m.helpScroll = m.helpCursor - rows + 1
	}
	m.helpScroll = max(0, min(m.helpScroll, len(helpSections)-rows))
}

// backToHelpMenu steps from a page to the menu, the cursor on the section
// just left.
func (m *tuiModel) backToHelpMenu() {
	if m.helpSection >= 0 {
		m.helpCursor = m.helpSection
	}
	m.helpFind = helpQuery{}
	m.helpSection = -1
	m.helpDirect = false
	m.helpScroll = 0
	m.followHelpCursor()
}

// helpDigit is a section's number key: "1" is 0.
func helpDigit(s string) (int, bool) {
	if len(s) != 1 || s[0] < '1' || s[0] > '9' {
		return 0, false
	}
	return int(s[0] - '1'), true
}

// kHelpPage is a section's keys: scrolling, and the ways out. Esc closes a
// page that its screen opened directly and steps back through the menu
// otherwise; ← always steps back to the menu.
func (m *tuiModel) kHelpPage(s string) (tea.Model, tea.Cmd) {
	bound := helpScrollBound(m)
	switch {
	case s == "esc" && m.helpDirect && m.helpSection >= 0:
		m.closeHelp()
	case s == "esc" || s == "left":
		m.backToHelpMenu()
	case s == "up" || s == "k":
		m.helpScroll = max(0, m.helpScroll-1)
	case s == "down" || s == "j":
		m.helpScroll = min(bound, m.helpScroll+1)
	case s == "pageup" || s == "pgup":
		m.helpScroll = max(0, m.helpScroll-m.helpBodyRows())
	case s == "pagedown" || s == "pgdn" || s == "pgdown":
		m.helpScroll = min(bound, m.helpScroll+m.helpBodyRows())
	case s == "home":
		m.helpScroll = 0
	case s == "end":
		m.helpScroll = bound
	}
	return m, nil
}

// helpScrollBound is the scroll where a page's last body line is visible.
func helpScrollBound(m *tuiModel) int {
	if m.helpSection >= len(helpSections) || (m.helpSection < 0 && !m.helpFind.active()) {
		return 0
	}
	overflow := len(helpBody(m, m.helpSection, m.helpFind)) - m.helpBodyRows()
	return max(0, overflow)
}

// helpBodyRows is how many body lines a page fits under the frame, the title
// and the hints. Small screens drop the spacer rows, never the content.
func (m *tuiModel) helpBodyRows() int {
	rows := m.height - 7 // 2 frame border, title, 2 spacers, hints
	if m.height < 12 {
		rows = m.height - 5
	}
	return max(1, rows)
}

// helpInnerWidth is the width a help line may reach, inside the frame. The
// frame's border and padding take six columns together, so a small terminal
// gets a smaller page rather than an overlay wider than the screen.
func helpInnerWidth(m *tuiModel) int {
	return max(12, m.width-6)
}

// renderHelp is the overlay: the section menu, the page its state sits on,
// or — when called while help is closed, as a direct call does — the page the
// current screen calls for.
func renderHelp(m *tuiModel) string {
	head := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.theme.Theme().Get(render.ColorHighlightFg)))
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Theme().Get(render.ColorDate)))

	title, body, hints := helpOwnership(m, head)
	scroll := m.helpScroll
	if !m.showHelp {
		scroll = 0
	}
	windowed := helpWindowLines(body, scroll, m.helpBodyRows())
	full := m.helpAssemble(title, windowed, hints, dim)
	return renderHelpFrame(full, m.theme.Theme().Get(render.ColorBorder), lipgloss.Color("#1a1a35"))
}

// helpOwnership resolves which page the overlay shows now: the menu, a page,
// or — while a search is set — the page's matching keys, or every page's
// from the menu.
func helpOwnership(m *tuiModel, head lipgloss.Style) (string, []string, string) {
	section, q := m.helpSection, m.helpFind
	if !m.showHelp {
		section, q = contextualHelpSection(m), helpQuery{}
	}
	if section >= len(helpSections) {
		section = -1
	}
	name := "HELP"
	if section >= 0 {
		name = helpSections[section].name
	}
	switch {
	case q.active():
		title := truncateTo(fmt.Sprintf("  ⚡ %s — search %s", name, q.prompt()), helpInnerWidth(m))
		return title, helpBody(m, section, q), helpSearchHints(q)
	case section < 0:
		return "  ⚡ HELP — topics", helpMenuLines(m, head), helpMenuHints()
	}
	return fmt.Sprintf("  ⚡ %s — keybindings", name), helpBody(m, section, q), helpPageHints()
}

// helpBody is the laid-out body of a page, or of a search: section's rows
// that match q, or, for the menu (section < 0), every section's matches under
// its name. The scroll bound and the window share it.
func helpBody(m *tuiModel, section int, q helpQuery) []string {
	if section >= 0 {
		body := helpSectionLines(m, q.filter(m.helpRows(section)))
		if len(body) == 0 {
			return []string{noHelpMatch}
		}
		return body
	}
	head := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.theme.Theme().Get(render.ColorHighlightFg)))
	var body []string
	for i, sec := range helpSections {
		rows := q.filter(m.helpRows(i))
		if len(rows) == 0 {
			continue
		}
		body = append(body, head.Render(truncateTo(" "+sec.name, helpInnerWidth(m))))
		body = append(body, helpSectionLines(m, rows)...)
	}
	if len(body) == 0 {
		return []string{noHelpMatch}
	}
	return body
}

// helpAssemble frames the page body under its title and hint line, dropping
// the spacers on small screens and clipping from the top if anything still
// overflows, so the overlay never outruns the terminal.
func (m *tuiModel) helpAssemble(title string, windowed []string, hints string, dim lipgloss.Style) string {
	marked := dim.Render(markHints(truncateTo(hints, helpInnerWidth(m)),
		hint{"Esc", "esc"}, hint{"Enter", "enter"}, hint{"q", "q"}))
	full := make([]string, 0, len(windowed)+5)
	full = append(full, title)
	if m.height >= 12 {
		full = append(full, "")
	}
	full = append(full, windowed...)
	if m.height >= 12 {
		full = append(full, "")
	}
	full = append(full, marked)
	lines := clipContentTop(full, m.height-2)
	return strings.Join(lines, "\n")
}

// clipContentTop keeps a rendered panel's last lines when it cannot fit,
// which is what losing the top loses least: the title and hints hold.
func clipContentTop(lines []string, budget int) []string {
	if budget < 1 || len(lines) <= budget {
		return lines
	}
	return lines[len(lines)-budget:]
}

// helpWindowLines is the slice of body lines from scroll that fits rows.
func helpWindowLines(body []string, scroll, rows int) []string {
	if scroll > len(body) {
		scroll = len(body)
	}
	scroll = max(0, scroll)
	end := min(len(body), scroll+rows)
	return body[scroll:end]
}

// helpMenuHints is the section list's navigation line.
func helpMenuHints() string {
	return "Enter: open   1-9: jump   /: search all   ←/Esc: close   q: close"
}

// helpPageHints is a key page's navigation line.
func helpPageHints() string {
	return "↑/↓ PgUp/PgDn: scroll   /: search   ←: topics   Esc: back   q: close"
}

// helpSearchHints is the navigation line while a search narrows the help.
func helpSearchHints(q helpQuery) string {
	if q.typing {
		return "type to filter   Enter: keep   Backspace: erase   Esc: clear"
	}
	return "↑/↓: scroll   /: edit search   Esc: clear search   ←: topics   q: close"
}

// helpMenuLines lays the section menu out: a numbered list with
// descriptions, the cursor's row highlighted and clickable.
func helpMenuLines(m *tuiModel, head lipgloss.Style) []string {
	inner := helpInnerWidth(m)
	var body []string
	for i, sec := range helpSections {
		line := fmt.Sprintf("%d  %-14s %s", i+1, sec.name, sec.desc)
		if i == m.helpCursor {
			body = append(body, rowZone(frameHelp, i, 0, head.Render(truncateTo("> "+line, inner))))
			continue
		}
		body = append(body, " "+truncateTo(line, inner-1))
	}
	return body
}

// helpSectionLines lays a section's rows out: the key cell padded to a width
// shared by every row, so all descriptions start at one display column, and
// — on a wide terminal — a second column beside the first to halve the page's
// height.
func helpSectionLines(m *tuiModel, rows []helpRow) []string {
	inner := helpInnerWidth(m)
	keyW, descW := 0, 0
	for _, row := range rows {
		keyW = max(keyW, runewidth.StringWidth(row.keys))
		descW = max(descW, runewidth.StringWidth(row.desc))
	}
	// Two columns only when the widest cell fits half the page: a cell wider
	// than its column is clipped at the frame, and no scrolling brings it back.
	two := inner >= 80 && len(rows) >= 12 && helpCellWidth(keyW, descW) <= inner/2-1
	colW := inner
	if two {
		colW = inner/2 - 1
	}
	left, right := rows, []helpRow(nil)
	if two {
		half := (len(rows) + 1) / 2
		left, right = rows[:half], rows[half:]
	}

	var body []string
	for i := range left {
		line := helpCell(left[i], keyW, colW)
		if two && i < len(right) {
			line += helpCell(right[i], keyW, colW)
		}
		// MaxWidth, not rune cutting: the line carries colour codes. It is
		// the frame's inner width, not the terminal's: the border and the
		// frame's own padding take their columns too.
		body = append(body, lipgloss.NewStyle().MaxWidth(helpInnerWidth(m)).Render(line))
	}
	return body
}

// helpCellWidth is the display width of a cell, as helpCell lays it out.
func helpCellWidth(keyW, descW int) int {
	return 2 + keyW + 2 + descW
}

// helpCell is one row: its key padded to keyW, then its description, the
// whole cell filling colW so a second column starts at a fixed position.
func helpCell(row helpRow, keyW, colW int) string {
	line := fmt.Sprintf("  %s  %s", padTo(row.keys, keyW), row.desc)
	pad := colW - runewidth.StringWidth(line)
	if pad < 0 {
		pad = 0
	}
	return line + strings.Repeat(" ", pad)
}

// padTo right-pads s to width display columns.
func padTo(s string, width int) string {
	pad := width - runewidth.StringWidth(s)
	if pad < 0 {
		return s
	}
	return s + strings.Repeat(" ", pad)
}
