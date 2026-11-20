// Package outbox provides an offline send queue: RFC822 messages spooled to
// disk when offline, when asynchronous queuing is requested, or to be sent at
// a later time.
//
// An entry is two files named by its ID: <id>.json (the Entry) and <id>.eml
// (the message). An entry to be sent now lives in the outbox directory itself
// and is format 1, with no format field, exactly as every earlier build wrote
// it. An entry with a due time (send_after) is format 2 and lives in the
// scheduled subdirectory: builds from before format 2 read only the outbox
// directory's own *.json files and never check a format, so one that found a
// scheduled entry beside the others would send it at once. The subdirectory is
// what keeps them from finding it; the format field is what refuses an entry
// in a format newer still.
package outbox

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/sarielhp/feral/fileformat"
	"github.com/sarielhp/feral/localdir"
)

var seqCounter uint64

// EntryFormat is the newest entry format this build reads. Format 1 (the field
// absent) is sent by the next flush; format 2 carries a due time.
const EntryFormat = 2

// formatScheduled is the format of an entry with a due time.
const formatScheduled = 2

// scheduledSubdir holds the entries with a due time; see the package comment.
const scheduledSubdir = "scheduled"

// Entry describes one spooled message in the outbox.
type Entry struct {
	ID        string    `json:"id"`
	Account   string    `json:"account"`
	To        []string  `json:"to"`
	Subject   string    `json:"subject"`
	CreatedAt time.Time `json:"created_at"`
	Attempts  int       `json:"attempts"`
	LastError string    `json:"last_error,omitempty"`
	// Format is the entry's format; zero means 1.
	Format int `json:"format,omitempty"`
	// SendAfter is when the entry becomes due; zero means at once.
	SendAfter time.Time `json:"send_after,omitzero"`
}

// Due reports whether a flush at now may send the entry.
func (e Entry) Due(now time.Time) bool {
	return e.SendAfter.IsZero() || !now.Before(e.SendAfter)
}

// TooNew reports whether a newer build wrote the entry, in a format this one
// does not read: it is shown, and refused, never sent.
func (e Entry) TooNew() bool { return e.Format > EntryFormat }

// FlushResult summarizes the transmission outcome for one queued entry.
type FlushResult struct {
	ID      string `json:"id"`
	Account string `json:"account"`
	Subject string `json:"subject"`
	Success bool   `json:"success"`
	Err     string `json:"err,omitempty"`
}

// Dir returns the canonical outbox directory path for a download root.
func Dir(downloadDir string) string {
	return localdir.Path(downloadDir, "outbox")
}

func scheduledDir(dir string) string { return filepath.Join(dir, scheduledSubdir) }

func sanitizeID(id string) string {
	clean := filepath.Base(id)
	clean = strings.TrimSuffix(clean, ".json")
	clean = strings.TrimSuffix(clean, ".eml")
	return clean
}

// generateID names an entry by time, a per-process sequence and a random
// suffix. Time and sequence alone collided across processes: two sends in the
// same millisecond from two feral processes got the same ID, and the second
// atomic write replaced the first message.
func generateID(now time.Time) string {
	seq := atomic.AddUint64(&seqCounter, 1) % 10000
	var nonce [4]byte
	_, _ = rand.Read(nonce[:])
	return fmt.Sprintf("%s-%04d-%s", now.Format("20060102-150405.000"), seq, hex.EncodeToString(nonce[:]))
}

// reserveID claims a fresh ID by creating its metadata file exclusively, so no
// other writer can take the same name even if the random suffix repeated. An
// ID that names an entry in the other of the two directories is passed over
// too, since an ID is looked up in both.
func reserveID(dir, other string, now time.Time) (string, error) {
	for range 8 {
		id := generateID(now)
		if _, err := os.Lstat(filepath.Join(other, id+".json")); err == nil {
			continue
		}
		f, err := os.OpenFile(filepath.Join(dir, id+".json"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		return id, f.Close()
	}
	return "", errors.New("could not allocate a unique outbox ID")
}

// Enqueue spools an outbound message into the outbox directory, to be sent by
// the next flush.
func Enqueue(dir, account string, to []string, subject string, raw []byte) (*Entry, error) {
	return EnqueueEntry(dir, Entry{Account: account, To: to, Subject: subject}, raw)
}

// EnqueueEntry spools raw with the account, recipients, subject and due time
// taken from e; the rest of e is filled in here. An entry with a due time goes
// to the scheduled subdirectory, in format 2.
func EnqueueEntry(dir string, e Entry, raw []byte) (*Entry, error) {
	home, other := dir, scheduledDir(dir)
	format := 0
	if !e.SendAfter.IsZero() {
		home, other, format = scheduledDir(dir), dir, formatScheduled
	}
	if err := os.MkdirAll(home, 0700); err != nil {
		return nil, fmt.Errorf("creating outbox directory: %w", err)
	}
	now := time.Now().UTC()
	id, err := reserveID(home, other, now)
	if err != nil {
		return nil, fmt.Errorf("reserving outbox entry: %w", err)
	}
	entry := &Entry{
		ID:        id,
		Account:   e.Account,
		To:        e.To,
		Subject:   e.Subject,
		CreatedAt: now,
		Format:    format,
		SendAfter: e.SendAfter,
	}

	emlPath := filepath.Join(home, id+".eml")
	jsonPath := filepath.Join(home, id+".json")
	if err = atomicWrite(emlPath, raw); err != nil {
		_ = os.Remove(jsonPath)
		return nil, fmt.Errorf("writing outbox payload: %w", err)
	}
	if err := writeMeta(home, *entry); err != nil {
		_ = os.Remove(emlPath)
		_ = os.Remove(jsonPath)
		return nil, fmt.Errorf("writing outbox metadata: %w", err)
	}
	return entry, nil
}

// List returns all queued entries in the outbox, both directories, ordered
// from oldest to newest. An entry in a format newer than this build reads is
// listed, so it is seen, with TooNew set.
func List(dir string) ([]Entry, error) {
	results, err := listIn(dir)
	if err != nil {
		return nil, err
	}
	scheduled, err := listIn(scheduledDir(dir))
	if err != nil {
		return nil, err
	}
	results = append(results, scheduled...)
	sort.Slice(results, func(i, j int) bool {
		return results[i].CreatedAt.Before(results[j].CreatedAt)
	})
	return results, nil
}

// listIn reads the entries in one directory. A file that does not parse is
// passed over, as it always was, unless its format says a newer build wrote
// it: that one is listed by its ID and format alone.
func listIn(dir string) ([]Entry, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading outbox directory: %w", err)
	}
	var results []Entry
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
			continue
		}
		item, ok := readListed(filepath.Join(dir, f.Name()))
		if ok {
			results = append(results, item)
		}
	}
	return results, nil
}

