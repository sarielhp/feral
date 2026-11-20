package watcher

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sarielhp/feral/cache/dirlock"
	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/mailclient"
	"github.com/sarielhp/feral/report"
)

const (
	// DefaultInterval is how often the watcher polls when none is given.
	DefaultInterval = 120 * time.Second
	// MinInterval is the shortest interval a caller may ask for. Polling faster
	// than this is a way to be rate-limited, not a way to see mail sooner.
	MinInterval = 30 * time.Second
	// MaxBackoff caps the retry delay after a failed poll.
	MaxBackoff = 10 * time.Minute
	// DefaultLimit is how many recent messages each poll examines. It is the
	// window new mail is detected in, not a cap on how much is announced.
	DefaultLimit = 50
	// staleGrace is how far before the previous poll a message may be dated
	// and still be announced. An older message that appears in the window was
	// not delivered since: something above it was deleted, or (on Outlook,
	// whose IDs change on a move) it was filed. The slack covers sender clock
	// skew and delayed delivery.
	staleGrace = 6 * time.Hour
	// deliverTimeout bounds notify-send and the --on-mail hook, so one that
	// hangs cannot stop polling.
	deliverTimeout = 30 * time.Second
)

// Options is what a watch run was asked to do.
type Options struct {
	Interval time.Duration
	Account  string
	OnMail   string
	NoNotify bool
	NoFlush  bool
	Limit    int
	// QuietFirstPoll records what the first poll finds without announcing
	// it. A watcher that lives and dies with the TUI would otherwise open
	// every session with a popup for mail the user is about to read.
	QuietFirstPoll bool
}

func (o Options) normalized() Options {
	if o.Interval < MinInterval {
		o.Interval = DefaultInterval
	}
	if o.Limit <= 0 {
		o.Limit = DefaultLimit
	}
	return o
}

// Deps is what the watcher needs from the layers around it. Every field is
// supplied by the caller, so the package names no backend and prints nothing.
type Deps struct {
	Config   *cfg_g.Config
	Reporter report.Reporter

	// ClientFor builds a reader for one account. It is how the watcher reaches
	// a backend without importing one.
	ClientFor func(acc cfg_acc.AccountConfig) (mailclient.Reader, error)

	// Flush sends whatever the offline outbox holds. Nil skips it.
	Flush func() error

	// Wake brings back the snoozed messages that are due at now. Nil skips
	// it. Unlike Flush it runs on every poll, the TUI's included, and
	// whether or not the poll itself succeeded: an unreachable account must
	// not keep another account's snoozed mail hidden.
	Wake func(ctx context.Context, now time.Time) error
	// Archive files the new mail of a muted thread out of the inbox
	// (archiveMuted). Nil leaves muted threads' mail in the inbox, announced.
	Archive Archiver

	// Notifier overrides the desktop notifier. Nil means notify-send, unless
	// Options.NoNotify was set, in which case alerts are logged.
	Notifier Notifier

	// Alert shows a stalled-relay alert. Nil means notify-send, unless
	// Options.NoNotify was set, in which case the alert is only logged.
	Alert func(title, body string) error

	// Now is the clock. Nil means time.Now.
	Now func() time.Time

	// clients keeps each account's client from one poll to the next; Run
	// sets it.
	clients *clientCache
	// relays remembers which relay accounts have been announced; Run sets it.
	relays *relayTracker
}

// clientCache keeps one client per account across polls, so a poll costs
// the inbox listing alone rather than a sign-in and, on JMAP, a session fetch
// as well. A client whose listing failed is dropped and built afresh next
// time, so a sign-in renewed meanwhile takes effect.
type clientCache struct {
	clients map[string]mailclient.Reader
}

// client is the account's client: the cached one when Run keeps a cache, or
// a new one.
func (d Deps) client(account cfg_acc.AccountConfig) (mailclient.Reader, error) {
	if d.clients == nil {
		return d.ClientFor(account)
	}
	if c, ok := d.clients.clients[account.Name]; ok {
		return c, nil
	}
	c, err := d.ClientFor(account)
	if err != nil {
		return nil, err
	}
	d.clients.clients[account.Name] = c
	return c, nil
}

// forget drops the account's cached client.
func (d Deps) forget(name string) {
	if d.clients != nil {
		delete(d.clients.clients, name)
	}
}

// ErrAlreadyWatching is Run's answer when another watcher -- a `feral
// watch`, or a TUI that started one -- already polls this download directory.
var ErrAlreadyWatching = errors.New("another feral watcher is already running")

