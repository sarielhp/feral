package contacts

import (
	"strings"
	"testing"
)

func TestParseMuttAliases_Basic(t *testing.T) {
	input := `
# Sample mutt aliases
alias alice Alice Smith <alice@example.com>
alias bob Bob Jones <bob@example.com>
alias bare user@example.org
alias multi Alice Smith <alice@example.com>, Bob Jones <bob@example.com>
`
	contacts, err := ParseMuttAliases(strings.NewReader(input), "mutt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(contacts) != 4 {
		t.Fatalf("expected 4 contacts, got %d", len(contacts))
	}

	var alice *Contact
	for i := range contacts {
		if contacts[i].DisplayAlias() == "alice" {
			alice = &contacts[i]
			break
		}
	}
	if alice == nil {
		t.Fatalf("alice not found")
	}
	if alice.Name != "Alice Smith" {
		t.Errorf("expected name 'Alice Smith', got %q", alice.Name)
	}
	if alice.PrimaryEmail() != "alice@example.com" {
		t.Errorf("expected email 'alice@example.com', got %q", alice.PrimaryEmail())
	}
}

func TestParseMuttAliases_NestedAndGroup(t *testing.T) {
	input := `
alias a Alice <alice@example.com>
alias b Bob <bob@example.com>
alias team a b
alias bigteam team, charlie@example.com
`
	contacts, err := ParseMuttAliases(strings.NewReader(input), "mutt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var team *Contact
	for i := range contacts {
		if contacts[i].DisplayAlias() == "team" {
			team = &contacts[i]
			break
		}
	}
	if team == nil {
		t.Fatalf("team group not found")
	}
	if len(team.Emails) != 2 {
		t.Errorf("expected 2 emails in team, got %d (%v)", len(team.Emails), team.Emails)
	}

	var bigteam *Contact
	for i := range contacts {
		if contacts[i].DisplayAlias() == "bigteam" {
			bigteam = &contacts[i]
			break
		}
	}
	if bigteam == nil {
		t.Fatalf("bigteam group not found")
	}
	if len(bigteam.Emails) != 3 {
		t.Errorf("expected 3 emails in bigteam, got %d (%v)", len(bigteam.Emails), bigteam.Emails)
	}
}

func TestParseMuttAliases_CircularReference(t *testing.T) {
	input := `
alias x y
alias y x
`
	contacts, err := ParseMuttAliases(strings.NewReader(input), "mutt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(contacts) != 0 {
		t.Errorf("expected 0 contacts for circular empty aliases, got %d", len(contacts))
	}
}

func TestParseMuttFile_RealFileIfExists(t *testing.T) {
	path := "../tmp/mail_aliases"
	contacts, err := ParseMuttFile(path)
	if err != nil {
		t.Skip("tmp/mail_aliases does not exist, skipping")
	}
	if len(contacts) < 500 {
		t.Errorf("expected at least 500 contacts, got %d", len(contacts))
	}
}

// A mutt group alias is recognised as a group, a person is not: the TUI asked
// for "alias:group" while the importer wrote "mutt:group:", so no group was
// ever treated as one.
func TestMuttGroupIsGroup(t *testing.T) {
	got, err := ParseMuttAliases(strings.NewReader(
		"alias alice Alice <alice@example.com>\nalias team alice, bob@example.com\n"), "mutt")
	if err != nil {
		t.Fatal(err)
	}
	groups := map[string]bool{}
	for _, c := range got {
		groups[c.DisplayAlias()] = c.IsGroup()
	}
	if groups["alice"] || !groups["team"] {
		t.Fatalf("IsGroup by alias = %v", groups)
	}
	if id := GroupID("mutt", "team"); !(Contact{ID: id}).IsGroup() {
		t.Fatalf("GroupID %q is not a group", id)
	}
}
