// Package snooze hides a message until a time and then puts it back in the
// inbox, unread.
//
// Snoozing moves the message into a Snoozed folder on its own account and
// records it in <download_dir>/snooze.json. The record is what brings it
// back: nothing on the server says when a message in Snoozed is due, so a
// record lost is a message left in a folder nobody looks at. That is why a
// damaged file is an error and never an empty list, why the record is
// written before the move and removed only after the move back, and why a
// message that cannot be woken -- its account gone, the message deleted --
// keeps its record and is reported instead of dropped.
//
// What wakes a message is a watcher: `feral watch`, or the one the TUI runs
// while it is open, on every poll; `feral msg snooze --wake` does it by hand.
// Nothing here reads the clock: every caller passes now.
package snooze

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sarielhp/feral/cache/dirlock"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/fileformat"
)

// FileName is the record's name in the download directory.
const FileName = "snooze.json"

// Entry is one snoozed message.
type Entry struct {
	Account string   `json:"account"`
	ID      email.ID `json:"id"`
	// Until is when the message goes back to the inbox.
	Until time.Time `json:"until"`
	// FromFolder is the folder it was snoozed from; Folder is the snooze
	// folder it waits in, as the server spells it.
	FromFolder string    `json:"from_folder"`
	Folder     string    `json:"folder"`
	Subject    string    `json:"subject,omitempty"`
	From       string    `json:"from,omitempty"`
	SnoozedAt  time.Time `json:"snoozed_at"`
}

// Due reports whether the entry's time has come.
func (e Entry) Due(now time.Time) bool { return !now.Before(e.Until) }

// Matches reports whether ref -- a message ID as the user typed it, full or
// short -- names the entry's message.
func (e Entry) Matches(ref string) bool { return email.IDMatches(e.ID, ref) }

func (e Entry) same(account string, id email.ID) bool {
	return strings.EqualFold(e.Account, account) && e.ID == id
}

// File is snooze.json.
type File struct {
	Format  int     `json:"format,omitempty"` // see fileformat; absent means 1
	Entries []Entry `json:"entries"`
}

// Path names the record beneath a download directory.
func Path(downloadDir string) string { return filepath.Join(downloadDir, FileName) }

// Load reads the record. A missing file is an empty one; a file that does
// not parse, or that a newer build wrote, is an error.
func Load(downloadDir string) (*File, error) {
	if downloadDir == "" {
		return nil, errors.New("snooze: no download directory configured")
	}
	path := Path(downloadDir)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &File{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	f := &File{}
	if err := json.Unmarshal(data, f); err != nil {
		return nil, fmt.Errorf("parse %s: %w (snoozed messages are listed there; repair it rather than delete it)", path, err)
	}
	if err := fileformat.Check(path, f.Format); err != nil {
		return nil, err
	}
	return f, nil
}

// save writes the record atomically, entries in the order they are due.
func (f *File) save(downloadDir string) error {
	f.Format = fileformat.Current
	sort.SliceStable(f.Entries, func(i, j int) bool { return f.Entries[i].Until.Before(f.Entries[j].Until) })
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return fmt.Errorf("encode snooze record: %w", err)
	}
	if err := dirlock.AtomicWrite(Path(downloadDir), append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write snooze record: %w", err)
	}
	return nil
}

// update is a read-modify-write of the record under its lock. The lock is
// held for the file alone, never across a request to a server. change
// reports whether it changed anything; nothing is written when it did not.
func update(downloadDir string, change func(f *File) bool) error {
	release, err := dirlock.GuardFile(Path(downloadDir))
	defer release()
	if err != nil {
		return err
	}
	f, err := Load(downloadDir)
	if err != nil {
		return err
	}
	if !change(f) {
		return nil
	}
	return f.save(downloadDir)
}

// Put records e, replacing the account's entry for the same message, and
// returns the entry it replaced, or nil. A message snoozed again while it
// waits in the snooze folder keeps the folder it was first snoozed from
// (keepOrigin): the snooze folder is not where it came from.
func Put(downloadDir string, e Entry, keepOrigin bool) (*Entry, error) {
	var previous *Entry
	err := update(downloadDir, func(f *File) bool {
		for i, old := range f.Entries {
			if !old.same(e.Account, e.ID) {
				continue
			}
			if keepOrigin {
				e.FromFolder, e.SnoozedAt = old.FromFolder, old.SnoozedAt
			}
			previous = &old
			f.Entries[i] = e
			return true
		}
		f.Entries = append(f.Entries, e)
		return true
	})
	return previous, err
}

// Remove drops the account's entry for id, if there is one.
func Remove(downloadDir, account string, id email.ID) error {
	return update(downloadDir, func(f *File) bool {
		kept := f.Entries[:0]
		for _, e := range f.Entries {
			if !e.same(account, id) {
				kept = append(kept, e)
			}
		}
		changed := len(kept) != len(f.Entries)
		f.Entries = kept
		return changed
	})
}

// Relabel renames an account in the record, for `account rename`. It is
// idempotent: a rerun after a crash finds nothing left to rename.
func Relabel(downloadDir, from, to string) error {
	if downloadDir == "" || from == to {
		return nil
	}
	if _, err := os.Stat(Path(downloadDir)); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return update(downloadDir, func(f *File) bool {
		changed := false
		for i := range f.Entries {
			if f.Entries[i].Account == from {
				f.Entries[i].Account = to
				changed = true
			}
		}
		return changed
	})
}

// Select returns the entries each ref names, and the refs that name none.
// A ref matching several entries -- a short ID shared across accounts --
// selects them all; the caller says which it acted on.
func Select(entries []Entry, refs []string) (matched []Entry, unknown []string) {
	taken := make(map[int]bool)
	for _, ref := range refs {
		found := false
		for i, e := range entries {
			if !e.Matches(ref) {
				continue
			}
			found = true
			if !taken[i] {
				taken[i] = true
				matched = append(matched, e)
			}
		}
		if !found {
			unknown = append(unknown, ref)
		}
	}
	return matched, unknown
}