// Run polls every watched account until ctx is cancelled, then returns nil.
//
// Only one watcher runs per download directory: two would share one
// watermark, and a message both of them saw arrive would be announced twice.
// The first to start holds a lock for as long as it runs, and any other
// returns ErrAlreadyWatching at once.
//
// It never panics and never exits the process: a network failure is a warning
// and a longer wait, and one account failing does not stop the others.
func Run(ctx context.Context, deps Deps, opts Options) error {
	opts = opts.normalized()
	rep := report.Or(deps.Reporter)
	if deps.Config == nil {
		return cfg_g.ErrNoAccounts
	}
	release, err := HoldWatchLock(deps.Config.CacheRoot)
	if err != nil {
		return err
	}
	defer release()
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	notifier := deps.Notifier
	if notifier == nil && !opts.NoNotify {
		notifier = notifySendNotifier{}
	}

	if deps.Alert == nil && !opts.NoNotify {
		deps.Alert = notifySendAlert
	}
	deps.clients = &clientCache{clients: map[string]mailclient.Reader{}}
	deps.relays = &relayTracker{alerted: map[string]time.Time{}}
	accounts, err := watchedAccounts(deps.Config, opts.Account)
	if err != nil {
		return err
	}
	rep.Infof("watch: polling %d account(s) every %s", len(accounts), opts.Interval)

	wait := opts.Interval
	quiet := opts.QuietFirstPoll
	for {
		cycleOpts, cycleNotifier := opts, notifier
		if quiet {
			cycleOpts.OnMail, cycleNotifier = "", recordOnly{rep}
			quiet = false
		}
		wakeSnoozed(ctx, deps, rep, now)
		err := pollOnce(ctx, deps, cycleOpts, rep, cycleNotifier, accounts, now)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			rep.Warnf("watch: %v", err)
			wait = backoff(wait)
		} else {
			wait = opts.Interval
			flushOutbox(deps, opts, rep)
		}
		if !sleep(ctx, wait) {
			return nil
		}
	}
}

// watchedAccounts resolves which accounts to poll: the named one, or all of
// them.
func watchedAccounts(config *cfg_g.Config, name string) ([]cfg_acc.AccountConfig, error) {
	if config == nil || len(config.Accounts) == 0 {
		return nil, cfg_g.ErrNoAccounts
	}
	if name == "" {
		return config.Accounts, nil
	}
	account, err := cfg_g.Lookup(config.Accounts, name)
	if err != nil {
		return nil, err
	}
	return []cfg_acc.AccountConfig{*account}, nil
}

// pollOnce runs one cycle over every watched account. It reports the failures
// it met rather than stopping at the first, so one unreachable account does not
// silence the rest.
func pollOnce(ctx context.Context, deps Deps, opts Options, rep report.Reporter,
	notifier Notifier, accounts []cfg_acc.AccountConfig, now func() time.Time) error {
	var errs []error
	for _, account := range accounts {
		if ctx.Err() != nil {
			return nil
		}
		// A local mbox file has no server to poll: what is new in it is
		// what the TUI finds when it reads the file.
		if account.IsMbox() {
			continue
		}
		if account.InboxMaxAge != "" {
			checkRelay(ctx, deps, rep, account, now())
			continue
		}
		if err := checkAccount(ctx, deps, opts, rep, notifier, account, now); err != nil {
			rep.Warnf("watch: %s: %v", account.Name, err)
			errs = append(errs, fmt.Errorf("%s: %w", account.Name, err))
		}
	}
	return errors.Join(errs...)
}

// checkAccount polls one account: list the newest messages in its inbox, work out which
// ones are new against the watermark, announce them, then record them.
func checkAccount(ctx context.Context, deps Deps, opts Options, rep report.Reporter,
	notifier Notifier, account cfg_acc.AccountConfig, now func() time.Time) error {
	client, err := deps.client(account)
	if err != nil {
		return err
	}
	ids, err := client.LatestInboxIDs(ctx, opts.Limit)
	if err != nil {
		deps.forget(account.Name)
		return fmt.Errorf("list the inbox: %w", err)
	}

	root := deps.Config.CacheRoot
	state, err := readState(root)
	if err != nil {
		return err
	}
	if !state.Known(account.Name) {
		if err := commitSeen(root, account.Name, ids, now()); err != nil {
			return err
		}
		rep.Infof("watch: %s: first run; recorded %d existing message(s) without notifying",
			account.Name, len(ids))
		return nil
	}

	since := state.Accounts[account.Name].LastChecked.Add(-staleGrace)
	arrivals := newMessages(ctx, rep, account, client, state.Unseen(account.Name, ids), since)
	arrivals = deps.archiveMuted(ctx, rep, account, arrivals)
	if len(arrivals) > 0 && ctx.Err() == nil {
		deliver(ctx, opts, rep, notifier, notificationsOf(account, arrivals))
	}
	if ctx.Err() != nil {
		// Stopped before the alerts went out -- the TUI closing mid-poll.
		// Leave the watermark alone, so the next watcher announces them.
		return nil
	}
	return commitSeen(root, account.Name, ids, now())
}

// recordOnly is the notifier of a quiet first poll: what it is handed is
// recorded as seen, and only counted in the log.
type recordOnly struct{ rep report.Reporter }

func (r recordOnly) Notify(_ context.Context, news []Notification) error {
	r.rep.Infof("watch: %s: %d message(s) arrived while nothing was watching; recorded without notifying",
		news[0].Account, len(news))
	return nil
}

