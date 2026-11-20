package tui

import (
	"strings"
	"testing"

	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/cache/virtual"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/mailclient"
)

// virtualBrowserFixture is a session whose account cache holds two saved result
// sets -- `last`, and one named after a query.
func virtualBrowserFixture(t *testing.T) *tuiModel {
	t.Helper()
	acc := cachedir.At(t.TempDir())
	for _, name := range []string{"last", "from:bob@example.com"} {
		if err := virtual.Save(acc, &virtual.Mailbox{
			Name:        name,
			Description: "Results for " + name,
			MessageIDs:  []email.ID{email.ID("m-" + name)},
		}); err != nil {
			t.Fatalf("Save(%q): %v", name, err)
		}
	}
	mock := &mailclient.MockMailClient{
		Cfg: &cfg_g.Config{SelectedAccount: "Work", AccountCache: acc},
		RawMessages: map[email.ID][]byte{
			"m-last":                 []byte("From: Alice <alice@example.com>\r\nSubject: recent\r\n\r\nbody\r\n"),
			"m-from:bob@example.com": []byte("From: Bob <bob@example.com>\r\nSubject: from bob\r\n\r\nbody\r\n"),
		},
	}
	m := newTuiModel(mock, "INBOX", nil)
	m.width, m.height = 100, 40
	return m
}

func TestVirtualBrowserListsSavedSearches(t *testing.T) {
	m := virtualBrowserFixture(t)
	press(m, "f7")
	if !m.virtualBrowserOpen {
		t.Fatal("F7 must open the saved-searches browser")
	}
	if len(m.virtualMailboxes) != 2 {
		t.Fatalf("listed %d saved searches, want 2: %+v", len(m.virtualMailboxes), m.virtualMailboxes)
	}
	if got := m.virtualMailboxes[0].Name; got != "from:bob@example.com" {
		t.Errorf("first listed name = %q, want the name-sorted first", got)
	}
	if !strings.Contains(m.View(), "SAVED SEARCHES") {
		t.Error("the browser must be drawn")
	}
	press(m, "esc")
	if m.virtualBrowserOpen {
		t.Error("Esc must close the browser")
	}
}

// Enter opens the selected set into the message list, reading it from the cache
// rather than querying a server folder that does not exist.
func TestVirtualBrowserOpensAMailboxAsTheMessageList(t *testing.T) {
	m := virtualBrowserFixture(t)
	press(m, "f7")

	cmd := press(m, "enter")
	if cmd == nil {
		t.Fatal("Enter must return a command")
	}
	m.Update(cmd())
	if m.virtualBrowserOpen {
		t.Error("opening a saved search must close the browser")
	}
	if want := virtualFolderPrefix + "from:bob@example.com"; m.currentF != want {
		t.Errorf("currentF = %q, want %q", m.currentF, want)
	}
	if !isVirtualView(m.currentF) {
		t.Error("the opened set must be recognised as a virtual view")
	}
	if len(m.emails) != 1 || m.emails[0].Subject != "from bob" {
		t.Errorf("emails = %+v, want the one saved message", m.emails)
	}
}

func TestVirtualBrowserDeletesASavedSearch(t *testing.T) {
	m := virtualBrowserFixture(t)
	press(m, "f7")

	press(m, "d")
	if len(m.virtualMailboxes) != 1 {
		t.Fatalf("delete left %d saved searches, want 1", len(m.virtualMailboxes))
	}
	if _, err := virtual.Load(m.client.Config().AccountCache, "from:bob@example.com"); err == nil {
		t.Error("the deleted saved search is still on disk")
	}
}

// A saved set names the folder of each of its rows, which is what makes it
// actionable: the refusal a remote search keeps must not apply to it.
func TestSavedSetViewIsNotRefused(t *testing.T) {
	m, mock := savedSetFixture(t)
	openSavedSet(t, m, "quarterly")

	if _, cmd := m.archiveCurrentEmail(); cmd == nil {
		t.Fatalf("archiving from a saved set must run: %v", m.err)
	}
	if len(mock.MovedEmails) != 0 {
		t.Error("the row is ghosted and the move deferred, so nothing is moved yet")
	}

	m.taggedIDs = map[email.ID]bool{"m2": true}
	if _, cmd := m.bulkDeleteTagged(); cmd == nil {
		t.Fatalf("a bulk delete from a saved set must run: %v", m.err)
	}

	m.currentF = "INBOX"
	if m.refuseInSearchView() {
		t.Error("a real folder must not be refused")
	}
}

