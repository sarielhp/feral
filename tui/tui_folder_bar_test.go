package tui

import (
	"strings"
	"testing"

	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/label"
)

// folderBarModel is a session with a small folder hierarchy, the folder tree
// open and the index behind it.
func folderBarModel(t *testing.T) *tuiModel {
	t.Helper()
	names := []string{"INBOX", "Archive", "Archive/2025", "Archive/2026", "Archive/2026/Taxes", "Teaching",
		"Teaching/CS473-2026-spring", "Receipts 2026"}
	var items []label.Item
	for _, n := range names {
		items = append(items, label.Item{Name: n, FullName: n})
	}
	m := newTuiModel(nil, "", nil)
	m.width, m.height = 90, 12
	m.folders, m.globalFolders = items, items
	m.tree, m.globalTree = label.BuildLabelTree(items), label.BuildLabelTree(items)
	m.currentF = "INBOX"
	m.emails = []email.Email{{ID: "x", Subject: "s"}}
	m = pressKey(m, runeKey("f"))
	if !m.treeOpen {
		t.Fatal("f did not open the folder tree")
	}
	return m
}

// The folder view has a top bar like the message list's, on its own colour so
// the two are not confused, and the live folder search is typed into it.
func TestFolderViewTopBarCarriesTheSearch(t *testing.T) {
	m := folderBarModel(t)
	screen := m.View()
	lines := strings.Split(email.StripEscapeSequences(screen), "\n")
	if len(lines) != m.height {
		t.Fatalf("folder view is %d lines, want the screen's %d", len(lines), m.height)
	}
	if !strings.Contains(lines[0], "[Folders] default - INBOX") || !strings.Contains(lines[0], "/: search") {
		t.Errorf("top bar = %q", lines[0])
	}
	folderBg := "48;2;30;90;58"
	firstLine := strings.SplitN(screen, "\n", 2)[0]
	if !strings.Contains(firstLine, folderBg) || strings.Contains(firstLine, "48;5;4m") {
		t.Errorf("top bar is not on the folder view's own background: %q", firstLine)
	}
	if !strings.Contains(lines[1], "INBOX") || !strings.Contains(lines[len(lines)-1], "/:Search") {
		t.Errorf("tree must start under the bar and the key bar stay last:\n%s", strings.Join(lines, "\n"))
	}

	m = pressKey(m, runeKey("/"))
	for _, r := range "2026" {
		m = pressKey(m, runeKey(string(r)))
	}
	lines = strings.Split(email.StripEscapeSequences(m.View()), "\n")
	for _, want := range []string{"/2026█", "4 match(es)", "Esc: end search"} {
		if !strings.Contains(lines[0], want) {
			t.Errorf("top bar while searching lacks %q: %q", want, lines[0])
		}
	}
	body := strings.Join(lines[1:len(lines)-1], "\n")
	for _, want := range []string{"Archive", "2026", "Taxes", "Receipts 2026", "CS473-2026-spring"} {
		if !strings.Contains(body, want) {
			t.Errorf("filtered tree lacks %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "2025") {
		t.Errorf("filtered tree keeps a folder that does not match:\n%s", body)
	}
}

func TestFolderViewTitleNamesItsPurpose(t *testing.T) {
	m := folderBarModel(t)
	m.savingMessageToFolder = true
	if got := m.folderViewTitle(); got != " [Save to folder] " {
		t.Errorf("saving title = %q", got)
	}
	m.savingMessageToFolder, m.bulkMovingToFolder = false, true
	if got := m.folderViewTitle(); !strings.HasPrefix(got, " [Move ") || !strings.HasSuffix(got, " tagged to] ") {
		t.Errorf("bulk move title = %q", got)
	}
}
