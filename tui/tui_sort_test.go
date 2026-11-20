package tui

import (
	"testing"
	"time"

	"github.com/sarielhp/feral/email"
)

func TestSortMenuNavigationAndSelection(t *testing.T) {
	m := &tuiModel{
		mode: modeIndex,
		rawEmails: []email.Email{
			{ID: "1", Subject: "Zebra", EmailDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), FromRaw: "Bob <b@example.com>"},
			{ID: "2", Subject: "Apple", EmailDate: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), FromRaw: "Alice <a@example.com>"},
		},
	}
	m.rebuildVisibleEmails()

	// Press 'o' to open sort menu
	press(m, "o")
	if !m.sortMenuOpen {
		t.Fatal("expected sortMenuOpen to be true after pressing 'o'")
	}

	// Navigate down and up
	press(m, "j")
	if m.sortMenuCursor != 1 {
		t.Fatalf("expected sortMenuCursor=1, got %d", m.sortMenuCursor)
	}
	press(m, "k")
	if m.sortMenuCursor != 0 {
		t.Fatalf("expected sortMenuCursor=0, got %d", m.sortMenuCursor)
	}

	// Press 's' to sort by Subject
	press(m, "s")
	if m.sortMenuOpen {
		t.Fatal("expected sortMenuOpen to close after selecting sort option")
	}
	if m.sortOption != SortSubject {
		t.Fatalf("expected sortOption=SortSubject, got %v", m.sortOption)
	}
	if len(m.emails) != 2 || m.emails[0].Subject != "Apple" {
		t.Fatalf("expected Apple first when sorted by Subject, got %v", m.emails[0].Subject)
	}

	// O opens the same menu, and what it picks sorts descending, as in mutt.
	press(m, "O")
	if !m.sortMenuOpen || !m.sortMenuReverse {
		t.Fatal("expected the reverse sort menu to open on 'O'")
	}
	press(m, "s")
	if !m.sortReverse || m.emails[0].Subject != "Zebra" {
		t.Fatalf("reverse by Subject: reverse=%v, %v first; want Zebra first", m.sortReverse, m.emails[0].Subject)
	}

	// Choosing through o again returns to ascending.
	press(m, "o")
	press(m, "s")
	if m.sortReverse || m.emails[0].Subject != "Apple" {
		t.Fatalf("o then s: reverse=%v, %v first; want ascending, Apple first", m.sortReverse, m.emails[0].Subject)
	}
}

func TestSortMenuAllNeomuttOptions(t *testing.T) {
	emails := []email.Email{
		{
			ID:           "1",
			Subject:      "Re: Beta",
			FromRaw:      "Charlie <c@example.com>",
			ToRaw:        "Zara <z@example.com>",
			EmailDate:    time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC),
			DateReceived: time.Date(2026, 1, 11, 0, 0, 0, 0, time.UTC),
			Size:         100,
			IsSpam:       true,
		},
		{
			ID:           "2",
			Subject:      "Alpha",
			FromRaw:      "Alice <a@example.com>",
			ToRaw:        "Bob <b@example.com>",
			EmailDate:    time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC),
			DateReceived: time.Date(2026, 1, 21, 0, 0, 0, 0, time.UTC),
			Size:         500,
		},
	}
	m := &tuiModel{
		mode:      modeIndex,
		rawEmails: emails,
	}

	// Sort by From: Alice then Charlie
	press(m, "o")
	press(m, "f")
	if m.emails[0].ID != "2" {
		t.Fatalf("Sort by From failed, expected ID 2 first, got %s", m.emails[0].ID)
	}

	// Sort by To (mutt's o): Bob (ID 2) then Zara (ID 1)
	press(m, "o")
	press(m, "o")
	if m.emails[0].ID != "2" {
		t.Fatalf("Sort by To failed, expected ID 2 first, got %s", m.emails[0].ID)
	}

	// Sort by Size: smallest first, as mutt does (ID 1 = 100, ID 2 = 500)
	press(m, "o")
	press(m, "z")
	if m.emails[0].ID != "1" {
		t.Fatalf("Sort by Size failed, expected ID 1 first, got %s", m.emails[0].ID)
	}

	// Sort by Score: Good first (ID 2), Spam last (ID 1)
	press(m, "o")
	press(m, "c")
	if m.emails[0].ID != "2" {
		t.Fatalf("Sort by Score failed, expected ID 2 first, got %s", m.emails[0].ID)
	}

	// Sort by Date received: oldest first, newest at the bottom (ID 1)
	press(m, "o")
	press(m, "r")
	if m.emails[0].ID != "1" || m.emails[1].ID != "2" {
		t.Fatalf("Sort by Date received failed, expected IDs 1,2, got %s,%s", m.emails[0].ID, m.emails[1].ID)
	}

	// Sort by Threads (mutt's t)
	press(m, "o")
	press(m, "t")
	if m.sortOption != SortThreads {
		t.Fatalf("Sort by Threads failed, option=%v", m.sortOption)
	}
}

func TestRenderSortMenu(t *testing.T) {
	m, _ := setupTestSaveFolderModel(t)
	m.sortMenuOpen = true
	view := renderSortMenu(m)
	if view == "" {
		t.Fatal("renderSortMenu returned empty string")
	}
}

// Like mutt, every sort runs ascending: oldest first with the newest at the
// bottom, and the reverse toggle flips that.
func TestSortDate_AscendingByDefaultAndReversible(t *testing.T) {
	emails := []email.Email{
		{ID: "new", EmailDate: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)},
		{ID: "old", EmailDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		{ID: "mid", EmailDate: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)},
	}
	order := func(reverse bool) string {
		cp := append([]email.Email(nil), emails...)
		sortEmailsFlat(cp, SortDate, reverse)
		return string(cp[0].ID) + "," + string(cp[1].ID) + "," + string(cp[2].ID)
	}
	if got := order(false); got != "old,mid,new" {
		t.Errorf("date order = %s, want old,mid,new", got)
	}
	if got := order(true); got != "new,mid,old" {
		t.Errorf("reversed date order = %s, want new,mid,old", got)
	}
}
