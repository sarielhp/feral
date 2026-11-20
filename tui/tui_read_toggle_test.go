package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/mailclient"
)

// readToggleFixture is a writable one-account session with one read and two
// unread messages in INBOX.
func readToggleFixture(t *testing.T) *tuiModel {
	t.Helper()
	cfg := &cfg_g.Config{
		Accounts:     []cfg_acc.AccountConfig{{Name: "Work", Type: "jmap"}},
		Paths:        cfg_g.Paths{CacheRoot: t.TempDir()},
		AccountCache: cachedir.At(t.TempDir()),
	}
	m := newTuiModel(&mailclient.MockMailClient{Cfg: cfg}, "INBOX", nil)
	m.emails = []email.Email{
		{ID: "m1", Subject: "Unread one"},
		{ID: "m2", Subject: "Read one", IsRead: true},
		{ID: "m3", Subject: "Unread two"},
	}
	m.eIdx = 0
	m.selectedID = "m1"
	m.isRead = map[email.ID]bool{"m2": true}
	return m
}

// N follows neomutt's toggle-new: the current message flips between read and
// unread, and the change reaches the server.
func TestToggleReadKeyFlipsCurrentMessage(t *testing.T) {
	m := readToggleFixture(t)

	mMod, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'N'}})
	tm := mMod.(*tuiModel)
	if !tm.emails[0].IsRead || !tm.isRead["m1"] {
		t.Errorf("N on an unread message must mark it read: emails[0]=%v isRead=%v", tm.emails[0].IsRead, tm.isRead["m1"])
	}
	if cmd == nil {
		t.Fatal("marking read must dispatch a server command")
	}
	if got := tm.isRead["m2"]; !got {
		t.Error("N must leave the other messages alone")
	}

	mMod, cmd = tm.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'N'}})
	tm = mMod.(*tuiModel)
	if tm.emails[0].IsRead || tm.isRead["m1"] {
		t.Error("N on a read message must mark it unread")
	}
	if cmd == nil {
		t.Fatal("marking unread must dispatch a server command")
	}
}

// x marks the whole view read, but only sends the messages that were unread.
func TestMarkFolderReadKeySendsOnlyUnread(t *testing.T) {
	m := readToggleFixture(t)

	mMod, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	tm := mMod.(*tuiModel)
	for i, em := range tm.emails {
		if !em.IsRead {
			t.Errorf("emails[%d] (%s) not marked read by x", i, em.ID)
		}
	}
	if cmd == nil {
		t.Fatal("x must dispatch a server command")
	}
	done := cmd().(bulkMarkReadDoneMsg)
	if len(done.ids) != 2 {
		t.Errorf("x must send only the two unread ids, got %d: %v", len(done.ids), done.ids)
	}
	if !done.read {
		t.Error("x must report the read direction")
	}
}

// x on an all-read view says so and asks no server to redo the work.
func TestMarkFolderReadKeyOnReadViewIsNoop(t *testing.T) {
	m := readToggleFixture(t)
	for i := range m.emails {
		m.emails[i].IsRead = true
	}

	mMod, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	tm := mMod.(*tuiModel)
	if cmd != nil {
		t.Error("x on an all-read view must not dispatch anything")
	}
	if !strings.Contains(tm.saveStatus, "already read") {
		t.Errorf("x on an all-read view must say so, got %q", tm.saveStatus)
	}
}

// A read-only session must not pretend to change read state.
func TestReadToggleRefusedInReadOnlyMode(t *testing.T) {
	m := readToggleFixture(t)
	m.client.Config().ReadOnly = true

	for _, key := range []rune{'N', 'x'} {
		mMod, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{key}})
		tm := mMod.(*tuiModel)
		if cmd != nil {
			t.Errorf("%q must not dispatch in read-only mode", key)
		}
		if tm.err == nil || !strings.Contains(tm.err.Error(), "Read-Only Mode") {
			t.Errorf("%q must report the read-only refusal, got %v", key, tm.err)
		}
	}
}
