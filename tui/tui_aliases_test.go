package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/contacts"
	"github.com/sarielhp/feral/email"

	tea "github.com/charmbracelet/bubbletea"
)

// The aliases browser: F6 opens it from the index, the reader, the compose
// screen, and over the open contacts picker; Ctrl+n walks to a new alias
// through an address that already exists.

func aliasModel(t *testing.T) *tuiModel {
	t.Helper()
	m := newTuiModel(nil, "", nil)
	m.cfg = &cfg_g.Config{Paths: cfg_g.Paths{ConfigDir: t.TempDir(), CacheRoot: t.TempDir()}}
	m.width, m.height = 80, 40
	book := contacts.NewBook()
	book.Contacts = []contacts.Contact{
		{ID: "1", Name: "Alice Adams", Emails: []string{"alice@example.com"}, Nicknames: []string{"ali"}},
		{ID: "2", Name: "Bob Brown", Emails: []string{"bob@example.com"}},
	}
	if err := contacts.SaveBook(contacts.DefaultPath(m.cfg), book); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestAliasBrowser_OpensFromIndex(t *testing.T) {
	m := aliasModel(t)
	press(m, "f6")
	if m.aliasView == nil || len(m.aliasView.results) != 2 {
		t.Fatalf("browser = %+v", m.aliasView)
	}
	if !strings.Contains(m.View(), "ALIASES") {
		t.Fatal("aliases browser not drawn over the index")
	}
	// The aliased contact sorts first and carries its alias.
	if m.aliasView.results[0].DisplayAlias() != "ali" {
		t.Fatalf("results[0] = %+v, want the aliased contact", m.aliasView.results[0])
	}
	press(m, "esc")
	if m.aliasView != nil {
		t.Fatal("esc did not close the browser")
	}
}

func TestAliasBrowser_SearchNarrowsAndWidens(t *testing.T) {
	m := aliasModel(t)
	press(m, "f6")
	typeInto(m, "bob")
	if len(m.aliasView.results) != 1 || m.aliasView.results[0].PrimaryEmail() != "bob@example.com" {
		t.Fatalf("results = %+v", m.aliasView.results)
	}
	for range 3 { // "bob" → "bo" → "b" → ""
		press(m, "backspace")
	}
	if len(m.aliasView.results) != 2 {
		t.Fatalf("backspace did not empty the search: %+v", m.aliasView.results)
	}
}

// The saved alias must be in contacts.json when the flow returns to the
// list, and the new contact joins the one it names.
func TestAliasBrowser_AddFlowFromIndex(t *testing.T) {
	m := aliasModel(t)
	press(m, "f6")
	press(m, "ctrl+n")
	if !m.aliasView.adding || m.aliasView.nickStage {
		t.Fatalf("add flow stage = adding %v nick %v", m.aliasView.adding, m.aliasView.nickStage)
	}
	if len(m.aliasView.addResults) != 2 {
		t.Fatalf("candidates = %+v", m.aliasView.addResults)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyDown}) // Bob, after Alice
	press(m, "enter")
	if !m.aliasView.nickStage || m.aliasView.nickname != "bob-brown" {
		t.Fatalf("nick stage = %v with nickname %q", m.aliasView.nickStage, m.aliasView.nickname)
	}
	press(m, "enter") // save it
	if m.aliasView == nil || m.aliasView.adding || m.aliasView.nickStage {
		t.Fatal("add flow did not return to the list")
	}
	if !strings.Contains(m.aliasView.status, "saved") {
		t.Fatalf("status = %q", m.aliasView.status)
	}
	saved, err := contacts.LoadBook(contacts.DefaultPath(m.cfg))
	if err != nil {
		t.Fatal(err)
	}
	var bob *contacts.Contact
	for i := range saved.Contacts {
		if saved.Contacts[i].PrimaryEmail() == "bob@example.com" {
			bob = &saved.Contacts[i]
		}
	}
	if bob == nil || len(bob.Aliases) != 1 || bob.Aliases[0] != "bob-brown" {
		t.Fatalf("saved contact = %+v", bob)
	}
	// Bob is saved with his alias; both contacts carry aliases now, so the
	// list orders them alphabetically by alias.
	if len(m.aliasView.results) != 2 || m.aliasView.results[1].DisplayAlias() != "bob-brown" {
		t.Fatalf("results after save = %+v", m.aliasView.results)
	}
}

func TestAliasBrowser_NickStageEscReturnsToCandidates(t *testing.T) {
	m := aliasModel(t)
	press(m, "f6")
	press(m, "ctrl+n")
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	press(m, "enter")
	if !m.aliasView.nickStage {
		t.Fatal("add flow did not reach the naming stage")
	}
	press(m, "esc")
	if !m.aliasView.adding || m.aliasView.nickStage {
		t.Fatalf("esc stage = adding %v nick %v", m.aliasView.adding, m.aliasView.nickStage)
	}
	press(m, "ctrl+n") // restarts cleanly: new query, cursor and nickname
	press(m, "down")
	press(m, "enter")
	if m.aliasView.nickname != "bob-brown" {
		t.Fatalf("the flow did not restart cleanly: %q", m.aliasView.nickname)
	}
}

