package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/cache/folderindex"
	"github.com/sarielhp/feral/cache/msg"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/label"
	"github.com/sarielhp/feral/mailclient"
)

func typeKeys(m *tuiModel, text string) {
	for _, r := range text {
		m.Update(runeKey(string(r)))
	}
}

func searchRows(ids ...string) []email.Email {
	rows := make([]email.Email, 0, len(ids))
	for _, id := range ids {
		rows = append(rows, email.Email{ID: email.ID(id), Subject: "subject " + id})
	}
	return rows
}

// cachedModel is on INBOX (a, b) with Archive (x) also in the cache, so "~"
// has a whole cache to put on screen.
func cachedModel(t *testing.T) *tuiModel {
	t.Helper()
	account := cachedir.At(t.TempDir())
	folders := map[string][]email.ID{"INBOX": {"a", "b"}, "Archive": {"x"}}
	for folder, ids := range folders {
		for i, id := range ids {
			raw := fmt.Sprintf("Date: Mon, %d Jan 2026 10:00:00 +0000\r\nFrom: s@example.com\r\nSubject: subject %s\r\n\r\nbody", i+1, id)
			if err := msg.Store(account, id, []byte(raw), time.Now()); err != nil {
				t.Fatal(err)
			}
		}
		if err := folderindex.ReplaceAll(account, folder, ids); err != nil {
			t.Fatal(err)
		}
	}
	client := &mailclient.MockMailClient{Cfg: &cfg_g.Config{AccountCache: account}}
	m := newTuiModel(client, "INBOX", nil)
	m.width, m.height = 80, 40
	m.folders = []label.Item{{Name: "INBOX", FullName: "INBOX"}, {Name: "Archive", FullName: "Archive"}}
	m.currentF = "INBOX"
	m.rawEmails = searchRows("a", "b")
	m.rebuildVisibleEmails()
	return m
}

// Esc on the message-view prompt used to reach the global Esc first: it left
// the message with the prompt still open, and every message opened after
// showed "/" where its body should be. Letters the global keys own (q, f, h)
// were swallowed the same way.
func TestDetailSearch_EscClosesPromptAndBodyReturns(t *testing.T) {
	m := newDetailModel(t, 100, 30, "INBOX", searchRows("test-001"))
	m.Update(runeKey("/"))
	typeKeys(m, "qfh")
	if m.mode != modeDetail || m.detailSearch != "qfh" {
		t.Fatalf("typing into the prompt: mode %v, query %q", m.mode, m.detailSearch)
	}
	m.Update(escKey)
	if m.inDetailSearch || m.detailSearch != "" || m.mode != modeDetail {
		t.Fatalf("after Esc: open %v, query %q, mode %v", m.inDetailSearch, m.detailSearch, m.mode)
	}
	content := m.buildDetailContent(100)
	if !strings.Contains(content, "team meeting tomorrow") {
		t.Errorf("body missing after Esc:\n%s", content)
	}
}

// A committed filter is cleared by the first Esc; the second leaves.
func TestDetailSearch_CommittedFilterClearedBeforeLeaving(t *testing.T) {
	m := newDetailModel(t, 100, 30, "INBOX", searchRows("test-001"))
	m.Update(runeKey("/"))
	typeKeys(m, "Alice")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.inDetailSearch || m.detailSearch != "Alice" {
		t.Fatalf("after Enter: open %v, query %q", m.inDetailSearch, m.detailSearch)
	}
	m.Update(escKey)
	if m.detailSearch != "" || m.mode != modeDetail {
		t.Fatalf("first Esc: query %q, mode %v", m.detailSearch, m.mode)
	}
	m.Update(escKey)
	if m.mode != modeIndex {
		t.Fatalf("second Esc: mode %v", m.mode)
	}
}

// The filter belongs to the message it was typed on.
func TestDetailSearch_EndsWhenAnotherMessageOpens(t *testing.T) {
	m := newDetailModel(t, 100, 30, "INBOX", searchRows("test-001", "test-002"))
	m.Update(runeKey("/"))
	typeKeys(m, "zz")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.navigateDetailEmail(1)
	if m.inDetailSearch || m.detailSearch != "" {
		t.Fatalf("after opening the next message: open %v, query %q", m.inDetailSearch, m.detailSearch)
	}
}

// Leaving with q or Tab, with a committed filter, does not carry it along.
func TestDetailSearch_LeavingClearsFilter(t *testing.T) {
	for _, key := range []tea.KeyMsg{runeKey("q"), {Type: tea.KeyTab}} {
		m := newDetailModel(t, 100, 30, "INBOX", searchRows("test-001"))
		m.detailSearch = "Alice"
		m.Update(key)
		if m.mode != modeIndex || m.detailSearch != "" {
			t.Errorf("%q: mode %v, query %q", key.String(), m.mode, m.detailSearch)
		}
	}
}

// "~" puts the whole cache on screen; Esc on the committed filter has to put
// the folder's rows back, which only the prompt's own Esc used to do.
func TestListSearch_GlobalFilterRestoresFolderOnEsc(t *testing.T) {
	m := cachedModel(t)
	m.Update(runeKey("~"))
	if !m.fuzzyGlobal || len(m.rawEmails) != 3 {
		t.Fatalf("~ did not start a global filter")
	}
	m.Update(runeKey("~")) // a second ~ must not keep the cache as the folder
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.Update(escKey)
	if m.fuzzySearch || m.fuzzyGlobal || m.searchQuery != "" || m.savedRawEmails != nil {
		t.Fatalf("filter still on: fuzzy %v global %v query %q", m.fuzzySearch, m.fuzzyGlobal, m.searchQuery)
	}
	if len(m.rawEmails) != 2 || m.currentF != "INBOX" {
		t.Fatalf("folder rows not restored: %d rows in %q", len(m.rawEmails), m.currentF)
	}
}

// A folder refresh that lands under "~" refreshes the rows Esc restores, and
// a message removed under "~" does not come back with them.
func TestListSearch_GlobalFilterKeepsFolderRowsCurrent(t *testing.T) {
	m := cachedModel(t)
	m.startFuzzySearch(true)

	m.handleEmailsLoaded(emailsLoadedMsg{folder: "INBOX", emails: searchRows("a", "b", "c")})
	if len(m.rawEmails) != 3 || m.savedRawEmails == nil || len(m.savedRawEmails) != 3 {
		t.Fatalf("refresh replaced the global rows: %v", m.rawEmails)
	}
	m.removeEmailByID("a")
	m.endListFilter()
	got := make([]string, 0, len(m.rawEmails))
	for _, e := range m.rawEmails {
		got = append(got, string(e.ID))
	}
	if strings.Join(got, ",") != "b,c" {
		t.Errorf("folder rows after Esc = %v, want b,c", got)
	}
}

// A filter does not follow the user into another folder.
func TestListSearch_EndsOnFolderChange(t *testing.T) {
	m := aliasModel(t)
	m.currentF = "INBOX"
	m.rawEmails = searchRows("a")
	m.rebuildVisibleEmails()
	m.Update(runeKey("/"))
	typeKeys(m, "zz")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.enterFolder("Archive")
	if m.searchQuery != "" || m.inSearch {
		t.Errorf("filter carried into Archive: %q (typing %v)", m.searchQuery, m.inSearch)
	}
}

func TestDropLastRune(t *testing.T) {
	cases := map[string]string{"": "", "ab": "a", "שלום": "שלו", "é": ""}
	for in, want := range cases {
		if got := dropLastRune(in); got != want {
			t.Errorf("dropLastRune(%q) = %q, want %q", in, got, want)
		}
	}
}
