package filebrowser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"
)

func TestFileBrowser_NavigationAndSelection(t *testing.T) {
	tmpDir := t.TempDir()
	subDir1 := filepath.Join(tmpDir, "docs")
	subDir2 := filepath.Join(tmpDir, "photos")
	_ = os.MkdirAll(subDir1, 0755)
	_ = os.MkdirAll(subDir2, 0755)
	_ = os.WriteFile(filepath.Join(tmpDir, "notes.txt"), []byte("hello"), 0644)

	m := New(tmpDir, "test.pdf", 80, 24)

	if m.CurrentDir != tmpDir {
		t.Errorf("expected currentDir %s, got %s", tmpDir, m.CurrentDir)
	}
	if len(m.Filtered) < 3 {
		t.Errorf("expected at least 3 entries, got %d", len(m.Filtered))
	}

	// 1. Move cursor down
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.Cursor != 1 {
		t.Errorf("expected cursor 1, got %d", m.Cursor)
	}

	// 2. Select save directory with 's'
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if cmd == nil {
		t.Fatalf("expected cmd on 's' press")
	}
	msg := cmd()
	selMsg, ok := msg.(SelectedMsg)
	if !ok {
		t.Fatalf("expected SelectedMsg, got %T", msg)
	}
	if selMsg.Dir != tmpDir {
		t.Errorf("expected SelectedMsg.Dir %s, got %s", tmpDir, selMsg.Dir)
	}
	if selMsg.FullPath != filepath.Join(tmpDir, "test.pdf") {
		t.Errorf("expected FullPath %s, got %s", filepath.Join(tmpDir, "test.pdf"), selMsg.FullPath)
	}
}

func TestFileBrowser_FolderCreation(t *testing.T) {
	tmpDir := t.TempDir()
	m := New(tmpDir, "test.pdf", 80, 24)

	// Press 'n' to create folder
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if !m.CreatingDir {
		t.Fatalf("expected CreatingDir=true")
	}

	// Type "new_folder"
	for _, r := range "new_folder" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Press Enter to confirm
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.CreatingDir {
		t.Errorf("expected CreatingDir=false after enter")
	}

	expectedPath := filepath.Join(tmpDir, "new_folder")
	if m.CurrentDir != expectedPath {
		t.Errorf("expected currentDir %s, got %s", expectedPath, m.CurrentDir)
	}
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("expected folder %s to be created on disk", expectedPath)
	}
}

func TestFileBrowser_Cancel(t *testing.T) {
	tmpDir := t.TempDir()
	m := New(tmpDir, "test.pdf", 80, 24)

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatalf("expected cmd on Esc")
	}
	msg := cmd()
	if _, ok := msg.(canceledMsg); !ok {
		t.Fatalf("expected canceledMsg, got %T", msg)
	}
}

func TestFileBrowser_View(t *testing.T) {
	tmpDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(tmpDir, "reports"), 0755)

	m := New(tmpDir, "invoice.pdf", 80, 24)
	v := m.View()

	if !strings.Contains(v, "SELECT SAVE DIRECTORY") {
		t.Errorf("expected title in View(), got: %s", v)
	}
	if !strings.Contains(v, "invoice.pdf") {
		t.Errorf("expected target filename in View(), got: %s", v)
	}
	if !strings.Contains(v, "reports") {
		t.Errorf("expected directory 'reports' listed in View(), got: %s", v)
	}
}

func TestFileBrowser_SelectFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "attachment.png")
	_ = os.WriteFile(filePath, []byte("pngdata"), 0644)

	m := NewFileSelector(tmpDir, 80, 24)
	if !m.AllowFileSelect {
		t.Fatalf("expected AllowFileSelect=true")
	}

	// Find attachment.png in entries
	foundIdx := -1
	for i, e := range m.Filtered {
		if e.Name == "attachment.png" {
			foundIdx = i
			break
		}
	}
	if foundIdx == -1 {
		t.Fatalf("attachment.png not found in entries")
	}

	m.Cursor = foundIdx
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("expected cmd on Enter")
	}
	msg := cmd()
	selMsg, ok := msg.(SelectedMsg)
	if !ok {
		t.Fatalf("expected SelectedMsg, got %T", msg)
	}
	if selMsg.FullPath != filePath {
		t.Errorf("expected FullPath %s, got %s", filePath, selMsg.FullPath)
	}
}

