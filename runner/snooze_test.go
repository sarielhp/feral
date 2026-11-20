package runner

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/cache/folderindex"
	"github.com/sarielhp/feral/cache/msg"
	"github.com/sarielhp/feral/cache/virtual"
	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/mailclient"
	"github.com/sarielhp/feral/report"
	"github.com/sarielhp/feral/snooze"
)

// A message named by the short ID a listing shows is found in the cache with
// its folder, snoozed on the client's own account, then woken by the
// service's clock and forgotten on request.
func TestSnoozeServiceRoundTrip(t *testing.T) {
	root := t.TempDir()
	account := cachedir.ForAccount(root, "work")
	id := email.ID("full-server-id-1")
	raw := []byte("From: Alice <alice@example.com>\r\nSubject: Lunch\r\n\r\nbody\r\n")
	if err := msg.Store(account, id, raw, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := folderindex.Add(account, "Lists", id); err != nil {
		t.Fatal(err)
	}
	config := &cfg_g.Config{SelectedAccount: "work", AccountCache: account,
		Accounts: []cfg_acc.AccountConfig{{Name: "work", Type: "jmap"}}}
	config.CacheRoot = root
	client := &mailclient.MockMailClient{Cfg: config, Labels: []string{"INBOX", "Lists"}}

	now := time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)
	svc := SnoozeService{Rep: report.Discard(), Now: func() time.Time { return now },
		ClientFor: func(cfg_acc.AccountConfig, *cfg_g.Config) (snooze.WakeClient, error) { return client, nil }}

	items, err := svc.Items(client, []string{string(email.ComputeShortID(id))})
	if err != nil || len(items) != 1 || items[0].ID != id || items[0].Folder != "Lists" || items[0].Subject != "Lunch" {
		t.Fatalf("Items = %+v, %v", items, err)
	}
	if _, missErr := svc.Items(client, []string{"nosuchid"}); missErr == nil {
		t.Error("an unknown message resolved")
	}
	done, err := svc.Snooze(config, client, items, now.Add(time.Hour))
	if err != nil || len(done) != 1 || done[0].Account != "work" || done[0].FromFolder != "Lists" {
		t.Fatalf("Snooze = %+v, %v", done, err)
	}

	if due, dueErr := svc.Select(config, nil); dueErr != nil || len(due) != 0 {
		t.Fatalf("due before its time: %+v, %v", due, dueErr)
	}
	if _, selErr := svc.Select(config, []string{"nosuch"}); selErr == nil || !strings.Contains(selErr.Error(), "not snoozed") {
		t.Errorf("unknown ref: %v", selErr)
	}
	now = now.Add(time.Hour)
	woken, err := svc.Wake(config, nil)
	back := client.MovedEmails[len(client.MovedEmails)-1]
	if err != nil || len(woken) != 1 || fmt.Sprint(back.MessageIDs) != "["+string(id)+"]" || back.DestLabel != client.InboxFolder().String() {
		t.Fatalf("Wake = %+v, %v; last move %+v", woken, err, back)
	}
	if left, _ := svc.List(config); len(left) != 0 {
		t.Errorf("record after wake: %+v", left)
	}

	if _, putErr := snooze.Put(root, snooze.Entry{Account: "gone", ID: "x", Until: now}, false); putErr != nil {
		t.Fatal(putErr)
	}
	if _, goneErr := svc.Wake(config, []string{"x"}); goneErr == nil || !strings.Contains(goneErr.Error(), "no longer configured") {
		t.Errorf("wake of an account that is gone: %v", goneErr)
	}
	forgotten, err := svc.Forget(config, []string{"x"})
	if err != nil || len(forgotten) != 1 {
		t.Fatalf("Forget = %+v, %v", forgotten, err)
	}
	if left, _ := svc.List(config); len(left) != 0 {
		t.Errorf("record after forget: %+v", left)
	}
}

// msg last saves a set and then msg snooze names a message from it: the
// cache index that holds the ID may be the set's, which is no server folder,
// so the folder comes from the set's record of where each message was; a
// folder index key, lowercased and escaped, comes back in the server's
// spelling.
func TestSnoozeItemsFromSavedSetAndEscapedFolder(t *testing.T) {
	root := t.TempDir()
	account := cachedir.ForAccount(root, "work")
	raw := []byte("From: a@example.com\r\nSubject: s\r\n\r\nb\r\n")
	for _, id := range []email.ID{"in-set", "nested"} {
		if err := msg.Store(account, id, raw, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	if err := virtual.Save(account, &virtual.Mailbox{Name: "last", MessageIDs: []email.ID{"in-set"},
		FolderMap: map[email.ID]string{"in-set": "INBOX"}}); err != nil {
		t.Fatal(err)
	}
	if err := folderindex.Add(account, "Projects/Q4 Plans", "nested"); err != nil {
		t.Fatal(err)
	}
	config := &cfg_g.Config{SelectedAccount: "work", AccountCache: account}
	client := &mailclient.MockMailClient{Cfg: config, Labels: []string{"INBOX", "Projects/Q4 Plans"}}
	items, err := SnoozeService{}.Items(client, []string{"in-set", "nested"})
	if err != nil {
		t.Fatal(err)
	}
	if items[0].Folder != "INBOX" || items[1].Folder != "Projects/Q4 Plans" {
		t.Errorf("folders %q, %q; want INBOX and Projects/Q4 Plans", items[0].Folder, items[1].Folder)
	}
}

func TestWatchedName(t *testing.T) {
	config := &cfg_g.Config{Accounts: []cfg_acc.AccountConfig{{Name: "work", DisplayName: "Work Mail"}}}
	for in, want := range map[string]string{"": "", "Work Mail": "work", "work": "work", "nosuch": "nosuch"} {
		if got := watchedName(config, in); got != want {
			t.Errorf("watchedName(%q) = %q; want %q", in, got, want)
		}
	}
}
