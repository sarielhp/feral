package watcher

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/mailclient"
)

type recordedAlert struct{ title, body string }

// relayDeps is testDeps for one relay account, with alerts captured.
func relayDeps(t *testing.T, client *mailclient.MockMailClient, rep *recordingReporter, alerts *[]recordedAlert) Deps {
	t.Helper()
	deps := testDeps(t, client, rep)
	deps.Config.Accounts = []cfg_acc.AccountConfig{{Name: "relay", Type: "gmail", InboxMaxAge: "15m"}}
	deps.Alert = func(title, body string) error {
		*alerts = append(*alerts, recordedAlert{title, body})
		return nil
	}
	deps.relays = &relayTracker{alerted: map[string]time.Time{}}
	return deps
}

func pollRelay(t *testing.T, deps Deps, rep *recordingReporter, at time.Time) {
	t.Helper()
	notifier := &fakeNotifier{}
	err := pollOnce(context.Background(), deps, Options{}.normalized(), rep, notifier, deps.Config.Accounts,
		func() time.Time { return at })
	if err != nil {
		t.Fatalf("a relay check must not fail the poll: %v", err)
	}
	if notifier.calls != 0 {
		t.Errorf("a relay's mail was announced as new mail: %+v", notifier.got)
	}
}

// A relay is checked for mail left in its inbox, alerted once when it goes
// stale, reminded only after relayReminder, and reported again when it
// drains -- and its passing mail is never announced as new mail.
func TestRelayAlertsOnceThenReminds(t *testing.T) {
	now := fixedNow()
	stuck := mailclient.InboxEntry{ID: "a", Received: now.Add(-20 * time.Minute), Subject: "stuck"}
	client := &mailclient.MockMailClient{InboxIDs: []email.ID{"a"}, InboxEntries: []mailclient.InboxEntry{stuck}}
	rep := &recordingReporter{}
	var alerts []recordedAlert
	deps := relayDeps(t, client, rep, &alerts)

	pollRelay(t, deps, rep, now)
	if len(alerts) != 1 || !rep.has("at least 20m old") {
		t.Fatalf("stale relay: alerts=%+v log=%v", alerts, rep.lines)
	}
	pollRelay(t, deps, rep, now.Add(2*time.Minute))
	if len(alerts) != 1 {
		t.Errorf("still stale two minutes later: want no second alert, got %d", len(alerts))
	}
	pollRelay(t, deps, rep, now.Add(relayReminder))
	if len(alerts) != 2 {
		t.Errorf("still stale after %s: want a reminder, got %d alert(s)", relayReminder, len(alerts))
	}

	client.InboxEntries = nil
	pollRelay(t, deps, rep, now.Add(relayReminder+time.Minute))
	if !rep.has("inbox drained") {
		t.Errorf("a drained relay is not reported: %v", rep.lines)
	}
	if len(alerts) != 2 {
		t.Errorf("draining must not raise an alert, got %d", len(alerts))
	}
}

// A relay that cannot be checked -- its own sign-in expired -- is alerted,
// not mistaken for a drained one, and its client is rebuilt next time.
func TestRelayThatCannotBeCheckedIsAlerted(t *testing.T) {
	client := &mailclient.MockMailClient{FailOn: map[string]error{
		"InboxMessageBefore": mailclient.ErrInteractiveAuthRequired,
	}}
	rep := &recordingReporter{}
	var alerts []recordedAlert
	deps := relayDeps(t, client, rep, &alerts)
	built := 0
	deps.ClientFor = func(cfg_acc.AccountConfig) (mailclient.Reader, error) {
		built++
		return client, nil
	}
	deps.clients = &clientCache{clients: map[string]mailclient.Reader{}}

	pollRelay(t, deps, rep, fixedNow())
	pollRelay(t, deps, rep, fixedNow().Add(time.Minute))
	if len(alerts) != 1 || !rep.has("cannot check") {
		t.Errorf("an uncheckable relay: alerts=%+v log=%v", alerts, rep.lines)
	}
	if built != 2 {
		t.Errorf("a failed client must be rebuilt on the next poll: built %d time(s)", built)
	}
}

func TestRelayAlertFailureIsLogged(t *testing.T) {
	now := fixedNow()
	client := &mailclient.MockMailClient{InboxEntries: []mailclient.InboxEntry{{ID: "a", Received: now.Add(-time.Hour)}}}
	rep := &recordingReporter{}
	var alerts []recordedAlert
	deps := relayDeps(t, client, rep, &alerts)
	deps.Alert = func(string, string) error { return errors.New("no notify-send") }
	pollRelay(t, deps, rep, now)
	if !rep.has("desktop notification failed") {
		t.Errorf("a failed alert must be logged: %v", rep.lines)
	}
}
