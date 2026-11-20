package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/sarielhp/feral/app"
	"github.com/sarielhp/feral/tui/render"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func (m *tuiModel) topBarAccountName() string {
	if m.Config() != nil {
		for _, a := range m.Config().Accounts {
			if m.Config().IsSelected(a) {
				return a.GetDisplayName()
			}
		}
	} else if m.cfg != nil {
		for _, a := range m.cfg.Accounts {
			if m.cfg.IsSelected(a) {
				return a.GetDisplayName()
			}
		}
	}
	return "default"
}

func (m *tuiModel) buildTopBarLeft(menuCS render.ColoredString, account, folder, unreadStr string) render.ColoredString {
	left := joinCol(menuCS, render.Plain(account).WithFg(render.ColorFg).WithBg(render.ColorTopBg)).WithBgAll(render.ColorTopBg)
	if m.isAccountReadOnly() {
		left = joinCol(left, render.Plain(" [READ-ONLY]").WithFg(render.ColorHighlightFg).WithBg(render.ColorTopBg)).WithBgAll(render.ColorTopBg)
	} else if m.isCurrentFolderReadOnly() {
		left = joinCol(left, render.Plain(" [RO-FOLDER]").WithFg(render.ColorHighlightFg).WithBg(render.ColorTopBg)).WithBgAll(render.ColorTopBg)
	}
	left = joinCol(left, render.Plain(" - ").WithFg(render.ColorFg).WithBg(render.ColorTopBg)).WithBgAll(render.ColorTopBg)
	left = joinCol(left, render.Plain(folder).WithFg(render.ColorFg).WithBg(render.ColorTopBg)).WithBgAll(render.ColorTopBg)
	left = joinCol(left, render.Plain(" | ").WithFg(render.ColorFg).WithBg(render.ColorTopBg)).WithBgAll(render.ColorTopBg)
	left = joinCol(left, render.Plain(unreadStr).WithFg(render.ColorDate).WithBg(render.ColorTopBg)).WithBgAll(render.ColorTopBg)
	if m.countTagged() > 0 {
		tagStr := fmt.Sprintf(" | %d tagged", m.countTagged())
		left = joinCol(left, render.Plain(tagStr).WithFg(render.ColorHighlightFg).WithBg(render.ColorTopBg)).WithBgAll(render.ColorTopBg)
	}
	return left
}

func (m *tuiModel) renderTopBarPrompt(leftStr, rightStr string, leftW, rightW int, topBgStyle lipgloss.Style) string {
	boxText, boxStyle, ok := m.topBarPromptBox()
	if !ok {
		return ""
	}
	return m.renderPromptBox(topBarFrame{leftStr, rightStr, leftW, rightW, topBgStyle}, boxText, boxStyle)
}

// topBarFrame is the fixed context a top-bar prompt box is drawn into: the
// rendered left and right segments, their widths, and the style that fills the
// gap between them.
type topBarFrame struct {
	leftStr    string
	rightStr   string
	leftW      int
	rightW     int
	topBgStyle lipgloss.Style
}

