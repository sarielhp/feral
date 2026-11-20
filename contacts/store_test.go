package contacts

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestStore_SaveLoadAndFilter(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "contacts.json")

	book, err := LoadBook(path)
	if err != nil {
		t.Fatalf("expected empty book, got error: %v", err)
	}
	if len(book.Contacts) != 0 {
		t.Fatalf("expected 0 contacts, got %d", len(book.Contacts))
	}

	book.Contacts = []Contact{
		{
			ID:        "mutt:dave",
			Name:      "Dave Rivers",
			Emails:    []string{"rivers@cs.example.edu"},
			Nicknames: []string{"dave", "rivers"},
			Groups:    []string{"algo"},
			Source:    "mutt",
		},
		{
			ID:        "google:work:c1",
			Name:      "Dave Rivers",
			Emails:    []string{"rivers@cs.example.edu"},
			Nicknames: []string{"dave"},
			Groups:    []string{"algo"},
			Source:    "google:work",
		},
		{
			ID:        "mutt:alice",
			Name:      "Alice Smith",
			Emails:    []string{"alice@example.com"},
			Nicknames: []string{"alice"},
			Source:    "mutt",
		},
	}

	if saveErr := SaveBook(path, book); saveErr != nil {
		t.Fatalf("SaveBook failed: %v", saveErr)
	}

	loaded, err := LoadBook(path)
	if err != nil {
		t.Fatalf("LoadBook failed: %v", err)
	}
	if len(loaded.Contacts) != 3 {
		t.Fatalf("expected 3 contacts, got %d", len(loaded.Contacts))
	}

	// Filter
	daves := Filter(loaded, "rivers")
	if len(daves) != 2 {
		t.Errorf("expected 2 daves matching 'rivers', got %d", len(daves))
	}

	// Search
	results := Search(loaded, "dave google")
	if len(results) != 1 {
		t.Errorf("expected 1 result matching 'dave google', got %d", len(results))
	} else if results[0].Source != "google:work" {
		t.Errorf("expected google:work source, got %q", results[0].Source)
	}

	// UpdateSource mutt
	newMutt := []Contact{
		{
			ID:        "mutt:bob",
			Name:      "Bob Jones",
			Emails:    []string{"bob@example.com"},
			Nicknames: []string{"bob"},
			Source:    "mutt",
		},
	}
	UpdateSource(loaded, "mutt", newMutt)

	// Google contact should still be there, mutt replaced
	if len(loaded.Contacts) != 2 {
		t.Errorf("expected 2 contacts after UpdateSource, got %d", len(loaded.Contacts))
	}
	hasGoogle := false
	hasBob := false
	for _, c := range loaded.Contacts {
		if c.Source == "google:work" {
			hasGoogle = true
		}
		if c.DisplayAlias() == "bob" {
			hasBob = true
		}
	}
	if !hasGoogle || !hasBob {
		t.Errorf("expected google:work and bob, got %v", loaded.Contacts)
	}
}

func TestSaveBook_AtomicAndDurable(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "nested", "contacts.json")

	book := &Book{
		Contacts: []Contact{
			{
				ID:     "c1",
				Name:   "Test Contact",
				Emails: []string{"test@example.com"},
				Source: "manual",
			},
		},
	}

	if err := SaveBook(path, book); err != nil {
		t.Fatalf("SaveBook in nested directory failed: %v", err)
	}

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat contacts file failed: %v", err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("file mode = %v, want 0600", fi.Mode().Perm())
	}

	loaded, err := LoadBook(path)
	if err != nil {
		t.Fatalf("LoadBook failed: %v", err)
	}
	if len(loaded.Contacts) != 1 || loaded.Contacts[0].Name != "Test Contact" {
		t.Errorf("loaded contacts mismatch: %+v", loaded.Contacts)
	}

	// Repeated overwrite
	book.Contacts = append(book.Contacts, Contact{ID: "c2", Name: "Second Contact", Source: "manual"})
	if saveErr := SaveBook(path, book); saveErr != nil {
		t.Fatalf("repeated SaveBook failed: %v", saveErr)
	}

	loaded2, err := LoadBook(path)
	if err != nil {
		t.Fatalf("LoadBook after update failed: %v", err)
	}
	if len(loaded2.Contacts) != 2 {
		t.Errorf("expected 2 contacts, got %d", len(loaded2.Contacts))
	}
}

// The aliases a user gives an imported contact belong to the user, not the
// source: re-importing the source must carry them over, whatever the source
// now says about the address.
func TestUpdateSource_KeepsUserAliases(t *testing.T) {
	google := func(nick string) Contact {
		c := Contact{ID: "google:work:people/1", Name: "Alice Adams",
			Emails: []string{"alice@example.com"}, Source: "google:work"}
		if nick != "" {
			c.Nicknames = []string{nick}
		}
		return c
	}
	mutt := Contact{ID: "mutt:carol", Name: "Carol", Emails: []string{"carol@example.com"},
		Nicknames: []string{"carol"}, Source: "mutt"}

	t.Run("google sync keeps the alias", func(t *testing.T) {
		book := &Book{Contacts: []Contact{google("")}}
		AddAlias(book, "alice@example.com", "", "my-alice")
		UpdateSource(book, "google:work", []Contact{google("al")})
		wantAliases(t, book, "alice@example.com", []string{"my-alice"}, "google:work")
		if got := Search(book, "my-alice"); len(got) != 1 {
			t.Errorf("search for the alias after sync: %d results", len(got))
		}
	})

	t.Run("mutt reimport keeps the alias", func(t *testing.T) {
		book := &Book{Contacts: []Contact{mutt}}
		AddAlias(book, "carol@example.com", "", "cc")
		UpdateSource(book, "mutt", []Contact{mutt})
		wantAliases(t, book, "carol@example.com", []string{"cc"}, "mutt")
	})

	t.Run("address gone upstream moves the alias to a manual contact", func(t *testing.T) {
		book := &Book{Contacts: []Contact{google("")}}
		AddAlias(book, "alice@example.com", "", "my-alice")
		UpdateSource(book, "google:work", nil)
		c := wantAliases(t, book, "alice@example.com", []string{"my-alice"}, "manual")
		if c.Name != "Alice Adams" || c.ID != "manual:alice@example.com" {
			t.Errorf("re-homed contact = %+v", c)
		}
	})

	t.Run("survives save and reload", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "contacts.json")
		book := &Book{Contacts: []Contact{google("")}}
		AddAlias(book, "alice@example.com", "", "my-alice")
		if err := SaveBook(path, book); err != nil {
			t.Fatal(err)
		}
		loaded, err := LoadBook(path)
		if err != nil {
			t.Fatal(err)
		}
		UpdateSource(loaded, "google:work", []Contact{google("")})
		wantAliases(t, loaded, "alice@example.com", []string{"my-alice"}, "google:work")
	})
}

// wantAliases asserts the book holds addr exactly once, with aliases, in a
// contact from source, and returns that contact.
func wantAliases(t *testing.T, book *Book, addr string, aliases []string, source string) Contact {
	t.Helper()
	var found []Contact
	for _, c := range book.Contacts {
		if holdsAddress(&c, addr) {
			found = append(found, c)
		}
	}
	if len(found) != 1 {
		t.Fatalf("%s held by %d contacts: %+v", addr, len(found), found)
	}
	c := found[0]
	if !reflect.DeepEqual(c.Aliases, aliases) || c.Source != source {
		t.Errorf("contact = %+v, want aliases %v from %s", c, aliases, source)
	}
	return c
}
