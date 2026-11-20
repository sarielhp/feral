package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// Every help screen is searchable, and a search shrinks what it shows to the
// rows that match.

func typeText(m *tuiModel, text string) {
	for _, r := range text {
		if r == ' ' {
			m.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
			continue
		}
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

func TestHelpQuery_MatchesEveryWordAnywhere(t *testing.T) {
	row := helpRow{"F2", "Save message (disk / label)"}
	cases := []struct {
		query string
		want  bool
	}{
		{"", true},
		{"save", true},
		{"SAVE disk", true},
		{"f2 label", true},
		{"save spam", false},
	}
	for _, tc := range cases {
		if got := (helpQuery{text: tc.query}).matches(row); got != tc.want {
			t.Errorf("%q matches = %v, want %v", tc.query, got, tc.want)
		}
	}
}

// A page search keeps the matching keys and drops the rest; the letters that
// otherwise close help (h, q) are text while typing, and Esc clears the
// search without closing help.
func TestHelp_PageSearchShrinksThePage(t *testing.T) {
	m := helpModel(t)
	press(m, "?") // the Messages page
	press(m, "/")
	if m.keyScreen() != scrHelpSearch {
		t.Fatalf("'/' did not start a search: screen %s", m.keyScreen())
	}
	typeText(m, "spam h q")
	if !m.showHelp || m.helpFind.text != "spam h q" {
		t.Fatalf("typed letters did not reach the search: open %v, query %q", m.showHelp, m.helpFind.text)
	}
	m.helpFind.text = "spam"
	view := m.View()
	if !strings.Contains(view, "Mark as spam") || strings.Contains(view, "Compose new email") {
		t.Fatalf("the page was not narrowed to the search:\n%s", view)
	}
	press(m, "enter")
	if m.helpFind.typing || m.keyScreen() != scrHelp || !strings.Contains(m.View(), "Mark as spam") {
		t.Fatal("Enter did not keep the filter and return the keys to the page")
	}
	press(m, "esc")
	if !m.showHelp || m.helpFind.active() || !strings.Contains(m.View(), "Compose new email") {
		t.Fatal("Esc did not clear the search and show the whole page")
	}
	press(m, "esc")
	if m.showHelp {
		t.Fatal("a second Esc did not close the direct page")
	}
}

// From the menu, a search covers every page, under each page's name.
func TestHelp_MenuSearchCoversEveryPage(t *testing.T) {
	m := helpModel(t)
	press(m, "?")
	press(m, "left") // the menu
	press(m, "/")
	typeText(m, "quit")
	view := m.View()
	for _, want := range []string{"General", "Quit from anywhere", "search /quit"} {
		if !strings.Contains(view, want) {
			t.Errorf("menu search lacks %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Compose new email") || strings.Contains(view, "the list: move, act, tag") {
		t.Errorf("menu search shows more than the matches:\n%s", view)
	}
	if got := m.overlayRowCount(frameHelp); got != 0 {
		t.Errorf("search results count as %d clickable menu rows", got)
	}
	press(m, "enter")
	press(m, "left")
	if !m.showHelp || m.helpSection != -1 || m.helpFind.active() {
		t.Fatal("← from a kept search did not return to the plain menu")
	}
}

func TestHelp_SearchWithNoMatchSaysSo(t *testing.T) {
	m := helpModel(t)
	press(m, "?")
	press(m, "/")
	typeText(m, "zzzz")
	if !strings.Contains(m.View(), strings.TrimSpace(noHelpMatch)) {
		t.Fatalf("an empty result is not reported:\n%s", m.View())
	}
}

// Closing help drops the search, so the next help opens whole.
func TestHelp_ReopenedHelpHasNoSearch(t *testing.T) {
	m := helpModel(t)
	press(m, "?")
	press(m, "/")
	typeText(m, "spam")
	press(m, "f1")
	if m.showHelp || m.helpFind.active() {
		t.Fatalf("F1 left help open %v or the search set %+v", m.showHelp, m.helpFind)
	}
	press(m, "?")
	if m.helpFind.active() || !strings.Contains(m.View(), "Compose new email") {
		t.Fatal("reopened help still filtered")
	}
}

// A search never pushes the overlay past the screen.
func TestHelp_SearchNeverExceedsTheScreen(t *testing.T) {
	for height := 6; height <= 45; height += 3 {
		for width := 40; width <= 120; width += 40 {
			for _, section := range []int{-1, 0} {
				m := helpModel(t)
				m.width, m.height = width, height
				m.showHelp = true
				m.helpSection = section
				m.helpFind = helpQuery{text: "e", typing: true}
				lines := strings.Split(m.View(), "\n")
				if len(lines) > height {
					t.Errorf("search on %d at %dx%d: %d lines > %d", section, width, height, len(lines), height)
				}
				helpLinesFitWidth(t, "search", width, height, lines)
			}
		}
	}
}

func TestSaveHelp_Search(t *testing.T) {
	m := helpModel(t)
	m.savingMessageToFolder = true
	m.treeOpen = true
	m.setSaveHelp(true)
	m.kSaveToFolder(key("/"))
	typeText(m, "subtree")
	view := renderSaveHelp(m)
	if !strings.Contains(view, "Expand / collapse subtree") || strings.Contains(view, "Navigate folder tree") {
		t.Fatalf("the save help was not narrowed:\n%s", view)
	}
	m.kSaveToFolder(key("esc"))
	if !m.saveHelpOpen || m.helpFind.active() {
		t.Fatal("Esc in the search should clear it and keep the help")
	}
	m.kSaveToFolder(key("x"))
	if m.saveHelpOpen {
		t.Fatal("any other key should still close the save help")
	}
}

func TestAttachmentHelp_Search(t *testing.T) {
	m := helpModel(t)
	m.attachmentDrawer = true
	m.setAttachmentHelp(true)
	m.kAttachmentHelp(key("/"))
	m.kAttachmentHelp(key("q")) // typed, not a close
	if !m.attachmentHelpOpen || m.helpFind.text != "q" {
		t.Fatalf("q did not reach the search: open %v, query %q", m.attachmentHelpOpen, m.helpFind.text)
	}
	m.helpFind.text = "calendar"
	view := renderAttachmentHelp(m)
	if !strings.Contains(view, "Google Calendar") || strings.Contains(view, "Next attachment") {
		t.Fatalf("the attachment help was not narrowed:\n%s", view)
	}
	m.kAttachmentHelp(key("enter"))
	m.kAttachmentHelp(key("esc"))
	if !m.attachmentHelpOpen || m.helpFind.active() {
		t.Fatal("Esc should clear a kept search before closing")
	}
	m.kAttachmentHelp(key("esc"))
	if m.attachmentHelpOpen {
		t.Fatal("Esc with no search should close the attachment help")
	}
}
