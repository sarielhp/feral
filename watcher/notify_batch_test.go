package watcher

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/feral/cache/dirlock"
	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/mailclient"
)

// Mail that arrives together is one notification, however many messages it
// is, while the --on-mail hook still sees each message.
func TestCheckAccountNotifiesOncePerPoll(t *testing.T) {
	rep := &recordingReporter{}
	notifier := &fakeNotifier{}
	messages := map[email.ID][]byte{
		"a": rawMessage("old", "alice@example.com"),
		"b": rawMessage("first", "bob@example.com"),
		"c": rawMessage("second", "carol@example.com"),
		"d": rawMessage("third", "dave@example.com"),
	}
	client := newMock([]cfg_acc.MessageFolderRef{{MessageID: "a", Folder: "Inbox"}}, messages)
	deps := testDeps(t, client, rep)
	account := deps.Config.Accounts[0]
	if err := checkAccount(context.Background(), deps, Options{}, rep, notifier, account, deps.Now); err != nil {
		t.Fatal(err)
	}
	client.InboxIDs = []email.ID{"b", "c", "d"}
	if err := checkAccount(context.Background(), deps, Options{}, rep, notifier, account, deps.Now); err != nil {
		t.Fatal(err)
	}
	if notifier.calls != 1 || len(notifier.got) != 3 {
		t.Fatalf("three new messages made %d notification(s) of %d, want 1 of 3", notifier.calls, len(notifier.got))
	}
}

func TestNotificationTextCountsListsAndEscapes(t *testing.T) {
	one := []Notification{{Account: "work", From: "Ann <ann@x.example>", Subject: "<b>Don't</b> & more"}}
	title, body := notificationText(one)
	if title != "New mail — work" {
		t.Errorf("single title = %q", title)
	}
	if strings.ContainsAny(body, "<>") || !strings.Contains(body, "&lt;b&gt;Don't&lt;/b&gt; &amp; more") {
		t.Errorf("body is not escaped for markup: %q", body)
	}

	var many []Notification
	for i := range 8 {
		many = append(many, Notification{Account: "work", From: "s", Subject: string(rune('a' + i))})
	}
	title, body = notificationText(many)
	if title != "8 new messages — work" {
		t.Errorf("batch title = %q", title)
	}
	lines := strings.Split(body, "\n")
	if len(lines) != notifyListed+1 || lines[notifyListed] != "…and 3 more" {
		t.Errorf("batch body lists %d lines, want %d and a remainder: %q", len(lines), notifyListed+1, body)
	}
}

// A second watcher on the same download directory refuses at once rather
// than announcing every message a second time, and the lock goes when the
// first one stops.
func TestRunRefusesASecondWatcher(t *testing.T) {
	deps := testDeps(t, newMock(nil, nil), nil)
	release, err := dirlock.TryLock(lockFile(deps.Config.CacheRoot)) // a running watcher
	if err != nil {
		t.Fatal(err)
	}
	if beside := Run(context.Background(), deps, Options{}); !errors.Is(beside, ErrAlreadyWatching) {
		t.Fatalf("Run beside a running watcher = %v, want ErrAlreadyWatching", beside)
	}
	release()

	stopped, stop := context.WithCancel(context.Background())
	stop()
	if after := Run(stopped, deps, Options{}); after != nil {
		t.Fatalf("Run once the other stopped = %v", after)
	}
	again, err := dirlock.TryLock(lockFile(deps.Config.CacheRoot))
	if err != nil {
		t.Fatalf("Run kept its lock after returning: %v", err)
	}
	again()
}

// knownAccount returns deps whose account has been polled once, with "a"
// seen, and the client now offering "b" as new mail.
func knownAccount(t *testing.T, rep *recordingReporter) Deps {
	t.Helper()
	messages := map[email.ID][]byte{
		"a": rawMessage("old", "alice@example.com"),
		"b": rawMessage("fresh", "bob@example.com"),
	}
	client := newMock([]cfg_acc.MessageFolderRef{{MessageID: "a", Folder: "Inbox"}}, messages)
	deps := testDeps(t, client, rep)
	if err := checkAccount(context.Background(), deps, Options{}, rep, &fakeNotifier{},
		deps.Config.Accounts[0], deps.Now); err != nil {
		t.Fatal(err)
	}
	client.InboxIDs = []email.ID{"b"}
	return deps
}

