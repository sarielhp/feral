package snooze

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sarielhp/feral/cache/meta"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/label"
	"github.com/sarielhp/feral/mailclient"
)

// FolderName is the folder a snoozed message waits in, created on its
// account the first time one is snoozed there.
const FolderName = "Snoozed"

// ErrPast is a snooze time that is not after now.
var ErrPast = errors.New("a snooze must end in the future")

// ErrIDsChangeOnMove refuses an account whose message IDs the server
// replaces on a move: the ID recorded before the move would name nothing
// after it, and the message could not be found to wake.
var ErrIDsChangeOnMove = errors.New("this account's cache holds message IDs that change when a message moves, " +
	"so a snoozed message could not be found again; run `feral utils cache outlook-ids` first")

// Client is what snoozing needs of the message's account.
type Client interface {
	mailclient.LabelReader
	mailclient.ConfigProvider
	mailclient.BackendInfo
	EnsureLabelExists(name label.Folder) error
	MoveEmail(messageIDs []email.ID, source, dest label.Folder) error
}

// Item is a message to snooze: its ID, the folder it is in now, and what
// the listing shows of it.
type Item struct {
	ID      email.ID
	Folder  string
	Subject string
	From    string
}

// Request is when to snooze until, on which account, recorded where.
type Request struct {
	Root    string // the download directory, which holds snooze.json
	Account string
	Until   time.Time
	Now     time.Time
}

// Snooze moves each item into the account's snooze folder and records when
// it is to come back. The record is written before the move and withdrawn if
// the move fails, so no failure leaves a message in the snooze folder with
// nothing to bring it back. A message already in the snooze folder is
// snoozed again: its time changes and it does not move. It returns the
// entries it recorded, and the failures of the rest.
func Snooze(c Client, req Request, items []Item) ([]Entry, error) {
	folder, err := prepare(c, req)
	if err != nil {
		return nil, err
	}
	var done []Entry
	var errs []error
	for _, item := range items {
		e, err := snoozeOne(c, req, folder, item)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", email.ComputeShortID(item.ID), err))
			continue
		}
		done = append(done, e)
	}
	return done, errors.Join(errs...)
}

// prepare refuses what cannot be snoozed before anything changes, and
// resolves the snooze folder, creating it if the account has none.
func prepare(c Client, req Request) (label.Folder, error) {
	if c == nil {
		return label.Folder{}, errors.New("snooze: no account")
	}
	if req.Account == "" {
		return label.Folder{}, errors.New("snooze: the message's account is not known")
	}
	if req.Root == "" {
		// Path("") is a relative snooze.json: a record no watcher would read.
		return label.Folder{}, errors.New("snooze: no download directory to keep the record in")
	}
	if !req.Until.After(req.Now) {
		return label.Folder{}, ErrPast
	}
	// Read the record first: a damaged one refuses before the server is
	// touched, rather than after a message has gone into the folder.
	if _, err := Load(req.Root); err != nil {
		return label.Folder{}, err
	}
	if err := writable(c); err != nil {
		return label.Folder{}, err
	}
	return EnsureFolder(c)
}

// writable refuses an account whose moves would only be simulated -- under
// --read-only or --dry-run, or one marked read-only -- because the record
// would describe a move that never happened, and a local mbox file, which
// has one folder.
func writable(c Client) error {
	if c.BackendType() == "mbox" {
		return errors.New("an mbox account has only its inbox; there is nowhere to snooze a message to")
	}
	cfg := c.Config()
	if cfg == nil {
		return nil
	}
	if cfg.ReadOnly || cfg.DryRun {
		return fmt.Errorf("account %s is read-only; nothing was snoozed", cfg.SelectedAccount)
	}
	if meta.IDsChangeOnMove(cfg.AccountCache) {
		return ErrIDsChangeOnMove
	}
	return nil
}

// EnsureFolder finds the account's snooze folder by its exact name, in any
// case, and creates it when there is none. A folder that merely ends in the
// name -- Projects/Snoozed -- is someone else's and is not used.
func EnsureFolder(c Client) (label.Folder, error) {
	names, err := c.GetMatchingLabels("")
	if err != nil {
		return label.Folder{}, fmt.Errorf("list folders: %w", err)
	}
	for _, name := range names {
		if strings.EqualFold(name, FolderName) {
			return label.FromServer(name), nil
		}
	}
	folder, err := label.ForCreate(FolderName)
	if err != nil {
		return label.Folder{}, err
	}
	if err := c.EnsureLabelExists(folder); err != nil {
		return label.Folder{}, fmt.Errorf("create folder %s: %w", FolderName, err)
	}
	return folder, nil
}

func snoozeOne(c Client, req Request, folder label.Folder, item Item) (Entry, error) {
	e := Entry{
		Account: req.Account, ID: item.ID, Until: req.Until,
		FromFolder: item.Folder, Folder: folder.String(),
		Subject: item.Subject, From: item.From, SnoozedAt: req.Now,
	}
	if item.Folder == "" {
		return Entry{}, errors.New("the folder it is in is not known")
	}
	waiting := strings.EqualFold(item.Folder, folder.String())
	previous, err := Put(req.Root, e, waiting)
	if err != nil {
		return Entry{}, err
	}
	if waiting {
		// Snoozed again: it is already where it waits, and came from where
		// it was first snoozed from.
		if previous != nil {
			e.FromFolder, e.SnoozedAt = previous.FromFolder, previous.SnoozedAt
		}
		return e, nil
	}
	if err := c.MoveEmail([]email.ID{item.ID}, label.FromServer(item.Folder), folder); err != nil {
		moveErr := fmt.Errorf("move to %s: %w", folder, err)
		if undoErr := withdraw(req, item.ID, previous); undoErr != nil {
			return Entry{}, errors.Join(moveErr, fmt.Errorf("the snooze record could not be put back: %w", undoErr))
		}
		return Entry{}, moveErr
	}
	return e, nil
}

// withdraw undoes a Put whose move failed: it restores the entry the Put
// replaced -- a message snoozed before keeps its record -- or removes the new
// one.
func withdraw(req Request, id email.ID, previous *Entry) error {
	if previous != nil {
		_, err := Put(req.Root, *previous, false)
		return err
	}
	return Remove(req.Root, req.Account, id)
}
