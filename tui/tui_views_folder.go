package tui

import (
	"fmt"
	"strings"

	"github.com/sarielhp/feral/tui/render"

	"github.com/charmbracelet/lipgloss"
)

// folderVisibleHeight is the rows the tree gets: everything between its top
// bar and its key bar.
func folderVisibleHeight(m *tuiModel) int {
	return max(1, m.height-2)
}

func renderFolderTree(m *tuiModel) string {
	if len(m.entries) == 0 {
		if m.folderSearch != "" {
			return fmt.Sprintf("(no folder matches %q)", m.folderSearch)
		}
		return "(no folders)"
	}
	vis := folderVisibleHeight(m)
	start := m.cursor - vis/2
	if start < 0 {
		start = 0
	}
	end := start + vis
	if end > len(m.entries) {
		end = len(m.entries)
		start = end - vis
		if start < 0 {
			start = 0
		}
	}
	var lines []string
	for i := start; i < end; i++ {
		lines = append(lines, treeZone(i, render.MakeLabelLine(m.entries[i], m.cursor == i, m.theme.Theme(), m.width)))
	}
	return strings.Join(lines, "\n")
}

func renderFolderOverlay(m *tuiModel, content string) string {
	// No frame: it ran along the terminal's edges, and at Width(m.width) its
	// right side fell past the last column. The top bar and the key bar mark
	// the edges instead.
	panel := lipgloss.NewStyle().
		Width(m.width).
		Height(folderVisibleHeight(m)).
		Render(content)
	barText := " F:Close  /:Search  Tab:Subtree  Enter:Select  j/k:Nav  q:Quit"
	if m.inFolderSearch {
		barText = " Type to narrow  Esc:End search  ↑/↓:Nav  Enter:Select"
	}
	bar := lipgloss.NewStyle().Width(m.width).Render(barText)
	return lipgloss.JoinVertical(lipgloss.Left, renderFolderTopBar(m), panel, bar)
}

// folderViewTitle names what the folder view is open for: browsing, or
// choosing where a message or the tagged set goes.
func (m *tuiModel) folderViewTitle() string {
	switch {
	case m.bulkMovingToFolder:
		return fmt.Sprintf(" [Move %d tagged to] ", m.countTagged())
	case m.savingMessageToFolder:
		return " [Save to folder] "
	}
	return " [Folders] "
}

// folderSearchMatches counts the listed folders whose path holds the search;
// the rest of a filtered tree are their parents, kept for the hierarchy.
func (m *tuiModel) folderSearchMatches() int {
	q := strings.ToLower(m.folderSearch)
	n := 0
	for _, e := range m.entries {
		if e.Node != nil && strings.Contains(strings.ToLower(e.Node.FullName), q) {
			n++
		}
	}
	return n
}

// renderFolderTopBar is the folder view's counterpart of the message list's
// top bar, on a background of its own so the two views are not confused: what
// the view is for, the account and the folder on screen, and the live search
// in the middle while one is typed.
func renderFolderTopBar(m *tuiModel) string {
	t := m.theme.Theme()
	bg := render.ColorFolderTopBg
	text := func(s string, fg render.ColorID) render.ColoredString { return render.Plain(s).WithFg(fg).WithBg(bg) }

	left := joinCol(text(m.folderViewTitle(), render.ColorHighlightFg), text(m.topBarAccountName()+" - "+m.currentF, render.ColorFg))
	hintText := "/: search  F8: recent  h/?: help"
	if m.inFolderSearch {
		hintText = "Esc: end search"
		if m.folderSearch != "" {
			left = joinCol(left, text(fmt.Sprintf(" | %d match(es)", m.folderSearchMatches()), render.ColorDate))
		}
	}
	right := text(hintText+" ", render.ColorHighlightFg)
	leftW, rightW := left.Width(), right.Width()
	if m.width < leftW+rightW {
		left = left.TruncateTo(max(0, m.width-rightW))
		right = right.TruncateTo(m.width)
		leftW, rightW = left.Width(), right.Width()
	}
	leftStr, rightStr := left.Render(t), right.Render(t)
	bgStyle := lipgloss.NewStyle().Background(lipgloss.Color(t.Get(bg)))
	if m.inFolderSearch {
		box := lipgloss.NewStyle().Foreground(lipgloss.Color("#cc1111")).Background(lipgloss.Color("#d8d8d8")).Bold(true)
		return m.renderPromptBox(topBarFrame{leftStr, rightStr, leftW, rightW, bgStyle}, " /"+m.folderSearch+"█ ", box)
	}
	return leftStr + bgStyle.Render(strings.Repeat(" ", max(0, m.width-leftW-rightW))) + rightStr
}

func renderDownloadingDialog(width, height int) string {
	return renderStatusDialog(width, height, " Downloading... ")
}

// The loads that change the server from the index views. Esc cannot recall
// them, so it must not hide them: the work would go on unseen.
const (
	loadUnspamming = "Unspamming..."
	loadMovingSpam = "Moving [SPAM] messages..."
)

// loadIsMutation reports whether the loading indicator is for a server change
// that Esc must not dismiss.
func (m *tuiModel) loadIsMutation() bool {
	return m.showLoad == loadUnspamming || m.showLoad == loadMovingSpam || m.showLoad == loadMovingAccounts
}

// endFetchLoad takes down a fetch's busy notice when its rows arrive. A
// change in flight keeps its own: a background mail check landing midway
// through a transfer would otherwise uncover the list it is changing.
func (m *tuiModel) endFetchLoad() {
	if !m.loadIsMutation() {
		m.showLoad = ""
	}
}

// dismissLoad handles Esc or q over the loading indicator. A read is simply
// hidden, which is the way out of a stalled fetch; a server change keeps its
// dialog and says it cannot be canceled.
func (m *tuiModel) dismissLoad() {
	if m.loadIsMutation() {
		m.loadEscNoted = true
		return
	}
	m.showLoad = ""
}

func renderMutationDialog(width, height int, message string, escNoted bool) string {
	text := " " + message + " "
	if escNoted {
		text += "\n\nThis cannot be canceled; please wait."
	}
	return renderStatusDialog(width, height, text)
}

// renderStatusDialog centres a bordered one-line status message over the screen.
func renderStatusDialog(width, height int, message string) string {
	dialog := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#4a9e6e")).
		Padding(1, 3).
		Bold(true).
		Foreground(lipgloss.Color("#ffdd44")).
		Background(lipgloss.Color("#2b2b2b")).
		Render(message)
	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		Align(lipgloss.Center, lipgloss.Center).
		Render(dialog)
}
