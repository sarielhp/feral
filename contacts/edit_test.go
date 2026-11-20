package contacts

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"google.golang.org/api/people/v1"
)

func googleAlice(emails ...string) Contact {
	if len(emails) == 0 {
		emails = []string{"alice@example.com"}
	}
	return Contact{ID: "google:work:people/1", Name: "Alice Adams", Emails: emails,
		Groups: []string{"friends"}, Source: "google:work"}
}

func aliceEdit() Change {
	return Change{
		Name:    "Alice A. Adams",
		Emails:  []string{"alice@home.example", "alice@example.com"},
		Groups:  []string{"friends", "chess"},
		Aliases: []string{"al"},
		Notes:   "met at STOC",
	}
}

func TestEdit_ImportedKeepsOverlay(t *testing.T) {
	book := &Book{Contacts: []Contact{googleAlice()}}
	c, err := Edit(book, "google:work:people/1", aliceEdit())
	if err != nil {
		t.Fatal(err)
	}
	if c.Name != "Alice A. Adams" || !reflect.DeepEqual(c.Emails, []string{"alice@home.example", "alice@example.com"}) {
		t.Errorf("effective fields = %q %v", c.Name, c.Emails)
	}
	if !c.IsEdited() || c.Origin.Name != "Alice Adams" {
		t.Errorf("origin not kept: %+v", c.Origin)
	}
	if !reflect.DeepEqual(c.Groups, []string{"friends", "chess"}) {
		t.Errorf("groups = %v", c.Groups)
	}
}

func TestEdit_ManualInPlace(t *testing.T) {
	book := NewBook()
	if _, err := AddContact(book, "Bob", []string{"Bob <BOB@example.com>"}); err != nil {
		t.Fatal(err)
	}
	c, err := Edit(book, "manual:bob@example.com", Change{Name: "Robert", Emails: []string{"bob@example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	if c.Name != "Robert" || c.Origin != nil || c.Edits != nil {
		t.Errorf("manual contact = %+v", c)
	}
}

func TestEdit_Refusals(t *testing.T) {
	book := &Book{Contacts: []Contact{googleAlice(),
		{ID: "mutt:bob", Emails: []string{"bob@example.com"}, Nicknames: []string{"bob"}, Source: "mutt"}}}
	if _, err := Edit(book, "google:work:people/1", Change{Emails: nil}); err == nil {
		t.Error("a contact with no address was accepted")
	}
	if _, err := Edit(book, "google:work:people/1", Change{Emails: []string{"not an address"}}); err == nil {
		t.Error("a bad address was accepted")
	}
	_, err := Edit(book, "google:work:people/1", Change{Emails: []string{"alice@example.com"}, Aliases: []string{"bob"}})
	var taken *AliasTakenError
	if !errors.As(err, &taken) {
		t.Errorf("err = %v, want AliasTakenError", err)
	}
}

// The edits to an imported contact are the user's: a re-sync keeps them,
// still shows what the source changed, and keeps them when the person is
// gone upstream.
func TestUpdateSource_CarriesEdits(t *testing.T) {
	edited := func(t *testing.T) *Book {
		t.Helper()
		book := &Book{Contacts: []Contact{googleAlice()}}
		if _, err := Edit(book, "google:work:people/1", aliceEdit()); err != nil {
			t.Fatal(err)
		}
		return book
	}

	t.Run("re-sync keeps the edits", func(t *testing.T) {
		book := edited(t)
		UpdateSource(book, "google:work", []Contact{googleAlice()})
		c := FindByID(book, "google:work:people/1")
		if c == nil || c.Name != "Alice A. Adams" || c.Notes != "met at STOC" || !reflect.DeepEqual(c.Aliases, []string{"al"}) {
			t.Fatalf("after sync = %+v", c)
		}
	})

	t.Run("address changed upstream", func(t *testing.T) {
		book := edited(t)
		UpdateSource(book, "google:work", []Contact{googleAlice("alice@new.example")})
		c := FindByID(book, "google:work:people/1")
		want := []string{"alice@home.example", "alice@new.example"}
		if c == nil || !reflect.DeepEqual(c.Emails, want) {
			t.Fatalf("emails = %v, want %v", c.Emails, want)
		}
	})

	t.Run("gone upstream becomes manual", func(t *testing.T) {
		book := edited(t)
		UpdateSource(book, "google:work", nil)
		c := FindByAddress(book, "alice@home.example")
		if c == nil || c.Source != SourceManual || c.Name != "Alice A. Adams" || c.Notes != "met at STOC" {
			t.Fatalf("orphan = %+v", c)
		}
	})

	t.Run("survives save and reload", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "contacts.json")
		if err := SaveBook(path, edited(t)); err != nil {
			t.Fatal(err)
		}
		book, err := LoadBook(path)
		if err != nil {
			t.Fatal(err)
		}
		UpdateSource(book, "google:work", []Contact{googleAlice()})
		if c := FindByID(book, "google:work:people/1"); c == nil || c.Name != "Alice A. Adams" {
			t.Fatalf("after reload and sync = %+v", c)
		}
	})
}

