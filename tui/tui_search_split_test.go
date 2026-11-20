package tui

import (
	"github.com/sarielhp/feral/email"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func createTestEmailsForSearchSplit() []email.Email {
	t0 := time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)
	return []email.Email{
		{ID: "msg-1", Subject: "Important Invoice", FromRaw: "Billing <billing@corp.com>", FromEmail: "billing@corp.com", EmailDate: t0},
		{ID: "msg-2", Subject: "Lunch meeting tomorrow", FromRaw: "Alice <alice@corp.com>", FromEmail: "alice@corp.com", EmailDate: t0.Add(time.Hour)},
		{ID: "msg-3", Subject: "Weekly Newsletter", FromRaw: "News <news@updates.com>", FromEmail: "news@updates.com", EmailDate: t0.Add(2 * time.Hour)},
		{ID: "msg-4", Subject: "Second invoice reminder", FromRaw: "Billing <billing@corp.com>", FromEmail: "billing@corp.com", EmailDate: t0.Add(3 * time.Hour)},
	}
}

func TestTopBar_SearchRendering(t *testing.T) {
	m := newTuiModel(nil, "INBOX", nil)
	m.width = 100
	m.height = 30
	m.rawEmails = createTestEmailsForSearchSplit()
	m.rebuildVisibleEmails()

	// Initial State: no search
	topBarNormal := renderTopBar(m)
	if strings.Contains(topBarNormal, "/invoice") {
		t.Errorf("expected top bar not to contain search query, got: %s", topBarNormal)
	}

	// Active search typing
	m.inSearch = true
	m.searchQuery = "invoice"
	topBarSearching := renderTopBar(m)
	if !strings.Contains(topBarSearching, "/invoice█") {
		t.Errorf("expected top bar to contain active search box with cursor, got: %s", topBarSearching)
	}

	// Filter active but typing finished (inSearch = false)
	m.inSearch = false
	topBarFiltered := renderTopBar(m)
	if !strings.Contains(topBarFiltered, "/invoice") {
		t.Errorf("expected top bar to contain search filter box, got: %s", topBarFiltered)
	}
}

func TestSearch_TabTransitionsToFilteredMailbox(t *testing.T) {
	m := newTuiModel(nil, "INBOX", nil)
	m.width = 100
	m.height = 30
	m.rawEmails = createTestEmailsForSearchSplit()
	m.rebuildVisibleEmails()

	// Open search with '/'
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = res.(*tuiModel)
	if !m.inSearch {
		t.Fatal("expected inSearch=true after '/'")
	}

	// Type 'invoice'
	for _, r := range "invoice" {
		res, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = res.(*tuiModel)
	}
	if m.searchQuery != "invoice" {
		t.Fatalf("expected searchQuery='invoice', got %q", m.searchQuery)
	}
	if len(m.emails) != 2 {
		t.Fatalf("expected 2 filtered emails for 'invoice', got %d", len(m.emails))
	}

	// Press Tab to move back to mailbox
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = res.(*tuiModel)
	if m.inSearch {
		t.Errorf("expected inSearch=false after Tab")
	}
	if len(m.emails) != 2 {
		t.Errorf("expected filtered list of 2 emails to be preserved, got %d", len(m.emails))
	}
	if m.emails[0].Subject != "Important Invoice" || m.emails[1].Subject != "Second invoice reminder" {
		t.Errorf("unexpected filtered emails: %+v", m.emails)
	}

	// Navigate filtered mailbox with down arrow / j
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = res.(*tuiModel)
	if m.eIdx != 1 {
		t.Errorf("expected eIdx=1 after KeyDown, got %d", m.eIdx)
	}

	// Navigate up with k
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = res.(*tuiModel)
	if m.eIdx != 0 {
		t.Errorf("expected eIdx=0 after KeyUp, got %d", m.eIdx)
	}

	// Press Tab when out of search box to jump back into search box
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = res.(*tuiModel)
	if !m.inSearch {
		t.Errorf("expected inSearch=true after Tab when search box exists")
	}

	// Press Tab again to jump back out to mailbox
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = res.(*tuiModel)
	if m.inSearch {
		t.Errorf("expected inSearch=false after second Tab")
	}

	// Press Esc to clear filter
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = res.(*tuiModel)
	if m.searchQuery != "" {
		t.Errorf("expected searchQuery to be cleared after Esc, got %q", m.searchQuery)
	}
	if len(m.emails) != 4 {
		t.Errorf("expected full list of 4 emails restored, got %d", len(m.emails))
	}
}