func TestFileBrowser_PathNavigationAndAutocomplete(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "documents_folder")
	_ = os.MkdirAll(subDir, 0755)

	m := New(tmpDir, "file.txt", 80, 24)

	// Press ':' to enter path
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(":")})
	if !m.EnteringPath {
		t.Fatalf("expected EnteringPath=true")
	}

	// Type prefix of subDir
	prefix := filepath.Join(tmpDir, "doc")
	for _, r := range prefix {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Press Tab to autocomplete
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	expectedCompleted := subDir + "/"
	if m.PathInput != expectedCompleted {
		t.Errorf("expected autocomplete to '%s', got '%s'", expectedCompleted, m.PathInput)
	}

	// Press Enter to navigate
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.EnteringPath {
		t.Errorf("expected EnteringPath=false after Enter")
	}
	if m.CurrentDir != subDir {
		t.Errorf("expected currentDir to be '%s', got '%s'", subDir, m.CurrentDir)
	}
}

func TestFileBrowser_JumpBookmarks(t *testing.T) {
	tmpDir := t.TempDir()
	m := New(tmpDir, "file.txt", 80, 24)

	// The default jumps no longer include the system temporary directory:
	// the program keeps its own files under /tmp/<user>/feral, not in /tmp.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("4")})
	if m.CurrentDir != filepath.Clean(tmpDir) {
		t.Errorf("jump 4 moved to '%s'; no such bookmark should exist", m.CurrentDir)
	}
}

// A caller's locations replace the defaults: each digit jumps to its
// location, the footer names them, and a digit past the list does nothing.
func TestFileBrowser_CallerLocations(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	m := New(first, "file.txt", 100, 30)
	m.SetLocations([]Location{{Path: second}, {Label: "start", Path: first}, {Path: ""}, {Path: second}})
	if len(m.Locations) != 2 {
		t.Fatalf("SetLocations kept %d locations, want 2 (empty and duplicate paths dropped)", len(m.Locations))
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("1")})
	if m.CurrentDir != second {
		t.Errorf("after 1, CurrentDir = %q, want %q", m.CurrentDir, second)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	if m.CurrentDir != first {
		t.Errorf("after 2, CurrentDir = %q, want %q", m.CurrentDir, first)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	if m.CurrentDir != first {
		t.Errorf("after 3, which names no location, CurrentDir = %q, want %q unchanged", m.CurrentDir, first)
	}

	footer := zone.Scan(m.renderFooter())
	for _, want := range []string{"[1] " + displayDir(second, 28), "[2] start"} {
		if !strings.Contains(footer, want) {
			t.Errorf("footer lacks %q:\n%s", want, footer)
		}
	}
	if strings.Contains(footer, "Downloads") {
		t.Errorf("footer still shows a default jump:\n%s", footer)
	}
}

func TestFileBrowser_WindowSizeMsg(t *testing.T) {
	tmpDir := t.TempDir()
	m := New(tmpDir, "file.txt", 80, 24)

	m, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if cmd != nil {
		t.Errorf("expected nil cmd on WindowSizeMsg")
	}
	if m.Width != 120 || m.Height != 40 {
		t.Errorf("expected size (120, 40), got (%d, %d)", m.Width, m.Height)
	}

	view := m.View()
	if view == "" {
		t.Errorf("expected non-empty view after resize")
	}

	// Shrink to very small size
	m, _ = m.Update(tea.WindowSizeMsg{Width: 25, Height: 8})
	if m.Width != 25 || m.Height != 8 {
		t.Errorf("expected size (25, 8), got (%d, %d)", m.Width, m.Height)
	}
	smallView := m.View()
	if smallView == "" {
		t.Errorf("expected non-empty view on small terminal")
	}
}
