package transfer

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/feral/cache/folderindex"
	"github.com/sarielhp/feral/cache/msg"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/label"
	"github.com/sarielhp/feral/report"
)

// DeleteRemover deletes only what still hashes to what was uploaded: a copy
// that differs is left (its ID went to another message, as a re-read mbox
// renumbers duplicates), one already gone is skipped, and a removal with no
// hash is never made blind.
func TestDeleteRemoverVerifiesContent(t *testing.T) {
	src, _ := moverAccounts(t, nil)
	acct := src.Cfg.AccountCache
	store := func(id email.ID, raw string) {
		if err := msg.Store(acct, id, []byte(raw), time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	store("same", sampleRaw)
	store("changed", sampleRaw+"\nmore")
	store("nohash", sampleRaw)
	if err := folderindex.ReplaceAll(acct, "INBOX", []email.ID{"same", "changed", "nohash"}); err != nil {
		t.Fatal(err)
	}

	remove := DeleteRemover(src, report.Discard())
	kept, err := remove(label.FromServer("INBOX"), []Removal{
		{ID: "same", Hash: ContentHash([]byte(sampleRaw))},
		{ID: "changed", Hash: ContentHash([]byte(sampleRaw))},
		{ID: "nohash"},
		{ID: "gone", Hash: ContentHash([]byte(sampleRaw))},
	})
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if len(src.DeletedMessages) != 1 || src.DeletedMessages[0] != "same" {
		t.Errorf("deleted %v; want only the message whose content matched", src.DeletedMessages)
	}
	if len(kept) != 2 {
		t.Errorf("kept %v; want the changed and the unhashed message reported as left", kept)
	}
}

// With Defer, a Mover removes everything it moved in one call at Flush, and
// the journal owes nothing afterwards.
func TestMoverDeferredRemovalIsOneCall(t *testing.T) {
	src, dst := moverAccounts(t, nil)
	var calls []removal
	m, err := NewMover(src, dst, label.FromServer("INBOX"), Options{Remove: recordingRemover(&calls, nil), Defer: true})
	if err != nil {
		t.Fatal(err)
	}
	it := sampleItem(t, src.Cfg.AccountCache)
	if res := m.Move(it); !res.OK() {
		t.Fatalf("Move: %+v", res)
	}
	if len(calls) != 0 {
		t.Fatalf("a deferred removal was made at once: %+v", calls)
	}
	j, _ := OpenJournal(src.Cfg.AccountCache)
	if got := j.Pending(); len(got) != 1 || got[0].Hash != ContentHash(it.Raw) {
		t.Errorf("journal before Flush: %+v; want the message owed, with its hash", got)
	}
	ids, kept, err := m.Flush()
	if err != nil || len(ids) != 1 || len(kept) != 0 || len(calls) != 1 {
		t.Fatalf("Flush = %v, %v, %v; calls %+v", ids, kept, err, calls)
	}
	_ = m.Close()
	j, _ = OpenJournal(src.Cfg.AccountCache)
	if len(j.Pending()) != 0 {
		t.Errorf("journal after Flush: %+v", j.Pending())
	}
}

// MoveAll moves what it can, removes all of it in one call under Defer, and
// reports by message what it could not: here one that cannot be read and one
// the Remover leaves in place.
func TestMoveAllReportsWhatDidNotMove(t *testing.T) {
	src, dst := moverAccounts(t, nil)
	raw2 := strings.Replace(sampleRaw, "<m1@", "<m2@", 1)
	src.RawMessages = map[email.ID][]byte{"m1": []byte(sampleRaw), "m2": []byte(raw2)}
	var calls int
	remove := func(_ label.Folder, msgs []Removal) ([]email.ID, error) {
		calls++
		if len(msgs) != 2 {
			t.Errorf("removal batch %v; want both uploaded messages in one call", msgs)
		}
		return []email.ID{"m2"}, nil
	}
	m, err := NewMover(src, dst, label.FromServer("INBOX"), Options{Remove: remove, Defer: true})
	if err != nil {
		t.Fatal(err)
	}
	moved, err := m.MoveAll([]email.ID{"m1", "m2", "missing"}, label.FromServer("INBOX"))
	if !slices.Equal(moved, []email.ID{"m1"}) || calls != 1 {
		t.Errorf("moved %v in %d call(s); want [m1] in one", moved, calls)
	}
	if !errors.Is(err, ErrLeftInPlace) || !strings.Contains(err.Error(), "missing: read") {
		t.Errorf("err = %v; want the kept message and the unreadable one named", err)
	}
	if len(dst.UploadedEmails) != 2 {
		t.Errorf("uploaded %d; want 2", len(dst.UploadedEmails))
	}
}
