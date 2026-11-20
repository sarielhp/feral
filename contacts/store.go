package contacts

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/sarielhp/feral/cache/dirlock"
	"github.com/sarielhp/feral/cfg_g"
)

// DefaultPath returns the path to the global contacts.json file.
func DefaultPath(config *cfg_g.Config) string {
	if config != nil && config.ConfigDir != "" {
		return filepath.Join(config.ConfigDir, "contacts.json")
	}
	dir, err := cfg_g.UserConfigDir()
	if err != nil {
		return "contacts.json"
	}
	return filepath.Join(dir, "contacts.json")
}

// LoadBook reads the Book from the specified JSON file, or returns an empty Book if not found.
func LoadBook(path string) (*Book, error) {
	return loadJSONFile(path, "contacts", NewBook)
}

// loadJSONFile reads a JSON document into a new T, returning an empty one when
// the file does not exist. The noun names the document in error messages.
func loadJSONFile[T any](path, noun string, empty func() *T) (*T, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return empty(), nil
		}
		return nil, fmt.Errorf("failed to read %s file %s: %w", noun, path, err)
	}

	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("failed to parse %s file %s: %w", noun, path, err)
	}
	return &v, nil
}

// SaveBook writes the Book to the specified JSON file with directory creation and atomic rename.
func SaveBook(path string, book *Book) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	book.Updated = time.Now().UTC()
	data, err := json.MarshalIndent(book, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize contacts: %w", err)
	}

	if err := dirlock.AtomicWrite(path, data, 0o600); err != nil {
		return fmt.Errorf("failed to commit contacts file %s: %w", path, err)
	}
	return nil
}

// Update applies change to the book at path under a lock and saves the
// result, so two writers -- the TUI, the contacts manager, an import or a
// sync, in any process -- never overwrite each other's work with a copy they
// loaded earlier. The book is read after the lock is taken. When change
// returns an error nothing is written and the error is returned. Returns the
// book as saved.
func Update(path string, change func(*Book) error) (*Book, error) {
	release, err := dirlock.GuardFile(path)
	defer release()
	if err != nil {
		return nil, fmt.Errorf("lock contacts file %s: %w", path, err)
	}
	book, err := LoadBook(path)
	if err != nil {
		return nil, err
	}
	if err := change(book); err != nil {
		return nil, err
	}
	if err := SaveBook(path, book); err != nil {
		return nil, err
	}
	return book, nil
}

// UpdateSource replaces contacts matching sourcePrefix with the given newContacts,
// leaving all contacts from other sources intact. What the user added to a
// replaced contact -- aliases, notes, edits -- is not the source's to drop:
// it moves to the contact now standing for the same person, or to a manual
// contact when none does. A contact the user hid stays hidden.
func UpdateSource(book *Book, sourcePrefix string, newContacts []Contact) {
	kept, replaced := splitBySource(book.Contacts, sourcePrefix)
	keptHidden, oldHidden := splitBySource(book.Hidden, sourcePrefix)
	fresh, stillHidden := keepHidden(slices.Clone(newContacts), oldHidden)
	book.Contacts = carryUserEdits(slices.Concat(kept, fresh), replaced)
	book.Hidden = slices.Concat(keptHidden, stillHidden)
	book.Updated = time.Now().UTC()
}

// splitBySource separates the contacts from sourcePrefix from the rest.
func splitBySource(list []Contact, sourcePrefix string) (others, matching []Contact) {
	for _, c := range list {
		if strings.HasPrefix(c.Source, sourcePrefix) {
			matching = append(matching, c)
			continue
		}
		others = append(others, c)
	}
	return others, matching
}

// Filter returns contacts matching a single case-insensitive substring across Name, Email, or Nickname.
func Filter(book *Book, term string) []Contact {
	if book == nil {
		return nil
	}
	clean := strings.TrimSpace(strings.ToLower(term))
	if clean == "" {
		return book.Contacts
	}

	var results []Contact
	for _, c := range book.Contacts {
		if matchContact(c, clean) {
			results = append(results, c)
		}
	}
	return results
}

// Search returns contacts where all space-separated terms in query match at least one field.
func Search(book *Book, query string) []Contact {
	if book == nil {
		return nil
	}
	terms := strings.Fields(strings.ToLower(query))
	if len(terms) == 0 {
		return book.Contacts
	}

	var results []Contact
	for _, c := range book.Contacts {
		allMatched := true
		for _, term := range terms {
			if !matchContact(c, term) {
				allMatched = false
				break
			}
		}
		if allMatched {
			results = append(results, c)
		}
	}
	return results
}

func matchContact(c Contact, term string) bool {
	if strings.Contains(strings.ToLower(c.Name), term) {
		return true
	}
	for _, e := range c.Emails {
		if strings.Contains(strings.ToLower(e), term) {
			return true
		}
	}
	for _, n := range c.AllAliases() {
		if strings.Contains(strings.ToLower(n), term) {
			return true
		}
	}
	for _, g := range c.Groups {
		if strings.Contains(strings.ToLower(g), term) {
			return true
		}
	}
	if strings.Contains(strings.ToLower(c.Source), term) {
		return true
	}
	return strings.Contains(strings.ToLower(c.Notes), term)
}
