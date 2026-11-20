package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/cache/dirlock"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/fileformat"
)

// The pending-reads ledger is the read and unread marks the user made here
// that the server has not yet confirmed, one file per account beside the read
// state. It exists because the read state is a cache of the server's: every
// folder sync wrote the server's answer over the local one, so a mark whose
// server call failed, or had not landed yet when a sync's listing was taken,
// was silently undone and the message showed N again.
//
// A mark is recorded here in the same locked update that records it in the
// read state. A server sync (UpdateReadState) keeps a pending mark's local
// value; the mark is confirmed when its server call succeeds, and dropped
// after a while either way, so a change made in another client wins in the end.
const pendingReadsFile = "pending_reads.json"

const (
	// PendingReadTTL is how long an unconfirmed mark outranks the server.
	// After it, the server's state wins: the mark either reached it by then or
	// is not going to, and a change made since in another client must show.
	PendingReadTTL = 24 * time.Hour
	// PendingReadMaxTries is how many failed retries drop a mark.
	PendingReadMaxTries = 10
	// confirmedReadGrace is how long a confirmed mark still outranks a sync. A
	// listing taken before the server applied the change -- Gmail's search
	// index trails its modifications -- would otherwise undo it once.
	confirmedReadGrace = 2 * time.Minute
)

// PendingRead is one local read mark awaiting the server.
type PendingRead struct {
	Read      bool      `json:"read"`
	At        time.Time `json:"at"`
	Tries     int       `json:"tries,omitempty"`
	Confirmed bool      `json:"confirmed,omitempty"`
}

func (p PendingRead) expired(now time.Time) bool {
	if p.Confirmed {
		return now.Sub(p.At) > confirmedReadGrace
	}
	return now.Sub(p.At) > PendingReadTTL || p.Tries >= PendingReadMaxTries
}

func pendingPath(downloadDir string) string {
	return filepath.Join(downloadDir, pendingReadsFile)
}

// loadPending reads the ledger. A damaged one is logged and treated as empty:
// it only ever protects marks for a day, and refusing it would stop every
// sync and every mark for that account. One a newer build wrote is refused
// instead -- treating it as empty would overwrite it.
func loadPending(downloadDir string) (map[email.ID]PendingRead, error) {
	pending := make(map[email.ID]PendingRead)
	data, err := os.ReadFile(pendingPath(downloadDir))
	if errors.Is(err, os.ErrNotExist) {
		return pending, nil
	}
	if err == nil {
		if refuse := fileformat.RefuseWrapped(pendingPath(downloadDir), data); refuse != nil {
			return nil, refuse
		}
		err = json.Unmarshal(data, &pending)
	}
	if err != nil {
		slog.Warn("ignoring unreadable pending-reads ledger", slog.String("dir", downloadDir), slog.Any("error", err))
		return make(map[email.ID]PendingRead), nil
	}
	return pending, nil
}

func savePending(downloadDir string, pending map[email.ID]PendingRead) error {
	if len(pending) == 0 {
		err := os.Remove(pendingPath(downloadDir))
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	data, err := json.Marshal(pending)
	if err != nil {
		return err
	}
	return dirlock.AtomicWrite(pendingPath(downloadDir), data, 0600)
}

// updateReadAndPending runs apply on the read state and the ledger together,
// under the account's lock, dropping expired ledger entries first, and saves
// both.
func updateReadAndPending(account cachedir.Account, apply func(state map[email.ID]bool, pending map[email.ID]PendingRead)) error {
	downloadDir := account.Path()
	if downloadDir == "" {
		return nil
	}
	release := guard(downloadDir)
	defer release()

	state, err := loadStateStrict(downloadDir, readStateFile)
	if err != nil {
		return fmt.Errorf("preserve unreadable %s: %w", readStateFile.preserve, err)
	}
	pending, err := loadPending(downloadDir)
	if err != nil {
		return err
	}
	now := time.Now()
	for id, p := range pending {
		if p.expired(now) {
			delete(pending, id)
		}
	}
	apply(state, pending)
	if err := saveState(downloadDir, readStateFile, state); err != nil {
		return err
	}
	return savePending(downloadDir, pending)
}

// setRead writes one read decision into the read state: read IDs are present
// and true, unread IDs absent, as MarkIDsUnread has always left them.
func setRead(state map[email.ID]bool, id email.ID, read bool) {
	if read {
		state[id] = true
		return
	}
	delete(state, id)
}

// recordLocalReads marks ids read or unread locally and records each as
// pending until the server confirms it.
func recordLocalReads(account cachedir.Account, ids []email.ID, read bool) error {
	return updateReadAndPending(account, func(state map[email.ID]bool, pending map[email.ID]PendingRead) {
		recordPending(state, pending, ids, read)
	})
}

func recordPending(state map[email.ID]bool, pending map[email.ID]PendingRead, ids []email.ID, read bool) {
	now := time.Now()
	for _, id := range ids {
		setRead(state, id, read)
		pending[id] = PendingRead{Read: read, At: now}
	}
}

// LoadPendingReads returns the account's local read marks the server has not
// confirmed, for retrying. Confirmed and expired entries are left out.
func LoadPendingReads(account cachedir.Account) map[email.ID]PendingRead {
	downloadDir := account.Path()
	out := make(map[email.ID]PendingRead)
	if downloadDir == "" {
		return out
	}
	now := time.Now()
	all, err := loadPending(downloadDir)
	if err != nil {
		return out
	}
	for id, p := range all {
		if !p.Confirmed && !p.expired(now) {
			out[id] = p
		}
	}
	return out
}

// ConfirmReads records that the server accepted the marks on ids. Each keeps
// outranking a sync for a short grace, against a listing taken before the
// server applied it, and is then dropped.
func ConfirmReads(account cachedir.Account, ids []email.ID) error {
	return updatePending(account, ids, func(p PendingRead) (PendingRead, bool) {
		p.Confirmed, p.At = true, time.Now()
		return p, true
	})
}

// NoteReadRetryFailed counts a failed retry of the marks on ids; a mark that
// has failed PendingReadMaxTries times is dropped.
func NoteReadRetryFailed(account cachedir.Account, ids []email.ID) error {
	return updatePending(account, ids, func(p PendingRead) (PendingRead, bool) {
		p.Tries++
		return p, p.Tries < PendingReadMaxTries
	})
}

func updatePending(account cachedir.Account, ids []email.ID, change func(PendingRead) (PendingRead, bool)) error {
	return updateReadAndPending(account, func(_ map[email.ID]bool, pending map[email.ID]PendingRead) {
		for _, id := range ids {
			p, ok := pending[id]
			if !ok {
				continue
			}
			if changed, keep := change(p); keep {
				pending[id] = changed
				continue
			}
			delete(pending, id)
		}
	})
}
