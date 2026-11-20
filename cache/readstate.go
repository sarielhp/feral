package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/fileformat"

	"github.com/sarielhp/feral/cache/dirlock"
)

// stateFile names one of the two per-account boolean state caches -- read marks
// and replied-to marks -- together with the labels its errors carry. The two
// caches are the same format in different files, so every read, write and
// locked update is one implementation parameterized by this.
type stateFile struct {
	name     string // filename under the account cache
	load     string // label in read/decode errors, e.g. "read-state"
	preserve string // label in the preserve-unreadable error, e.g. "read state"
}

var (
	readStateFile    = stateFile{name: "seen_cache.json", load: "read-state", preserve: "read state"}
	repliedStateFile = stateFile{name: "replied_cache.json", load: "replied-state", preserve: "replied state"}
)

func (sf stateFile) path(downloadDir string) string {
	return filepath.Join(downloadDir, sf.name)
}

func loadStateStrict(downloadDir string, sf stateFile) (map[email.ID]bool, error) {
	if downloadDir == "" {
		return make(map[email.ID]bool), nil
	}
	data, err := os.ReadFile(sf.path(downloadDir))
	if errors.Is(err, os.ErrNotExist) {
		return make(map[email.ID]bool), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s cache: %w", sf.load, err)
	}
	if err := fileformat.RefuseWrapped(sf.path(downloadDir), data); err != nil {
		return nil, err
	}
	state := make(map[email.ID]bool)
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("decode %s cache: %w", sf.load, err)
	}
	return state, nil
}

func saveState(downloadDir string, sf stateFile, ids map[email.ID]bool) error {
	if downloadDir == "" {
		return nil
	}
	data, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	return dirlock.AtomicWrite(sf.path(downloadDir), data, 0600)
}

// updateState executes apply on the current state under a directory lock,
// reloading persistent state strictly so corrupted state is preserved rather
// than overwritten and concurrent committed writes are not lost.
func updateState(account cachedir.Account, sf stateFile, apply func(map[email.ID]bool)) error {
	downloadDir := account.Path()
	if downloadDir == "" {
		return nil
	}
	release := guard(downloadDir)
	defer release()

	state, err := loadStateStrict(downloadDir, sf)
	if err != nil {
		return fmt.Errorf("preserve unreadable %s: %w", sf.preserve, err)
	}
	apply(state)
	return saveState(downloadDir, sf, state)
}

// guard acquires the cache lock, returning the release function. A lock failure
// is logged and tolerated: the in-process mutex is still held, so this degrades
// to in-process locking rather than failing the user's command.
func guard(downloadDir string) func() {
	release, err := dirlock.Guard(downloadDir)
	if err != nil {
		slog.Debug("cache lock unavailable, continuing with in-process lock only",
			slog.String("dir", downloadDir), slog.Any("error", err))
	}
	return release
}

func loadReadStateStrict(downloadDir string) (map[email.ID]bool, error) {
	return loadStateStrict(downloadDir, readStateFile)
}

func loadReadState(downloadDir string) map[email.ID]bool {
	state, err := loadReadStateStrict(downloadDir)
	if err != nil {
		slog.Warn("failed to load read state", slog.String("dir", downloadDir), slog.Any("error", err))
		return make(map[email.ID]bool)
	}
	return state
}

func saveReadState(downloadDir string, readIDs map[email.ID]bool) error {
	return saveState(downloadDir, readStateFile, readIDs)
}

// LoadReadState returns the account's read marks as the set of IDs recorded
// read. A missing or unreadable cache yields an empty set rather than an error,
// so a damaged file never stops a message being shown.
func LoadReadState(account cachedir.Account) map[email.ID]bool {
	downloadDir := account.Path()
	return loadReadState(downloadDir)
}

// UpdateReadState writes a server's read state into the cache: it executes
// apply on the current read state under a directory lock, reloading persistent
// state strictly so corrupted state is preserved rather than overwritten and
// concurrent committed writes are not lost. It is how a folder sync records
// what the server reported, so a mark still pending in the ledger keeps its
// local value: the listing may predate it, or its server call may have failed
// and be waiting for a retry. See pendingreads.go.
func UpdateReadState(account cachedir.Account, apply func(map[email.ID]bool)) error {
	return updateReadAndPending(account, func(state map[email.ID]bool, pending map[email.ID]PendingRead) {
		apply(state)
		for id, p := range pending {
			setRead(state, id, p.Read)
		}
	})
}

func loadRepliedStateStrict(downloadDir string) (map[email.ID]bool, error) {
	return loadStateStrict(downloadDir, repliedStateFile)
}

// LoadRepliedState returns the account's replied-to marks as the set of IDs
// recorded replied. A missing or unreadable cache yields an empty set rather
// than an error.
func LoadRepliedState(account cachedir.Account) map[email.ID]bool {
	downloadDir := account.Path()
	state, err := loadRepliedStateStrict(downloadDir)
	if err != nil {
		slog.Warn("failed to load replied state", slog.String("dir", downloadDir), slog.Any("error", err))
		return make(map[email.ID]bool)
	}
	return state
}

func saveRepliedState(downloadDir string, repliedIDs map[email.ID]bool) error {
	return saveState(downloadDir, repliedStateFile, repliedIDs)
}

// UpdateRepliedState executes apply on the current replied state under a directory lock.
func UpdateRepliedState(account cachedir.Account, apply func(map[email.ID]bool)) error {
	return updateState(account, repliedStateFile, apply)
}

// MarkIDsRead records ids as read, a mark the user made here. It locks
// downloadDir, reloads persistent state so concurrent committed writes are not
// lost, merges snapshot and ids, and saves atomically. Each of ids is also
// recorded as pending until ConfirmReads, so a server sync cannot undo it.
func MarkIDsRead(account cachedir.Account, ids []email.ID, snapshot map[email.ID]bool) error {
	return updateReadAndPending(account, func(state map[email.ID]bool, pending map[email.ID]PendingRead) {
		for id, marked := range snapshot {
			if marked {
				state[id] = true
			}
		}
		recordPending(state, pending, ids, true)
	})
}

// MarkIDsUnread clears ids from the read marks, so the messages read as new
// again. It reloads persistent state under the directory lock before removing
// them, so a concurrent committed write is not lost. Like MarkIDsRead, it
// records each as pending until the server confirms it.
func MarkIDsUnread(account cachedir.Account, ids []email.ID) error {
	return recordLocalReads(account, ids, false)
}

// MarkIDsReplied records ids as replied-to. See MarkIDsRead.
func MarkIDsReplied(account cachedir.Account, ids []email.ID, snapshot map[email.ID]bool) error {
	return markIDs(account, repliedStateFile, ids, snapshot)
}

func markIDs(account cachedir.Account, sf stateFile, ids []email.ID, snapshot map[email.ID]bool) error {
	return updateState(account, sf, func(current map[email.ID]bool) {
		for id, marked := range snapshot {
			if marked {
				current[id] = true
			}
		}
		for _, id := range ids {
			current[id] = true
		}
	})
}
