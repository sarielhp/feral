package tui

import (
	"errors"
	"github.com/sarielhp/feral/cache/cachedir"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"

	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/mailclient"
)

func setupTestSpamMenuModel(t *testing.T, readOnly bool) (*tuiModel, *mailclient.MockMailClient) {
	t.Helper()
	tempDir := t.TempDir()
	cfg := &cfg_g.Config{
		Paths: cfg_g.Paths{CacheRoot: tempDir}, AccountCache: cachedir.At(tempDir),
		ReadOnly: readOnly,
	}
	mock := &mailclient.MockMailClient{Cfg: cfg, InboxFolderVal: "INBOX"}
	m := newTuiModel(mock, "INBOX", nil)
	m.cfg = cfg
	m.width = 80
	m.height = 24
	m.rawEmails = []email.Email{
		{ID: "msg-1", Subject: "First", EmailDate: time.Now()},
		{ID: "msg-2", Subject: "Second", EmailDate: time.Now()},
	}
	m.rebuildVisibleEmails()
	m.eIdx = 0
	m.selectedID = "msg-1"
	return m, mock
}

// F5 opens the dialog; Enter on the first entry learns the current message as
// spam and gives it the [SPAM] badge in place -- it is not moved.
func TestSpamMenu_SpamCurrentLearnsWithoutMoving(t *testing.T) {
	m, mock := setupTestSpamMenuModel(t, false)

	press(m, "f5")
	if !m.spamMenuOpen {
		t.Fatal("expected spamMenuOpen=true after F5")
	}
	if m.spamMenuCursor != 0 {
		t.Fatalf("expected cursor at first entry, got %d", m.spamMenuCursor)
	}

	press(m, "enter")
	if m.spamMenuOpen {
		t.Error("expected dialog to close after Enter")
	}
	if !emailIsSpam(m, "msg-1") {
		t.Error("expected msg-1 to carry the [SPAM] badge")
	}
	if len(m.emails) != 2 {
		t.Errorf("learning must not remove the message, emails=%d", len(m.emails))
	}
	if len(mock.MovedEmails) != 0 {
		t.Errorf("learning must not move the message, moves=%d", len(mock.MovedEmails))
	}
}

func TestSpamMenu_NavigationWraps(t *testing.T) {
	m, _ := setupTestSpamMenuModel(t, false)
	press(m, "f5")

	press(m, "k") // up from first wraps to last
	if m.spamMenuCursor != len(spamMenuItems())-1 {
		t.Errorf("up from first should wrap to last, got %d", m.spamMenuCursor)
	}
	press(m, "j") // down from last wraps to first
	if m.spamMenuCursor != 0 {
		t.Errorf("down from last should wrap to first, got %d", m.spamMenuCursor)
	}
	press(m, "esc")
	if m.spamMenuOpen {
		t.Error("expected dialog to close on Esc")
	}
}

// "Move all [SPAM]" targets every message in the folder carrying the [SPAM]
// badge, and the accepted moves drop those messages from the index.
func TestSpamMenu_FlushMovesAllSpamBadged(t *testing.T) {
	m, _ := setupTestSpamMenuModel(t, false)
	m.rawEmails[0].IsSpam = true
	m.rawEmails[1].IsSpam = true
	if got := m.spamCount(); got != 2 {
		t.Fatalf("spamCount=%d, want 2", got)
	}

	model, _ := m.handleMarkedSpamDone(markedSpamDoneMsg{ids: m.spamIDs()})
	mm := model.(*tuiModel)
	if len(mm.emails) != 0 {
		t.Errorf("moved messages should be removed, emails=%d", len(mm.emails))
	}
}

// A message learned as spam is the one a later flush acts on: spamCurrent
// marks it [SPAM], and spamIDs then includes it without any session marking.
func TestSpamMenu_LearnedMessageIsFlushed(t *testing.T) {
	m, _ := setupTestSpamMenuModel(t, false)
	if m.spamCount() != 0 {
		t.Fatalf("nothing should be [SPAM] initially, got %d", m.spamCount())
	}

	press(m, "f5")
	press(m, "enter") // "Spam current message" on msg-1
	if got := m.spamCount(); got != 1 {
		t.Fatalf("spamCount=%d after learning one, want 1", got)
	}
	if ids := m.spamIDs(); len(ids) != 1 || ids[0] != "msg-1" {
		t.Errorf("spamIDs=%v, want [msg-1]", ids)
	}
}