// newMessages reads the summary of each new message worth announcing. It runs
// after the watermark has been read and before it is written, so a message
// whose summary cannot be read is still recorded as seen rather than
// announced again forever. A message dated before since is recorded without
// being announced -- or archived, if its thread is muted; see staleGrace.
func newMessages(ctx context.Context, rep report.Reporter, account cfg_acc.AccountConfig,
	client mailclient.Reader, ids []email.ID, since time.Time) []*email.Email {
	var arrivals []*email.Email
	for _, id := range ids {
		if ctx.Err() != nil {
			return arrivals
		}
		summary, err := client.GetMessageSummary(id)
		if err != nil {
			rep.Warnf("watch: %s: cannot read %s: %v", account.Name, id, err)
			continue
		}
		if !summary.EmailDate.IsZero() && summary.EmailDate.Before(since) {
			continue
		}
		summary.ID = id
		arrivals = append(arrivals, summary)
	}
	return arrivals
}

// notificationsOf is what one poll of an account announces: one desktop
// notification for all of arrivals, and the --on-mail hook once per message.
func notificationsOf(account cfg_acc.AccountConfig, arrivals []*email.Email) []Notification {
	news := make([]Notification, 0, len(arrivals))
	for _, summary := range arrivals {
		news = append(news, Notification{
			Account: account.Name,
			From:    email.FormatSenderVerbose(summary.FromRaw),
			Subject: summary.Subject,
			Date:    summary.FormattedDate,
			ID:      summary.ID,
		})
	}
	return news
}

// deliver shows one account's new mail: a single desktop notification for
// the lot -- ten messages arriving together are one popup, not ten -- or a log
// line per message when there is no notifier, and then the --on-mail hook for
// each.
func deliver(ctx context.Context, opts Options, rep report.Reporter, notifier Notifier, news []Notification) {
	notified := false
	if notifier != nil {
		if err := notifier.Notify(ctx, news); err != nil {
			rep.Warnf("watch: desktop notification failed: %v", err)
		} else {
			notified = true
		}
	}
	for _, n := range news {
		if !notified {
			rep.Infof("watch: [%s] %s -- %s", n.Account, n.From, n.Subject)
		}
		if opts.OnMail == "" || ctx.Err() != nil {
			continue
		}
		if err := runHook(ctx, opts.OnMail, n); err != nil {
			rep.Warnf("watch: --on-mail %s failed: %v", opts.OnMail, err)
		}
	}
}

// flushOutbox sends anything queued while offline, once a clean cycle has
// confirmed the network is reachable.
//
// Under --read-only or --dry-run it does nothing: flushing is a server
// mutation, and the client decorator would refuse each send anyway, leaving a
// failed-attempt record behind on every poll.
func flushOutbox(deps Deps, opts Options, rep report.Reporter) {
	if opts.NoFlush || deps.Flush == nil {
		return
	}
	if deps.Config != nil && (deps.Config.ReadOnly || deps.Config.DryRun) {
		return
	}
	if err := deps.Flush(); err != nil {
		rep.Warnf("watch: outbox flush failed: %v", err)
	}
}

// wakeSnoozed brings back the snoozed messages that are due, before the
// poll, so the poll that follows sees them in the inbox.
//
// Under --read-only or --dry-run it does nothing: moving a message back is a
// server mutation, and the record would be updated for a move the read-only
// decorator only simulated -- the message would stay in the snooze folder
// with nothing left to bring it back.
func wakeSnoozed(ctx context.Context, deps Deps, rep report.Reporter, now func() time.Time) {
	if deps.Wake == nil || ctx.Err() != nil {
		return
	}
	if deps.Config != nil && (deps.Config.ReadOnly || deps.Config.DryRun) {
		return
	}
	if err := deps.Wake(ctx, now()); err != nil {
		rep.Warnf("watch: snoozed messages: %v", err)
	}
}

// readState loads the watermark under the cache lock, so a second watcher
// sharing the directory reads a consistent file.
func readState(root string) (*State, error) {
	release, err := dirlock.Guard(root)
	if release != nil {
		defer release()
	}
	if err != nil {
		return nil, err
	}
	return LoadState(root)
}

// commitSeen records ids as seen under the cache lock.
func commitSeen(root, name string, ids []email.ID, at time.Time) error {
	release, err := dirlock.Guard(root)
	if release != nil {
		defer release()
	}
	if err != nil {
		return err
	}
	state, err := LoadState(root)
	if err != nil {
		return err
	}
	state.MarkSeen(name, ids, at)
	return state.Save(root)
}

// backoff doubles the wait after a failure, up to MaxBackoff.
func backoff(current time.Duration) time.Duration {
	next := current * 2
	switch {
	case next > MaxBackoff:
		return MaxBackoff
	case next < DefaultInterval:
		return DefaultInterval
	default:
		return next
	}
}

// sleep waits for d, or reports false if ctx was cancelled first.
func sleep(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