// A watcher started with QuietFirstPoll records what arrived while nothing
// watched instead of opening with a popup for it.
func TestRunQuietFirstPollRecordsWithoutNotifying(t *testing.T) {
	rep := &recordingReporter{}
	deps := knownAccount(t, rep)
	notifier := &fakeNotifier{}
	deps.Notifier = notifier
	ctx, cancel := context.WithCancel(context.Background())
	deps.Now = func() time.Time { cancel(); return fixedNow() } // stop after the first poll

	if err := Run(ctx, deps, Options{QuietFirstPoll: true}); err != nil {
		t.Fatal(err)
	}
	if notifier.calls != 0 {
		t.Fatalf("the quiet first poll notified: %+v", notifier.got)
	}
	if !rep.has("recorded without notifying") {
		t.Fatalf("the quiet poll did not log what it recorded: %v", rep.lines)
	}
	state, err := LoadState(deps.Config.CacheRoot)
	if err != nil {
		t.Fatal(err)
	}
	if unseen := state.Unseen("work", []email.ID{"b"}); len(unseen) != 0 {
		t.Fatalf("the quiet poll left %v unseen", unseen)
	}
}

// cancelNotifier stops the watcher while it is delivering, as closing the
// TUI mid-poll does.
type cancelNotifier struct{ cancel context.CancelFunc }

func (c cancelNotifier) Notify(context.Context, []Notification) error {
	c.cancel()
	return nil
}

// Mail found by a poll that was stopped mid-way stays unseen, so whichever
// watcher runs next announces it rather than nobody.
func TestCheckAccountCancelledKeepsMailUnseen(t *testing.T) {
	rep := &recordingReporter{}
	deps := knownAccount(t, rep)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := checkAccount(ctx, deps, Options{}, rep, cancelNotifier{cancel},
		deps.Config.Accounts[0], deps.Now); err != nil {
		t.Fatal(err)
	}
	state, err := LoadState(deps.Config.CacheRoot)
	if err != nil {
		t.Fatal(err)
	}
	if unseen := state.Unseen("work", []email.ID{"b"}); len(unseen) != 1 {
		t.Fatal("a cancelled poll recorded its mail as seen")
	}
}

// Run keeps each account's client between polls, so a poll costs the inbox
// listing alone, and builds a new one after a failed listing.
func TestClientCacheReusesUntilAListingFails(t *testing.T) {
	rep := &recordingReporter{}
	client := newMock([]cfg_acc.MessageFolderRef{{MessageID: "a", Folder: "Inbox"}},
		map[email.ID][]byte{"a": rawMessage("old", "alice@example.com")})
	deps := testDeps(t, client, rep)
	built := 0
	deps.ClientFor = func(cfg_acc.AccountConfig) (mailclient.Reader, error) {
		built++
		return client, nil
	}
	deps.clients = &clientCache{clients: map[string]mailclient.Reader{}}
	account := deps.Config.Accounts[0]
	poll := func() error {
		return checkAccount(context.Background(), deps, Options{}, rep, &fakeNotifier{}, account, deps.Now)
	}

	for range 3 {
		if err := poll(); err != nil {
			t.Fatal(err)
		}
	}
	if built != 1 {
		t.Fatalf("three polls built %d clients, want 1", built)
	}
	client.FailOn = map[string]error{"LatestInboxIDs": errors.New("token revoked")}
	if err := poll(); err == nil {
		t.Fatal("a failed listing was not reported")
	}
	client.FailOn = nil
	if err := poll(); err != nil {
		t.Fatal(err)
	}
	if built != 2 {
		t.Fatalf("after a failed listing %d clients were built, want a fresh one (2)", built)
	}
}
