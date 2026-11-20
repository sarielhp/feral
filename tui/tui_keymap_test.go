package tui

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/keymap"
)

// The default keys, written out as the dispatchers matched them before the
// actions had names. The table must produce exactly this: a key that moved,
// vanished or appeared fails here, where the random walk might not notice.
var pinnedSharedKeys = map[string]string{
	"E": "archive", "d": "delete", "S": "spam", "U": "unspam", "u": "unspam", "B": "block",
	"N": "toggle_read", "x": "mark_all_read", "s": "save_to_folder", "w": "browser", "W": "browser",
	"c": "compose", "v": "attachments", "a": "alias_sender", "L": "lock_folder", "m": "menu",
	"|": "pipe", "ctrl+f": "server_search", "f5": "spam_menu", "f7": "saved_searches", "D": "deadlines",
	"r": "reply", "g": "reply_all", "b": "forward", "f6": "aliases", "/": "search",
	"z": "undo", "H": "snooze", "X": "unsubscribe", "M": "mute", "G": "reply_list",
}

var pinnedListKeys = map[string]string{
	"j": "down", "k": "up", "e": "open", " ": "split_preview", "o": "sort", "O": "sort_reverse",
	"~": "fuzzy_all", "!": "fuzzy_folder", "+": "load_more", "R": "refresh", "t": "tag",
	"T": "tag_pattern", ";": "tag_prefix", "ctrl+a": "tag_all", "ctrl+u": "clear_tags",
	"=": "filter_list",
}

var pinnedMessageKeys = map[string]string{
	"j": "scroll_down", "k": "scroll_up", "l": "focus_pane", "p": "hide_pane", "P": "hide_pane",
}

func pinnedKeys(own map[string]string) map[string]string {
	out := maps.Clone(pinnedSharedKeys)
	maps.Copy(out, own)
	return out
}

func TestKeymap_DefaultsArePinned(t *testing.T) {
	if err := keyTable.Check(); err != nil {
		t.Fatalf("the table is not a valid keymap: %v", err)
	}
	for group, want := range map[string]map[string]string{
		groupList: pinnedKeys(pinnedListKeys), groupMessage: pinnedKeys(pinnedMessageKeys),
	} {
		got := map[string]string{}
		for _, g := range keyTable.Groups {
			if g.Name != group {
				continue
			}
			for _, b := range g.Bindings {
				for _, k := range b.Keys {
					got[k] = b.Action
				}
			}
		}
		if !maps.Equal(got, want) {
			t.Errorf("%s: keys %v\nwant %v", group, got, want)
		}
	}
}

// Every action with a handler runs it from its group's dispatcher, and every
// inline one is in a dispatcher: pressing each default key on the screen of
// its group must reach the action, which the pinned table cannot show.
func TestKeymap_EveryActionHasAHandlerOrIsInline(t *testing.T) {
	inline := []string{actDown, actUp, actOpen, actSplit, actSort, actSortReverse,
		actScrollDown, actScrollUp, actFocusPane, actHidePane}
	for group, handlers := range keyHandlers {
		for action, run := range handlers {
			if (run == nil) != slices.Contains(inline, action) {
				t.Errorf("%s.%s: handler %v, inline %v", group, action, run != nil, slices.Contains(inline, action))
			}
		}
	}
}

// Alt chords open the menu on the list and in a message, whatever the key:
// the letters are the actions'. Alt+arrows still move, as they did.
func TestKeymap_AltOpensTheMenu(t *testing.T) {
	list := newTuiModel(nil, "", nil)
	list.kIndexActions(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("E"), Alt: true})
	if !list.menuOpen {
		t.Error("Alt+E on the list did not open the menu")
	}
	// Alt+F6 used to open the aliases in a message, and the menu on the list.
	msg := newTuiModel(nil, "", nil)
	msg.mode = modeDetail
	msg.kDetail(tea.KeyMsg{Type: tea.KeyF6, Alt: true})
	if !msg.menuOpen || msg.aliasView != nil {
		t.Errorf("Alt+F6 in a message: menu %v, aliases %v", msg.menuOpen, msg.aliasView != nil)
	}
	msg.menuOpen = false
	msg.vp.Height = 5
	msg.vp.SetContent("1\n2\n3\n4\n5\n6\n7\n8\n9\n10")
	msg.vp.GotoBottom()
	before := msg.vp.YOffset
	msg.kDetail(tea.KeyMsg{Type: tea.KeyUp, Alt: true})
	if msg.menuOpen || msg.vp.YOffset != before-1 {
		t.Errorf("Alt+↑ in a message: menu %v, offset %d from %d", msg.menuOpen, msg.vp.YOffset, before)
	}
}

