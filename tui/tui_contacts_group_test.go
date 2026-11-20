package tui

import (
	"strings"
	"testing"

	"github.com/sarielhp/feral/contacts"
)

// A mutt group alias stands for all its members wherever a contact becomes a
// recipient. The TUI looked for "alias:group" in the ID while the importer
// writes "mutt:group:team", so a group completed to its first member under
// the name "Group: team".

func muttTeam() contacts.ScoredContact {
	return contacts.ScoredContact{Contact: contacts.Contact{
		ID: contacts.GroupID("mutt", "team"), Name: "Group: team",
		Emails:    []string{"alice@example.com", "bob@example.com"},
		Nicknames: []string{"team"}, Groups: []string{"team"}, Source: "mutt",
	}}
}

func TestGroupAlias_CompletesToAllMembers(t *testing.T) {
	r := formatCompletionString(muttTeam())
	if r != "alice@example.com, bob@example.com" {
		t.Fatalf("formatted = %q", r)
	}
	list := appendRecipients("bob@example.com", []string{r})
	if list != "bob@example.com, alice@example.com" {
		t.Fatalf("appended = %q, want each member once", list)
	}
}

func TestGroupAlias_PickerShowsTheGroup(t *testing.T) {
	if line := contactLine(muttTeam()); !strings.Contains(line, "group of 2") {
		t.Fatalf("picker line = %q", line)
	}
}

// The new-alias flow names one address, so it does not offer a group.
func TestGroupAlias_NotOfferedForANewAlias(t *testing.T) {
	m := aliasModel(t)
	book, err := contacts.LoadBook(contacts.DefaultPath(m.cfg))
	if err != nil {
		t.Fatal(err)
	}
	book.Contacts = append(book.Contacts, muttTeam().Contact)
	if err := contacts.SaveBook(contacts.DefaultPath(m.cfg), book); err != nil {
		t.Fatal(err)
	}
	press(m, "f6")
	press(m, "ctrl+n")
	for _, sc := range m.aliasView.addResults {
		if sc.IsGroup() {
			t.Fatalf("group offered as a new-alias address: %+v", sc)
		}
	}
	if len(m.aliasView.addResults) != 2 {
		t.Fatalf("candidates = %+v", m.aliasView.addResults)
	}
}
