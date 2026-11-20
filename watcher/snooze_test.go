package watcher

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/mailclient"
	"github.com/sarielhp/feral/snooze"
)

// A watcher run wakes what its clock says is due -- the TUI's watcher too,
// which never flushes -- and leaves what is not.
func TestRunWakesSnoozedMessagesByItsClock(t *testing.T) {
	client := newMock(nil, nil)
	deps := testDeps(t, client, &recordingReporter{})
	root := deps.Config.CacheRoot
	for _, e := range []snooze.Entry{
		{Account: "work", ID: "due", Until: fixedNow().Add(-time.Minute), Folder: snooze.FolderName},
		{Account: "work", ID: "later", Until: fixedNow().Add(time.Minute), Folder: snooze.FolderName},
	} {
		if _, err := snooze.Put(root, e, false); err != nil {
			t.Fatal(err)
		}
	}
	waker := &snooze.Waker{Root: root, Accounts: deps.Config.Accounts,
		ClientFor: func(cfg_acc.AccountConfig) (snooze.WakeClient, error) { return client, nil }}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var sawNow time.Time
	deps.Wake = func(ctx context.Context, now time.Time) error {
		sawNow = now
		_, err := waker.Due(ctx, now)
		cancel() // one cycle is the test
		return err
	}
	if err := Run(ctx, deps, Options{NoNotify: true, NoFlush: true}); err != nil {
		t.Fatal(err)
	}
	if !sawNow.Equal(fixedNow()) {
		t.Errorf("Wake saw %v; want the injected clock %v", sawNow, fixedNow())
	}
	if len(client.MovedEmails) != 1 || fmt.Sprint(client.MovedEmails[0].MessageIDs) != "[due]" ||
		client.MovedEmails[0].DestLabel != client.InboxFolder().String() || fmt.Sprint(client.MarkedUnread) != "[due]" {
		t.Errorf("moved %+v, unread %v; want due moved to INBOX and unread", client.MovedEmails, client.MarkedUnread)
	}
	f, err := snooze.Load(root)
	if err != nil || len(f.Entries) != 1 || f.Entries[0].ID != "later" {
		t.Errorf("record = %+v, %v; want only later", f, err)
	}
}

func TestWakeSnoozedHonoursReadOnlyAndDryRun(t *testing.T) {
	rep := &recordingReporter{}
	calls := 0
	deps := Deps{Wake: func(context.Context, time.Time) error { calls++; return fmt.Errorf("boom") }}
	for _, cfg := range []*cfg_g.Config{{ReadOnly: true}, {DryRun: true}} {
		deps.Config = cfg
		wakeSnoozed(context.Background(), deps, rep, fixedNow)
	}
	if calls != 0 {
		t.Fatalf("wake ran %d times under --read-only/--dry-run", calls)
	}
	deps.Config = &cfg_g.Config{}
	wakeSnoozed(context.Background(), deps, rep, fixedNow)
	if calls != 1 || !rep.has("snoozed messages: boom") {
		t.Errorf("calls %d, report %v", calls, rep.lines)
	}
}

var _ snooze.WakeClient = (*mailclient.MockMailClient)(nil)