func rebound(t *testing.T, file string) *keymap.Keymap {
	t.Helper()
	km, err := keymap.Parse(keyTable, "keys.json", []byte(file))
	if err != nil {
		t.Fatal(err)
	}
	return km
}

func twoMessageList(t *testing.T) *tuiModel {
	t.Helper()
	m := newTuiModel(nil, "", nil)
	m.mode = modeIndex
	m.currentF = "INBOX"
	m.width, m.height = 100, 30
	at := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	m.rawEmails = []email.Email{
		{ID: "e0", Subject: "one", MessageID: "<e0>", FromRaw: "A <a@example.com>", EmailDate: at},
		{ID: "e1", Subject: "two", MessageID: "<e1>", FromRaw: "B <b@example.com>", EmailDate: at.Add(time.Minute)},
	}
	m.rebuildVisibleEmails()
	m.eIdx, m.selectedID = 0, "e0"
	return m
}

// A rebound action runs on its new key and no longer on its old one, in its
// own group only; the spam banner names the key that undoes it.
func TestKeymap_ReboundKeysAct(t *testing.T) {
	m := twoMessageList(t)
	m.keyMap = rebound(t, `{"format": 1, "list": {"archive": "A", "unspam": "Y"}}`)
	press(m, "E")
	if len(m.emails) != 2 {
		t.Fatalf("E still archived on the list: %d rows left", len(m.emails))
	}
	press(m, "A")
	if len(m.emails) != 1 || !m.isPending("e0") {
		t.Fatalf("A did not archive: %d rows, pending %v", len(m.emails), m.isPending("e0"))
	}
	press(m, "S")
	if !strings.Contains(m.View(), "Undo spam marking by pressing 'Y'") {
		t.Error("the spam banner does not name the rebound undo key")
	}
	if a := m.keys().Action(groupMessage, "E"); a != "archive" {
		t.Errorf("E in a message runs %q: a list override moved it", a)
	}
}

// The task's own example binds archive to the key that opens a message; it
// is refused, naming both actions, rather than silently shadowing one.
func TestKeymap_ClashWithAnInlineActionIsRefused(t *testing.T) {
	_, err := keymap.Parse(keyTable, "keys.json", []byte(`{"format":1,"list":{"archive":"e"}}`))
	if err == nil || !strings.Contains(err.Error(), "list.archive") || !strings.Contains(err.Error(), "of open") {
		t.Fatalf("archive on e: %v", err)
	}
	for _, reserved := range []string{"q", "f", "?", "esc", "tab", "f1", "f9", "ctrl+c", "ctrl+d", "ctrl+q", "up", "shift+left", "f8"} {
		file := `{"message": {"archive": "` + reserved + `"}}`
		if _, err := keymap.Parse(keyTable, "keys.json", []byte(file)); err == nil {
			t.Errorf("%q was accepted though the screens take it first", reserved)
		}
	}
}

