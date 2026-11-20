package watcher

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/sarielhp/feral/cache/dirlock"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/fileformat"
)

// maxSeen bounds the per-account watermark. Each poll records the newest
// messages it fetched, so an old entry falls out of the window on its own; the
// cap only stops a state file growing without limit if a backend keeps
// returning an ever-longer list.
const maxSeen = 500

// State is the watcher's watermark: what it has already seen, per account.
type State struct {
	Format   int                `json:"format,omitempty"` // file format (see fileformat); absent means 1
	Accounts map[string]Account `json:"accounts"`
}

// Account is one account's watermark.
type Account struct {
	LastChecked time.Time `json:"last_checked_timestamp"`
	LastSeenIDs []string  `json:"last_seen_ids"`
}

// stateFile names the watermark beneath a download directory.
func stateFile(downloadDir string) string {
	return filepath.Join(downloadDir, "watcher", "state.json")
}

// lockFile names the lock a running watcher holds beneath a download directory.
func lockFile(downloadDir string) string {
	return filepath.Join(downloadDir, "watcher", "watch.lock")
}

// LoadState reads the watermark, treating a missing file as an empty one. A
// file that exists but does not parse is an error: the watcher must not answer
// "nothing seen yet" to a damaged file, or its next poll would announce every
// message in the account.
func LoadState(downloadDir string) (*State, error) {
	data, err := os.ReadFile(stateFile(downloadDir))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &State{Accounts: map[string]Account{}}, nil
		}
		return nil, fmt.Errorf("read watcher state: %w", err)
	}
	state := &State{}
	if err := json.Unmarshal(data, state); err != nil {
		return nil, fmt.Errorf("parse watcher state %s: %w", stateFile(downloadDir), err)
	}
	if err := fileformat.Check(stateFile(downloadDir), state.Format); err != nil {
		return nil, err
	}
	if state.Accounts == nil {
		state.Accounts = map[string]Account{}
	}
	return state, nil
}

// Save writes the watermark atomically, so a crash mid-write cannot leave a
// half-written file for the next run to parse.
func (s *State) Save(downloadDir string) error {
	s.Format = fileformat.Current
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("encode watcher state: %w", err)
	}
	data = append(data, '\n')
	if err := dirlock.AtomicWrite(stateFile(downloadDir), data, 0600); err != nil {
		return fmt.Errorf("write watcher state: %w", err)
	}
	return nil
}

// Known reports whether the account has a watermark yet. An account the watcher
// has never polled is on its first run, which records what it finds without
// announcing it.
func (s *State) Known(name string) bool {
	_, ok := s.Accounts[name]
	return ok
}

// Unseen returns the ids the account has not seen, in the order given.
func (s *State) Unseen(name string, ids []email.ID) []email.ID {
	account, ok := s.Accounts[name]
	if !ok {
		return ids
	}
	seen := make(map[string]bool, len(account.LastSeenIDs))
	for _, id := range account.LastSeenIDs {
		seen[id] = true
	}
	var unseen []email.ID
	for _, id := range ids {
		if !seen[string(id)] {
			unseen = append(unseen, id)
		}
	}
	return unseen
}

// MarkSeen records ids as seen and stamps the account's last-checked time. The
// newest ids are kept ahead of older ones, and the list is capped.
func (s *State) MarkSeen(name string, ids []email.ID, at time.Time) {
	if s.Accounts == nil {
		s.Accounts = map[string]Account{}
	}
	account := s.Accounts[name]

	seen := make([]string, 0, len(ids)+len(account.LastSeenIDs))
	have := make(map[string]bool, len(ids)+len(account.LastSeenIDs))
	for _, id := range ids {
		value := string(id)
		if !have[value] {
			have[value] = true
			seen = append(seen, value)
		}
	}
	for _, value := range account.LastSeenIDs {
		if !have[value] {
			have[value] = true
			seen = append(seen, value)
		}
	}
	if len(seen) > maxSeen {
		seen = seen[:maxSeen]
	}

	account.LastSeenIDs = seen
	account.LastChecked = at
	s.Accounts[name] = account
}

// HoldWatchLock takes the lock a running watcher holds beneath downloadDir, so
// no watcher -- `feral watch`, or the one a TUI starts -- runs until release
// is called. It returns ErrAlreadyWatching when one is running now. A change
// that renames the message IDs the watcher keeps, such as migrating an
// account's cache, takes it: a watcher running beside it would record the old
// IDs again.
func HoldWatchLock(downloadDir string) (release func(), err error) {
	release, err = dirlock.TryLock(lockFile(downloadDir))
	if errors.Is(err, dirlock.ErrHeld) {
		return nil, ErrAlreadyWatching
	}
	if err != nil {
		return nil, fmt.Errorf("watch lock: %w", err)
	}
	return release, nil
}

// RekeySeen renames the account's seen message IDs through rename, under the
// cache lock, dropping any it does not map. Without it, an account whose IDs
// changed would have its whole inbox announced as new on the next poll.
func RekeySeen(downloadDir, name string, rename map[email.ID]email.ID) error {
	release, err := dirlock.Guard(downloadDir)
	if release != nil {
		defer release()
	}
	if err != nil {
		return err
	}
	state, err := LoadState(downloadDir)
	if err != nil {
		return err
	}
	account, ok := state.Accounts[name]
	if !ok {
		return nil
	}
	seen := make([]string, 0, len(account.LastSeenIDs))
	for _, id := range account.LastSeenIDs {
		if newID, found := rename[email.ID(id)]; found {
			seen = append(seen, string(newID))
		}
	}
	// None survived: forget the account instead, so the next poll records the
	// inbox as seen, as for an account it has never watched. An entry left
	// with no IDs would announce the whole inbox as new.
	if len(seen) == 0 {
		delete(state.Accounts, name)
		return state.Save(downloadDir)
	}
	account.LastSeenIDs = seen
	state.Accounts[name] = account
	return state.Save(downloadDir)
}

// SeenIDs returns the account's seen message IDs.
func SeenIDs(downloadDir, name string) ([]email.ID, error) {
	state, err := LoadState(downloadDir)
	if err != nil {
		return nil, err
	}
	ids := make([]email.ID, 0, len(state.Accounts[name].LastSeenIDs))
	for _, id := range state.Accounts[name].LastSeenIDs {
		ids = append(ids, email.ID(id))
	}
	return ids, nil
}

// RenameAccount moves the account's watermark from the name from to the name
// to, under the cache lock, so a renamed account is not polled as one never
// seen -- which records its inbox silently, but forgets what was announced.
// It is idempotent: a state with no entry under from is left alone.
func RenameAccount(downloadDir, from, to string) error {
	release, err := dirlock.Guard(downloadDir)
	if release != nil {
		defer release()
	}
	if err != nil {
		return err
	}
	state, err := LoadState(downloadDir)
	if err != nil {
		return err
	}
	account, ok := state.Accounts[from]
	if !ok || from == to {
		return nil
	}
	delete(state.Accounts, from)
	if _, exists := state.Accounts[to]; !exists {
		state.Accounts[to] = account
	}
	return state.Save(downloadDir)
}
