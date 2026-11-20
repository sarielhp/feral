package tui

import (
	"errors"
	"testing"

	"github.com/sarielhp/feral/cache"
	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/mailclient"
)

func pendingClient(t *testing.T, readOnly bool) (*mailclient.MockMailClient, cachedir.Account) {
	t.Helper()
	acc := cachedir.At(t.TempDir())
	return &mailclient.MockMailClient{Cfg: &cfg_g.Config{
		ReadOnly: readOnly, AccountCache: acc, Paths: cfg_g.Paths{CacheRoot: t.TempDir()},
	}}, acc
}

// A mark whose server call fails stays pending, so no sync undoes it and the
// next load retries it.
func TestMarkReadFailureStaysPending(t *testing.T) {
	client, acc := pendingClient(t, false)
	client.FailOn = map[string]error{"MarkAsRead": errors.New("network down")}
	m := newTuiModel(client, "INBOX", nil)
	m.rawEmails = []email.Email{{ID: "x"}}
	m.rebuildVisibleEmails()

	if cmd := m.markAsReadCmd("x"); cmd != nil {
		cmd()
	}
	if p, ok := cache.LoadPendingReads(acc)["x"]; !ok || !p.Read {
		t.Fatalf("the failed mark is not pending: %v", cache.LoadPendingReads(acc))
	}

	client.FailOn = nil
	if cmd := m.markAsReadCmd("y"); cmd != nil {
		cmd()
	}
	if _, ok := cache.LoadPendingReads(acc)["y"]; ok {
		t.Error("a mark the server accepted is still offered for retry")
	}
}

// Retrying sends the pending marks in their direction and confirms the ones
// the server accepts; a failure is counted instead.
func TestRetryPendingReads(t *testing.T) {
	client, acc := pendingClient(t, false)
	_ = cache.MarkIDsRead(acc, []email.ID{"r"}, nil)
	_ = cache.MarkIDsUnread(acc, []email.ID{"u"})

	client.FailOn = map[string]error{"MarkAsUnread": errors.New("still down")}
	retryPendingReads(client)
	pending := cache.LoadPendingReads(acc)
	if _, ok := pending["r"]; ok {
		t.Error("the accepted read mark is still pending")
	}
	if p, ok := pending["u"]; !ok || p.Tries != 1 {
		t.Errorf("the failed unread mark: %+v (present %v), want one counted try", p, ok)
	}
}

// A folder load retries pending marks first -- never under read-only, where
// the client simulates the mark and confirming it would drop a mark the server
// never saw.
func TestLoadRetriesPendingReadsUnlessReadOnly(t *testing.T) {
	for _, readOnly := range []bool{false, true} {
		client, acc := pendingClient(t, readOnly)
		_ = cache.MarkIDsRead(acc, []email.ID{"r"}, nil)
		m := newTuiModel(client, "INBOX", nil)
		m.cfg = client.Cfg
		if cmd := m.loadEmailsCmd("INBOX"); cmd != nil {
			cmd()
		}
		_, stillPending := cache.LoadPendingReads(acc)["r"]
		if readOnly != stillPending {
			t.Errorf("read-only=%v: pending after load = %v", readOnly, stillPending)
		}
	}
}

// A mark made while a load is in flight survives the load's older snapshot of
// the read state; a mark made before the load was dispatched is left to it.
func TestLoadKeepsMarksMadeSinceDispatch(t *testing.T) {
	m := newTuiModel(nil, "INBOX", nil)
	m.rawEmails = []email.Email{{ID: "before"}, {ID: "during"}, {ID: "replied"}}
	m.rebuildVisibleEmails()

	m.noteRead("before", true)
	gen := m.flagGen // the load is dispatched here
	m.noteRead("during", true)
	m.noteReplied("replied")

	stale := map[email.ID]bool{"before": true, "during": false, "replied": false}
	res, _ := m.Update(emailsLoadedMsg{
		folder: "INBOX", emails: []email.Email{{ID: "before"}, {ID: "during"}, {ID: "replied"}},
		isRead: stale, isReplied: map[email.ID]bool{}, flagGen: gen,
	})
	m = res.(*tuiModel)

	if !visibleRow(t, m, "during").IsRead {
		t.Error("a mark made while the load was in flight was undone by its snapshot")
	}
	if !visibleRow(t, m, "replied").IsReplied {
		t.Error("a reply made while the load was in flight lost its r")
	}
	if _, kept := m.flagEdits["before"]; kept {
		t.Error("a mark the load came after should be left to it")
	}
	if _, kept := m.flagEdits["during"]; !kept {
		t.Error("a mark newer than the load must outlive it, for the next one")
	}
}
