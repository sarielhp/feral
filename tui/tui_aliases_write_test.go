package tui

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/sarielhp/feral/contacts"

	tea "github.com/charmbracelet/bubbletea"
)

// How the aliases browser writes contacts.json: never over a file it could not
// read, never from a stale copy, and never when the session is a dry run.

// saveBobAlias walks the add flow to the naming stage for Bob and saves the
// suggested nickname.
func saveBobAlias(t *testing.T, m *tuiModel) {
	t.Helper()
	press(m, "ctrl+n")
	m.Update(tea.KeyMsg{Type: tea.KeyDown}) // Bob, after Alice
	press(m, "enter")
	if m.aliasView == nil || !m.aliasView.nickStage {
		t.Fatal("add flow did not reach the naming stage")
	}
	press(m, "enter")
}

func bookBytes(t *testing.T, m *tuiModel) []byte {
	t.Helper()
	data, err := os.ReadFile(contacts.DefaultPath(m.cfg))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func bobAliases(t *testing.T, m *tuiModel) []string {
	t.Helper()
	book, err := contacts.LoadBook(contacts.DefaultPath(m.cfg))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range book.Contacts {
		if c.PrimaryEmail() == "bob@example.com" {
			return c.Aliases
		}
	}
	return nil
}

// A damaged contacts.json is reported and left alone: saving used to
// dereference the nil book and crash the TUI, and saving an empty one would
// have replaced the user's file.
func TestAliasBrowser_DamagedBookIsNeverWritten(t *testing.T) {
	m := aliasModel(t)
	damaged := []byte("{broken")
	if err := os.WriteFile(contacts.DefaultPath(m.cfg), damaged, 0o600); err != nil {
		t.Fatal(err)
	}
	press(m, "f6")
	if m.aliasView == nil || !strings.Contains(m.aliasView.status, "Contacts not loaded") {
		t.Fatalf("browser = %+v, want the load error shown", m.aliasView)
	}
	press(m, "ctrl+n")
	if m.aliasView.adding {
		t.Fatal("the add flow started over an unreadable book")
	}
	_ = m.View()
	if !bytes.Equal(bookBytes(t, m), damaged) {
		t.Fatal("the damaged contacts.json was rewritten")
	}
}

// A file that goes bad after the browser opened is not overwritten either.
func TestAliasBrowser_BookDamagedWhileOpen(t *testing.T) {
	m := aliasModel(t)
	press(m, "f6")
	press(m, "ctrl+n")
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	press(m, "enter")
	damaged := []byte("{broken")
	if err := os.WriteFile(contacts.DefaultPath(m.cfg), damaged, 0o600); err != nil {
		t.Fatal(err)
	}
	press(m, "enter")
	if !strings.Contains(m.aliasView.status, "not saved") {
		t.Fatalf("status = %q", m.aliasView.status)
	}
	if !bytes.Equal(bookBytes(t, m), damaged) {
		t.Fatal("the damaged contacts.json was rewritten")
	}
}

// A candidate that is not an address is refused, not reported as saved.
func TestAliasBrowser_RefusalIsNotSuccess(t *testing.T) {
	m := aliasModel(t)
	press(m, "f6")
	before := bookBytes(t, m)
	b := m.aliasView
	b.adding, b.nickStage = true, true
	b.picked = contacts.Contact{Name: "Nobody", Emails: []string{"not-an-address"}}
	b.nickname = "nobody"
	press(m, "enter")
	if strings.Contains(b.status, "saved for") || !strings.Contains(b.status, "not saved") {
		t.Fatalf("status = %q, want a refusal", b.status)
	}
	if !bytes.Equal(bookBytes(t, m), before) {
		t.Fatal("a refused alias rewrote contacts.json")
	}
}

// Read-only forbids the server and recording what did not happen; an alias is
// neither, so it is saved, and says only that it was.
func TestAliasBrowser_ReadOnlySavesAliases(t *testing.T) {
	m := aliasModel(t)
	m.cfg.ReadOnly = true
	press(m, "f6")
	saveBobAlias(t, m)
	if got := bobAliases(t, m); len(got) != 1 || got[0] != "bob-brown" {
		t.Fatalf("aliases = %v, status %q", got, m.aliasView.status)
	}
	if strings.Contains(m.aliasView.status, "new contact") {
		t.Errorf("status = %q: Bob was already a contact", m.aliasView.status)
	}
	if m.showError {
		t.Fatalf("error banner in read-only: %v", m.err)
	}
}

// A dry run reports the alias and writes nothing.
func TestAliasBrowser_DryRunWritesNothing(t *testing.T) {
	m := aliasModel(t)
	m.cfg.DryRun = true
	press(m, "f6")
	before := bookBytes(t, m)
	saveBobAlias(t, m)
	if !strings.Contains(m.aliasView.status, "Dry run") {
		t.Fatalf("status = %q", m.aliasView.status)
	}
	if !bytes.Equal(bookBytes(t, m), before) {
		t.Fatal("a dry run wrote contacts.json")
	}
}

// Opening the browser sorts what it shows, not the book behind it.
func TestAliasBrowser_OpeningLeavesBookOrder(t *testing.T) {
	m := aliasModel(t)
	book := contacts.NewBook()
	book.Contacts = []contacts.Contact{
		{ID: "2", Name: "Bob Brown", Emails: []string{"bob@example.com"}},
		{ID: "1", Name: "Alice Adams", Emails: []string{"alice@example.com"}, Nicknames: []string{"ali"}},
	}
	if err := contacts.SaveBook(contacts.DefaultPath(m.cfg), book); err != nil {
		t.Fatal(err)
	}
	press(m, "f6")
	if m.aliasView.results[0].ID != "1" {
		t.Fatalf("list order = %+v, want the aliased contact first", m.aliasView.results)
	}
	if m.aliasView.book.Contacts[0].ID != "2" {
		t.Fatalf("book reordered by opening the browser: %+v", m.aliasView.book.Contacts)
	}
}

// A contact written by another process while the browser was open survives
// the save.
func TestAliasBrowser_SaveKeepsConcurrentWrite(t *testing.T) {
	m := aliasModel(t)
	press(m, "f6")
	path := contacts.DefaultPath(m.cfg)
	book, err := contacts.LoadBook(path)
	if err != nil {
		t.Fatal(err)
	}
	book.Contacts = append(book.Contacts, contacts.Contact{ID: "3", Name: "Carol", Emails: []string{"carol@example.com"}})
	if err = contacts.SaveBook(path, book); err != nil {
		t.Fatal(err)
	}
	saveBobAlias(t, m)
	saved, err := contacts.LoadBook(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Contacts) != 3 {
		t.Fatalf("contacts after save = %+v, want Carol kept", saved.Contacts)
	}
}
