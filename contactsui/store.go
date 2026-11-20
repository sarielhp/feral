package contactsui

import (
	"github.com/sarielhp/feral/contacts"
)

// Store is where the lists read contacts from and the manager writes them
// to. FileStore is the real one; tests use an in-memory store.
type Store interface {
	// Load returns the book and the harvested history. A missing file is an
	// empty book, not an error.
	Load() (*contacts.Book, *contacts.HistoryBook, error)
	// Update applies change to the book as it is now, saves it, and returns
	// what was saved; when change fails nothing is saved.
	Update(change func(*contacts.Book) error) (*contacts.Book, error)
}

// FileStore keeps the book in contacts.json and reads the history from
// suggested_contacts.json.
type FileStore struct {
	BookPath    string
	HistoryPath string
}

// Load reads both files. A history that cannot be read is treated as empty:
// it only ranks, and the book is still usable without it.
func (s FileStore) Load() (*contacts.Book, *contacts.HistoryBook, error) {
	book, err := contacts.LoadBook(s.BookPath)
	if err != nil {
		return nil, nil, err
	}
	history, herr := contacts.LoadHistory(s.HistoryPath)
	if herr != nil {
		history = contacts.NewHistoryBook()
	}
	return book, history, nil
}

// Update changes contacts.json under its lock.
func (s FileStore) Update(change func(*contacts.Book) error) (*contacts.Book, error) {
	return contacts.Update(s.BookPath, change)
}

// SuggestSearch ranks contacts and history the way Tab-completion does.
func SuggestSearch(book *contacts.Book, history *contacts.HistoryBook, blacklist []string, limit int) func(string) []contacts.ScoredContact {
	return func(query string) []contacts.ScoredContact {
		return contacts.Suggest(book, history, blacklist, query, limit)
	}
}
