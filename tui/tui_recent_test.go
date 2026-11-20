package tui

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sarielhp/feral/cache"
	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/cache/folderindex"
	"github.com/sarielhp/feral/cache/msg"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/label"
	"github.com/sarielhp/feral/mailclient"
)

// recentFixture caches three folders, A (5 messages), B (3) and C (2), whose
// message n is dated n days after a fixed day, so the newest is the last row.
func recentFixture(t *testing.T) (*tuiModel, cachedir.Account) {
	t.Helper()
	dir := t.TempDir()
	account := cachedir.At(dir)
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	for folder, count := range map[string]int{"A": 5, "B": 3, "C": 2} {
		var ids []email.ID
		for n := range count {
			id := email.ID(fmt.Sprintf("%s%d", strings.ToLower(folder), n))
			date := base.AddDate(0, 0, n)
			eml := fmt.Sprintf("From: s@example.com\nSubject: %s %d\nDate: %s\n\nbody", folder, n, date.Format(time.RFC1123Z))
			if err := msg.Store(account, id, []byte(eml), date); err != nil {
				t.Fatal(err)
			}
			ids = append(ids, id)
		}
		if err := folderindex.ReplaceAll(account, folder, ids); err != nil {
			t.Fatal(err)
		}
	}
	cfg := &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: dir}, AccountCache: account}
	m := newTuiModel(&mailclient.MockMailClient{Cfg: cfg}, "A", nil)
	m.width, m.height = 100, 30
	m.folders = []label.Item{{Name: "A", FullName: "A"}, {Name: "B", FullName: "B"}, {Name: "C", FullName: "C"}}
	m.globalFolders = m.folders
	m.globalTree = label.BuildLabelTree(m.folders)
	m.currentF = "A"
	m.loadCachedEmailsForFolder("A")
	return m, account
}

func selectRow(t *testing.T, m *tuiModel, i int) {
	t.Helper()
	if i >= len(m.emails) {
		t.Fatalf("row %d of %d", i, len(m.emails))
	}
	m.eIdx = i
	m.selectedID = m.emails[i].ID
}

func pressKey(m *tuiModel, k tea.KeyMsg) *tuiModel {
	model, _ := m.handleKey(k)
	return model.(*tuiModel)
}