// R in a saved-search view re-reads the set from the cache: there is no folder
// of that name to ask a server for.
func TestRefreshInAVirtualViewReReadsTheSet(t *testing.T) {
	m := virtualBrowserFixture(t)
	m.currentF = virtualFolderPrefix + "last"
	m.emails = nil

	cmd := press(m, "R")
	if cmd == nil {
		t.Fatal("R in a saved-search view must reload the set")
	}
	m.Update(cmd())
	if m.currentF != virtualFolderPrefix+"last" {
		t.Errorf("currentF = %q, want the view left as it was", m.currentF)
	}
	if len(m.emails) != 1 || m.emails[0].Subject != "recent" {
		t.Errorf("emails = %+v, want the set's one message", m.emails)
	}
}

// A remote search view is not a folder: R never asks a server for the
// "%search:" pseudo-folder but re-runs the query as a search, and + loads
// more only when the last run filled its limit, otherwise saying that
// everything is shown.
func TestRefreshInARemoteSearchViewSearchesAndCompleteSetLoadsNoMore(t *testing.T) {
	m := virtualBrowserFixture(t)
	m.currentF = searchFolderPrefix + "q"

	if cmd := press(m, "+"); cmd != nil {
		t.Error("+ must not search again when every match is already shown")
	}
	if m.err != nil || !strings.Contains(m.saveStatus, "are shown") {
		t.Errorf("err = %v, status %q; want the all-shown status", m.err, m.saveStatus)
	}
	cmd := press(m, "R")
	if cmd == nil {
		t.Fatal("R in a search view must re-run the query")
	}
	cmd()
	mock := m.client.(*mailclient.MockMailClient)
	if mock.LastSearchQuery != "q" || mock.LastSearchLimit != remoteSearchPageSize {
		t.Errorf("R searched %q with limit %d, want q with %d", mock.LastSearchQuery, mock.LastSearchLimit, remoteSearchPageSize)
	}
}