// emailIsSpam reports whether the message with id carries the [SPAM] badge.
func emailIsSpam(m *tuiModel, id email.ID) bool {
	for _, e := range m.rawEmails {
		if e.ID == id {
			return e.IsSpam
		}
	}
	return false
}

// The F5 dialog must use the terminal width, not a narrow fixed box. At 120
// columns it should be wider than the old 46-column constant.
func TestSpamMenu_DialogUsesTerminalWidth(t *testing.T) {
	m, _ := setupTestSpamMenuModel(t, false)
	m.width = 120
	m.spamMenuOpen = true

	menu := stripANSI(renderSpamMenu(m))
	maxW := 0
	for _, line := range strings.Split(menu, "\n") {
		if w := runewidth.StringWidth(line); w > maxW {
			maxW = w
		}
	}
	if maxW <= 46 {
		t.Errorf("dialog width %d did not grow with the terminal", maxW)
	}
	if maxW > m.width {
		t.Errorf("dialog width %d exceeds terminal width %d", maxW, m.width)
	}
}

// The dialog is composited over the dimmed screen, so the message list stays
// visible behind it rather than the modal replacing the whole terminal.
func TestSpamMenu_ShowsDimmedBackground(t *testing.T) {
	m, _ := setupTestSpamMenuModel(t, false)
	m.width = 100
	m.height = 30
	m.spamMenuOpen = true

	out := renderSpamMenu(m)
	if !strings.Contains(stripANSI(out), "First") {
		t.Errorf("expected the message list behind the dialog, got:\n%s", out)
	}
	if n := len(strings.Split(out, "\n")); n != m.height {
		t.Errorf("composite should fill %d lines, got %d", m.height, n)
	}
}

