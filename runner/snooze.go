package runner

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sarielhp/feral/cache"
	"github.com/sarielhp/feral/cache/msg"
	"github.com/sarielhp/feral/cache/virtual"
	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/mailclient"
	"github.com/sarielhp/feral/mailops"
	"github.com/sarielhp/feral/report"
	"github.com/sarielhp/feral/snooze"
)

// SnoozeService is `msg snooze`: it finds the messages the user named in the
// account's cache, snoozes them, and lists, wakes or forgets what is snoozed.
// It is the seam between the snooze package, which names no backend, and
// the client factory.
type SnoozeService struct {
	Rep report.Reporter
	// Now is the clock; nil means time.Now.
	Now func() time.Time
	// ClientFor builds an account's client for waking; nil means the
	// factory. Tests replace it.
	ClientFor func(acc cfg_acc.AccountConfig, config *cfg_g.Config) (snooze.WakeClient, error)
}

func (s SnoozeService) now() time.Time {
	if s.Now == nil {
		return time.Now()
	}
	return s.Now()
}

// Items resolves refs -- message IDs as the user typed them -- in the
// client's account cache, with the folder each is in and what a listing
// shows of it.
func (s SnoozeService) Items(client mailclient.MailClient, refs []string) ([]snooze.Item, error) {
	if client == nil || client.Config() == nil {
		return nil, errors.New("no account")
	}
	account := client.Config().AccountCache
	folders, err := client.GetMatchingLabels("")
	if err != nil {
		return nil, fmt.Errorf("list folders: %w", err)
	}
	items := make([]snooze.Item, 0, len(refs))
	for _, ref := range refs {
		id, key, err := cache.FindCachedEmailByID(account, ref)
		if err != nil {
			return nil, fmt.Errorf("message %s: %w", ref, err)
		}
		folder := serverFolder(folders, key)
		if folder == "" {
			// The index that holds it may be a saved set's (msg last,
			// a search), which records the folder each message came from.
			folder = serverFolder(folders, virtual.FindMessageFolder(account, id))
		}
		if folder == "" {
			return nil, fmt.Errorf("message %s: the cache does not say which folder holds it; open the folder first", ref)
		}
		item := snooze.Item{ID: id, Folder: folder}
		if cached, err := msg.CachedEmail(account, id); err == nil && cached != nil {
			item.Subject, item.From = cached.Subject, email.FormatSenderVerbose(cached.FromRaw)
		}
		items = append(items, item)
	}
	return items, nil
}

// serverFolder is the server's name for a folder the cache names, by its
// index key -- a lowercased, escaped form of the folder -- or by its name in
// any case: a move must name the folder as the server spells it. "" when
// none matches.
func serverFolder(folders []string, key string) string {
	if key == "" {
		return ""
	}
	for _, f := range folders {
		if cfg_g.SanitizeLabelForCache(f) == key || strings.EqualFold(f, key) {
			return f
		}
	}
	return ""
}

// Snooze snoozes items, all in the client's account, until the given time.
func (s SnoozeService) Snooze(config *cfg_g.Config, client mailclient.MailClient, items []snooze.Item, until time.Time) ([]snooze.Entry, error) {
	if config == nil || client == nil || client.Config() == nil {
		return nil, errors.New("no account")
	}
	return snooze.Snooze(client, snooze.Request{
		Root:    config.CacheRoot,
		Account: string(client.Config().SelectedAccount),
		Until:   until,
		Now:     s.now(),
	}, items)
}

// List returns every snoozed message, the soonest due first.
func (s SnoozeService) List(config *cfg_g.Config) ([]snooze.Entry, error) {
	if config == nil {
		return nil, cfg_g.ErrNoAccounts
	}
	f, err := snooze.Load(config.CacheRoot)
	if err != nil {
		return nil, err
	}
	return f.Entries, nil
}

// Select returns the snoozed messages refs name -- or, with no refs, those
// due now -- refusing a ref that names none.
func (s SnoozeService) Select(config *cfg_g.Config, refs []string) ([]snooze.Entry, error) {
	entries, err := s.List(config)
	if err != nil {
		return nil, err
	}
	if len(refs) == 0 {
		now := s.now()
		var due []snooze.Entry
		for _, e := range entries {
			if e.Due(now) {
				due = append(due, e)
			}
		}
		return due, nil
	}
	matched, unknown := snooze.Select(entries, refs)
	if len(unknown) > 0 {
		return nil, fmt.Errorf("not snoozed: %s", strings.Join(unknown, ", "))
	}
	return matched, nil
}

// Wake brings the selected messages back to the inbox now, unread.
func (s SnoozeService) Wake(config *cfg_g.Config, refs []string) ([]snooze.Entry, error) {
	entries, err := s.Select(config, refs)
	if err != nil || len(entries) == 0 {
		return nil, err
	}
	return s.waker(config, "").Wake(context.Background(), entries)
}

// Forget drops the selected messages' records and leaves the messages where
// they are: for a message deleted meanwhile, or one moved out of the snooze
// folder by hand.
func (s SnoozeService) Forget(config *cfg_g.Config, refs []string) ([]snooze.Entry, error) {
	entries, err := s.Select(config, refs)
	if err != nil {
		return nil, err
	}
	for i, e := range entries {
		if err := snooze.Remove(config.CacheRoot, e.Account, e.ID); err != nil {
			return entries[:i], err
		}
	}
	return entries, nil
}

// waker is a snooze.Waker over the configured accounts, reaching each through
// the factory.
func (s SnoozeService) waker(config *cfg_g.Config, only string) *snooze.Waker {
	clientFor := s.ClientFor
	if clientFor == nil {
		rep := report.Or(s.Rep)
		clientFor = func(acc cfg_acc.AccountConfig, config *cfg_g.Config) (snooze.WakeClient, error) {
			return mailops.NewMailClient(acc, config, rep)
		}
	}
	return &snooze.Waker{
		Root:      config.CacheRoot,
		Accounts:  config.Accounts,
		Only:      only,
		Reporter:  s.Rep,
		ClientFor: func(acc cfg_acc.AccountConfig) (snooze.WakeClient, error) { return clientFor(acc, config) },
	}
}