func TestSplitPreview_ToggleAndNavigation(t *testing.T) {
	m := newTuiModel(nil, "INBOX", nil)
	m.width = 100
	m.height = 30
	m.rawEmails = createTestEmailsForSearchSplit()
	m.rebuildVisibleEmails()

	if m.splitPreview {
		t.Fatal("expected splitPreview=false initially")
	}

	// Press Space to toggle split preview
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = res.(*tuiModel)
	if !m.splitPreview {
		t.Fatal("expected splitPreview=true after Space")
	}

	// Set sample detail content
	m.detailH = "From: Billing <billing@corp.com>\nSubject: Important Invoice"
	m.detail = "Line 1: Invoice attached\nLine 2: Due in 30 days\nLine 3: Thank you\nLine 4: Support contact"
	m.detailVpDirty = true

	view := m.View()
	if !strings.Contains(view, "[Preview:") {
		t.Errorf("expected view to contain Preview divider, got:\n%s", view)
	}
	if !strings.Contains(view, "Invoice attached") {
		t.Errorf("expected view to contain preview message content, got:\n%s", view)
	}

	// Test Up/Down arrows scroll the preview viewport
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = res.(*tuiModel)
	if m.eIdx != 0 {
		t.Errorf("expected selected email in index to stay at eIdx=0 when pressing down in split mode, got %d", m.eIdx)
	}

	// Test Left/Right arrows navigate to next/prev email
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = res.(*tuiModel)
	if m.eIdx != 1 {
		t.Errorf("expected eIdx=1 after KeyRight in split mode, got %d", m.eIdx)
	}

	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m = res.(*tuiModel)
	if m.eIdx != 0 {
		t.Errorf("expected eIdx=0 after KeyLeft in split mode, got %d", m.eIdx)
	}

	// Press Space again to close split preview
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = res.(*tuiModel)
	if m.splitPreview {
		t.Errorf("expected splitPreview=false after second Space")
	}
}

func TestSearch_MatchesCCList(t *testing.T) {
	m := newTuiModel(nil, "INBOX", nil)
	m.rawEmails = []email.Email{
		{
			ID:        "msg-1",
			Subject:   "Budget proposal",
			FromRaw:   "Alice <alice@corp.com>",
			FromEmail: "alice@corp.com",
			ToRaw:     "Bob <bob@corp.com>",
			ToEmail:   "bob@corp.com",
			CcRaw:     "Charlie Brown <charlie@corp.com>, Dave <dave@corp.com>",
			CcEmail:   "charlie@corp.com",
		},
		{
			ID:        "msg-2",
			Subject:   "General announcement",
			FromRaw:   "HR <hr@corp.com>",
			FromEmail: "hr@corp.com",
		},
	}
	m.rebuildVisibleEmails()

	// Search for Charlie by name
	m.searchQuery = "charlie"
	m.rebuildVisibleEmails()
	if len(m.emails) != 1 || m.emails[0].ID != "msg-1" {
		t.Fatalf("expected 1 match for CC name 'charlie', got %d", len(m.emails))
	}

	// Search for Dave by email
	m.searchQuery = "dave@corp.com"
	m.rebuildVisibleEmails()
	if len(m.emails) != 1 || m.emails[0].ID != "msg-1" {
		t.Fatalf("expected 1 match for CC email 'dave@corp.com', got %d", len(m.emails))
	}
}

func TestSearch_MatchesThreadSenderAndCCList(t *testing.T) {
	t0 := time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)
	m := newTuiModel(nil, "INBOX", nil)
	m.sortOption = SortThreads

	// Thread with root from Alice, reply from Bob with Dave in CC
	root := email.Email{
		ID:        "e0",
		MessageID: "<e0>",
		Subject:   "Architecture Review",
		FromRaw:   "Alice Smith <alice@corp.com>",
		FromEmail: "alice@corp.com",
		EmailDate: t0,
	}
	reply := email.Email{
		ID:        "e1",
		MessageID: "<e1>",
		InReplyTo: "<e0>",
		Subject:   "Re: Architecture Review",
		FromRaw:   "Bob Dylan <bob@corp.com>",
		FromEmail: "bob@corp.com",
		CcRaw:     "Dave Clark <dave@corp.com>",
		CcEmail:   "dave@corp.com",
		EmailDate: t0.Add(time.Hour),
	}
	unrelated := email.Email{
		ID:        "e2",
		MessageID: "<e2>",
		Subject:   "Coffee break",
		FromRaw:   "Frank <frank@corp.com>",
		FromEmail: "frank@corp.com",
		EmailDate: t0.Add(2 * time.Hour),
	}

	m.rawEmails = []email.Email{root, reply, unrelated}
	m.rebuildVisibleEmails()

	// Initial check: thread root is collapsed and shows both senders
	if len(m.emails) != 2 {
		t.Fatalf("expected 2 roots, got %d", len(m.emails))
	}
	if !strings.Contains(m.emails[0].ThreadSenderSummary, "Alice") || !strings.Contains(m.emails[0].ThreadSenderSummary, "Bob") {
		t.Fatalf("expected ThreadSenderSummary to contain Alice and Bob, got %q", m.emails[0].ThreadSenderSummary)
	}

	// Search for second sender in thread: 'Bob'
	m.searchQuery = "Bob"
	m.rebuildVisibleEmails()
	if len(m.emails) != 1 || m.emails[0].ID != "e0" {
		t.Fatalf("expected 1 match for sender list 'Bob', got %d", len(m.emails))
	}

	// Search for CC recipient in thread reply: 'Dave'
	m.searchQuery = "dave"
	m.rebuildVisibleEmails()
	if len(m.emails) != 1 || m.emails[0].ID != "e0" {
		t.Fatalf("expected 1 match for CC in thread 'dave', got %d", len(m.emails))
	}
}