// topBarPromptBox returns the text and style of whichever top-bar prompt is
// open, or ok=false when none is.
func (m *tuiModel) topBarPromptBox() (string, lipgloss.Style, bool) {
	switch {
	case m.inSearch || m.searchQuery != "":
		prefix := "/"
		if m.fuzzySearch && m.fuzzyGlobal {
			prefix = "~"
		} else if m.fuzzySearch {
			prefix = "!"
		}
		cursor := ""
		if m.inSearch {
			cursor = "█"
		}
		return " " + prefix + m.searchQuery + cursor + " ",
			lipgloss.NewStyle().Foreground(lipgloss.Color("#cc1111")).Background(lipgloss.Color("#d8d8d8")).Bold(true), true
	case m.inPipePrompt:
		return " | " + m.pipeCommand + "█ ",
			lipgloss.NewStyle().Foreground(lipgloss.Color("#11cc11")).Background(lipgloss.Color("#222222")).Bold(true), true
	case m.inTagPatternPrompt:
		return " Tag Pattern: " + m.tagPatternQuery + "█ ",
			lipgloss.NewStyle().Foreground(lipgloss.Color("#ffd700")).Background(lipgloss.Color("#222222")).Bold(true), true
	case m.inTagPrefix:
		return " ; (E:archive d:delete s:move a:to account m:read M:unread S:spam v:save x:mbox) ",
			lipgloss.NewStyle().Foreground(lipgloss.Color("#ffd700")).Background(lipgloss.Color("#331133")).Bold(true), true
	case m.inSaveNamePrompt:
		label := "Save as saved search: "
		if m.saveNameKind == saveNameMbox {
			label = "Export to mbox: "
		}
		return " " + label + m.saveNameQuery + "█ ",
			lipgloss.NewStyle().Foreground(lipgloss.Color("#ffd700")).Background(lipgloss.Color("#331133")).Bold(true), true
	case m.inMovePrompt:
		return fmt.Sprintf(" Move %d to [account:]folder: %s█ ", m.countTagged(), m.moveQuery),
			lipgloss.NewStyle().Foreground(lipgloss.Color("#ffd700")).Background(lipgloss.Color("#331133")).Bold(true), true
	case m.inSnoozePrompt:
		return " Snooze until (+2h, tomorrow 8am, 2026-12-01 09:00): " + m.snoozeQuery + "█ ",
			lipgloss.NewStyle().Foreground(lipgloss.Color("#ffd700")).Background(lipgloss.Color("#331133")).Bold(true), true
	case m.inRemoteSearch:
		return " Remote: " + m.remoteSearchQuery + "█ ",
			lipgloss.NewStyle().Foreground(lipgloss.Color("#00ffff")).Background(lipgloss.Color("#112233")).Bold(true), true
	default:
		return "", lipgloss.Style{}, false
	}
}

// renderPromptBox centres a prompt box between the top bar's left and right
// segments, truncating the text to the space the frame leaves.
func (m *tuiModel) renderPromptBox(f topBarFrame, boxText string, style lipgloss.Style) string {
	boxText = truncateBoxText(boxText, max(6, m.width-f.leftW-f.rightW-4))
	boxW := len([]rune(boxText))

	remaining := max(0, m.width-f.leftW-f.rightW-boxW)
	leftPad := 1
	if remaining >= 2 {
		leftPad = 2
	}
	rightPad := max(0, remaining-leftPad)

	var sb strings.Builder
	sb.WriteString(f.leftStr)
	sb.WriteString(f.topBgStyle.Render(strings.Repeat(" ", leftPad)))
	sb.WriteString(style.Render(boxText))
	sb.WriteString(f.topBgStyle.Render(strings.Repeat(" ", rightPad)))
	sb.WriteString(f.rightStr)
	return sb.String()
}

// truncateBoxText shortens text to maxW runes, ending it with an ellipsis and a
// space when it had to cut.
func truncateBoxText(text string, maxW int) string {
	if len([]rune(text)) <= maxW {
		return text
	}
	return string([]rune(text)[:max(1, maxW-1)]) + "… "
}

