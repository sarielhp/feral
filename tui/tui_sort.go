package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/tui/render"
)

// SortOption specifies how messages in a folder or label are ordered,
// supporting the options provided by neomutt.
type SortOption int

// Sort options for folder message ordering.
const (
	SortThreads SortOption = iota
	SortDate
	SortDateReceived
	SortFrom
	SortTo
	SortSubject
	SortSize
	SortScore
	SortUnsorted
)

// String describes the sort option for display.
func (s SortOption) String() string {
	switch s {
	case SortThreads:
		return "Threads"
	case SortDate:
		return "Date sent"
	case SortDateReceived:
		return "Date received"
	case SortFrom:
		return "From (sender)"
	case SortTo:
		return "To (recipient)"
	case SortSubject:
		return "Subject"
	case SortSize:
		return "Size"
	case SortScore:
		return "Score / Spam"
	case SortUnsorted:
		return "Unsorted"
	default:
		return "Unknown"
	}
}

type sortMenuItem struct {
	label    string
	shortcut string
	option   SortOption
}

// sortMenuItems are mutt's sort keys: o picks the order, O picks it reversed.
func sortMenuItems() []sortMenuItem {
	return []sortMenuItem{
		{label: "Date sent (d)", shortcut: "d", option: SortDate},
		{label: "Date received (r)", shortcut: "r", option: SortDateReceived},
		{label: "From sender (f)", shortcut: "f", option: SortFrom},
		{label: "To recipient (o)", shortcut: "o", option: SortTo},
		{label: "Subject (s)", shortcut: "s", option: SortSubject},
		{label: "Size (z)", shortcut: "z", option: SortSize},
		{label: "Score / Spam (c)", shortcut: "c", option: SortScore},
		{label: "Threads (t)", shortcut: "t", option: SortThreads},
		{label: "Unsorted (u)", shortcut: "u", option: SortUnsorted},
	}
}

// openSortMenu opens the sort menu; reverse says the choice made in it sorts
// descending, as mutt's O does.
func (m *tuiModel) openSortMenu(reverse bool) (tea.Model, tea.Cmd) {
	m.sortMenuOpen = true
	m.sortMenuReverse = reverse
	m.sortMenuCursor = 0
	return m, nil
}

func (m *tuiModel) kSortMenu(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := k.String()
	items := sortMenuItems()
	switch s {
	case "up", "k":
		m.sortMenuCursor--
		if m.sortMenuCursor < 0 {
			m.sortMenuCursor = len(items) - 1
		}
		return m, nil
	case "down", "j":
		m.sortMenuCursor++
		if m.sortMenuCursor >= len(items) {
			m.sortMenuCursor = 0
		}
		return m, nil
	case "enter":
		return m.selectSortOption(items[m.sortMenuCursor].option)
	case "esc", "q":
		m.sortMenuOpen = false
		return m, nil
	case "f1":
		return m, m.showNotice("Select sort order: d, r, f, o, s, z, c, t, u")
	default:
		return m.handleSortShortcut(s)
	}
}

func (m *tuiModel) handleSortShortcut(s string) (tea.Model, tea.Cmd) {
	switch s {
	case "d":
		return m.selectSortOption(SortDate)
	case "r":
		return m.selectSortOption(SortDateReceived)
	case "f":
		return m.selectSortOption(SortFrom)
	case "o":
		return m.selectSortOption(SortTo)
	case "s":
		return m.selectSortOption(SortSubject)
	case "z":
		return m.selectSortOption(SortSize)
	case "c":
		return m.selectSortOption(SortScore)
	case "t":
		return m.selectSortOption(SortThreads)
	case "u":
		return m.selectSortOption(SortUnsorted)
	default:
		return m, nil
	}
}

func (m *tuiModel) selectSortOption(opt SortOption) (tea.Model, tea.Cmd) {
	m.sortOption = opt
	m.sortReverse = m.sortMenuReverse
	m.sortMenuOpen = false
	return m.applySort()
}

func (m *tuiModel) applySort() (tea.Model, tea.Cmd) {
	m.rebuildVisibleEmails()
	if len(m.emails) > 0 {
		m.eIdx = len(m.emails) - 1 // with nothing selected, the newest, as in mutt
		for i, em := range m.emails {
			if em.ID == m.selectedID {
				m.eIdx = i
				break
			}
		}
		m.selectedID = m.emails[m.eIdx].ID
	}
	desc := m.sortOption.String()
	if m.sortReverse {
		desc += " (reversed)"
	}
	return m, m.showNotice("Sorted by " + desc)
}

