package tui

import (
	"errors"
	"github.com/sarielhp/feral/label"

	"github.com/sarielhp/feral/calendar"
	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/mailclient"
)

// What the TUI needs from the layers beneath it, declared here and bound in
// main's composition root. This replaces a struct of 15 nil-able function
// fields, three of which were never called.

// ErrNotConfigured reports a dependency that was never bound.
var ErrNotConfigured = errors.New("backend dependency not configured")

// ClientProvider resolves the backend client for an account.
type ClientProvider interface {
	ForSelected(config *cfg_g.Config) (mailclient.MailClient, error)
	ForAccount(config *cfg_g.Config, acc cfg_acc.AccountConfig) (mailclient.MailClient, error)
	ForAccountIndex(config *cfg_g.Config, index int) (mailclient.MailClient, error)
}

// FolderResolver names the folder a backend files archived, deleted, spam and
// draft mail into, which differs per backend and per account.
type FolderResolver interface {
	ResolveArchiveTarget(client mailclient.MailClient) (label.Folder, error)
	ResolveTrashTarget(client mailclient.MailClient) (label.Folder, error)
	ResolveDraftsTarget(client mailclient.MailClient) (label.Folder, error)
	// ResolveSpamTarget names the folder ReportSpam files mail into, which
	// is where undoing a spam mark moves it back from.
	ResolveSpamTarget(client mailclient.MailClient) (label.Folder, error)
}

// CalendarCheckResult details how an ICS event matches an existing Google Calendar event.
type CalendarCheckResult struct {
	InCalendar  bool
	ExactMatch  bool
	Discrepancy string
	Summary     string
	Event       *calendar.ICSEvent
}

// CalendarOps adds an .ics payload to the user's calendar.
type CalendarOps interface {
	AddICS(config *cfg_g.Config, icsData []byte) (summary string, existed bool, err error)
	CheckICS(config *cfg_g.Config, icsData []byte) (inCalendar bool, summary string, event *calendar.ICSEvent, err error)
}

// VerifyingCalendarOps is the richer form: a calendar that reads the event back
// after writing it and can say whether duplicates were created.
//
// It is an optional upgrade rather than part of CalendarOps because not every
// calendar can do it, and the TUI has a meaningfully different report for each
// case. Implementations that can verify satisfy it; the TUI type-asserts.
type VerifyingCalendarOps interface {
	CalendarOps
	AddICSVerified(config *cfg_g.Config, icsData []byte) (summary string, existed, verified, noDuplicates bool, err error)
	AddICSAsNew(config *cfg_g.Config, icsData []byte) (summary string, err error)
	CheckICSDetailed(config *cfg_g.Config, icsData []byte) (CalendarCheckResult, error)
}

// MboxSource is one account's slice of a tagged set: the messages and the
// client that can read them. A set tagged in a unified inbox spans accounts,
// so the writer must not have to guess which client owns which message.
type MboxSource struct {
	Client mailclient.MailClient
	IDs    []email.ID
}

// MboxExporter appends messages to a local mbox file.
type MboxExporter interface {
	AppendMessagesToMbox(path string, sources []MboxSource) (int, error)
}

// AccountMover moves messages from a folder of one account into a folder of
// another: the upload, the confirmation and the removal from the source that
// a server-side move does in one step within an account.
type AccountMover interface {
	// MoveAcross moves ids out of source on src into dest on dst and returns
	// those that left src; the error names each one that did not.
	MoveAcross(src, dst mailclient.MailClient, source label.Folder, ids []email.ID, dest label.Folder) ([]email.ID, error)
}

// Backend is everything the TUI needs from below.
type Backend struct {
	Clients  ClientProvider
	Folders  FolderResolver
	Calendar CalendarOps
	Mbox     MboxExporter
	Transfer AccountMover
}

// Accessors tolerate both a nil Backend and a nil member, so call sites need no
// guards. cmdArchive and cmdDelete previously dereferenced m.bk unchecked.

func (b *Backend) clients() ClientProvider {
	if b == nil || b.Clients == nil {
		return noClients{}
	}
	return b.Clients
}

func (b *Backend) folders() FolderResolver {
	if b == nil || b.Folders == nil {
		return noFolders{}
	}
	return b.Folders
}

// hasCalendar reports whether calendar support was wired at all, which decides
// whether the TUI offers the action rather than failing it.
func (b *Backend) hasCalendar() bool {
	return b != nil && b.Calendar != nil
}

// mbox is the wired mbox writer, or one that reports not-configured, so the
// export path needs no nil guard.
func (b *Backend) mbox() MboxExporter {
	if b == nil || b.Mbox == nil {
		return noMbox{}
	}
	return b.Mbox
}

// transfer is the wired account mover, or one that reports not-configured.
func (b *Backend) transfer() AccountMover {
	if b == nil || b.Transfer == nil {
		return noTransfer{}
	}
	return b.Transfer
}

type noTransfer struct{}

func (noTransfer) MoveAcross(mailclient.MailClient, mailclient.MailClient, label.Folder, []email.ID, label.Folder) ([]email.ID, error) {
	return nil, ErrNotConfigured
}

type noMbox struct{}

func (noMbox) AppendMessagesToMbox(string, []MboxSource) (int, error) {
	return 0, ErrNotConfigured
}

type noClients struct{}

func (noClients) ForSelected(*cfg_g.Config) (mailclient.MailClient, error) {
	return nil, ErrNotConfigured
}

func (noClients) ForAccount(*cfg_g.Config, cfg_acc.AccountConfig) (mailclient.MailClient, error) {
	return nil, ErrNotConfigured
}

func (noClients) ForAccountIndex(*cfg_g.Config, int) (mailclient.MailClient, error) {
	return nil, ErrNotConfigured
}

type noFolders struct{}

func (noFolders) ResolveArchiveTarget(mailclient.MailClient) (label.Folder, error) {
	return label.Folder{}, ErrNotConfigured
}
func (noFolders) ResolveTrashTarget(mailclient.MailClient) (label.Folder, error) {
	return label.Folder{}, ErrNotConfigured
}
func (noFolders) ResolveDraftsTarget(mailclient.MailClient) (label.Folder, error) {
	return label.Folder{}, ErrNotConfigured
}
func (noFolders) ResolveSpamTarget(mailclient.MailClient) (label.Folder, error) {
	return label.Folder{}, ErrNotConfigured
}

// verifyingCalendar reports whether the wired calendar can confirm what it
// wrote. The TUI has a different, richer report when it can.
func (b *Backend) verifyingCalendar() (VerifyingCalendarOps, bool) {
	if !b.hasCalendar() {
		return nil, false
	}
	v, ok := b.Calendar.(VerifyingCalendarOps)
	return v, ok
}