func renderTopBar(m *tuiModel) string {
	t := m.theme.Theme()
	account := m.topBarAccountName()
	folder := m.currentF
	if folder == "" && len(m.folders) > 0 {
		folder = m.folders[0].FullName
	}
	unread := 0
	for _, e := range m.rawEmails {
		if !e.IsRead {
			unread++
		}
	}
	unreadStr := fmt.Sprintf("%d unread", unread)

	menuLabel := " [Menu] "
	var menuCS render.ColoredString
	if m.menuOpen {
		menuCS = render.Plain(menuLabel).WithFg(render.ColorTopBg).WithBg(render.ColorHighlightFg)
	} else {
		menuCS = render.Plain(menuLabel).WithFg(render.ColorHighlightFg).WithBg(render.ColorTopBg)
	}

	left := m.buildTopBarLeft(menuCS, account, folder, unreadStr)
	helpHint := "h/?: help"
	if isSearchView(m.currentF) && m.remoteSearchMore {
		helpHint = m.keyHint(groupList, actLoadMore) + ": more  " + helpHint
	}
	if m.mouseOff {
		helpHint = mouseOffHint + "  " + helpHint
	}
	right := render.Plain(helpHint).WithFg(render.ColorHighlightFg).WithBg(render.ColorTopBg)

	leftW := left.Width()
	rightW := right.Width()
	topBgStyle := lipgloss.NewStyle().Background(lipgloss.Color(t.Get(render.ColorTopBg)))

	// A top bar wider than the screen would wrap; keep the hint and shave
	// the account, folder and unread segments instead.
	if m.width < leftW+rightW {
		left = left.TruncateTo(m.width - rightW)
		right = right.TruncateTo(m.width)
		leftW, rightW = left.Width(), right.Width()
	}
	leftStr := left.Render(t)
	rightStr := right.Render(t)

	if prompt := m.renderTopBarPrompt(leftStr, rightStr, leftW, rightW, topBgStyle); prompt != "" {
		return prompt
	}

	spacesNeeded := max(0, m.width-leftW-rightW)
	return leftStr + topBgStyle.Render(strings.Repeat(" ", spacesNeeded)) + rightStr
}

func joinCol(cols ...render.ColoredString) render.ColoredString {
	var all []render.Segment
	for _, c := range cols {
		all = append(all, c.Segments...)
	}
	return render.ColoredString{Segments: all}
}

// emailRows renders the index rows for emails[start:end] at the given widths.
// It is the shared body of the full index and the split-view index pane.
func (m *tuiModel) emailRows(start, end, width, subjWidth int) []string {
	t := m.theme.Theme()
	rows := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		idxVal := m.emails[i].ThreadIndex
		if idxVal == 0 {
			idxVal = i + 1
		}
		ghosted := m.isPending(m.emails[i].ID)
		row := render.EmailRowStyled(m.emails[i], width, subjWidth, idxVal, render.RowStyle{
			Highlight: i == m.eIdx, Ghosted: ghosted, Tagged: m.isTagged(m.emails[i].ID),
		})
		rows = append(rows, indexZone(i, row.Render(t)))
	}
	return rows
}

