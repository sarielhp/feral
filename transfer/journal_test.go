package transfer

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/fileformat"
	"github.com/sarielhp/feral/label"
	"github.com/sarielhp/feral/report"
)

// removal records what a Remover was asked to do.
type removal struct {
	folder label.Folder
	ids    []email.ID
}

func recordingRemover(calls *[]removal, fail error) Remover {
	return func(folder label.Folder, msgs []Removal) ([]email.ID, error) {
		ids := make([]email.ID, len(msgs))
		for i, m := range msgs {
			ids[i] = m.ID
		}
		*calls = append(*calls, removal{folder: folder, ids: ids})
		return nil, fail
	}
}

func TestJournalRecovery(t *testing.T) {
	tempDir := t.TempDir()
	journal, err := OpenJournal(cachedir.At(tempDir))
	if err != nil {
		t.Fatalf("failed to open journal: %v", err)
	}
	_ = journal.RecordUpload(Entry{SourceFolder: "source_folder", SourceID: "src_001", DestID: "dest_001", DestFolder: "dest_folder"})
	_ = journal.RecordUpload(Entry{SourceFolder: "source_folder", SourceID: "src_002", DestID: "dest_002", DestFolder: "dest_folder"})

	reopened, err := OpenJournal(cachedir.At(tempDir))
	if err != nil {
		t.Fatalf("failed to reopen journal: %v", err)
	}
	if len(reopened.Entries) != 2 {
		t.Fatalf("expected 2 journal entries, got %d", len(reopened.Entries))
	}

	var calls []removal
	Recover(reopened, recordingRemover(&calls, nil), report.Discard())
	if len(reopened.Entries) != 0 {
		t.Errorf("expected journal to be empty after recovery, got %d entries", len(reopened.Entries))
	}
	if len(calls) != 1 || calls[0].folder.String() != "source_folder" || len(calls[0].ids) != 2 {
		t.Errorf("recovery removed %+v; want one batch of 2 from source_folder", calls)
	}
	if _, err := os.Stat(filepath.Join(tempDir, JournalFile)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("an empty journal was left on disk: %v", err)
	}
}

// Each entry is recovered against its own source folder. Format 1 kept one
// folder for the whole file, so a run over another folder recovered the first
// run's entries against the wrong one.
func TestJournalRecoversEachEntryInItsOwnFolder(t *testing.T) {
	journal, err := OpenJournal(cachedir.At(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	_ = journal.RecordUpload(Entry{SourceFolder: "INBOX", SourceID: "same", DestFolder: "keep/a"})
	_ = journal.RecordUpload(Entry{SourceFolder: "receipts", SourceID: "same", DestFolder: "keep/b"})
	if len(journal.Entries) != 2 {
		t.Fatalf("one ID in two folders collapsed into %d entries", len(journal.Entries))
	}

	var calls []removal
	Recover(journal, recordingRemover(&calls, nil), report.Discard())
	if len(calls) != 2 || calls[0].folder.String() != "INBOX" || calls[1].folder.String() != "receipts" {
		t.Errorf("recovery removed %+v; want one batch from INBOX and one from receipts", calls)
	}
}

// A removal that fails during recovery leaves its entries owed.
func TestJournalRecoveryKeepsWhatFailed(t *testing.T) {
	journal, err := OpenJournal(cachedir.At(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	_ = journal.RecordUpload(Entry{SourceFolder: "INBOX", SourceID: "a"})
	var calls []removal
	Recover(journal, recordingRemover(&calls, errors.New("offline")), report.Discard())
	if len(journal.Pending()) != 1 {
		t.Errorf("a failed removal was dropped from the journal")
	}
}

// A format-1 journal, from a build before entries carried their folder, is
// read with its top-level folder given to every entry.
func TestJournalReadsFormatOne(t *testing.T) {
	dir := t.TempDir()
	legacy := `{"source_folder":"received","entries":{"m1":{"source_id":"m1","dest_id":"d1","dest_folder":"keep/x"}}}`
	if err := os.WriteFile(filepath.Join(dir, JournalFile), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	journal, err := OpenJournal(cachedir.At(dir))
	if err != nil {
		t.Fatalf("a format-1 journal was refused: %v", err)
	}
	got := journal.Pending()
	if len(got) != 1 || got[0].SourceFolder != "received" || got[0].SourceID != "m1" || got[0].DestID != "d1" {
		t.Errorf("format-1 journal read as %+v", got)
	}
}

// A journal a newer build wrote is refused: it is an interrupted move's only
// record, and misreading it could leave a duplicate.
func TestOpenJournalRefusesNewerFormat(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, JournalFile), []byte(`{"format":3,"entries":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := OpenJournal(cachedir.At(dir))
	var tooNew *fileformat.TooNewError
	if !errors.As(err, &tooNew) {
		t.Errorf("a newer journal was accepted (err %v)", err)
	}
}

func TestOpenJournalCorruptedJSON(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, JournalFile), []byte(`{invalid-json`), 0600); err != nil {
		t.Fatalf("failed to write corrupted journal: %v", err)
	}
	_, err := OpenJournal(cachedir.At(tempDir))
	if err == nil {
		t.Fatal("expected error opening corrupted journal, got nil")
	}
	if !strings.Contains(err.Error(), "failed to parse existing splice journal") {
		t.Fatalf("unexpected error message: %v", err)
	}
}
