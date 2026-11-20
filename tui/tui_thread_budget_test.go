package tui

import (
	"fmt"
	"testing"
	"time"

	"github.com/sarielhp/feral/email"

	tea "github.com/charmbracelet/bubbletea"
)

// threadFixture builds a thread from parent relations: each entry names a
// message and the message it replies to ("" for a root). Messages are dated
// in the order given, so a listing shows them in that order, and every root
// has its own subject, so only the reply headers thread them.
func threadFixture(replies [][2]string) []email.Email {
	base := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	emails := make([]email.Email, 0, len(replies))
	for i, r := range replies {
		e := email.Email{ID: email.ID(r[0]), Subject: "Thread " + r[0], MessageID: email.HeaderID("<" + r[0] + ">"),
			FromRaw: "A <a@b.com>", EmailDate: base.Add(time.Duration(i) * time.Minute)}
		if r[1] != "" {
			e.InReplyTo = "<" + r[1] + ">"
		}
		emails = append(emails, e)
	}
	return emails
}

// wideThread is a root with n direct replies, each with k replies of its own.
func wideThread(n, k int) [][2]string {
	replies := [][2]string{{"root", ""}}
	for i := 0; i < n; i++ {
		child := fmt.Sprintf("c%d", i)
		replies = append(replies, [2]string{child, "root"})
		for j := 0; j < k; j++ {
			replies = append(replies, [2]string{fmt.Sprintf("%s-%d", child, j), child})
		}
	}
	return replies
}

func tabOn(t *testing.T, m *tuiModel, idx int) *tuiModel {
	t.Helper()
	m.eIdx = idx
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	return res.(*tuiModel)
}

// Tab opens replies breadth-first, nearest first, and stops before the
// message whose replies would show more than threadExpandBudget rows: the
// root's three replies and the first reply's children fit (3 + 4), the
// second reply's children would not (3 + 4 + 4).
func TestThread_TabOpensBreadthFirstWithinBudget(t *testing.T) {
	m := newTuiModel(nil, "INBOX", nil)
	m.height = 40
	m.rawEmails = threadFixture(wideThread(3, 4))
	m.rebuildVisibleEmails()

	m = tabOn(t, m, 0)
	if got := len(m.emails); got != 1+3+4 {
		t.Fatalf("after one Tab %d rows are visible, want %d (root, three replies, the first opened)", got, 1+3+4)
	}
	if m.emails[1].ID != "c0" || m.emails[1].ThreadCollapsed || m.emails[2].ID != "c0-0" {
		t.Errorf("rows 1-2 are %s (collapsed=%v), %s; want c0 opened with c0-0 under it", m.emails[1].ID, m.emails[1].ThreadCollapsed, m.emails[2].ID)
	}
	last := m.emails[len(m.emails)-1]
	if last.ID != "c2" || !last.ThreadCollapsed {
		t.Errorf("the last row is %s (collapsed=%v), want c2 still folded", last.ID, last.ThreadCollapsed)
	}

	// The next Tab cannot open c1 within the budget either, so it falls back
	// to opening the whole next level; then everything is open and the Tab
	// after folds the thread.
	m = tabOn(t, m, 0)
	if got := len(m.emails); got != 1+3+12 {
		t.Fatalf("after two Tabs %d rows are visible, want %d", got, 1+3+12)
	}
	m = tabOn(t, m, 0)
	if got := len(m.emails); got != 1 {
		t.Fatalf("after three Tabs %d rows are visible, want the folded root alone", got)
	}
}

// When even the first message to open would exceed the budget, Tab falls
// back to opening one level across the subtree, so a thread with many direct
// replies still unfolds.
func TestThread_TabFallsBackToOneLevelWhenBudgetTooSmall(t *testing.T) {
	m := newTuiModel(nil, "INBOX", nil)
	m.height = 40
	m.rawEmails = threadFixture(wideThread(12, 1))
	m.rebuildVisibleEmails()

	m = tabOn(t, m, 0)
	if got := len(m.emails); got != 1+12 {
		t.Fatalf("after one Tab %d rows are visible, want the root and its 12 replies", got)
	}
	for _, e := range m.emails[1:] {
		if !e.ThreadCollapsed {
			t.Errorf("%s opened on the fallback step; only the root's level should", e.ID)
		}
	}
}

// Opening a thread scrolls the list so the opened rows are on screen, with
// the current message moved up to the top line when they need the whole
// page, and never scrolls the current message off the top.
func TestThread_TabRevealsOpenedRows(t *testing.T) {
	m := newTuiModel(nil, "INBOX", nil)
	m.height = 8 // six list rows
	replies := [][2]string{{"a", ""}, {"b", ""}, {"c", ""}}
	replies = append(replies, wideThread(2, 2)...)
	replies = append(replies, [2]string{"z", ""})
	m.rawEmails = threadFixture(replies)
	m.rebuildVisibleEmails()
	rootIdx := 3
	if m.emails[rootIdx].ID != "root" {
		t.Fatalf("row %d is %s, want root", rootIdx, m.emails[rootIdx].ID)
	}

	m = tabOn(t, m, rootIdx)
	if got := len(m.emails); got != 5+6 {
		t.Fatalf("%d rows visible, want %d", got, 5+6)
	}
	if m.indexStart != rootIdx {
		t.Errorf("indexStart = %d, want %d: the root should be the top line when its subtree fills the page", m.indexStart, rootIdx)
	}

	// A subtree that fits below the current row only scrolls as far as needed.
	m.indexStart = 0
	m.height = 20            // eighteen rows: root at 3 plus six below fit without scrolling
	m = tabOn(t, m, rootIdx) // folds
	m = tabOn(t, m, rootIdx) // opens again
	if m.indexStart != 0 {
		t.Errorf("indexStart = %d, want 0: everything already fits", m.indexStart)
	}
}
