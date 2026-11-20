package transfer

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/fileformat"
)

// JournalFile is the journal's name in the source account's cache. cache/meta
// and outlookids look for it by this name.
const JournalFile = "splice_journal.json"

// JournalFormat is the journal's own file format. Format 1 had one source
// folder for the whole file and keyed entries by message ID alone, so a run
// over another folder of the same account recovered the first run's entries
// against the wrong folder. Format 2 records the source folder in each entry
// and keys by (folder, ID).
const JournalFormat = 2

// Entry is a message confirmed on the destination and not yet removed from
// its source.
type Entry struct {
	SourceFolder string `json:"source_folder"`
	SourceID     string `json:"source_id"`
	DestID       string `json:"dest_id"`
	DestFolder   string `json:"dest_folder"`
	// Hash is the SHA-256 of the bytes that were uploaded. A source whose IDs
	// can change under it -- an mbox file, where a duplicate Message-ID's -N
	// suffix depends on what precedes it -- removes a message only if it still
	// hashes to this, so recovery never deletes a copy that was not uploaded.
	Hash string `json:"hash,omitempty"`
}

func entryKey(folder, id string) string { return folder + "\n" + id }

// Journal is the crash record of a move between accounts: an entry is written
// once a message is confirmed on the destination, and dropped once it is gone
// from the source. An entry left behind is a removal an interrupted run never
// made, which Recover finishes.
type Journal struct {
	mu      sync.Mutex
	path    string
	Format  int              `json:"format"`
	Entries map[string]Entry `json:"entries"`
}

// journalFile is the shape on disk, both formats: format 1 kept the source
// folder once, at the top.
type journalFile struct {
	Format       int              `json:"format,omitempty"`
	SourceFolder string           `json:"source_folder,omitempty"`
	Entries      map[string]Entry `json:"entries"`
}

// OpenJournal loads the journal in the account's cache, or starts an empty
// one. A damaged journal is an error, never an empty one: an empty journal
// would forget removals that are owed.
func OpenJournal(acct cachedir.Account) (*Journal, error) {
	fp := filepath.Join(acct.Path(), JournalFile)
	j := &Journal{path: fp, Format: JournalFormat, Entries: make(map[string]Entry)}
	data, err := os.ReadFile(fp)
	if errors.Is(err, os.ErrNotExist) {
		return j, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read splice journal %q: %w", fp, err)
	}
	var onDisk journalFile
	if err := json.Unmarshal(data, &onDisk); err != nil {
		return nil, fmt.Errorf("failed to parse existing splice journal %q: %w", fp, err)
	}
	if err := fileformat.CheckAt(fp, onDisk.Format, JournalFormat); err != nil {
		return nil, err
	}
	for key, e := range onDisk.Entries {
		if onDisk.Format < 2 {
			// Format 1: keyed by source ID, one folder for all.
			e.SourceFolder = onDisk.SourceFolder
			if e.SourceID == "" {
				e.SourceID = key
			}
		}
		j.Entries[entryKey(e.SourceFolder, e.SourceID)] = e
	}
	return j, nil
}

// RecordUpload records that e's message is confirmed on the destination.
func (j *Journal) RecordUpload(e Entry) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.Entries[entryKey(e.SourceFolder, e.SourceID)] = e
	return j.save()
}

// RecordDeleted drops the entry for id in folder once it is gone from the
// source, and removes the file when nothing is left.
func (j *Journal) RecordDeleted(folder, id string) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	delete(j.Entries, entryKey(folder, id))
	if len(j.Entries) == 0 {
		return j.removeFile()
	}
	return j.save()
}

// Pending is every entry still owed a removal, in a stable order.
func (j *Journal) Pending() []Entry {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := make([]Entry, 0, len(j.Entries))
	for _, e := range j.Entries {
		out = append(out, e)
	}
	slices.SortFunc(out, func(a, b Entry) int {
		return cmp.Or(cmp.Compare(a.SourceFolder, b.SourceFolder), cmp.Compare(a.SourceID, b.SourceID))
	})
	return out
}

func (j *Journal) save() error {
	j.Format = JournalFormat
	data, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		return err
	}
	tmp := j.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, j.path)
}

// Close removes the journal file if nothing is owed.
func (j *Journal) Close() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if len(j.Entries) == 0 {
		return j.removeFile()
	}
	return nil
}

// removeFile deletes the journal file; one already gone is not an error.
func (j *Journal) removeFile() error {
	if err := os.Remove(j.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