func renderIndex(m *tuiModel) string {
	width := m.width
	if m.menuOpen {
		width = m.width - 19
	}
	headerWidth := m.indexHeaderWidth(width)
	subjWidth := width - headerWidth
	t := m.theme.Theme()

	numLines := m.height - 2 - m.bannerLines()
	if numLines <= 0 {
		numLines = 1
	}

	if len(m.emails) == 0 {
		return renderRainbowBannerEmptyState(width, numLines, t)
	}

	var lines []string
	if m.mode == modeDetail {
		start := max(0, m.eIdx-2)
		end := start + 5
		if end > len(m.emails) {
			end = len(m.emails)
		}
		return strings.Join(m.emailRows(start, end, width, subjWidth), "\n")
	}

	start := m.indexStart
	end := start + numLines
	if end > len(m.emails) {
		end = len(m.emails)
	}
	lines = m.emailRows(start, end, width, subjWidth)
	for len(lines) < numLines {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// The help overlay lives in tui_help.go (its tree of sections) and
// tui_help_data.go (the sections themselves). renderHelpFrame draws a help
// panel with the theme's border.

// renderHelpFrame draws a rendered help panel with the theme's border.
func renderHelpFrame(content, borderColor string, helpBg lipgloss.Color) string {
	return lipgloss.NewStyle().
		Background(helpBg).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(borderColor)).
		Padding(0, 2).
		Render(content)
}

// renderDimmedOverlay composites content (a dialog) over base (the screen
// underneath, already rendered). The base is darkened with dimLine so the
// terminal stays visible behind the dialog, and the dialog is centred on top of
// it. An empty base falls back to solid filler, which is what a caller with no
// underlying screen relies on.
func renderDimmedOverlay(width, height int, base, content string) string {
	contentLines := strings.Split(content, "\n")
	contentHeight := len(contentLines)
	if contentHeight > height {
		return content
	}

	baseLines := dimmedBaseLines(width, height, base)
	topPad := (height - contentHeight) / 2

	boxWidth := 0
	for _, l := range contentLines {
		boxWidth = max(boxWidth, lipgloss.Width(l))
	}
	left := max(0, (width-boxWidth)/2)
	for i, l := range contentLines {
		baseLines[topPad+i] = spliceDialogRow(baseLines[topPad+i], l, left, boxWidth)
	}
	return strings.Join(baseLines, "\n")
}

// spliceDialogRow places one dialog line over a dimmed base row at column
// left, keeping the base visible on both sides. A dialog line narrower than the
// widest one is padded so the right-hand base resumes at a fixed column.
func spliceDialogRow(base, line string, left, boxWidth int) string {
	line += strings.Repeat(" ", max(0, boxWidth-lipgloss.Width(line)))
	prefix := ansi.Truncate(base, left, "")
	prefix += strings.Repeat(" ", max(0, left-ansi.StringWidth(prefix)))
	return prefix + "\x1b[0m" + line + "\x1b[0m" + ansi.TruncateLeft(base, left+boxWidth, "")
}

// dimmedBaseLines returns exactly height lines of the dimmed background. Each
// source line is darkened; missing lines become a dark filler so the composite
// always fills the terminal.
func dimmedBaseLines(width, height int, base string) []string {
	filler := fmt.Sprintf("\x1b[48;2;26;26;26m%s\x1b[0m", strings.Repeat(" ", width))
	lines := make([]string, height)
	for i := range lines {
		lines[i] = filler
	}
	if base == "" {
		return lines
	}
	src := strings.Split(base, "\n")
	for i := 0; i < height && i < len(src); i++ {
		lines[i] = dimLine(src[i])
	}
	return lines
}

func renderErrorBanner(m *tuiModel) string {
	if m.err == nil {
		return ""
	}
	errStr := fmt.Sprintf("⚠️ Error: %v | Press %s to retry, Esc to dismiss", m.err, m.keyHint(groupList, "refresh"))
	if isAuthError(m.err) {
		// The error itself says to run `account login` from a terminal, which
		// is right for the CLI; here 'L' does exactly that in place, showing
		// the sign-in link and code. Say so, or nobody finds the key.
		errStr = fmt.Sprintf("🔑 %s needs to sign in again | Press L to sign in now (shows the link and code), Esc to dismiss",
			m.currentAccountLabel())
	}
	return lipgloss.NewStyle().
		Width(m.width).
		Foreground(lipgloss.Color("#ffffff")).
		Background(lipgloss.Color("#d32f2f")).
		Bold(true).
		Render(errStr)
}

func renderUndoSpamBanner(m *tuiModel) string {
	bannerText := " ↩ " + m.undoSpamBanner + " "
	return lipgloss.NewStyle().
		Width(m.width).
		Background(lipgloss.Color("#d84315")).
		Foreground(lipgloss.Color("#ffffff")).
		Bold(true).
		Render(bannerText)
}

func renderDiag(m *tuiModel) string {
	logs := app.GetDiagLogs()
	content := strings.Join(logs, "\n")

	m.diagVp.Width = max(20, m.width)
	m.diagVp.Height = max(5, m.height-1)
	m.diagVp.SetContent(content)
	m.diagVp.GotoBottom()

	// No frame, as for the folder tree: the panel is the screen.
	panel := lipgloss.NewStyle().
		Width(m.width).
		Height(m.diagVp.Height).
		Render(m.diagVp.View())

	bar := lipgloss.NewStyle().
		Width(m.width).
		Foreground(lipgloss.Color(m.theme.Theme().Get(render.ColorHighlightFg))).
		Render(" Diagnostics Panel | Ctrl+D: Close | j/k: Scroll | End: Bottom")

	return lipgloss.JoinVertical(lipgloss.Bottom, panel, bar)
}

func renderAttachmentDrawer(m *tuiModel) string {
	if len(m.attachments) == 0 {
		return ""
	}
	width := m.width
	if m.menuOpen {
		width = m.width - 19
	}
	var lines []string
	t := m.theme.Theme()

	for i, att := range m.attachments {
		kb := len(att.Data) / 1024
		kbStr := fmt.Sprintf("%d KB", kb)
		if kb > 1024 {
			kbStr = fmt.Sprintf("%.1f MB", float64(kb)/1024.0)
		}

		prefix := "  "
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(t.Get(render.ColorFg)))
		if i == m.attCursor {
			prefix = "> "
			style = style.Bold(true).Foreground(lipgloss.Color(t.Get(render.ColorHighlightFg)))
		}
		attIcon := getAttachmentIcon(att)
		lines = append(lines, prefix+attIcon+style.Render(fmt.Sprintf("%d. %s (%s)", i+1, att.Filename, kbStr)))
	}

	content := strings.Join(lines, "\n")
	// A rule above separates the drawer from the message, and one line
	// separates the list from the preview. The boxes it had ran along the
	// terminal's edges, where a border separates nothing.
	rule := lipgloss.NewStyle().Foreground(lipgloss.Color(t.Get(render.ColorBorder))).Render(strings.Repeat("─", max(1, width)))
	var drawerPanel string
	if width >= 70 {
		listW := min(36, max(26, width/3))
		leftBox := lipgloss.NewStyle().Width(listW).Render(content)
		rightBox := renderSelectedAttachmentPreview(m, width-listW-1)
		drawerPanel = rule + "\n" + lipgloss.JoinHorizontal(lipgloss.Top, leftBox, rightBox)
	} else {
		drawerPanel = rule + "\n" + lipgloss.NewStyle().Width(width).Render(content)
	}

	statusText := " F1: help  |  Enter/v: preview  |  o: open  |  s: quick save"
	if m.saveStatus != "" {
		if time.Since(m.saveStatusTime) < 4*time.Second {
			statusText = " Status: " + m.saveStatus
		}
	}

	// One row, cut rather than wrapped: wrapping pushed the drawer a row down.
	statusBar := lipgloss.NewStyle().
		Width(width).
		MaxWidth(width).
		MaxHeight(1).
		Foreground(lipgloss.Color(t.Get(render.ColorDate))).
		Render(statusText)

	return lipgloss.JoinVertical(lipgloss.Bottom, drawerPanel, statusBar)
}

func renderMenu(m *tuiModel) string {
	t := m.theme.Theme()
	borderColor := t.Get(render.ColorBorder)
	borderStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(borderColor))

	var lines []string
	lines = append(lines, " "+borderStyle.Render("┌────────────────┐"))
	for i, item := range m.menuItems {
		var line string
		if i == m.menuCursor {
			line = lipgloss.NewStyle().
				Foreground(lipgloss.Color(t.Get(render.ColorHighlightFg))).
				Bold(true).
				Render(fmt.Sprintf(" > %-12s ", item))
		} else {
			line = lipgloss.NewStyle().
				Foreground(lipgloss.Color(t.Get(render.ColorFg))).
				Render(fmt.Sprintf("   %-12s ", item))
		}
		lines = append(lines, rowZone(frameBase, i, 0, " "+borderStyle.Render("│")+line+borderStyle.Render("│")))
	}
	lines = append(lines, " "+borderStyle.Render("└────────────────┘"))
	return strings.Join(lines, "\n")
}