func TestRevert(t *testing.T) {
	book := &Book{Contacts: []Contact{googleAlice()}}
	if _, err := Edit(book, "google:work:people/1", aliceEdit()); err != nil {
		t.Fatal(err)
	}
	c, err := Revert(book, "google:work:people/1")
	if err != nil {
		t.Fatal(err)
	}
	if c.Name != "Alice Adams" || c.IsEdited() || c.Origin != nil || len(c.Emails) != 1 {
		t.Errorf("reverted = %+v", c)
	}
	if c.Notes != "met at STOC" {
		t.Errorf("revert dropped the notes")
	}
}

func TestRemove_HidesImportedAcrossSync(t *testing.T) {
	book := &Book{Contacts: []Contact{googleAlice()}}
	if _, err := AddContact(book, "", []string{"bob@example.com"}); err != nil {
		t.Fatal(err)
	}
	if hidden, err := Remove(book, "manual:bob@example.com"); err != nil || hidden {
		t.Fatalf("manual remove: hidden=%v err=%v", hidden, err)
	}
	if hidden, err := Remove(book, "google:work:people/1"); err != nil || !hidden {
		t.Fatalf("imported remove: hidden=%v err=%v", hidden, err)
	}
	UpdateSource(book, "google:work", []Contact{googleAlice()})
	if len(book.Contacts) != 0 || len(book.Hidden) != 1 {
		t.Fatalf("after sync: %d shown, %d hidden", len(book.Contacts), len(book.Hidden))
	}
	if _, err := Unhide(book, "google:work:people/1"); err != nil || len(book.Contacts) != 1 {
		t.Fatalf("unhide: %v", err)
	}
	UpdateSource(book, "google:work", nil)
	if len(book.Hidden) != 0 {
		t.Errorf("a hidden contact gone upstream was kept")
	}
}

func TestMerge(t *testing.T) {
	book := &Book{Contacts: []Contact{googleAlice()}}
	if _, err := AddContact(book, "A. Adams", []string{"aa@uni.example"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := SetAlias(book, "aa@uni.example", "", "", "ada"); err != nil {
		t.Fatal(err)
	}
	c, err := Merge(book, "google:work:people/1", "manual:aa@uni.example")
	if err != nil {
		t.Fatal(err)
	}
	if len(book.Contacts) != 1 || !reflect.DeepEqual(c.Emails, []string{"alice@example.com", "aa@uni.example"}) ||
		!reflect.DeepEqual(c.Aliases, []string{"ada"}) {
		t.Errorf("merged = %+v (book has %d)", c, len(book.Contacts))
	}
	if _, err := AddGroup(book, "Theory Group", []string{"x@example.com"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Merge(book, "google:work:people/1", GroupID(SourceManual, "theory-group")); err == nil {
		t.Error("merged a group into a person")
	}
	if len(book.Contacts) != 2 {
		t.Errorf("a refused merge changed the book: %d contacts", len(book.Contacts))
	}
}

func TestAddContact_RefusesKnownAddress(t *testing.T) {
	book := &Book{Contacts: []Contact{googleAlice()}}
	if _, err := AddContact(book, "", []string{"ALICE@example.com"}); err == nil {
		t.Error("added a second contact for a known address")
	}
}

func TestGoogleIDFallbackIsStable(t *testing.T) {
	a := personToContactForTest("", "x@example.com")
	b := personToContactForTest("", "y@example.com")
	if a.ID == b.ID {
		t.Errorf("two contacts without a resource name share ID %q", a.ID)
	}
}

func personToContactForTest(resource, addr string) Contact {
	p := &people.Person{ResourceName: resource, EmailAddresses: []*people.EmailAddress{{Value: addr}}}
	return personToContact(p, "google:work", nil)
}