// A broken keys file stops the TUI before it starts, naming the file.
func TestKeymap_BrokenFileStopsTheTUI(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, keymap.FileName)
	if err := os.WriteFile(path, []byte(`{"list": {"archiv": "A"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := &cfg_g.Config{}
	cfg.ConfigDir = dir
	err := InitTuiWithClient(cfg, nil, "", "", nil, nil)
	if err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "list.archiv") {
		t.Fatalf("InitTuiWithClient: %v", err)
	}
	if err := InitTuiCompose(cfg, nil, ComposeOptions{}, nil, nil); err == nil {
		t.Fatal("the compose screen started with a broken keymap")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if km, err := loadKeymap(cfg); err != nil || km.Action(groupList, "E") != "archive" {
		t.Fatalf("no keys file must be the defaults: %v", err)
	}
}

// Every key a keys file may give an action reaches the action: the reserved
// list is written by hand, and a key it misses but a screen takes first --
// kGlobalKey's, a global one -- would be a binding that silently does
// nothing. Each key of the random walk is tried as archive in each group.
func TestKeymap_EveryAcceptedKeyReachesItsAction(t *testing.T) {
	accepted := 0
	for _, group := range []string{groupList, groupMessage} {
		for _, k := range fsmKeys {
			quoted, err := json.Marshal(k)
			if err != nil {
				t.Fatal(err)
			}
			km, err := keymap.Parse(keyTable, "keys.json", []byte(`{"`+group+`": {"archive": `+string(quoted)+`}}`))
			if err != nil {
				continue // refused: reserved, or another action's
			}
			accepted++
			m := twoMessageList(t)
			if group == groupMessage {
				m.mode = modeDetail
			}
			m.keyMap = km
			m.Update(fsmKey(k))
			if m.pendingEmails["e0"] != pendingArchive {
				t.Errorf("%s: archive bound to %q, but pressing it did not archive", group, k)
			}
		}
	}
	if accepted < 20 {
		t.Fatalf("only %d keys accepted: the sweep tests nothing", accepted)
	}
}

// The inline actions -- navigation, open, the split preview, sorting, the
// panes -- follow their keys too, and leave their old ones.
func TestKeymap_InlineActionsFollowTheirKeys(t *testing.T) {
	m := twoMessageList(t)
	m.keyMap = rebound(t, `{"list": {"open": "y", "up": "i", "down": "n", "split_preview": "Y", "sort": "I", "sort_reverse": "J"}}`)
	steps := []struct {
		key   string
		check func() bool
		what  string
	}{
		{"n", func() bool { return m.eIdx == 1 }, "down moves to the second row"},
		{"j", func() bool { return m.eIdx == 1 }, "j no longer moves"},
		{"i", func() bool { return m.eIdx == 0 }, "up moves back"},
		{"I", func() bool { return m.sortMenuOpen && !m.sortMenuReverse }, "sort opens the menu"},
		{"J", func() bool { return m.sortMenuOpen && m.sortMenuReverse }, "sort_reverse opens it reversed"},
		{"Y", func() bool { return m.splitPreview }, "split_preview opens the preview"},
		{"n", func() bool { return m.eIdx == 1 }, "down in the preview goes to the next message"},
		{"Y", func() bool { return !m.splitPreview }, "split_preview closes it"},
		{"e", func() bool { return m.mode == modeIndex }, "e no longer opens"},
		{"y", func() bool { return m.mode == modeDetail }, "open opens the message"},
	}
	for _, s := range steps {
		m.sortMenuOpen = false
		m.Update(fsmKey(s.key))
		if !s.check() {
			t.Fatalf("%q: %s failed", s.key, s.what)
		}
	}
}

func TestKeymap_MessageInlineActionsFollowTheirKeys(t *testing.T) {
	m := twoMessageList(t)
	m.mode = modeDetail
	m.keyMap = rebound(t, `{"message": {"scroll_up": "i", "scroll_down": "n", "focus_pane": "y", "hide_pane": "Y"}}`)
	m.vp.Height = 5
	m.vp.SetContent("1\n2\n3\n4\n5\n6\n7\n8\n9\n10")
	for _, s := range []struct {
		key  string
		want int
	}{{"n", 1}, {"j", 1}, {"i", 0}} {
		m.Update(fsmKey(s.key))
		if m.vp.YOffset != s.want {
			t.Fatalf("%q: offset %d, want %d", s.key, m.vp.YOffset, s.want)
		}
	}
	m.hideRightPane = true
	m.Update(fsmKey("p"))
	if !m.hideRightPane {
		t.Fatal("p still toggles the pane")
	}
	m.Update(fsmKey("Y"))
	if m.hideRightPane {
		t.Fatal("hide_pane did not show the pane")
	}
	m.attachments = []email.Attachment{{Filename: "i.ics", ContentType: "text/calendar"}}
	m.Update(fsmKey("l"))
	if m.rightPaneFocus {
		t.Fatal("l still focuses the pane")
	}
	m.Update(fsmKey("y"))
	if !m.rightPaneFocus {
		t.Fatal("focus_pane did not focus the pane")
	}
}

// The printed defaults load back as the defaults.
func TestKeymap_DefaultFileRoundTrips(t *testing.T) {
	km := rebound(t, string(DefaultKeysJSON()))
	for _, g := range keyTable.Groups {
		for _, b := range g.Bindings {
			if km.Changed(g.Name, b.Action) {
				t.Errorf("%s.%s changed", g.Name, b.Action)
			}
		}
	}
}
