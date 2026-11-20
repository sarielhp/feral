package transfer

import (
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/label"
	"github.com/sarielhp/feral/mailclient"
	"github.com/sarielhp/feral/report"
)

// Removal is one message to take out of the source: its ID there and the
// SHA-256 (hex) of the bytes that reached the destination, empty if unknown.
type Removal struct {
	ID   email.ID
	Hash string
}

// Remover takes messages out of a folder of the source account once they are
// confirmed on the destination. What "out" means is the caller's: a server
// account moves them to its trash (TrashRemover's job in the caller, which
// knows where trash is), an mbox file deletes them (DeleteRemover). It is
// given whole batches, so a source that rewrites a file can do it once.
//
// It returns the messages it deliberately left in the source, such as one
// whose content shows it is not the message that was moved: nothing more is
// owed for them, but they were not moved either.
type Remover func(folder label.Folder, msgs []Removal) (kept []email.ID, err error)

// ErrLeftInPlace reports a message the Remover left in the source.
var ErrLeftInPlace = errors.New("left in the source: it is not the message that was moved")

// Options is how a Mover removes and reports.
type Options struct {
	// Remove takes moved messages out of the source. Nil makes the Mover
	// copy-only: Move refuses, and no journal is opened.
	Remove Remover
	// Defer holds removals until Flush, which makes one Remove call for all of
	// them: what an mbox source needs, since each call rewrites the file.
	Defer bool
	// Reporter receives journal recovery's progress; nil discards it.
	Reporter report.Reporter
}

// Item is one message to transfer: its source ID, its parsed form and raw
// bytes, and the destination folder chosen for it.
type Item struct {
	ID     email.ID
	Parsed *email.Email
	Raw    []byte
	Dest   label.Folder
}

// Stage is the step a transfer stopped at.
type Stage int

// The steps of a transfer, in order. StageNone means it did not stop.
const (
	StageNone Stage = iota
	StageEnsure
	StageUpload
	StageConfirm
	StageRemove
)

// Result is what happened to one Item.
type Result struct {
	// Existed is true when the message was already in the destination, so
	// nothing was uploaded.
	Existed bool
	// DestID is the message's ID on the destination, when the upload or copy
	// returned one.
	DestID email.ID
	// Failed is the step that failed, or StageNone.
	Failed Stage
	Err    error
}

// OK reports whether the transfer completed.
func (r Result) OK() bool { return r.Failed == StageNone }

func failed(stage Stage, err error) Result { return Result{Failed: stage, Err: err} }

// Mover transfers messages out of one source folder into folders of a
// destination account. It deduplicates on the destination by Message-ID
// (Subject+Date only when a side lacks one), so an upload is idempotent and a
// rerun after a crash never duplicates; and it removes a message from the
// source only after confirming it arrived, recording each confirmed message in
// a journal until it is removed.
type Mover struct {
	src, dst mailclient.MailClient
	source   label.Folder
	opts     Options
	dests    FolderCache
	ensured  map[label.Folder]bool
	journal  *Journal  // nil when copying, or when nothing may be recorded
	pending  []Removal // confirmed and owed a removal, under Defer
}

// NewMover prepares a transfer from source on src to dst. With a Remover it
// opens the source account's journal and first finishes any removals an
// interrupted run left owed. Under --read-only or --dry-run on the source no
// journal is kept: removal is simulated there, and a journal would record a
// change that never happened.
func NewMover(src, dst mailclient.MailClient, source label.Folder, opts Options) (*Mover, error) {
	if opts.Reporter == nil {
		opts.Reporter = report.Discard()
	}
	m := &Mover{
		src: src, dst: dst, source: source, opts: opts,
		dests:   make(FolderCache),
		ensured: make(map[label.Folder]bool),
	}
	if opts.Remove == nil || !recordable(src) {
		return m, nil
	}
	j, err := OpenJournal(src.Config().AccountCache)
	if err != nil {
		return nil, fmt.Errorf("cross-account move refusing to run without its crash journal: %w", err)
	}
	m.journal = j
	Recover(j, opts.Remove, opts.Reporter)
	return m, nil
}

// recordable reports whether the source account may have its journal written.
func recordable(src mailclient.MailClient) bool {
	cfg := src.Config()
	return cfg != nil && !cfg.AccountCache.IsZero() && !cfg.ReadOnly && !cfg.DryRun
}

// Close releases the journal, removing its file when nothing is owed.
func (m *Mover) Close() error {
	if m.journal == nil {
		return nil
	}
	return m.journal.Close()
}

// Copy puts it in its destination folder, unless a message the same as it is
// already there, and leaves the source alone.
func (m *Mover) Copy(it Item) Result {
	if InDest(m.dst, it.Dest, it.ID, it.Parsed, m.dests) {
		return Result{Existed: true}
	}
	if err := m.ensureDest(it.Dest); err != nil {
		return failed(StageEnsure, err)
	}
	destID, err := m.copyOrUpload(it)
	if err != nil {
		return failed(StageUpload, err)
	}
	m.dests.For(it.Dest).Add(it.ID, destID, it.Parsed)
	return Result{DestID: destID}
}