func runeKey(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

// Opening a folder from the tree used to keep the old folder's row number:
// from row 0 of A, C opened on its row 0, c0, rather than its newest, and a
// larger folder on an unrelated message. It opens on the newest.
func TestOpeningAFolderFromTheTreeSelectsItsNewestMessage(t *testing.T) {
	m, _ := recentFixture(t)
	selectRow(t, m, 0)
	m = pressKey(m, runeKey("f"))
	if !m.treeOpen {
		t.Fatal("f did not open the folder tree")
	}
	m.cursor = slices.IndexFunc(m.entries, func(e label.TreeEntry) bool { return e.Node.FullName == "C" })
	if m.cursor < 0 {
		t.Fatal("folder C is not in the tree")
	}
	m = pressKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.currentF != "C" || m.selectedID != "c1" {
		t.Errorf("opened %q on %q, want C on its newest message c1", m.currentF, m.selectedID)
	}
}

// A folder reopens on the message it had selected when it was left, and the
// server's listing arriving afterwards does not move the selection.
func TestAFolderReopensOnItsRememberedMessage(t *testing.T) {
	m, _ := recentFixture(t)
	selectRow(t, m, 1)
	m.enterFolder("B")
	if m.selectedID != "b2" {
		t.Fatalf("B opened on %q, want its newest, b2", m.selectedID)
	}
	selectRow(t, m, 0)
	m.enterFolder("A")
	if m.selectedID != "a1" {
		t.Errorf("A reopened on %q, want a1, where it was left", m.selectedID)
	}
	m.updateSelectionAfterEmailsLoaded()
	if m.selectedID != "a1" {
		t.Errorf("the server listing moved the selection to %q", m.selectedID)
	}
	m.enterFolder("B")
	if m.selectedID != "b0" {
		t.Errorf("B reopened on %q, want b0", m.selectedID)
	}
}

// Positions are per account: a folder of the same name in another account
// does not open on this one's message.
func TestRememberedPositionsAreKeptPerAccount(t *testing.T) {
	m, _ := recentFixture(t)
	selectRow(t, m, 2)
	m.rememberPosition()
	other := &cfg_g.Config{AccountCache: cachedir.At(t.TempDir())}
	m.client = &mailclient.MockMailClient{Cfg: other}
	if _, ok := m.folderPositions[m.positionKey("A")]; ok {
		t.Error("another account's folder A shares this account's position")
	}
}

func TestRecentFoldersPickerGoesBackAndJumps(t *testing.T) {
	m, account := recentFixture(t)
	m.enterFolder("B")
	m.enterFolder("C")
	m = pressKey(m, tea.KeyMsg{Type: tea.KeyF8})
	if !m.recentOpen || !slices.Equal(m.recentFolders, []string{"B", "A"}) {
		t.Fatalf("picker open=%v lists %q, want B then A without the current C", m.recentOpen, m.recentFolders)
	}
	screen := renderRecentFolders(m)
	for _, want := range []string{"RECENT FOLDERS", "1  B", "2  A"} {
		if !strings.Contains(screen, want) {
			t.Errorf("picker lacks %q:\n%s", want, screen)
		}
	}
	m = pressKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.recentOpen || m.currentF != "B" {
		t.Fatalf("Enter left the picker open=%v on %q, want back in B", m.recentOpen, m.currentF)
	}
	m = pressKey(m, tea.KeyMsg{Type: tea.KeyF8})
	m = pressKey(m, runeKey("2"))
	if m.currentF != "A" {
		t.Errorf("2 opened %q, want A, the second row", m.currentF)
	}
	m = pressKey(m, tea.KeyMsg{Type: tea.KeyF8})
	m = pressKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.recentOpen || m.currentF != "A" {
		t.Errorf("Esc left the picker open=%v on %q", m.recentOpen, m.currentF)
	}
	got, err := cache.LoadRecentFolders(account)
	if err != nil || !slices.Equal(got, []string{"A", "B", "C"}) {
		t.Errorf("saved list = %q, %v; want A, B, C", got, err)
	}
}

// The list is on disk, so a new session starts with it.
func TestRecentFoldersOutliveTheSession(t *testing.T) {
	m, account := recentFixture(t)
	m.enterFolder("C")
	cfg := &cfg_g.Config{AccountCache: account}
	next := newTuiModel(&mailclient.MockMailClient{Cfg: cfg}, "B", nil)
	next.currentF = "B"
	next = pressKey(next, tea.KeyMsg{Type: tea.KeyF8})
	if !slices.Equal(next.recentFolders, []string{"C", "A"}) {
		t.Errorf("a new session lists %q, want C then A", next.recentFolders)
	}
}

func TestRecentFoldersPickerByMouseAndNotInCompose(t *testing.T) {
	m, _ := recentFixture(t)
	m.enterFolder("B")
	m.enterFolder("C")
	m.openRecentFolders()
	model, _ := m.clickOverlayRow(frameRecent, 1)
	m = model.(*tuiModel)
	if !m.recentOpen || m.recentCursor != 1 {
		t.Fatal("a first click on an unselected row must select it, not open it")
	}
	model, _ = m.clickOverlayRow(frameRecent, 1)
	m = model.(*tuiModel)
	if m.recentOpen || m.currentF != "A" {
		t.Errorf("a second click left open=%v on %q, want A", m.recentOpen, m.currentF)
	}
	m.mode = modeCompose
	m = pressKey(m, tea.KeyMsg{Type: tea.KeyF8})
	if m.recentOpen {
		t.Error("F8 in compose opened the picker, which would abandon the draft")
	}
}

// A search's results and a saved set are reopened through Ctrl+f and F7, so
// they are not recorded as folders.
func TestOnlyFoldersAreRecordedAsVisits(t *testing.T) {
	for folder, want := range map[string]bool{"INBOX": true, "": false, searchFolderPrefix + "q": false, virtualFolderPrefix + "s": false} {
		if got := recordsVisit(folder); got != want {
			t.Errorf("recordsVisit(%q) = %v, want %v", folder, got, want)
		}
	}
	if recentDigitIndex('1') != 0 || recentDigitIndex('9') != 8 || recentDigitIndex('0') != 9 || recentDigitLabel(9) != "0" {
		t.Error("digits must map 1-9 to the first nine rows and 0 to the tenth")
	}
}