// savedSetFixture is a session whose account cache holds one saved set, and
// whose two rows came from two different folders of the one account that owns
// it -- which is what an action on the set has to tell apart.
func savedSetFixture(t *testing.T) (*tuiModel, *mailclient.MockMailClient) {
	t.Helper()
	acc := cachedir.At(t.TempDir())
	if err := virtual.Save(acc, &virtual.Mailbox{
		Name:        "quarterly",
		Description: "Tagged set of 2 message(s)",
		MessageIDs:  []email.ID{"m1", "m2"},
		FolderMap:   map[email.ID]string{"m1": "INBOX", "m2": "Archive"},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	mock := &mailclient.MockMailClient{
		Cfg:            &cfg_g.Config{SelectedAccount: "Work", AccountCache: acc},
		InboxFolderVal: "INBOX",
		RawMessages: map[email.ID][]byte{
			"m1": []byte("From: Alice <alice@example.com>\r\nSubject: one\r\n\r\nbody\r\n"),
			"m2": []byte("From: Bob <bob@example.com>\r\nSubject: two\r\n\r\nbody\r\n"),
		},
	}
	bk := &Backend{Folders: stubFolders{ArchiveFn: func(mailclient.MailClient) (string, error) {
		return "Archive", nil
	}}}
	m := newTuiModel(mock, "INBOX", bk)
	m.width, m.height = 100, 40
	return m, mock
}

// openSavedSet opens the named saved set into the message list the way F7 does,
// which is also what records it as the view's set.
func openSavedSet(t *testing.T, m *tuiModel, name string) {
	t.Helper()
	press(m, "f7")
	if cmd := press(m, "enter"); cmd != nil {
		m.Update(cmd())
	}
	if want := virtualFolderPrefix + name; m.currentF != want {
		t.Fatalf("currentF = %q, want %q", m.currentF, want)
	}
}

// The folder on screen is the set's name, not a folder a message can be moved
// out of: each row's own folder comes from the set.
func TestSavedSetNamesEachRowsFolder(t *testing.T) {
	m, _ := savedSetFixture(t)
	openSavedSet(t, m, "quarterly")

	ids := []email.ID{"m1", "m2"}
	plan := m.planAction(ids, m.currentF)
	batches, err := m.batches(plan, ids)
	if err != nil {
		t.Fatalf("batches: %v", err)
	}
	if len(batches) != 2 {
		t.Fatalf("got %d batch(es), want one per source folder", len(batches))
	}
	got := map[email.ID]string{}
	for _, b := range batches {
		for _, id := range b.ids {
			got[id] = b.folder
		}
	}
	if got["m1"] != "INBOX" || got["m2"] != "Archive" {
		t.Errorf("source folders = %v, want INBOX for m1 and Archive for m2", got)
	}
	if cl, folder, err := m.target(plan, "m2"); err != nil || cl == nil || folder != "Archive" {
		t.Errorf("target(m2) = (%v, %q, %v), want the active client and Archive", cl, folder, err)
	}
}

// Archiving from a saved set moves each message out of its own folder, so the
// servers see the source folder the set recorded rather than the set's name.
func TestBulkArchiveFromASavedSetUsesEachRowsFolder(t *testing.T) {
	m, mock := savedSetFixture(t)
	openSavedSet(t, m, "quarterly")
	m.taggedIDs = map[email.ID]bool{"m1": true, "m2": true}

	_, cmd := m.bulkArchiveTagged()
	if cmd == nil {
		t.Fatal("expected a command")
	}
	if _, ok := cmd().(bulkArchiveDoneMsg); !ok {
		t.Fatal("expected bulkArchiveDoneMsg")
	}
	got := map[email.ID]string{}
	for _, mv := range mock.MovedEmails {
		got[mv.MessageIDs[0]] = mv.SourceLabel
	}
	if got["m1"] != "INBOX" || got["m2"] != "Archive" {
		t.Errorf("moved from %v, want INBOX for m1 and Archive for m2", got)
	}
}

// A message that has been filed elsewhere is no longer where the set says it
// is, so the set stops listing it: a refresh must not resurrect the row.
func TestSavedSetDropsAMovedRow(t *testing.T) {
	m, _ := savedSetFixture(t)
	openSavedSet(t, m, "quarterly")

	m.Update(archiveDoneMsg{id: "m1"})

	vm, err := virtual.Load(m.client.Config().AccountCache, "quarterly")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(vm.MessageIDs) != 1 || vm.MessageIDs[0] != "m2" {
		t.Errorf("MessageIDs = %v, want only m2", vm.MessageIDs)
	}
	if _, ok := vm.FolderMap["m1"]; ok {
		t.Errorf("FolderMap still places m1: %v", vm.FolderMap)
	}
	if m.showError {
		t.Errorf("updating the set failed: %v", m.err)
	}
}

// A set saved by `search` records no folder for its rows, so an action on one
// refuses it by name instead of moving it out of a folder it was never in.
func TestSavedSetRefusesAMessageItCannotPlace(t *testing.T) {
	m, _ := savedSetFixture(t)
	if err := virtual.Save(m.client.Config().AccountCache, &virtual.Mailbox{
		Name:       "quarterly",
		MessageIDs: []email.ID{"m1"},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	openSavedSet(t, m, "quarterly")

	ids := []email.ID{"m1"}
	plan := m.planAction(ids, m.currentF)
	batches, err := m.batches(plan, ids)
	if err == nil || !strings.Contains(err.Error(), "does not record a folder") {
		t.Fatalf("err = %v, want the unplaced-message refusal", err)
	}
	if len(batches) != 0 {
		t.Errorf("got %d batch(es), want none", len(batches))
	}
	if _, _, err = m.target(plan, "m1"); err == nil {
		t.Error("target must refuse a message the set cannot place")
	}
	// Reading and marking read need a client, not a folder, so the same row
	// still opens: the folder is what an action has to name, not what a read
	// has to.
	cl, err := m.targetClient(plan, "m1")
	if err != nil || cl == nil {
		t.Fatalf("targetClient(m1) = (%v, %v), want the account's client", cl, err)
	}
	read, ok := m.loadDetailCmd("m1")().(detailMsg)
	if !ok {
		t.Fatal("reading a saved set's row must load it")
	}
	if strings.Contains(read.body, "Could not resolve") {
		t.Errorf("a read must not need a folder: %s", read.body)
	}
}

// An action that names a folder without a plan -- a copy, or blocking a sender
// -- asks the row's own folder in a saved set, and the folder on screen outside
// one.
func TestSourceFolderOfNamesTheRowsFolder(t *testing.T) {
	m, _ := savedSetFixture(t)
	openSavedSet(t, m, "quarterly")

	if got := m.sourceFolderOf("m2"); got != "Archive" {
		t.Errorf("sourceFolderOf(m2) = %q, want the folder the set records", got)
	}
	m.currentF = "INBOX"
	if got := m.sourceFolderOf("m2"); got != "INBOX" {
		t.Errorf("outside a saved set the folder on screen answers, got %q", got)
	}
}
