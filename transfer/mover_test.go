package transfer

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/cache/msg"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/label"
	"github.com/sarielhp/feral/mailclient"
	"github.com/sarielhp/feral/report"
)

const sampleRaw = "Message-ID: <m1@example.com>\nDate: Wed, 02 Sep 2026 10:00:00 +0000\nFrom: a@example.com\nSubject: one\n\nbody"

func moverAccounts(t *testing.T, srcCfg func(*cfg_g.Config)) (src, dst *mailclient.MockMailClient) {
	t.Helper()
	sc := &cfg_g.Config{SelectedAccount: "src", AccountCache: cachedir.At(t.TempDir())}
	if srcCfg != nil {
		srcCfg(sc)
	}
	src = &mailclient.MockMailClient{BackendTypeVal: "mbox", Cfg: sc}
	dst = &mailclient.MockMailClient{BackendTypeVal: "gmail", Cfg: &cfg_g.Config{SelectedAccount: "dst", AccountCache: cachedir.At(t.TempDir())}}
	return src, dst
}

func sampleItem(t *testing.T, acct cachedir.Account) Item {
	t.Helper()
	if err := msg.Store(acct, "m1", []byte(sampleRaw), time.Now()); err != nil {
		t.Fatal(err)
	}
	return Item{ID: "m1", Parsed: email.ParseReader(bytes.NewReader([]byte(sampleRaw)), "m1", ""), Raw: []byte(sampleRaw), Dest: label.FromServer("archive/received")}
}

// A move uploads, confirms, removes, and leaves no journal behind.
func TestMoverMoves(t *testing.T) {
	src, dst := moverAccounts(t, nil)
	var calls []removal
	m, err := NewMover(src, dst, label.FromServer("INBOX"), Options{Remove: recordingRemover(&calls, nil)})
	if err != nil {
		t.Fatal(err)
	}
	res := m.Move(sampleItem(t, src.Cfg.AccountCache))
	if !res.OK() || res.DestID == "" || res.Existed {
		t.Fatalf("Move = %+v; want an upload", res)
	}
	if len(dst.UploadedEmails) != 1 || len(calls) != 1 || calls[0].ids[0] != "m1" {
		t.Errorf("uploads %d, removals %+v; want one of each", len(dst.UploadedEmails), calls)
	}
	if err = m.Close(); err != nil {
		t.Fatal(err)
	}
	j, err := OpenJournal(src.Cfg.AccountCache)
	if err != nil || len(j.Pending()) != 0 {
		t.Errorf("journal after a clean move: %v, %v", j.Pending(), err)
	}
}

// A message already on the destination is not uploaded again; a removal that
// fails leaves the message owed in the journal for the next run.
func TestMoverDedupsAndJournalsAFailedRemoval(t *testing.T) {
	src, dst := moverAccounts(t, nil)
	it := sampleItem(t, src.Cfg.AccountCache)
	if err := msg.Store(dst.Cfg.AccountCache, "d1", []byte(sampleRaw), time.Now()); err != nil {
		t.Fatal(err)
	}
	dst.DownloadedIDs = map[string][]email.ID{"archive/received": {"d1"}}

	var calls []removal
	m, err := NewMover(src, dst, label.FromServer("INBOX"), Options{Remove: recordingRemover(&calls, errors.New("locked"))})
	if err != nil {
		t.Fatal(err)
	}
	res := m.Move(it)
	if res.Failed != StageRemove || !res.Existed {
		t.Fatalf("Move = %+v; want the removal to fail after a dedup hit", res)
	}
	if len(dst.UploadedEmails) != 0 {
		t.Errorf("a message already on the destination was uploaded again")
	}
	_ = m.Close()
	j, err := OpenJournal(src.Cfg.AccountCache)
	if err != nil || len(j.Pending()) != 1 {
		t.Errorf("the owed removal was not journaled: %v, %v", j.Pending(), err)
	}
}

// Under --read-only or --dry-run the source's removal is simulated, so nothing
// is journaled: the journal would record a change that never happened.
func TestMoverKeepsNoJournalWhenTheSourceCannotChange(t *testing.T) {
	for name, set := range map[string]func(*cfg_g.Config){
		"read-only": func(c *cfg_g.Config) { c.ReadOnly = true },
		"dry-run":   func(c *cfg_g.Config) { c.DryRun = true },
	} {
		src, dst := moverAccounts(t, set)
		m, err := NewMover(src, dst, label.FromServer("INBOX"), Options{Remove: recordingRemover(new([]removal), errors.New("blocked"))})
		if err != nil {
			t.Fatal(err)
		}
		_ = m.Move(sampleItem(t, src.Cfg.AccountCache))
		_ = m.Close()
		j, err := OpenJournal(src.Cfg.AccountCache)
		if err != nil || len(j.Pending()) != 0 {
			t.Errorf("%s: journal %v, %v; want nothing recorded", name, j.Pending(), err)
		}
	}
}

// A Mover opened without a Remover copies, and refuses to move.
func TestMoverCopyOnly(t *testing.T) {
	src, dst := moverAccounts(t, nil)
	m, err := NewMover(src, dst, label.FromServer("INBOX"), Options{Reporter: report.Discard()})
	if err != nil {
		t.Fatal(err)
	}
	it := sampleItem(t, src.Cfg.AccountCache)
	if res := m.Move(it); res.OK() {
		t.Error("a copy-only Mover moved")
	}
	if res := m.Copy(it); !res.OK() || len(dst.UploadedEmails) != 1 {
		t.Errorf("Copy = %+v, %d upload(s)", res, len(dst.UploadedEmails))
	}
	if res := m.Copy(it); !res.Existed || len(dst.UploadedEmails) != 1 {
		t.Errorf("a second Copy = %+v, %d upload(s); want a dedup hit", res, len(dst.UploadedEmails))
	}
}
