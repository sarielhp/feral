package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The help overlay is a tree: the screen opens the section that describes it,
// the section menu is one step back, and no page is ever taller than its
// screen — every rung of every screen size.

func helpModel(t *testing.T) *tuiModel {
	t.Helper()
	m := newTuiModel(nil, "", nil)
	m.width, m.height = 80, 40
	return m
}

func TestHelp_OpensTheScreenOwnSection(t *testing.T) {
	cases := []struct {
		name  string
		mode  int
		open  func(*tuiModel)
		turns string
	}{
		{"compose", modeCompose, func(m *tuiModel) {
			m.mode = modeCompose
			m.compose = &composeState{}
		}, "Compose"},
		{"detail", modeDetail, func(m *tuiModel) { m.mode = modeDetail }, "Reading"},
		{"index", modeIndex, func(*tuiModel) {}, "Messages"},
	}
	for _, tc := range cases {
		m := helpModel(t)
		tc.open(m)
		model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
		m = model.(*tuiModel)
		if !m.showHelp {
			t.Fatalf("%s: '?' did not open help", tc.name)
		}
		view := m.View()
		if !strings.Contains(view, "⚡ "+tc.turns+" — keybindings") {
			t.Errorf("%s: help opened the wrong page:\n%s", tc.name, view)
		}
	}
}

// Esc closes a page its screen opened; ← steps back to the section menu,
// and Enter on a menu row — or its number — opens the page again.
func TestHelp_MenuNavigation(t *testing.T) {
	m := helpModel(t)
	press(m, "?") // the index screen opens the Messages page
	if m.helpDirect != true {
		t.Fatal("the contextual page is not marked as direct")
	}
	press(m, "left")
	if m.showHelp != true || m.helpSection != -1 {
		t.Fatalf("page did not step back to the menu: help %v section %v", m.showHelp, m.helpSection)
	}
	if !strings.Contains(m.View(), "HELP — topics") {
		t.Fatal("the section menu is not drawn")
	}
	press(m, "enter") // cursor starts on Messages
	if m.helpSection != sectionIndex("Messages") {
		t.Fatalf("menu opened section %d", m.helpSection)
	}
	press(m, "left")
	press(m, "down")
	press(m, "enter")
	if m.helpSection != sectionIndex("Reading") {
		t.Fatalf("cursor move + open landed on %d", m.helpSection)
	}

	// A page opened through the menu is not direct: Esc steps back out of it.
	m.helpScroll = 0
	press(m, "esc")
	if m.helpSection != -1 {
		t.Fatalf("esc on an indirect page went to %d, want the menu", m.helpSection)
	}
	press(m, "3") // digits jump from the menu
	if m.helpSection != sectionIndex("Compose") {
		t.Fatalf("digit jumped to %d", m.helpSection)
	}
	press(m, "esc") // direct, opened through the menu path? no — indirect
	if m.showHelp != true {
		t.Fatal("esc closed an indirect page; it should step back")
	}
	press(m, "q") // q closes from the menu
	if m.showHelp {
		t.Fatal("q did not close help")
	}
}

func TestHelp_HandFCloseAnywhere(t *testing.T) {
	m := helpModel(t)
	m.mode = modeDetail
	press(m, "h")
	if !m.showHelp {
		t.Fatal("h did not open help")
	}
	press(m, "h")
	if m.showHelp {
		t.Fatal("h did not close help from its page")
	}
	press(m, "h")
	press(m, "esc") // direct page: esc closes
	if m.showHelp {
		t.Fatal("esc did not close the direct page")
	}
}

// The overlay never exceeds the screen, at any of the tree's rungs.
func TestHelp_NeverExceedsTheScreen(t *testing.T) {
	for height := 6; height <= 60; height += 3 {
		for width := 40; width <= 120; width += 20 {
			sweepHelpSizes(t, width, height)
		}
	}
}

// sweepHelpSizes checks one terminal size: every section page and the menu.
func sweepHelpSizes(t *testing.T, width, height int) {
	t.Helper()
	for i := range helpSections {
		m := helpModel(t)
		m.width, m.height = width, height
		m.showHelp = true
		m.helpSection = i
		lines := strings.Split(m.View(), "\n")
		if name := helpSections[i].name; len(lines) > height {
			t.Errorf("%s at %dx%d: %d lines > %d", name, width, height, len(lines), height)
		}
		helpLinesFitWidth(t, helpSections[i].name, width, height, lines)
	}
	m := helpModel(t)
	m.width, m.height = width, height
	m.showHelp = true
	m.helpSection = -1
	if lines := strings.Split(m.View(), "\n"); len(lines) > height {
		t.Errorf("menu at %dx%d: %d lines > %d", width, height, len(lines), height)
	}
}

func helpLinesFitWidth(t *testing.T, name string, width, height int, lines []string) {
	t.Helper()
	for _, line := range lines {
		if w := lipgloss.Width(line); w > width {
			t.Errorf("%s at %dx%d: line width %d > %d", name, width, height, w, width)
			break
		}
	}
}

// Scrolling reaches every key of a page taller than its screen: laid over the
// whole scroll range, the windows cover every row, so no key is unreachable.
func TestHelp_ScrollReachesEveryKey(t *testing.T) {
	m := helpModel(t)
	m.width, m.height = 80, 14
	press(m, "?") // Messages page, taller than the screen
	sec := helpSections[sectionIndex("Messages")]
	bound := helpScrollBound(m)
	if bound < 1 {
		t.Fatalf("no scroll bound on a tall page: bound %d, %d keys", bound, len(sec.rows))
	}
	body := helpBody(m, sectionIndex("Messages"), helpQuery{})
	rows := m.helpBodyRows()
	for _, row := range sec.rows {
		reached := false
		for s := 0; s <= bound && !reached; s++ {
			for _, line := range helpWindowLines(body, s, rows) {
				if strings.Contains(line, row.desc) {
					reached = true
				}
			}
		}
		if !reached {
			t.Errorf("scrolling never shows %q", row.desc)
		}
	}
}

// Every screen's contextual page names the keys the user came from: the
// alias browser is reachable from the index, the reader and compose, so all
// three pages name F6.
func TestHelp_DocumentsAliasesEverywhere(t *testing.T) {
	for _, mode := range []int{modeIndex, modeDetail, modeCompose} {
		m := helpModel(t)
		m.mode = mode
		if mode == modeCompose {
			m.compose = &composeState{}
		}
		helpMsg := renderHelp(m)
		if !strings.Contains(helpMsg, "F6") {
			t.Errorf("mode %d help does not name F6", mode)
		}
	}
}

// The contact section names the new-alias flow's keys.
func TestHelp_DocumentsTheAliasBrowser(t *testing.T) {
	m := helpModel(t)
	m.showHelp = true
	m.helpSection = sectionIndex("Contacts")
	helpMsg := renderHelp(m)
	for _, want := range []string{"F5", "F6", "Ctrl+n"} {
		if !strings.Contains(helpMsg, want) {
			t.Errorf("contacts help page lost %q:\n%s", want, helpMsg)
		}
	}
}
