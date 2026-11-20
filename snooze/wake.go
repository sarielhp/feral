package snooze

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sarielhp/feral/cache"
	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/label"
	"github.com/sarielhp/feral/mailclient"
	"github.com/sarielhp/feral/report"
)

// WakeClient is what waking needs of a message's account. It moves with
// MoveEmail into InboxFolder, never MoveToInbox: that is the unspam path,
// which on Gmail and JMAP also files the message under the unspam training
// label and, on JMAP, does not report a message the server refused.
type WakeClient interface {
	mailclient.ConfigProvider
	InboxFolder() label.Folder
	MoveEmail(messageIDs []email.ID, source, dest label.Folder) error
	MarkAsUnread(messageIDs []email.ID) error
}

// errAccountGone is an entry whose account is no longer configured.
var errAccountGone = errors.New("its account is no longer configured")

// Waker brings snoozed messages back to the inbox. A watcher keeps one for
// as long as it runs, so a message that cannot be woken is reported once per
// problem rather than on every poll.
type Waker struct {
	Root string
	// Accounts is every configured account: an entry whose account is not
	// among them is reported, never dropped.
	Accounts []cfg_acc.AccountConfig
	// Only limits waking to one account's entries; empty is every account.
	Only string
	// ClientFor builds a client for an account.
	ClientFor func(acc cfg_acc.AccountConfig) (WakeClient, error)
	Reporter  report.Reporter

	warned map[string]string
}

// Due wakes every entry whose time has come. A message that cannot be woken
// keeps its entry and is reported through the Reporter; the error is only
// for a record that cannot be read or written.
func (w *Waker) Due(ctx context.Context, now time.Time) ([]Entry, error) {
	f, err := Load(w.Root)
	if err != nil {
		return nil, err
	}
	var due []Entry
	for _, e := range f.Entries {
		if e.Due(now) && (w.Only == "" || strings.EqualFold(e.Account, w.Only)) {
			due = append(due, e)
		}
	}
	woken, failures, err := w.wake(ctx, due)
	for _, fail := range failures {
		w.warnOnce(fail)
	}
	return woken, err
}

// Wake brings the entries back now, whatever their time, and returns those
// it woke and the failures of the rest, whose entries are kept.
func (w *Waker) Wake(ctx context.Context, entries []Entry) ([]Entry, error) {
	woken, failures, err := w.wake(ctx, entries)
	errs := []error{err}
	for _, fail := range failures {
		errs = append(errs, fail)
	}
	return woken, errors.Join(errs...)
}

// failure is an entry that could not be woken.
type failure struct {
	entry Entry
	err   error
}

func (f failure) Error() string {
	return fmt.Sprintf("snoozed message %s in %s (%q): %v; it stays snoozed",
		email.ComputeShortID(f.entry.ID), f.entry.Account, f.entry.Subject, f.err)
}

func (w *Waker) wake(ctx context.Context, entries []Entry) ([]Entry, []failure, error) {
	clients := map[string]WakeClient{}
	var woken []Entry
	var failures []failure
	for _, e := range entries {
		if ctx.Err() != nil {
			break
		}
		if err := w.wakeOne(e, clients); err != nil {
			failures = append(failures, failure{entry: e, err: err})
			continue
		}
		// The message is back. A record that cannot be updated would wake
		// it again next time, which marks it unread once more and moves it
		// to where it already is: harmless, but worth saying.
		if err := Remove(w.Root, e.Account, e.ID); err != nil {
			return woken, failures, fmt.Errorf("woke %s but could not update the snooze record: %w",
				email.ComputeShortID(e.ID), err)
		}
		woken = append(woken, e)
	}
	return woken, failures, nil
}

// wakeOne marks the message unread and moves it to the inbox -- in that
// order, so a retry after a failed move repeats only what is idempotent --
// and then records it unread in the local read cache.
func (w *Waker) wakeOne(e Entry, clients map[string]WakeClient) error {
	client, err := w.client(e.Account, clients)
	if err != nil {
		return err
	}
	cfg := client.Config()
	if cfg != nil && (cfg.ReadOnly || cfg.DryRun) {
		return errors.New("the account is read-only")
	}
	ids := []email.ID{e.ID}
	if err := client.MarkAsUnread(ids); err != nil {
		return fmt.Errorf("mark it unread: %w", err)
	}
	if err := client.MoveEmail(ids, label.FromServer(e.Folder), client.InboxFolder()); err != nil {
		return fmt.Errorf("move it from %s to the inbox: %w", e.Folder, err)
	}
	if cfg != nil && !cfg.AccountCache.IsZero() {
		if cache.MarkIDsUnread(cfg.AccountCache, ids) == nil {
			_ = cache.ConfirmReads(cfg.AccountCache, ids)
		}
	}
	return nil
}

// client is the account's client, built once per wake.
func (w *Waker) client(name string, clients map[string]WakeClient) (WakeClient, error) {
	if c, ok := clients[name]; ok {
		return c, nil
	}
	var acc *cfg_acc.AccountConfig
	for i := range w.Accounts {
		if strings.EqualFold(w.Accounts[i].Name, name) {
			acc = &w.Accounts[i]
			break
		}
	}
	if acc == nil {
		return nil, errAccountGone
	}
	if w.ClientFor == nil {
		return nil, errors.New("no way to reach the account")
	}
	c, err := w.ClientFor(*acc)
	if err != nil {
		return nil, err
	}
	clients[name] = c
	return c, nil
}

// warnOnce reports a failure the first time it is seen, and again only when
// the reason changes: a kept entry would otherwise warn on every poll.
func (w *Waker) warnOnce(f failure) {
	if w.warned == nil {
		w.warned = map[string]string{}
	}
	key := f.entry.Account + "\x00" + string(f.entry.ID)
	text := f.Error()
	if w.warned[key] == text {
		return
	}
	w.warned[key] = text
	report.Or(w.Reporter).Warnf("snooze: %s", text)
}
