package contacts

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
)

// Concurrent writers each add a contact; with Update every one survives,
// where a plain load-change-save would keep only the last writer's copy.
func TestUpdate_ConcurrentWritersAllLand(t *testing.T) {
	path := filepath.Join(t.TempDir(), "contacts.json")
	const writers = 20
	var wg sync.WaitGroup
	for i := range writers {
		wg.Go(func() {
			_, err := Update(path, func(b *Book) error {
				addr := fmt.Sprintf("p%d@example.com", i)
				b.Contacts = append(b.Contacts, Contact{ID: "manual:" + addr, Emails: []string{addr}, Source: "manual"})
				return nil
			})
			if err != nil {
				t.Errorf("Update: %v", err)
			}
		})
	}
	wg.Wait()

	book, err := LoadBook(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(book.Contacts) != writers {
		t.Fatalf("got %d contacts, want %d", len(book.Contacts), writers)
	}
}

func TestUpdate_ErrorWritesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "contacts.json")
	if _, err := Update(path, func(b *Book) error {
		b.Contacts = append(b.Contacts, Contact{ID: "manual:a@example.com", Emails: []string{"a@example.com"}})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	refused := errors.New("refused")
	_, err := Update(path, func(b *Book) error {
		b.Contacts = nil
		return refused
	})
	if !errors.Is(err, refused) {
		t.Fatalf("err = %v, want %v", err, refused)
	}
	book, _ := LoadBook(path)
	if len(book.Contacts) != 1 {
		t.Fatalf("a refused change was saved: %d contacts", len(book.Contacts))
	}
}