// dimLine darkens truecolor runs wherever they appear in an SGR sequence. The
// render package packs foreground and background into one sequence, so a
// strictly five-parameter parser would miss every coloured row.
func TestDimLine_DarkensTruecolorRuns(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"\x1b[38;2;255;255;255mx\x1b[0m", "\x1b[38;2;107;107;107mx\x1b[0m"},
		{"\x1b[48;2;204;34;34my\x1b[0m", "\x1b[48;2;85;14;14my\x1b[0m"},
		{"\x1b[38;2;255;221;68;48;2;204;34;34mz\x1b[0m", "\x1b[38;2;107;92;28;48;2;85;14;14mz\x1b[0m"},
		{"\x1b[1;38;2;255;255;255mb\x1b[0m", "\x1b[1;38;2;107;107;107mb\x1b[0m"},
		{"\x1b[38;5;196mkeep\x1b[0m", "\x1b[38;5;196mkeep\x1b[0m"},
		{"plain", "plain"},
	}
	for _, c := range cases {
		if got := dimLine(c.in); got != c.want {
			t.Errorf("dimLine(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSpamMenu_ReadOnlyRefusesSpamCurrent(t *testing.T) {
	m, _ := setupTestSpamMenuModel(t, true)
	press(m, "f5")
	press(m, "enter")

	if !m.showError || m.err == nil || !strings.Contains(m.err.Error(), "Read-Only Mode") {
		t.Fatalf("expected read-only refusal, got err=%v showError=%v", m.err, m.showError)
	}
	if emailIsSpam(m, "msg-1") {
		t.Error("read-only must not mark the message as spam")
	}
}

// Help page: every description must start at one column per layout column,
// computed from display width, not byte length. The arrow glyphs (↓/↑/←/→) in
// the keys are three bytes each, so a byte-length column would leave the rows
// that carry them with less padding and the descriptions ragged.
func TestRenderHelp_DescsAlignByDisplayWidth(t *testing.T) {
	m := newTuiModel(nil, "", nil)
	m.width = 100
	m.height = 40

	lines := strings.Split(renderHelp(m), "\n")
	sec := helpSections[sectionIndex("Messages")]

	// Literal columns, not the layout formula restated: at 100 columns the
	// Messages page is two columns, its widest key 12 cells wide, and the
	// descriptions start 4 cells past the key column inside a 3-cell frame
	// edge: 3+4+12 on the left, and 3+46+4+12 on the right, after a 46-cell
	// column. If the layout changes, these numbers are the thing to re-derive.
	two := true
	wantLeft, wantRight := 19, 65

	half := (len(sec.rows) + 1) / 2
	for i, row := range sec.rows {
		got := -1
		for _, line := range lines {
			if col := displayColumn(stripANSI(line), row.desc); col >= 0 {
				got = col
				break
			}
		}
		if got < 0 {
			t.Errorf("%q missing from the help page", row.desc)
			continue
		}
		want := wantLeft
		if two && i >= half {
			want = wantRight
		}
		if got != want {
			t.Errorf("%q starts at display column %d, want %d (left %d, right %d)", row.desc, got, want, wantLeft, wantRight)
		}
	}
}

// displayColumn returns the terminal display column at which entry begins in
// plain (ANSI already stripped), or -1 when absent. strings.Index would return
// the byte offset, which is a different number wherever an earlier glyph —
// e.g. the 3-byte arrow runs ↓ ↑ ← → — occupies its own display columns.
func displayColumn(plain, entry string) int {
	byteIdx := strings.Index(plain, entry)
	if byteIdx < 0 {
		return -1
	}
	return runewidth.StringWidth(plain[:byteIdx])
}

// stripANSI removes escape sequences so display columns can be measured. It
// used to assume every one ended in m, and a zone marker (ESC [ N z) made it
// read on to the next m.
func stripANSI(s string) string {
	return ansi.Strip(s)
}

// helpColumnsAlign reports whether every description on the rendered help
// page starts at one display column per layout column. Rows whose keys carry
// a multibyte arrow are the ones a byte-length column broke.
func helpColumnsAlign(m *tuiModel) bool {
	lines := strings.Split(renderHelp(m), "\n")
	sec := helpSections[sectionIndex("Messages")]

	keyW := 0
	for _, row := range sec.rows {
		keyW = max(keyW, runewidth.StringWidth(row.keys))
	}
	inner := helpInnerWidth(m)
	two := inner >= 80 && len(sec.rows) >= 12
	colW := inner
	if two {
		colW = inner/2 - 1
	}
	pad := 3 // the frame's border and its side padding
	wantLeft, wantRight := pad+4+keyW, pad+4+keyW
	if two {
		wantRight = pad + colW + 4 + keyW
	}
	half := (len(sec.rows) + 1) / 2

	for i, row := range sec.rows {
		got := -1
		for _, line := range lines {
			if idx := displayColumn(stripANSI(line), row.desc); idx >= 0 {
				got = idx
				break
			}
		}
		if got < 0 {
			return false // the page did not show every row
		}
		want := wantLeft
		if two && i >= half {
			want = wantRight
		}
		if got != want {
			return false
		}
	}
	return true
}

func TestDimmedOverlayKeepsBackdropAroundDialog(t *testing.T) {
	base := strings.Repeat("\x1b[38;2;200;200;200m"+strings.Repeat("x", 40)+"\x1b[0m\n", 5)
	lines := strings.Split(renderDimmedOverlay(40, 5, base, "[BOX]"), "\n")
	row := ansi.Strip(lines[2])
	if want := strings.Repeat("x", 17) + "[BOX]" + strings.Repeat("x", 18); row != want {
		t.Fatalf("dialog row = %q, want %q", row, want)
	}
	if !strings.Contains(lines[2], "38;2;84;84;84") {
		t.Fatalf("backdrop beside dialog is not dimmed: %q", lines[2])
	}
}

func TestHandleSpamClassifiedFailureRollsBackBadge(t *testing.T) {
	m := &tuiModel{
		rawEmails: []email.Email{{ID: "a", IsSpam: true}},
		emails:    []email.Email{{ID: "a", IsSpam: true}},
	}
	m.handleSpamClassified(spamClassifiedMsg{id: "a", err: errors.New("boom")})
	if m.rawEmails[0].IsSpam || m.emails[0].IsSpam || !m.showError || m.err == nil {
		t.Fatalf("badge not rolled back or error not surfaced: %+v err=%v", m.rawEmails[0], m.err)
	}
}

func TestHandleMarkedSpamDoneKeepsFailedMessages(t *testing.T) {
	m := &tuiModel{
		rawEmails: []email.Email{{ID: "a", IsSpam: true}, {ID: "b", IsSpam: true}},
		emails:    []email.Email{{ID: "a", IsSpam: true}, {ID: "b", IsSpam: true}},
	}
	m.handleMarkedSpamDone(markedSpamDoneMsg{ids: []email.ID{"a", "b"}, failed: map[email.ID]error{"b": errors.New("boom")}})
	if len(m.rawEmails) != 1 || m.rawEmails[0].ID != "b" {
		t.Fatalf("failed message must stay listed, got %+v", m.rawEmails)
	}
	if !strings.Contains(m.saveStatus, "1 of 2") || !m.showError {
		t.Fatalf("partial failure not reported: %q err=%v", m.saveStatus, m.err)
	}
}