func readListed(path string) (Entry, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Entry{}, false
	}
	var item Entry
	parseErr := json.Unmarshal(data, &item)
	if parseErr != nil {
		var probe struct {
			Format int `json:"format"`
		}
		if json.Unmarshal(data, &probe) != nil || probe.Format <= EntryFormat {
			return Entry{}, false
		}
		item = Entry{Format: probe.Format}
	}
	if item.ID == "" || parseErr != nil {
		item.ID = sanitizeID(path)
	}
	return item, true
}

// locate returns the directory holding the entry id: the scheduled
// subdirectory if it is there, else the outbox directory.
func locate(dir, id string) string {
	if _, err := os.Lstat(filepath.Join(scheduledDir(dir), id+".json")); err == nil {
		return scheduledDir(dir)
	}
	return dir
}

// Get returns the metadata entry and raw RFC822 payload for a specific ID. An
// entry in a format newer than this build reads is refused.
func Get(dir, id string) (*Entry, []byte, error) {
	cleanID := sanitizeID(id)
	home := locate(dir, cleanID)
	jsonPath := filepath.Join(home, cleanID+".json")
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, nil, fmt.Errorf("reading outbox entry %q: %w", cleanID, err)
	}
	var probe struct {
		Format int `json:"format"`
	}
	if json.Unmarshal(data, &probe) == nil {
		if tooNew := fileformat.CheckAt(jsonPath, probe.Format, EntryFormat); tooNew != nil {
			return nil, nil, tooNew
		}
	}
	var item Entry
	if unmarshalErr := json.Unmarshal(data, &item); unmarshalErr != nil {
		return nil, nil, fmt.Errorf("parsing outbox entry %q: %w", cleanID, unmarshalErr)
	}

	raw, err := os.ReadFile(filepath.Join(home, cleanID+".eml"))
	if err != nil {
		return nil, nil, fmt.Errorf("reading outbox payload %q: %w", cleanID, err)
	}
	return &item, raw, nil
}

// Remove deletes an outbox entry and its associated raw message payload,
// wherever it is.
func Remove(dir, id string) error {
	cleanID := sanitizeID(id)
	for _, home := range []string{dir, scheduledDir(dir)} {
		for _, ext := range []string{".json", ".eml"} {
			if err := os.Remove(filepath.Join(home, cleanID+ext)); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}
	return nil
}

// RecordAttempt updates the retry counter and last error message for an entry.
func RecordAttempt(dir, id string, attemptErr error) error {
	item, _, err := Get(dir, id)
	if err != nil {
		return err
	}
	item.Attempts++
	item.LastError = ""
	if attemptErr != nil {
		item.LastError = attemptErr.Error()
	}
	return writeMeta(locate(dir, sanitizeID(id)), *item)
}

// writeMeta writes an entry's metadata into home, the directory it lives in.
func writeMeta(home string, item Entry) error {
	data, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling outbox metadata: %w", err)
	}
	return atomicWrite(filepath.Join(home, sanitizeID(item.ID)+".json"), data)
}

func atomicWrite(dest string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, dest)
}

// Relabel moves every queued entry for the account from to the account to,
// so a queued message is still sent from the account after it is renamed. It
// returns how many entries it changed, and is idempotent. An entry in a format
// newer than this build reads is refused rather than rewritten, which would
// drop whatever that format added.
func Relabel(dir, from, to string) (int, error) {
	entries, err := List(dir)
	if err != nil {
		return 0, err
	}
	changed := 0
	for _, item := range entries {
		home := locate(dir, sanitizeID(item.ID))
		// An entry this build could not read names no account it can trust.
		if item.TooNew() && (item.Account == "" || strings.EqualFold(item.Account, from)) {
			return changed, fileformat.CheckAt(filepath.Join(home, sanitizeID(item.ID)+".json"), item.Format, EntryFormat)
		}
		if !strings.EqualFold(item.Account, from) || item.Account == to {
			continue
		}
		item.Account = to
		if err := writeMeta(home, item); err != nil {
			return changed, err
		}
		changed++
	}
	return changed, nil
}
