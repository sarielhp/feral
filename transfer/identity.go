package transfer

import (
	"bytes"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/cache/msg"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/label"
	"github.com/sarielhp/feral/mailclient"
)

// Confirmation polls the destination this many times, doubling the delay
// between attempts, before it gives up. Variables so a test can shorten them.
var (
	MaxConfirmRetries = 5
	ConfirmRetryDelay = 2 * time.Second
)

// Same reports whether two parsed messages are the same message. A
// Message-ID on both sides is authoritative: if they disagree the answer is
// no. Subject and a non-zero Date decide only when at least one side lacks a
// Message-ID.
func Same(src, dest *email.Email) bool {
	if src == nil || dest == nil {
		return false
	}
	if src.MessageID != "" && dest.MessageID != "" {
		return src.MessageID.Same(dest.MessageID)
	}
	return src.Subject != "" && dest.Subject == src.Subject && !src.EmailDate.IsZero() && src.EmailDate.Equal(dest.EmailDate)
}

// IdentityIndex indexes the identity of messages without reference to any
// folder: each Message-ID, plus the Subject+Date fallback for messages that
// lack one. mbox deduplication and the destination check of a transfer both
// match on exactly these, so the matching core lives here once.
type IdentityIndex struct {
	messageIDs            map[email.HeaderID]struct{}
	subjectDatesWithID    map[string]struct{}
	subjectDatesWithoutID map[string]struct{}
}

// NewIdentityIndex returns an empty index sized for capacity messages.
func NewIdentityIndex(capacity int) *IdentityIndex {
	return &IdentityIndex{
		messageIDs:            make(map[email.HeaderID]struct{}, capacity),
		subjectDatesWithID:    make(map[string]struct{}, capacity),
		subjectDatesWithoutID: make(map[string]struct{}, capacity),
	}
}

// Add records em's identity.
func (idx *IdentityIndex) Add(em *email.Email) {
	if em == nil {
		return
	}
	if em.MessageID != "" {
		idx.messageIDs[em.MessageID.Folded()] = struct{}{}
	}
	if em.Subject != "" && !em.EmailDate.IsZero() {
		k := subjectDateKey(em)
		if em.MessageID != "" {
			idx.subjectDatesWithID[k] = struct{}{}
			return
		}
		idx.subjectDatesWithoutID[k] = struct{}{}
	}
}

// Contains reports whether a message the index holds is the same message as
// em, by the rules of Same.
func (idx *IdentityIndex) Contains(em *email.Email) bool {
	if idx == nil || em == nil {
		return false
	}
	if em.MessageID != "" {
		if _, ok := idx.messageIDs[em.MessageID.Folded()]; ok {
			return true
		}
		if em.Subject != "" && !em.EmailDate.IsZero() {
			_, ok := idx.subjectDatesWithoutID[subjectDateKey(em)]
			return ok
		}
		return false
	}
	if em.Subject != "" && !em.EmailDate.IsZero() {
		k := subjectDateKey(em)
		if _, ok := idx.subjectDatesWithoutID[k]; ok {
			return true
		}
		_, ok := idx.subjectDatesWithID[k]
		return ok
	}
	return false
}

// subjectDateKey is the fallback identity of a message without a Message-ID:
// its subject together with its send time.
func subjectDateKey(em *email.Email) string {
	return fmt.Sprintf("%s|%d", em.Subject, em.EmailDate.Unix())
}

// FolderIndex is an IdentityIndex plus the server and cache IDs seen in one
// destination folder.
type FolderIndex struct {
	ids map[email.ID]bool
	*IdentityIndex
}

// NewFolderIndex returns an empty folder index sized for size messages.
func NewFolderIndex(size int) *FolderIndex {
	return &FolderIndex{
		ids:           make(map[email.ID]bool, size),
		IdentityIndex: NewIdentityIndex(size),
	}
}

// Add records a message by its source ID, its ID on the destination, and its
// identity. Either ID may be empty.
func (idx *FolderIndex) Add(msgID, serverID email.ID, em *email.Email) {
	if msgID != "" {
		idx.ids[msgID] = true
	}
	if serverID != "" {
		idx.ids[serverID] = true
	}
	idx.IdentityIndex.Add(em)
}

// Contains reports whether msgID, or a message the same as srcEmail, is in the
// folder.
func (idx *FolderIndex) Contains(msgID email.ID, srcEmail *email.Email) bool {
	if idx == nil {
		return false
	}
	if idx.ids[msgID] {
		return true
	}
	return idx.IdentityIndex.Contains(srcEmail)
}

// cacheSubdir is the cache subdirectory a destination folder is listed into.
func cacheSubdir(folder label.Folder) string {
	if sub := cachedir.SanitizeName(folder.String()); sub != "" {
		return sub
	}
	return folder.String()
}

