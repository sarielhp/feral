package mailops

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/feral/cache/meta"
	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/label"
	"github.com/sarielhp/feral/mailclient"
	"github.com/sarielhp/feral/report"
	"github.com/sarielhp/feral/transfer"
)

const twoMessages = "From a@example.com Tue Sep 01 09:00:00 2026\n" +
	"From: A <a@example.com>\nSubject: first\nDate: Tue, 01 Sep 2026 09:00:00 +0000\nMessage-ID: <one@example.com>\n\nbody one\n\n" +
	"From b@example.com Wed Sep 02 09:00:00 2026\n" +
	"From: B <b@example.com>\nSubject: second\nDate: Wed, 02 Sep 2026 09:00:00 +0000\nMessage-ID: <two@example.com>\n\nbody two\n"

// An account of type mbox is built by the ordinary factory: it gets its own
// cache under the root, a manifest, the read-only decorator when it asks for
// one, and no token.
func TestNewMailClientBuildsAnMboxAccount(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "archive.mbox")
	if err := os.WriteFile(path, []byte(twoMessages), 0600); err != nil {
		t.Fatal(err)
	}
	acc := cfg_acc.AccountConfig{Name: "old", Type: "mbox", Path: path, ReadOnly: true}
	cfg := &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: filepath.Join(dir, "cache"), ConfigDir: dir}}

	client, err := NewMailClient(acc, cfg, report.Discard())
	if err != nil {
		t.Fatalf("NewMailClient: %v", err)
	}
	if vErr := client.Validate(); vErr != nil {
		t.Fatalf("Validate: %v", vErr)
	}
	ids, err := client.FetchAndDownloadEmails(label.FromServer("INBOX"), "")
	if err != nil || len(ids) != 2 {
		t.Fatalf("FetchAndDownloadEmails = %v, %v; want 2 ids", ids, err)
	}
	if got := client.Config().AccountCache; got != cfg.AccountCacheFor(acc) {
		t.Errorf("account cache %v, want %v", got, cfg.AccountCacheFor(acc))
	}
	m, ok, err := meta.Read(cfg.AccountCacheFor(acc))
	if err != nil || !ok || m.IDScheme != meta.SchemeMbox {
		t.Errorf("manifest %+v (%v, %v), want the mbox scheme", m, ok, err)
	}
	if err := client.DeleteMessages(ids[:1], label.FromServer("INBOX")); !errors.Is(err, mailclient.ErrReadOnlyOperationBlocked) && err != nil {
		t.Errorf("DeleteMessages on a read-only mbox account: %v", err)
	}
	if tok, err := TokenPath(dir, acc); err != nil || tok != "" {
		t.Errorf("TokenPath = %q, %v; an mbox account has no token", tok, err)
	}
}

// An mbox account with no path is refused when its client is built, rather
// than reading whatever an empty path resolves to.
func TestNewMailClientRefusesAnMboxAccountWithoutAPath(t *testing.T) {
	dir := t.TempDir()
	acc := cfg_acc.AccountConfig{Name: "old", Type: "mbox"}
	cfg := &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: dir}}
	if _, err := NewMailClient(acc, cfg, report.Discard()); err == nil {
		t.Fatal("NewMailClient accepted an mbox account with no path")
	}
}

// A writable mbox account, built by the factory, deletes from its file through
// transfer.DeleteRemover -- only the message whose content was moved -- and
// the cache follows.
func TestMboxAccountDeleteRemoverRewritesTheFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "spool")
	if err := os.WriteFile(path, []byte(twoMessages), 0600); err != nil {
		t.Fatal(err)
	}
	acc := cfg_acc.AccountConfig{Name: "spool", Type: "mbox", Path: path}
	cfg := &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: filepath.Join(dir, "cache"), ConfigDir: dir}}
	client, err := NewMailClient(acc, cfg, report.Discard())
	if err != nil {
		t.Fatal(err)
	}
	inbox := client.InboxFolder()
	ids, err := client.FetchAndDownloadEmails(inbox, "")
	if err != nil || len(ids) != 2 {
		t.Fatalf("read: %v, %v", ids, err)
	}
	raw, err := client.GetRawMessage(ids[0])
	if err != nil {
		t.Fatal(err)
	}

	remove := transfer.DeleteRemover(client, report.Discard())
	kept, err := remove(inbox, []transfer.Removal{
		{ID: ids[0], Hash: transfer.ContentHash(raw)},
		{ID: ids[1], Hash: transfer.ContentHash([]byte("not this message"))},
	})
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if len(kept) != 1 || kept[0] != ids[1] {
		t.Errorf("kept %v; want the message whose content differs", kept)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "one@example.com") || !strings.Contains(string(data), "two@example.com") {
		t.Errorf("file after removal:\n%s", data)
	}
	after, err := client.FetchAndDownloadEmails(inbox, "")
	if err != nil || len(after) != 1 || after[0] != ids[1] {
		t.Errorf("IDs after removal: %v, %v; want [%s]", after, err, ids[1])
	}
}
