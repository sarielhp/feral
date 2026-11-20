package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/label"
	"github.com/sarielhp/feral/mailclient"
	"github.com/sarielhp/feral/mailops"
	"github.com/sarielhp/feral/report"
)

const spoolMessages = "From a@example.com Tue Sep 01 09:00:00 2026\n" +
	"From: A <a@example.com>\nSubject: first\nDate: Tue, 01 Sep 2026 09:00:00 +0000\nMessage-ID: <one@example.com>\n\nbody one\n\n" +
	"From b@example.com Wed Sep 02 09:00:00 2026\n" +
	"From: B <b@example.com>\nSubject: second\nDate: Wed, 02 Sep 2026 09:00:00 +0000\nMessage-ID: <two@example.com>\n\nbody two\n"

// A move out of a writable mbox account uploads to the destination and then
// rewrites the file without what moved, leaving no journal owed.
func TestTransferServiceMovesOutOfAnMbox(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "spool")
	if err := os.WriteFile(path, []byte(spoolMessages), 0o600); err != nil {
		t.Fatal(err)
	}
	acc := cfg_acc.AccountConfig{Name: "spool", Type: "mbox", Path: path}
	cfg := &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: filepath.Join(dir, "cache"), ConfigDir: dir}}
	src, err := mailops.NewMailClient(acc, cfg, report.Discard())
	if err != nil {
		t.Fatal(err)
	}
	ids, err := src.FetchAndDownloadEmails(src.InboxFolder(), "")
	if err != nil || len(ids) != 2 {
		t.Fatalf("read: %v, %v", ids, err)
	}
	dst := &mailclient.MockMailClient{BackendTypeVal: "gmail",
		Cfg: &cfg_g.Config{SelectedAccount: "home", AccountCache: cachedir.At(t.TempDir())}}

	moved, err := TransferService{}.MoveAcross(src, dst, src.InboxFolder(), ids[:1], label.FromServer("INBOX"))
	if err != nil || len(moved) != 1 || moved[0] != ids[0] {
		t.Fatalf("MoveAcross = %v, %v; want %s", moved, err, ids[0])
	}
	if len(dst.UploadedEmails) != 1 {
		t.Errorf("uploaded %d; want 1", len(dst.UploadedEmails))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "one@example.com") || !strings.Contains(string(data), "two@example.com") {
		t.Errorf("file after the move:\n%s", data)
	}
	if _, err := os.Stat(filepath.Join(src.Config().AccountCache.Path(), "splice_journal.json")); !os.IsNotExist(err) {
		t.Errorf("a journal was left owing: %v", err)
	}
}

// A read-only side refuses before anything is uploaded: a read-only source
// would only simulate the removal, copying while reporting a move.
func TestTransferServiceRefusesAReadOnlySide(t *testing.T) {
	writable := func(name string) *mailclient.MockMailClient {
		return &mailclient.MockMailClient{BackendTypeVal: "gmail", RawMessages: map[email.ID][]byte{"m1": []byte(spoolMessages)},
			Cfg: &cfg_g.Config{SelectedAccount: cfg_g.AccountName(name), AccountCache: cachedir.At(t.TempDir())}}
	}
	for _, tc := range []struct {
		name     string
		src, dst func() mailclient.MailClient
	}{
		{"source", func() mailclient.MailClient {
			c := writable("a")
			return mailclient.NewCheckingMailClient(mailclient.NewReadOnlyMailClient(c, "a"), "a", nil)
		}, func() mailclient.MailClient { return writable("b") }},
		{"destination", func() mailclient.MailClient { return writable("a") }, func() mailclient.MailClient {
			c := writable("b")
			c.Cfg.DryRun = true
			return c
		}},
	} {
		dst := tc.dst()
		_, err := TransferService{}.MoveAcross(tc.src(), dst, label.FromServer("INBOX"), []email.ID{"m1"}, label.FromServer("INBOX"))
		if err == nil || !strings.Contains(err.Error(), "read-only") {
			t.Errorf("%s read-only: err %v; want a refusal", tc.name, err)
		}
		if mock, ok := dst.(*mailclient.MockMailClient); ok && len(mock.UploadedEmails) != 0 {
			t.Errorf("%s read-only: %d uploaded; want none", tc.name, len(mock.UploadedEmails))
		}
	}
}
