package contacts

import (
	"reflect"
	"testing"
)

func TestAddAlias(t *testing.T) {
	empty := NewBook()
	created, isNew := AddAlias(empty, "dana@Example.com", "Dana Diaz", "dana")
	if !isNew || created == nil || created.ID != "manual:dana@example.com" || created.Source != "manual" {
		t.Fatalf("new contact = %+v, isNew %v", created, isNew)
	}

	// A second alias on the same address joins the existing contact.
	joined, isNew := AddAlias(empty, "DANA@example.com", "Ignored Name", "dd")
	if isNew {
		t.Fatalf("second alias created a new contact: %+v", joined)
	}
	if joined.Name != "Dana Diaz" {
		t.Errorf("existing name replaced: %q", joined.Name)
	}
	if !reflect.DeepEqual(joined.Aliases, []string{"dana", "dd"}) {
		t.Fatalf("aliases = %v", joined.Aliases)
	}

	// A duplicate nickname is folded, case-insensitively.
	dup, _ := AddAlias(empty, "dana@example.com", "", "DANA")
	if !reflect.DeepEqual(dup.Aliases, []string{"dana", "dd"}) {
		t.Fatalf("duplicate alias added: %v", dup.Aliases)
	}

	// A contact that had no name gains one when the alias gives it.
	nameless := NewBook()
	nameless.Contacts = []Contact{{ID: "h", Emails: []string{"harvest@example.com"}}}
	updated, _ := AddAlias(nameless, "harvest@example.com", "Harvest H.", "hv")
	if updated.Name != "Harvest H." {
		t.Errorf("empty name not filled: %q", updated.Name)
	}

	// Refusals: no book, no address, no nickname, non-address input. A
	// refusal is a nil contact, which is what the caller must check.
	refusals := []struct {
		name, addr, nick string
		book             *Book
	}{
		{"nil book", "a@b", "n", nil},
		{"empty address", "", "n", NewBook()},
		{"blank nickname", "a@b", "  ", NewBook()},
		{"non-address", "not-an-address", "n", NewBook()},
	}
	for _, r := range refusals {
		if c, isNew := AddAlias(r.book, r.addr, "", r.nick); c != nil || isNew {
			t.Errorf("%s accepted: %+v", r.name, c)
		}
	}
}

func TestAddAlias_LeavesSourceNicknamesAlone(t *testing.T) {
	book := NewBook()
	book.Contacts = []Contact{{
		ID: "mutt:bob", Emails: []string{"bob@example.com"},
		Nicknames: []string{"bob"}, Source: "mutt",
	}}
	c, _ := AddAlias(book, "bob@example.com", "", "robert")
	if !reflect.DeepEqual(c.Nicknames, []string{"bob"}) || !reflect.DeepEqual(c.Aliases, []string{"robert"}) {
		t.Fatalf("nicknames %v aliases %v", c.Nicknames, c.Aliases)
	}
	// The source already calls him bob: the same alias is not added twice.
	if c, _ = AddAlias(book, "bob@example.com", "", "BOB"); len(c.Aliases) != 1 {
		t.Errorf("alias duplicating a source nickname added: %v", c.Aliases)
	}
	if got := c.DisplayAlias(); got != "robert" {
		t.Errorf("DisplayAlias = %q, want the user alias", got)
	}
}

func TestSetAlias(t *testing.T) {
	fresh := func() *Book {
		b := NewBook()
		b.Contacts = []Contact{
			{ID: "a", Name: "Alice", Emails: []string{"alice@example.com"}, Nicknames: []string{"ali"}},
			{ID: "b", Name: "Bob", Emails: []string{"bob@example.com"}, Aliases: []string{"bob", "bb"}},
			{ID: "mutt:group:team", Name: "Group: team", Emails: []string{"bob@example.com", "carol@example.com"}, Nicknames: []string{"team"}},
		}
		return b
	}
	cases := []struct {
		name, addr, old, nick string
		want                  []string // the contact's aliases afterwards
		taken                 string   // the holder a collision names
		fails                 bool
	}{
		{name: "rename in place keeps the order", addr: "bob@example.com", old: "bob", nick: "robert", want: []string{"robert", "bb"}},
		{name: "rename is case-insensitive on the old name", addr: "BOB@example.com", old: "BOB", nick: "rob", want: []string{"rob", "bb"}},
		{name: "a case-only rename", addr: "bob@example.com", old: "bob", nick: "Bob", want: []string{"Bob", "bb"}},
		{name: "rename onto its own other alias drops the old", addr: "bob@example.com", old: "bob", nick: "BB", want: []string{"bb"}},
		{name: "empty removes", addr: "bob@example.com", old: "bob", nick: "", want: []string{"bb"}},
		{name: "add joins the person, not the group", addr: "carol@example.com", nick: "cc", want: []string{"cc"}},
		{name: "another contact's alias collides", addr: "alice@example.com", nick: "bb", taken: "Bob"},
		{name: "a source nickname collides", addr: "bob@example.com", old: "bob", nick: "ALI", taken: "Alice"},
		{name: "a group nickname collides", addr: "carol@example.com", nick: "team", taken: "Group: team"},
		{name: "renaming an alias it does not have", addr: "alice@example.com", old: "ali", nick: "x", fails: true},
		{name: "not an address", addr: "nobody", nick: "x", fails: true},
		{name: "adding nothing", addr: "bob@example.com", fails: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			book := fresh()
			c, _, err := SetAlias(book, tc.addr, "", tc.old, tc.nick)
			if tc.taken != "" {
				taken, ok := err.(*AliasTakenError)
				if !ok || taken.Holder.Name != tc.taken {
					t.Fatalf("err = %v, want a collision with %s", err, tc.taken)
				}
				if !reflect.DeepEqual(book.Contacts, fresh().Contacts) {
					t.Fatal("a refused alias changed the book")
				}
				return
			}
			if tc.fails {
				if err == nil {
					t.Fatalf("accepted: %+v", c)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if c.IsGroup() || !reflect.DeepEqual(c.Aliases, tc.want) {
				t.Fatalf("contact %s aliases = %v, want %v", c.ID, c.Aliases, tc.want)
			}
		})
	}
}