// BuildFolderIndex lists destFolder on the destination and indexes what is in
// it. A listing that fails gives an empty index, so the caller uploads rather
// than wrongly skipping.
func BuildFolderIndex(destClient mailclient.Reader, destFolder label.Folder) *FolderIndex {
	targetIDs, err := destClient.FetchAndDownloadEmails(destFolder, cacheSubdir(destFolder))
	if err != nil {
		slog.Warn("transfer: failed to fetch destination folder", slog.String("folder", destFolder.String()), slog.Any("error", err))
		return NewFolderIndex(0)
	}

	idx := NewFolderIndex(len(targetIDs))
	destDir := destClient.Config().AccountCache
	for _, eID := range targetIDs {
		idx.ids[eID] = true
		rawBytes, err := msg.Read(destDir, eID)
		if err != nil {
			slog.Warn("transfer: read error for cached email", slog.String("msgID", string(eID)), slog.Any("error", err))
			continue
		}
		if destEmail := email.ParseReader(bytes.NewReader(rawBytes), eID, ""); destEmail != nil {
			idx.Add(eID, "", destEmail)
		}
	}
	return idx
}

// FolderCache holds one FolderIndex per destination folder, built on first use.
type FolderCache map[label.Folder]*FolderIndex

// For returns the index of dest, creating an empty one if there is none.
func (c FolderCache) For(dest label.Folder) *FolderIndex {
	if c[dest] == nil {
		c[dest] = NewFolderIndex(1)
	}
	return c[dest]
}

// InDest reports whether a message already exists in the destination folder,
// building that folder's index on first use.
func InDest(destClient mailclient.Reader, destFolder label.Folder, msgID email.ID, srcEmail *email.Email, cache FolderCache) bool {
	idx, ok := cache[destFolder]
	if !ok {
		idx = BuildFolderIndex(destClient, destFolder)
		cache[destFolder] = idx
	}
	return idx.Contains(msgID, srcEmail)
}

// Confirm verifies that a message exists on the destination by listing the
// folder from the server, not the cache, and matching by ID, Message-ID, or
// Subject+Date. It retries with exponential backoff, for servers that are
// only eventually consistent.
func Confirm(destClient mailclient.Reader, destFolder label.Folder, msgID email.ID, srcEmail *email.Email) (bool, error) {
	sub := cacheSubdir(destFolder)
	delay := ConfirmRetryDelay
	for attempt := 1; attempt <= MaxConfirmRetries; attempt++ {
		targetIDs, err := destClient.FetchAndDownloadEmails(destFolder, sub)
		if err != nil {
			if attempt == MaxConfirmRetries {
				return false, fmt.Errorf("failed to fetch destination folder %q from server after %d attempts: %w", destFolder, attempt, err)
			}
			slog.Debug("transfer: confirm retry after fetch error", slog.Int("attempt", attempt), slog.Any("error", err))
		} else if listed(destClient.Config().AccountCache, targetIDs, msgID, srcEmail) {
			return true, nil
		}
		if attempt < MaxConfirmRetries {
			time.Sleep(delay)
			delay *= 2
		}
	}
	return false, fmt.Errorf("message %s not confirmed on server in destination folder %q after %d attempts", msgID, destFolder, MaxConfirmRetries)
}

// listed reports whether msgID, or a cached message the same as srcEmail, is
// among ids.
func listed(destDir cachedir.Account, ids []email.ID, msgID email.ID, srcEmail *email.Email) bool {
	for _, eID := range ids {
		if eID == msgID {
			return true
		}
		if srcEmail == nil {
			continue
		}
		rawBytes, err := msg.Read(destDir, eID)
		if err != nil {
			continue
		}
		if Same(srcEmail, email.ParseReader(bytes.NewReader(rawBytes), eID, "")) {
			return true
		}
	}
	return false
}

// SameAccount reports whether two clients act on the same account. It compares
// the backend and the account's cache or name, never the pointers: decorators
// such as CheckingMailClient are built per call around the same backend.
func SameAccount(c1, c2 mailclient.Reader) bool {
	if c1 == nil || c2 == nil {
		return false
	}
	if c1.BackendType() != c2.BackendType() {
		return false
	}
	cfg1 := c1.Config()
	cfg2 := c2.Config()
	if cfg1 == nil || cfg2 == nil {
		return false
	}
	if !cfg1.AccountCache.IsZero() && !cfg2.AccountCache.IsZero() {
		return cfg1.AccountCache == cfg2.AccountCache
	}
	if cfg1.SelectedAccount != "" && cfg2.SelectedAccount != "" {
		return strings.EqualFold(string(cfg1.SelectedAccount), string(cfg2.SelectedAccount))
	}
	if cfg1.Username != "" && cfg2.Username != "" {
		return strings.EqualFold(cfg1.Username, cfg2.Username)
	}
	return false
}
