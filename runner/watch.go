package runner

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/mailclient"
	"github.com/sarielhp/feral/mailops"
	"github.com/sarielhp/feral/organize"
	"github.com/sarielhp/feral/report"
	"github.com/sarielhp/feral/watcher"
)

// WatchService runs the background mail watcher for the CLI. It is the seam
// between the watcher, which names no backend, and the factory, which is the
// only package that may.
type WatchService struct {
	Rep report.Reporter
}

func (s WatchService) rep() report.Reporter { return report.Or(s.Rep) }

// WatchInBackground runs the watcher beside the TUI. Its reports go to the
// log -- the TUI's diagnostics panel -- because the terminal belongs to the
// TUI. The reporter cannot prompt, so an account whose sign-in has expired is
// reported there rather than starting a sign-in behind the TUI's back.
func (s WatchService) WatchInBackground(ctx context.Context, config *cfg_g.Config, opts watcher.Options) error {
	return WatchService{Rep: logReporter{}}.Watch(ctx, config, opts)
}

// logReporter sends reports to slog, the TUI's log.
type logReporter struct{}

func (logReporter) Infof(f string, a ...any)    { slog.Info(fmt.Sprintf(f, a...)) }
func (logReporter) Successf(f string, a ...any) { slog.Info(fmt.Sprintf(f, a...)) }
func (logReporter) Warnf(f string, a ...any)    { slog.Warn(fmt.Sprintf(f, a...)) }
func (logReporter) Errorf(f string, a ...any)   { slog.Error(fmt.Sprintf(f, a...)) }
func (logReporter) Printf(f string, a ...any)   { slog.Info(fmt.Sprintf(f, a...)) }
func (logReporter) Progress(int, int, string)   {}

// Watch polls the configured accounts until ctx is cancelled.
func (s WatchService) Watch(ctx context.Context, config *cfg_g.Config, opts watcher.Options) error {
	rep := s.rep()
	// One waker for the run, so a snoozed message that cannot be woken is
	// reported once rather than on every poll.
	if config == nil {
		return cfg_g.ErrNoAccounts
	}
	waker := SnoozeService{Rep: rep}.waker(config, watchedName(config, opts.Account))
	return watcher.Run(ctx, watcher.Deps{
		Config:   config,
		Reporter: rep,
		ClientFor: func(acc cfg_acc.AccountConfig) (mailclient.Reader, error) {
			return mailops.NewMailClient(acc, config, rep)
		},
		Flush: func() error {
			_, err := OutboxService{Rep: rep}.Flush(config)
			return err
		},
		Wake: func(ctx context.Context, now time.Time) error {
			woken, err := waker.Due(ctx, now)
			for _, e := range woken {
				rep.Infof("watch: %s: snoozed message back in the inbox: %s", e.Account, e.Subject)
			}
			return err
		},
		Archive: func(_ context.Context, acc cfg_acc.AccountConfig, ids []email.ID) error {
			client, err := mailops.NewMailClient(acc, config, rep)
			if err != nil {
				return err
			}
			return archiveFromInbox(client, ids)
		},
	}, opts)
}

// watchedName is the name of the one account a watch was limited to, as the
// snooze record spells it, or "" for every account. An identifier that names
// no account is the watcher's to refuse.
func watchedName(config *cfg_g.Config, ident string) string {
	if ident == "" {
		return ""
	}
	if acc, err := cfg_g.Lookup(config.Accounts, ident); err == nil {
		return acc.Name
	}
	return ident
}

// archiveFromInbox files ids out of the client's inbox into its archive
// folder: what the watcher does with the new mail of a muted thread. The
// client is the factory's, so the read-only decorator and the cache's
// reindexing see the move as they see any other.
func archiveFromInbox(client mailclient.MailClient, ids []email.ID) error {
	target, err := organize.ResolveArchiveTarget(client)
	if err != nil {
		return fmt.Errorf("resolve the archive folder: %w", err)
	}
	return client.MoveEmail(ids, client.InboxFolder(), target)
}