// Move puts it in its destination folder, confirms it arrived, and only then
// removes it from the source.
func (m *Mover) Move(it Item) Result {
	if m.opts.Remove == nil {
		return failed(StageRemove, errors.New("this transfer was opened copy-only"))
	}
	if err := m.ensureDest(it.Dest); err != nil {
		return failed(StageEnsure, err)
	}
	exists := InDest(m.dst, it.Dest, it.ID, it.Parsed, m.dests)
	var destID email.ID
	if !exists {
		var err error
		if destID, err = m.copyOrUpload(it); err != nil {
			return failed(StageUpload, err)
		}
	}
	m.dests.For(it.Dest).Add(it.ID, destID, it.Parsed)

	if destID == "" && !exists {
		if confirmed, err := Confirm(m.dst, it.Dest, it.ID, it.Parsed); !confirmed || err != nil {
			return failed(StageConfirm, err)
		}
	}

	removal := Removal{ID: it.ID, Hash: ContentHash(it.Raw)}
	m.record(Entry{SourceFolder: m.source.String(), SourceID: string(it.ID), DestID: string(destID), DestFolder: it.Dest.String(), Hash: removal.Hash})
	if m.opts.Defer {
		m.pending = append(m.pending, removal)
		return Result{Existed: exists, DestID: destID}
	}
	kept, err := m.opts.Remove(m.source, []Removal{removal})
	if err != nil {
		return Result{Existed: exists, DestID: destID, Failed: StageRemove, Err: err}
	}
	m.forget(m.source.String(), string(it.ID))
	if len(kept) > 0 {
		return Result{Existed: exists, DestID: destID, Failed: StageRemove, Err: ErrLeftInPlace}
	}
	return Result{Existed: exists, DestID: destID}
}

// Flush makes the removals Defer held back, in one Remove call, and returns
// the IDs removed and those the Remover left in the source. On failure nothing
// is forgotten: the journal still owes every one, and the next Mover over this
// account finishes them.
func (m *Mover) Flush() (removed, kept []email.ID, err error) {
	if len(m.pending) == 0 {
		return nil, nil, nil
	}
	pending := m.pending
	m.pending = nil
	kept, err = m.opts.Remove(m.source, pending)
	if err != nil {
		return nil, nil, err
	}
	left := make(map[email.ID]bool, len(kept))
	for _, id := range kept {
		left[id] = true
	}
	for _, r := range pending {
		m.forget(m.source.String(), string(r.ID))
		if !left[r.ID] {
			removed = append(removed, r.ID)
		}
	}
	return removed, kept, nil
}

// ContentHash is the hex SHA-256 of a message's raw bytes, as a Removal and
// the journal carry it.
func ContentHash(raw []byte) string {
	if raw == nil {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}

// record and forget write the journal when there is one. A journal write that
// fails does not stop the transfer: the journal only shortens the work of a
// rerun, which dedup on the destination already makes safe.
func (m *Mover) record(e Entry) {
	if m.journal != nil {
		_ = m.journal.RecordUpload(e)
	}
}

func (m *Mover) forget(folder, id string) {
	if m.journal != nil {
		_ = m.journal.RecordDeleted(folder, id)
	}
}

// ensureDest creates dest on the destination side once per Mover.
func (m *Mover) ensureDest(dest label.Folder) error {
	if m.ensured[dest] {
		return nil
	}
	if err := m.dst.EnsureLabelExists(dest); err != nil {
		return err
	}
	m.ensured[dest] = true
	return nil
}

// copyOrUpload copies within an account, or uploads to another, returning the
// message's ID on the destination side.
func (m *Mover) copyOrUpload(it Item) (email.ID, error) {
	if SameAccount(m.src, m.dst) {
		return it.ID, m.src.CopyEmail([]email.ID{it.ID}, m.source, it.Dest)
	}
	return m.dst.UploadRawEmail(it.Raw, it.Dest)
}

// Recover finishes the removals an interrupted run left in j, one batch per
// source folder, and drops each entry it completes. A batch that fails stays
// in the journal for the next run.
func Recover(j *Journal, remove Remover, rep report.Reporter) {
	pending := j.Pending()
	if len(pending) == 0 {
		return
	}
	rep.Warnf("Found %d uncompleted transfer(s) in journal from interrupted run. Resuming cleanup...", len(pending))
	byFolder := make(map[string][]Entry)
	var order []string
	for _, e := range pending {
		if _, seen := byFolder[e.SourceFolder]; !seen {
			order = append(order, e.SourceFolder)
		}
		byFolder[e.SourceFolder] = append(byFolder[e.SourceFolder], e)
	}
	for _, folder := range order {
		entries := byFolder[folder]
		msgs := make([]Removal, len(entries))
		for i, e := range entries {
			rep.Infof("Recovering: source %s was confirmed at dest %s in %s; deleting from %s...", e.SourceID, e.DestID, e.DestFolder, folder)
			msgs[i] = Removal{ID: email.ID(e.SourceID), Hash: e.Hash}
		}
		if _, err := remove(label.FromServer(folder), msgs); err != nil {
			rep.Warnf("Failed to delete %d message(s) from %s during journal recovery: %v", len(msgs), folder, err)
			continue
		}
		for _, e := range entries {
			_ = j.RecordDeleted(e.SourceFolder, e.SourceID)
		}
	}
}
