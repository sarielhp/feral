package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/email"
)

// serverSays is what a folder sync does: write the server's answer for ids.
func serverSays(t *testing.T, acc cachedir.Account, read bool, ids ...email.ID) {
	t.Helper()
	if err := UpdateReadState(acc, func(state map[email.ID]bool) {
		for _, id := range ids {
			state[id] = read
		}
	}); err != nil {
		t.Fatal(err)
	}
}

// A mark made here survives a sync whose listing still has the old state --
// the server call failed, or had not landed when the listing was taken.
func TestPendingRead_SyncKeepsLocalMark(t *testing.T) {
	acc := cachedir.At(t.TempDir())
	if err := MarkIDsRead(acc, []email.ID{"r"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := MarkIDsUnread(acc, []email.ID{"u"}); err != nil {
		t.Fatal(err)
	}
	serverSays(t, acc, false, "r")
	serverSays(t, acc, true, "u", "other")

	state := LoadReadState(acc)
	if !state["r"] {
		t.Error("a sync undid the local read mark")
	}
	if state["u"] {
		t.Error("a sync undid the local unread mark")
	}
	if !state["other"] {
		t.Error("a message with no local mark must take the server's state")
	}
}

// A confirmed mark still outranks a sync for a short grace -- against a
// listing taken before the server applied it -- and is no longer retried.
func TestPendingRead_ConfirmedIsNotRetriedButStillProtected(t *testing.T) {
	acc := cachedir.At(t.TempDir())
	_ = MarkIDsRead(acc, []email.ID{"r"}, nil)
	if err := ConfirmReads(acc, []email.ID{"r"}); err != nil {
		t.Fatal(err)
	}
	if got := LoadPendingReads(acc); len(got) != 0 {
		t.Errorf("a confirmed mark is offered for retry: %v", got)
	}
	serverSays(t, acc, false, "r")
	if !LoadReadState(acc)["r"] {
		t.Error("a stale listing undid a mark confirmed moments ago")
	}
}

// After its lifetime a mark no longer outranks the server: a change made since
// in another client must show.
func TestPendingRead_ExpiredMarkYieldsToServer(t *testing.T) {
	dir := t.TempDir()
	acc := cachedir.At(dir)
	_ = MarkIDsRead(acc, []email.ID{"old", "confirmed"}, nil)
	writeLedger(t, dir, map[email.ID]PendingRead{
		"old":       {Read: true, At: time.Now().Add(-PendingReadTTL - time.Minute)},
		"confirmed": {Read: true, At: time.Now().Add(-confirmedReadGrace - time.Second), Confirmed: true},
	})
	serverSays(t, acc, false, "old", "confirmed")
	state := LoadReadState(acc)
	if state["old"] || state["confirmed"] {
		t.Errorf("expired marks outranked the server: %v", state)
	}
	if _, err := os.Stat(filepath.Join(dir, pendingReadsFile)); !os.IsNotExist(err) {
		t.Error("an empty ledger should be removed")
	}
}

// Failed retries are counted, and a mark that keeps failing is dropped.
func TestPendingRead_RetryFailuresDropTheMark(t *testing.T) {
	acc := cachedir.At(t.TempDir())
	_ = MarkIDsRead(acc, []email.ID{"r"}, nil)
	for i := 1; i < PendingReadMaxTries; i++ {
		if err := NoteReadRetryFailed(acc, []email.ID{"r"}); err != nil {
			t.Fatal(err)
		}
		if got := LoadPendingReads(acc)["r"].Tries; got != i {
			t.Fatalf("after %d failures tries = %d", i, got)
		}
	}
	_ = NoteReadRetryFailed(acc, []email.ID{"r"})
	if got := LoadPendingReads(acc); len(got) != 0 {
		t.Errorf("a mark that failed %d times is still pending: %v", PendingReadMaxTries, got)
	}
}

// LoadPendingReads offers each unconfirmed mark with its direction.
func TestPendingRead_LoadOffersDirection(t *testing.T) {
	acc := cachedir.At(t.TempDir())
	_ = MarkIDsRead(acc, []email.ID{"r"}, nil)
	_ = MarkIDsUnread(acc, []email.ID{"u"})
	got := LoadPendingReads(acc)
	if len(got) != 2 || !got["r"].Read || got["u"].Read {
		t.Errorf("pending = %+v", got)
	}
}

// A damaged ledger is ignored rather than stopping marks and syncs.
func TestPendingRead_DamagedLedgerIgnored(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, pendingReadsFile), []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}
	acc := cachedir.At(dir)
	if err := MarkIDsRead(acc, []email.ID{"r"}, nil); err != nil {
		t.Fatalf("a damaged ledger stopped a mark: %v", err)
	}
	if !LoadPendingReads(acc)["r"].Read {
		t.Error("the mark was not recorded over the damaged ledger")
	}
}

func writeLedger(t *testing.T, dir string, pending map[email.ID]PendingRead) {
	t.Helper()
	data, err := json.Marshal(pending)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, pendingReadsFile), data, 0600); err != nil {
		t.Fatal(err)
	}
}
