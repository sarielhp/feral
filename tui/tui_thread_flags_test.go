package tui

import (
	"testing"
	"time"

	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/mailclient"

	tea "github.com/charmbracelet/bubbletea"
)

// flagChainFixture is a three-message chain e0 <- e1 <- e2 and a lone e3, all
// read except the ones named in unread.
func flagChainFixture(unread ...email.ID) []email.Email {
	base := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	rows := []email.Email{
		{ID: "e0", Subject: "Root", MessageID: "<e0>", EmailDate: base},
		{ID: "e1", Subject: "Re: Root", MessageID: "<e1>", InReplyTo: "<e0>", EmailDate: base.Add(time.Minute)},
		{ID: "e2", Subject: "Re: Root", MessageID: "<e2>", InReplyTo: "<e1>", EmailDate: base.Add(2 * time.Minute)},
		{ID: "e3", Subject: "Other", MessageID: "<e3>", EmailDate: base.Add(3 * time.Minute)},
	}
	for i := range rows {
		rows[i].IsRead = true
		for _, id := range unread {
			if rows[i].ID == id {
				rows[i].IsRead = false
			}
		}
	}
	return rows
}

func visibleRow(t *testing.T, m *tuiModel, id email.ID) email.Email {
	t.Helper()
	for _, e := range m.emails {
		if e.ID == id {
			return e
		}
	}
	t.Fatalf("%s is not visible", id)
	return email.Email{}
}

// A collapsed thread whose hidden messages include an unread one says so on
// the row that stands for it (the n flag); once expanded, the unread message
// shows its own N and the root no longer claims it.
func TestThread_CollapsedRootShowsHiddenUnread(t *testing.T) {
	m := newTuiModel(nil, "INBOX", nil)
	m.rawEmails = flagChainFixture("e2")
	m.rebuildVisibleEmails()

	if root := visibleRow(t, m, "e0"); !root.ThreadCollapsed || !root.ThreadHiddenUnread {
		t.Fatalf("collapsed root: collapsed=%v hiddenUnread=%v, want both", root.ThreadCollapsed, root.ThreadHiddenUnread)
	}
	if lone := visibleRow(t, m, "e3"); lone.ThreadHiddenUnread {
		t.Error("a message with no replies hides nothing")
	}

	m.eIdx = 0
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = res.(*tuiModel)
	if root := visibleRow(t, m, "e0"); root.ThreadHiddenUnread {
		t.Error("an expanded root hides nothing, so it must not show n")
	}
	if e2 := visibleRow(t, m, "e2"); e2.IsRead {
		t.Error("the unread reply must show its own N once visible")
	}
}

// A thread whose messages are all read shows no n.
func TestThread_AllReadShowsNoHiddenUnread(t *testing.T) {
	m := newTuiModel(nil, "INBOX", nil)
	m.rawEmails = flagChainFixture()
	m.rebuildVisibleEmails()
	if root := visibleRow(t, m, "e0"); root.ThreadHiddenUnread {
		t.Error("no unread message in the thread, yet the root shows n")
	}
}

// Reading the unread reply clears the root's n on the next rebuild: the read
// mark reaches the rows the tree is built from.
func TestThread_ReadingHiddenReplyClearsMarker(t *testing.T) {
	m := newTuiModel(nil, "INBOX", nil)
	m.rawEmails = flagChainFixture("e2")
	m.isRead["e2"] = true // what opening it records
	m.rebuildVisibleEmails()
	if root := visibleRow(t, m, "e0"); root.ThreadHiddenUnread {
		t.Error("e2 was read in this session, yet the root still shows n")
	}
}

// A message marked read stays read when the list is rebuilt -- by a delete, a
// sort, a search or a thread toggle. The mark used to reach only the visible
// row, and every rebuild copied the stale unread row back from rawEmails.
func TestReadMark_SurvivesListRebuild(t *testing.T) {
	client := &mailclient.MockMailClient{Cfg: &cfg_g.Config{
		Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir()),
	}}
	m := newTuiModel(client, "INBOX", nil)
	m.rawEmails = flagChainFixture("e3")
	m.rebuildVisibleEmails()
	for i := range m.emails {
		if m.emails[i].ID == "e3" {
			m.eIdx = i
		}
	}

	m.toggleReadCurrentEmail()
	if !visibleRow(t, m, "e3").IsRead {
		t.Fatal("toggle did not mark the row read")
	}
	m.rebuildVisibleEmails()
	if !visibleRow(t, m, "e3").IsRead {
		t.Error("the read mark was lost when the list was rebuilt")
	}

	m.toggleReadCurrentEmail()
	m.rebuildVisibleEmails()
	if visibleRow(t, m, "e3").IsRead {
		t.Error("the unread mark was lost when the list was rebuilt")
	}
}

// The unified inbox's maps are built from its rows, each carrying its own
// account's state, so a rebuild never lays another account's state over them.
func TestRowFlags_FromRows(t *testing.T) {
	rows := []email.Email{
		{ID: "a1", IsRead: true},
		{ID: "b1", IsReplied: true},
	}
	isRead, isReplied := rowFlags(rows)
	if !isRead["a1"] || isRead["b1"] || isReplied["a1"] || !isReplied["b1"] {
		t.Errorf("rowFlags: read=%v replied=%v", isRead, isReplied)
	}
	if _, ok := isRead["b1"]; !ok {
		t.Error("an unread row must be in the map, or a stale entry could not be overridden")
	}

	m := newTuiModel(nil, "INBOX", nil)
	m.isRead = map[email.ID]bool{"a1": false} // stale: the active account's older state
	m.rawEmails = rows
	m.isRead, m.isReplied = rowFlags(rows)
	m.rebuildVisibleEmails()
	if !visibleRow(t, m, "a1").IsRead {
		t.Error("the stale map overrode the fresh row")
	}
}

// A unified-inbox load merges its rows' state into the session's maps rather
// than replacing them: the per-folder cached loader reads the same maps, and a
// replacement left every message outside the inboxes unread on the way back.
func TestUnifiedLoad_KeepsOtherFoldersFlags(t *testing.T) {
	m := newTuiModel(nil, UnifiedInboxFolder, nil)
	m.isRead = map[email.ID]bool{"archived": true, "a1": false}
	m.isReplied = map[email.ID]bool{"archived": true}

	rows := []email.Email{{ID: "a1", IsRead: true}}
	isRead, isReplied := rowFlags(rows)
	res, _ := m.Update(emailsLoadedMsg{
		folder: UnifiedInboxFolder, emails: rows,
		isRead: isRead, isReplied: isReplied, mergeFlags: true,
	})
	m = res.(*tuiModel)

	if !m.isRead["archived"] || !m.isReplied["archived"] {
		t.Error("a message outside the unified rows lost its read or replied state")
	}
	if !m.isRead["a1"] {
		t.Error("the unified row's fresh state did not reach the session's map")
	}
	if !visibleRow(t, m, "a1").IsRead {
		t.Error("the unified row is not shown read")
	}
}
