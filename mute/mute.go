// Package mute is the record of the threads a user has muted, one file per
// account: muted_threads.json in the account's settings directory
// (cfg_g.AccountDir). A muted thread's new messages skip the inbox -- the
// watcher archives them as they arrive and does not announce them.
//
// The record is a user decision, not something the server reported, so it
// lives beside the account's white and black lists rather than in the cache,
// which is re-derivable and may be wiped. The directory is named after the
// account, and `account rename` moves the directory, file and all.
//
// A thread is named by the RFC 5322 Message-ID at its root
// (email.ThreadRoot). No backend surfaces a thread ID of its own through the
// mail client -- summaries are parsed from the message bytes -- so the header
// root is the one rule for every backend, and a key survives a move between
// folders, an Outlook ID change and a copy to another account. Each entry
// records its scheme, so a later build can add a backend's thread ID (Gmail's
// threadId) beside it without a format change.
package mute

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/sarielhp/feral/cache/dirlock"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/fileformat"
)

// FileName is the record's name inside the account's settings directory.
const FileName = "muted_threads.json"

// SchemeHeaderRoot is the key scheme of a thread named by its root
// Message-ID. It is the only scheme this build writes or matches; an entry in
// another is kept as it is and never matches.
const SchemeHeaderRoot = "header-root"

// ErrNoConfigDir is a mute with no configuration directory to keep it in. It
// is never answered by falling back to $HOME.
var ErrNoConfigDir = errors.New("no configuration directory: cannot keep a muted-thread list")

// ErrNoThreadID is a message with no Message-ID, References or In-Reply-To:
// there is nothing to recognize its thread's later messages by.
var ErrNoThreadID = errors.New("the message carries no Message-ID, References or In-Reply-To, so its thread cannot be recognized")

// Thread is one muted thread.
type Thread struct {
	Key    email.HeaderID `json:"key"`
	Scheme string         `json:"scheme"`
	// Subject is what `msg muted` shows; it plays no part in matching.
	Subject string    `json:"subject,omitempty"`
	MutedAt time.Time `json:"muted_at"`
}

// List is an account's muted threads, as the file holds them.
type List struct {
	Format  int      `json:"format"`
	Threads []Thread `json:"threads"`
}

// Path is the account's record: muted_threads.json in its settings directory
// under configDir. An empty configDir is ErrNoConfigDir.
func Path(configDir, account string) (string, error) {
	if configDir == "" {
		return "", ErrNoConfigDir
	}
	return filepath.Join(cfg_g.AccountDir(configDir, account), FileName), nil
}

// Load reads the record at path. A missing file is an empty list; one that
// cannot be read, does not parse or was written in a newer format is an
// error, never an empty list, because the next save would write the empty
// list over every mute it held.
func Load(path string) (*List, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &List{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read muted threads: %w", err)
	}
	list := &List{}
	if err := json.Unmarshal(data, list); err != nil {
		return nil, fmt.Errorf("parse muted threads %s: %w", path, err)
	}
	if err := fileformat.Check(path, list.Format); err != nil {
		return nil, err
	}
	return list, nil
}

// Save writes the record atomically, creating the settings directory if need
// be.
func (l *List) Save(path string) error {
	l.Format = fileformat.Current
	if l.Threads == nil {
		l.Threads = []Thread{}
	}
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return fmt.Errorf("encode muted threads: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	if err := dirlock.AtomicWrite(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write muted threads: %w", err)
	}
	return nil
}

// ThreadOf is the entry that would mute e's thread, stamped at now. A message
// with no thread identifier at all is ErrNoThreadID.
func ThreadOf(e *email.Email, now time.Time) (Thread, error) {
	root := email.ThreadRoot(e)
	if root == "" {
		return Thread{}, ErrNoThreadID
	}
	return Thread{Key: root, Scheme: SchemeHeaderRoot, Subject: e.Subject, MutedAt: now}, nil
}

// Add mutes t's thread, reporting false when it was already muted.
func (l *List) Add(t Thread) bool {
	if l.index(t.Key) >= 0 {
		return false
	}
	l.Threads = append(l.Threads, t)
	return true
}

// Remove unmutes the thread named key, reporting false when it was not muted.
func (l *List) Remove(key email.HeaderID) bool {
	i := l.index(key)
	if i < 0 {
		return false
	}
	l.Threads = append(l.Threads[:i], l.Threads[i+1:]...)
	return true
}

// Match is the muted thread e belongs to: one whose root is any of the
// Message-IDs e names -- its References, its In-Reply-To or its own. Any, not
// only e's root, because a mail program that trims References keeps the
// first entry and the last, and a reply to a reply still names one of them.
func (l *List) Match(e *email.Email) (Thread, bool) {
	if l == nil || len(l.Threads) == 0 || e == nil {
		return Thread{}, false
	}
	for _, id := range email.ThreadIDs(e) {
		if i := l.index(id); i >= 0 {
			return l.Threads[i], true
		}
	}
	return Thread{}, false
}

// index is the position of the header-root thread named key, or -1. An empty
// key names nothing.
func (l *List) index(key email.HeaderID) int {
	if key == "" {
		return -1
	}
	for i, t := range l.Threads {
		if t.Scheme == SchemeHeaderRoot && t.Key.Same(key) {
			return i
		}
	}
	return -1
}