// From the picker, Enter inserts the highlighted alias as a recipient and
// Esc steps back to the picker with the book reloaded.
func TestAliasBrowser_FromPicker(t *testing.T) {
	m := composeWithContacts(t)
	press(m, "f5")
	press(m, "f6")
	if m.aliasView == nil || !m.aliasView.fromPicker {
		t.Fatalf("browser = %+v, want opened from the picker", m.aliasView)
	}
	press(m, "enter") // Bob carries the alias bobby; his name is what lands
	if m.aliasView != nil || m.contactPick != nil {
		t.Fatal("insert did not close the overlay and the picker")
	}
	if m.compose.To != "Bob Brown <bob@example.com>" {
		t.Fatalf("To = %q", m.compose.To)
	}

	press(m, "f5")
	press(m, "f6")
	press(m, "esc")
	if m.aliasView != nil || m.contactPick == nil {
		t.Fatalf("alias %v picker %v", m.aliasView, m.contactPick)
	}
}

// In the reader, the flow is seeded with the open message's sender, who
// leads the candidates marked with where they came from.
func TestAliasBrowser_DetailSeedsTheSender(t *testing.T) {
	m := aliasModel(t)
	m.mode = modeDetail
	sender := email.Email{
		ID: "m1", Subject: "hello",
		FromEmail: "dana@corp.example", FromRaw: "Dana Diaz <dana@corp.example>",
	}
	m.rawEmails = []email.Email{sender}
	m.emails = []email.Email{sender}
	m.selectedID = sender.ID
	press(m, "f6")
	if addr, _ := m.currentSender(); addr != "dana@corp.example" {
		t.Fatalf("currentSender = %q", addr)
	}
	press(m, "ctrl+n")
	if len(m.aliasView.addResults) == 0 || m.aliasView.addResults[0].PrimaryEmail() != "dana@corp.example" {
		t.Fatalf("candidates = %+v", m.aliasView.addResults)
	}
	if m.aliasView.addResults[0].Source != "this message" {
		t.Fatalf("sender source = %q", m.aliasView.addResults[0].Source)
	}
	press(m, "enter")
	if m.aliasView.nickname != "dana-diaz" {
		t.Errorf("prefilled nickname = %q", m.aliasView.nickname)
	}
}

// senderModel is aliasModel with one message from sender on the list.
func senderModel(t *testing.T, from, raw string) *tuiModel {
	t.Helper()
	m := aliasModel(t)
	msg := email.Email{ID: "m1", Subject: "hello", FromEmail: from, FromRaw: raw}
	m.rawEmails = []email.Email{msg}
	m.emails = []email.Email{msg}
	m.selectedID = msg.ID
	return m
}

// a on the list goes straight to naming the sender; Enter saves the alias
// and closes the overlay, the status line saying what was saved.
func TestAliasSender_OneKeyFlow(t *testing.T) {
	m := senderModel(t, "dana@corp.example", "Dana Diaz <dana@corp.example>")
	press(m, "a")
	if m.aliasView == nil || !m.aliasView.nickStage {
		t.Fatalf("a did not open the naming stage: %+v", m.aliasView)
	}
	if !strings.Contains(m.View(), "Esc: cancel") {
		t.Error("naming stage does not say Esc cancels")
	}
	if m.aliasView.nickname != "dana-diaz" {
		t.Errorf("prefilled nickname = %q", m.aliasView.nickname)
	}
	press(m, "enter")
	if m.aliasView != nil {
		t.Fatal("saving did not close the overlay")
	}
	if !strings.Contains(m.saveStatus, `"dana-diaz" saved for dana@corp.example`) {
		t.Errorf("status = %q", m.saveStatus)
	}
	book, err := contacts.LoadBook(contacts.DefaultPath(m.cfg))
	if err != nil {
		t.Fatal(err)
	}
	if !aliasSaved(book, "dana@corp.example", "dana-diaz") {
		t.Errorf("alias not in the book: %+v", book.Contacts)
	}
}

// A sender already in the book is named on its saved entry, and Esc leaves
// the whole overlay rather than stepping back to a list never shown.
func TestAliasSender_KnownSenderAndEsc(t *testing.T) {
	m := senderModel(t, "bob@example.com", "B <bob@example.com>")
	m.mode = modeDetail
	press(m, "a")
	if m.aliasView == nil || m.aliasView.picked.Name != "Bob Brown" {
		t.Fatalf("picked = %+v", m.aliasView)
	}
	press(m, "esc")
	if m.aliasView != nil {
		t.Fatal("esc did not close the overlay")
	}
}

// aliasSaved reports whether the book holds nick on the contact for addr.
func aliasSaved(book *contacts.Book, addr, nick string) bool {
	for _, c := range book.Contacts {
		if strings.EqualFold(c.PrimaryEmail(), addr) && slices.Contains(c.AllAliases(), nick) {
			return true
		}
	}
	return false
}
