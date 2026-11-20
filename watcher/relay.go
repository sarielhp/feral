package watcher

import (
	"context"
	"os/exec"
	"time"

	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/inboxage"
	"github.com/sarielhp/feral/report"
)

// relayReminder is how often a relay that stays stale is announced again.
// Once per poll would be a popup every minute; once ever is forgotten.
const relayReminder = 30 * time.Minute

// relayTracker remembers, per relay account, whether the last check found it
// wrong and when that was last announced, so an alert fires when a relay
// goes wrong and then only as a reminder. It lives in memory: a restarted
// watcher announces a stale relay once more, which is what a person who just
// started it wants.
type relayTracker struct {
	alerted map[string]time.Time
}

// checkRelay polls a relay account: it is not announced for new mail -- its
// mail passes through on its way elsewhere -- but checked for mail that has
// sat in its inbox too long. A stale or uncheckable relay is reported, never
// returned as a poll error: it says nothing about the network, and backing
// off would only delay seeing it clear.
func checkRelay(ctx context.Context, deps Deps, rep report.Reporter, account cfg_acc.AccountConfig, now time.Time) {
	var st inboxage.Status
	client, err := deps.client(account)
	if err != nil {
		st = inboxage.Status{Account: account.Name, Err: err}
	} else {
		st = inboxage.Check(ctx, client, account, now)
	}
	if ctx.Err() != nil {
		return
	}
	if st.Err != nil {
		deps.forget(account.Name)
	}
	if deps.relays == nil {
		deps.relays = &relayTracker{alerted: map[string]time.Time{}}
	}
	deps.relays.record(deps, rep, st, now)
}

// record reports a relay's status when it changed or a reminder is due.
func (r *relayTracker) record(deps Deps, rep report.Reporter, st inboxage.Status, now time.Time) {
	last, wasBad := r.alerted[st.Account]
	if st.OK() {
		if wasBad {
			delete(r.alerted, st.Account)
			rep.Infof("watch: %s", st.Summary(now))
		}
		return
	}
	if wasBad && now.Sub(last) < relayReminder {
		return
	}
	r.alerted[st.Account] = now
	rep.Warnf("watch: %s", st.Summary(now))
	if deps.Alert == nil {
		return
	}
	if err := deps.Alert("Mail relay stalled — "+st.Account, markupEscaper.Replace(st.Summary(now))); err != nil {
		rep.Warnf("watch: desktop notification failed: %v", err)
	}
}

// notifySendAlert is the desktop alert for a stalled relay: critical
// urgency, so it stays on screen until dismissed.
func notifySendAlert(title, body string) error {
	ctx, cancel := context.WithTimeout(context.Background(), deliverTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "notify-send", "--app-name=feral", "--icon=dialog-warning",
		"--urgency=critical", "--", title, body)
	cmd.WaitDelay = time.Second
	return cmd.Run()
}
