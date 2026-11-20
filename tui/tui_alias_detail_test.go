package tui

import (
	"strings"
	"testing"

	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/contacts"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/mailclient"

	tea "github.com/charmbracelet/bubbletea"
)

// The entry window: Enter opens the highlighted entry with the actions on
// the person, the picker's Enter still inserts a recipient, and the actions
// leave the browser the way each of them should.

// Enter opens the entry, the window shows the person and its actions, and
// Enter, q and Esc each return to the list.
func TestAliasDetail_OpenAndBack(t *testing.T) {
	m := aliasModel(t)
	press(m, "f6")
	press(m, "enter") // Alice carries the nickname, so she sorts first
	if m.aliasView == nil || m.aliasView.detail == nil {
		t.Fatalf("Enter did not open the entry: %+v", m.aliasView)
	}
	if got := m.aliasView.detail.PrimaryEmail(); got != "alice@example.com" {
		t.Fatalf("detail = %+v, want Alice", m.aliasView.detail)
	}

	view := m.View()
	for _, want := range []string{
		"Alice Adams",
		"alice@example.com",
		"write to Alice Adams",
		"the latest 10 emails from alice@example.com",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("entry window lost %q:\n%s", want, view)
		}
	}

	press(m, "esc")
	if m.aliasView == nil || m.aliasView.detail != nil {
		t.Fatal("esc did not return to the list")
	}
	press(m, "enter")
	if m.aliasView == nil || m.aliasView.detail == nil {
		t.Fatal("enter did not reopen the entry")
	}
	press(m, "enter")
	if m.aliasView == nil || m.aliasView.detail != nil {
		t.Fatal("enter in the window did not return to the list")
	}
}

// From the picker, Enter still inserts the entry as a recipient instead of
// opening it: that is what the picker opened the browser for.
func TestAliasDetail_FromPickerStillInserts(t *testing.T) {
	m := composeWithContacts(t)
	press(m, "f5")
	press(m, "f6")
	press(m, "enter")
	if m.aliasView != nil || m.contactPick != nil {
		t.Fatalf("insert left overlays open: alias %v picker %v", m.aliasView != nil, m.contactPick != nil)
	}
	if m.compose.To != "Bob Brown <bob@example.com>" {
		t.Fatalf("To = %q, want the entry's person", m.compose.To)
	}
}

// From the list, c starts a compose to the person; over a compose session
// they join To, and into the field being edited rather than beside it.
func TestAliasDetail_Compose(t *testing.T) {
	m := aliasModel(t)
	press(m, "f6")
	typeInto(m, "bob")
	press(m, "enter")
	if m.aliasView == nil || m.aliasView.detail == nil {
		t.Fatal("the entry window did not open")
	}
	cmd := press(m, "c")
	if m.aliasView != nil {
		t.Fatal("the browser stayed open over the compose")
	}
	if cmd == nil {
		t.Fatal("c did not start a compose")
	}

	composing := aliasModel(t)
	composing.mode = modeCompose
	composing.compose = &composeState{From: "me@example.com", Subject: "s", Body: "b"}
	press(composing, "f6")
	typeInto(composing, "bob")
	press(composing, "enter")
	press(composing, "c")
	if composing.aliasView != nil || composing.compose == nil {
		t.Fatal("c over a compose session did not return to it")
	}
	if !strings.Contains(composing.compose.To, "bob@example.com") {
		t.Fatalf("To = %q, want the entry's person", composing.compose.To)
	}

	editing := aliasModel(t)
	editing.mode = modeCompose
	editing.compose = &composeState{From: "me@example.com", Subject: "s", Body: "b", FieldEditing: fieldTo}
	press(editing, "f6")
	typeInto(editing, "bob")
	press(editing, "enter")
	press(editing, "c")
	if editing.compose == nil || editing.compose.FieldInput != "Bob Brown <bob@example.com>" {
		t.Fatalf("FieldInput = %q, want the person in the field being edited", editing.compose.FieldInput)
	}
}

// l asks the server for the person's newest mail — from:<address>, with a
// limit — and shows the hits as the search view, leaving the reader behind.
func TestAliasDetail_LatestEmails(t *testing.T) {
	m := aliasModel(t)
	mock := &mailclient.MockMailClient{
		Cfg: &cfg_g.Config{SelectedAccount: "Personal", Paths: cfg_g.Paths{CacheRoot: t.TempDir()}},
		SearchResults: map[string][]email.ID{
			"from:bob@example.com": {"b1", "b2"},
		},
		RawMessages: map[email.ID][]byte{
			"b1": []byte("From: Bob <bob@example.com>\r\nSubject: one\r\n\r\nfirst\r\n"),
			"b2": []byte("From: Bob <bob@example.com>\r\nSubject: two\r\n\r\nsecond\r\n"),
		},
	}
	m.client = mock
	m.mode = modeDetail

	press(m, "f6")
	typeInto(m, "bob")
	press(m, "enter")
	cmd := press(m, "l")
	if m.aliasView != nil {
		t.Fatal("the browser stayed open over the results")
	}
	if m.mode != modeIndex {
		t.Fatalf("mode = %d, want the message list", m.mode)
	}
	// The batch is what the program runs: each command it holds is a step
	// the runtime would take, so take them here.
	var done *remoteSearchDoneMsg
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatalf("the search command produced %T", cmd())
	}
	for _, c := range batch {
		if msg, ok := c().(remoteSearchDoneMsg); ok {
			done = &msg
		}
	}
	if done == nil {
		t.Fatal("the batch did not carry the search")
	}
	if mock.LastSearchQuery != "from:bob@example.com" || mock.LastSearchLimit != aliasLastEmailsLimit {
		t.Fatalf("server asked %q with limit %d", mock.LastSearchQuery, mock.LastSearchLimit)
	}
	m.Update(*done)
	if m.currentF != "%search:from:bob@example.com" {
		t.Fatalf("currentF = %q, want the search view", m.currentF)
	}
	if len(m.rawEmails) != 2 {
		t.Fatalf("rawEmails = %d of them, want both hits", len(m.rawEmails))
	}
}

// A group stands for several people: the window offers writing to it, not
// reading one person's mail from it.
func TestAliasDetail_GroupRefusesLatestEmails(t *testing.T) {
	m := aliasModel(t)
	book, err := contacts.LoadBook(contacts.DefaultPath(m.cfg))
	if err != nil {
		t.Fatal(err)
	}
	book.Contacts = append(book.Contacts, contacts.Contact{
		ID:     contacts.GroupID("mutt", "team"),
		Name:   "team",
		Emails: []string{"a@example.com", "b@example.com"},
	})
	if err := contacts.SaveBook(contacts.DefaultPath(m.cfg), book); err != nil {
		t.Fatal(err)
	}

	press(m, "f6")
	typeInto(m, "team")
	press(m, "enter")
	if m.aliasView == nil || m.aliasView.detail == nil {
		t.Fatal("the entry window did not open on the group")
	}
	if !strings.Contains(m.View(), "Members:") {
		t.Errorf("the group's window does not show its members:\n%s", m.View())
	}
	press(m, "l")
	if m.aliasView == nil || m.aliasView.detail == nil {
		t.Fatal("a refused l closed the entry window")
	}
	if m.aliasView.status == "" {
		t.Error("a refused l should say why")
	}
}