// renderSortMenu draws the neomutt sort options menu centered over the screen.
func renderSortMenu(m *tuiModel) string {
	items := sortMenuItems()
	dialogWidth := min(76, max(20, m.width-4))
	if m.width > 0 && dialogWidth > m.width {
		dialogWidth = m.width
	}
	inner := max(10, dialogWidth-4)

	var content strings.Builder
	content.WriteString("\n")
	title := "SORT MESSAGES"
	if m.sortMenuReverse {
		title = "REVERSE SORT MESSAGES"
	}
	content.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#55ffff")).
		Width(inner).Align(lipgloss.Center).Render(title))
	content.WriteString("\n\n")

	for i, item := range items {
		activeMark := "  "
		if item.option == m.sortOption {
			activeMark = "● "
		}
		lineText := fmt.Sprintf(" %s%-26s", activeMark, item.label)
		if i == m.sortMenuCursor {
			content.WriteString(lipgloss.NewStyle().
				Foreground(lipgloss.Color("#55ff55")).Bold(true).
				Render(" ▶" + lineText))
			content.WriteString("\n")
		} else {
			content.WriteString(lipgloss.NewStyle().
				Foreground(lipgloss.Color("#cccccc")).
				Render("  " + lineText))
			content.WriteString("\n")
		}
	}

	content.WriteString("\n")
	content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).
		Render(markHints("  Enter: choose    ↑/↓: navigate    Esc: close", hint{"Enter: choose", "enter"}, hint{"Esc: close", "esc"})))

	dialogBox := lipgloss.NewStyle().
		Background(lipgloss.Color("#16162a")).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#55ffff")).
		Padding(1, 2).
		Width(dialogWidth).
		Render(content.String())

	return renderDimmedOverlay(m.width, m.height, m.renderBaseScreen(), dialogBox)
}

func sortEmailsFlat(emails []email.Email, opt SortOption, reverse bool) {
	if opt == SortUnsorted {
		if reverse {
			for i, j := 0, len(emails)-1; i < j; i, j = i+1, j-1 {
				emails[i], emails[j] = emails[j], emails[i]
			}
		}
		return
	}
	sort.SliceStable(emails, func(i, j int) bool {
		cmp := compareEmails(emails[i], emails[j], opt)
		if reverse {
			return cmp > 0
		}
		return cmp < 0
	})
}

func compareEmails(a, b email.Email, opt SortOption) int {
	switch opt {
	case SortDate:
		return compareDates(a.EmailDate, b.EmailDate)
	case SortDateReceived:
		d1, d2 := a.DateReceived, b.DateReceived
		if d1.IsZero() {
			d1 = a.EmailDate
		}
		if d2.IsZero() {
			d2 = b.EmailDate
		}
		return compareDates(d1, d2)
	case SortFrom:
		fA := strings.ToLower(email.FormatSender(a.FromRaw))
		fB := strings.ToLower(email.FormatSender(b.FromRaw))
		return strings.Compare(fA, fB)
	case SortTo:
		tA := strings.ToLower(email.FormatRecipient(a.ToRaw))
		tB := strings.ToLower(email.FormatRecipient(b.ToRaw))
		return strings.Compare(tA, tB)
	case SortSubject:
		sA := strings.ToLower(email.StripSubjectPrefix(a.Subject))
		sB := strings.ToLower(email.StripSubjectPrefix(b.Subject))
		return strings.Compare(sA, sB)
	case SortSize:
		if a.Size == b.Size {
			return 0
		}
		if a.Size < b.Size {
			return -1 // smallest first, as mutt does
		}
		return 1
	case SortScore:
		badA := a.IsSpam || a.IsPolitical || a.IsBlacklisted
		badB := b.IsSpam || b.IsPolitical || b.IsBlacklisted
		if badA != badB {
			if badA {
				return 1 // bad mail goes after good mail
			}
			return -1
		}
		return compareDates(a.EmailDate, b.EmailDate)
	default:
		return 0
	}
}

func compareDates(d1, d2 time.Time) int {
	if d1.Equal(d2) {
		return 0
	}
	if d1.Before(d2) {
		return -1 // oldest first, newest at the bottom, as mutt does
	}
	return 1
}

// indexHeaderWidth is the number of cells an index row width cells wide
// spends before its subject.
func (m *tuiModel) indexHeaderWidth(width int) int {
	return render.IndexHeaderWidth(width)
}
