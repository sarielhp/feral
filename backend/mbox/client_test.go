package mbox

import (
	"context"
	"errors"
	"github.com/sarielhp/feral/cache"
	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/label"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/feral/cache/folderindex"
	"github.com/sarielhp/feral/cache/msg"
	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/mailclient"
)

func TestIngestMbox_Fixtures(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "testdata", "fixtures.mbox")
	cacheDir := t.TempDir()

	ids, err := IngestMbox(fixturePath, cachedir.At(cacheDir), "fixtures.mbox")
	if err != nil {
		t.Fatalf("IngestMbox failed: %v", err)
	}
	if len(ids) != 10 {
		t.Fatalf("expected 10 messages in fixtures.mbox, got %d", len(ids))
	}

	// 2nd call should hit the cache
	cachedIDs, err := IngestMbox(fixturePath, cachedir.At(cacheDir), "fixtures.mbox")
	if err != nil {
		t.Fatalf("2nd IngestMbox failed: %v", err)
	}
	if len(cachedIDs) != len(ids) {
		t.Fatalf("expected %d cached IDs, got %d", len(ids), len(cachedIDs))
	}
	for i := range ids {
		if ids[i] != cachedIDs[i] {
			t.Errorf("mismatch at %d: %q != %q", i, ids[i], cachedIDs[i])
		}
	}
}

func TestMboxClient_BasicOps(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "testdata", "fixtures.mbox")
	cacheDir := t.TempDir()

	acc := cfg_acc.AccountConfig{
		Name:        "mbox",
		DisplayName: "fixtures.mbox",
		Type:        "mbox",
		ReadOnly:    true,
	}
	cfg := &cfg_g.Config{ReadOnly: true, Paths: cfg_g.Paths{CacheRoot: cacheDir}, AccountCache: cachedir.At(cacheDir)}

	client := NewClient(acc, cfg, fixturePath)
	if err := client.Validate(); err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	if client.BackendType() != "mbox" {
		t.Errorf("expected backend type mbox, got %q", client.BackendType())
	}
	if client.InboxFolder().String() != "INBOX" {
		t.Errorf("expected INBOX, got %q", client.InboxFolder())
	}

	items, err := client.GetLabelItems()
	if err != nil || len(items) != 1 || items[0].Name != "INBOX" {
		t.Fatalf("unexpected label items: %v, err: %v", items, err)
	}

	ids, err := client.FetchAndDownloadEmails(label.FromServer("INBOX"), "")
	if err != nil {
		t.Fatalf("FetchAndDownloadEmails failed: %v", err)
	}
	if len(ids) != 10 {
		t.Fatalf("expected 10 IDs, got %d", len(ids))
	}

	raw, err := client.GetRawMessage(ids[0])
	if err != nil || len(raw) == 0 {
		t.Fatalf("GetRawMessage failed: %v", err)
	}
	if !strings.Contains(string(raw), "Fixture 01") {
		t.Errorf("expected raw message to contain 'Fixture 01'")
	}

	summary, err := client.GetMessageSummary(ids[0])
	if err != nil || summary == nil {
		t.Fatalf("GetMessageSummary failed: %v", err)
	}
	if !strings.Contains(summary.Subject, "Fixture 01") {
		t.Errorf("expected summary subject to contain 'Fixture 01', got %q", summary.Subject)
	}

	// Search
	matches, err := client.SearchEmails("third", 10)
	if err != nil {
		t.Fatalf("SearchEmails failed: %v", err)
	}
	if len(matches) != 1 || matches[0] != ids[2] {
		t.Errorf("expected match for 'third' to be %q, got %v", ids[2], matches)
	}

	// Mutating ops must be blocked
	if err := client.DeleteLabel(label.FromServer("INBOX")); err != mailclient.ErrReadOnlyOperationBlocked {
		t.Errorf("expected ErrReadOnlyOperationBlocked on DeleteLabel, got %v", err)
	}
	if err := client.MoveEmail([]email.ID{email.ID(string(ids[0]))}, label.FromServer("INBOX"), label.FromServer("Trash")); err != mailclient.ErrReadOnlyOperationBlocked {
		t.Errorf("expected ErrReadOnlyOperationBlocked on MoveEmail, got %v", err)
	}
	if err := client.SendEmail([]byte("test")); err != mailclient.ErrReadOnlyOperationBlocked {
		t.Errorf("expected ErrReadOnlyOperationBlocked on SendEmail, got %v", err)
	}
}

func TestMboxClient_ContextCancellation(t *testing.T) {
	client := &client{
		localCfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-canceled

	if _, err := client.GetRawMessageContext(ctx, "m1"); !errors.Is(err, context.Canceled) {
		t.Errorf("GetRawMessageContext error = %v, want context.Canceled", err)
	}
	if _, err := client.GetRawMessagesBatchContext(ctx, []email.ID{"m1"}); !errors.Is(err, context.Canceled) {
		t.Errorf("GetRawMessagesBatchContext error = %v, want context.Canceled", err)
	}
	if _, err := client.GetMessageSummaryContext(ctx, "m1"); !errors.Is(err, context.Canceled) {
		t.Errorf("GetMessageSummaryContext error = %v, want context.Canceled", err)
	}
	if _, err := client.GetMessageSummariesBatchContext(ctx, []email.ID{"m1"}); !errors.Is(err, context.Canceled) {
		t.Errorf("GetMessageSummariesBatchContext error = %v, want context.Canceled", err)
	}
	if err := client.PrefetchMessagesContext(ctx, []email.ID{"m1"}); !errors.Is(err, context.Canceled) {
		t.Errorf("PrefetchMessagesContext error = %v, want context.Canceled", err)
	}
}

func TestMboxClient_BatchErrorAggregation(t *testing.T) {
	client := &client{
		localCfg: &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())},
	}

	missingIDs := []string{"msg-err-1", "msg-err-2"}

	// Test GetRawMessagesBatchContext accumulates errors for all failing IDs
	_, rawErr := client.GetRawMessagesBatchContext(context.Background(), email.IDs(missingIDs))
	if rawErr == nil {
		t.Fatal("expected non-nil error when raw messages are missing")
	}
	for _, id := range missingIDs {
		if !strings.Contains(rawErr.Error(), "id "+id) {
			t.Errorf("rawErr missing context for %q: got %v", id, rawErr)
		}
	}

	// Test GetMessageSummariesBatchContext accumulates errors for all failing IDs
	_, sumErr := client.GetMessageSummariesBatchContext(context.Background(), email.IDs(missingIDs))
	if sumErr == nil {
		t.Fatal("expected non-nil error when summaries are missing")
	}
	for _, id := range missingIDs {
		if !strings.Contains(sumErr.Error(), "id "+id) {
			t.Errorf("sumErr missing context for %q: got %v", id, sumErr)
		}
	}
}

func TestIngestMbox_Eml(t *testing.T) {
	dir := t.TempDir()
	// No From_ line, and a body line an mbox parser would unescape.
	raw := "From: a@example.com\r\nSubject: hi\r\nMessage-ID: <x1@example.com>\r\n\r\n>From here on\r\n"
	path := filepath.Join(dir, "msg.EML")
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	cacheDir := cachedir.At(filepath.Join(dir, "cache"))
	ids, err := IngestMbox(path, cacheDir, "msg.EML")
	if err != nil {
		t.Fatalf("IngestMbox: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("expected 1 message, got %d", len(ids))
	}
	got, err := msg.Read(cacheDir, ids[0])
	if err != nil {
		t.Fatalf("read cached message: %v", err)
	}
	if !strings.Contains(string(got), ">From here on") {
		t.Errorf("body was altered: %q", got)
	}
	inbox, err := folderindex.IDs(cacheDir, "INBOX")
	if err != nil || len(inbox) != 1 {
		t.Errorf("INBOX index = %v, %v; want the one message", inbox, err)
	}
}

func TestIngestMbox_EmlRejectsNonMessage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.eml")
	if err := os.WriteFile(path, []byte("just some text\nwith no headers\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := IngestMbox(path, cachedir.At(filepath.Join(dir, "cache")), "notes.eml"); err == nil {
		t.Fatal("expected a non-message .eml to be refused")
	}
}

// Search reads bodies through the viewer's extractor, so a base64 body matches
// on its text and not on its encoding.
func TestMboxClientSearchesBodies(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.mbox")
	body := "From x@example.com Tue Sep 01 09:00:00 2026\n" +
		"From: X <x@example.com>\nSubject: hello\nMessage-ID: <b64@example.com>\n" +
		"Content-Type: text/plain\nContent-Transfer-Encoding: base64\n\n" +
		"VGhlIHF1YXJ0ZXJseSB6ZXBoeXIgcmVwb3J0Cg==\n\n" +
		"From y@example.com Tue Sep 01 10:00:00 2026\n" +
		"From: Y <y@example.com>\nSubject: other\nMessage-ID: <plain@example.com>\n\nnothing here\n"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := &cfg_g.Config{AccountCache: cachedir.At(filepath.Join(dir, "cache"))}
	client := NewClient(cfg_acc.AccountConfig{Name: "m", Type: "mbox"}, cfg, path)
	if _, err := client.FetchAndDownloadEmails(label.FromServer("INBOX"), ""); err != nil {
		t.Fatal(err)
	}
	for query, want := range map[string]int{"ZEPHYR": 1, "VGhlIHF1": 0, "hello": 1, "y@example": 1, "absent": 0} {
		ids, err := client.SearchEmails(query, 0)
		if err != nil || len(ids) != want {
			t.Errorf("SearchEmails(%q) = %v, %v; want %d match(es)", query, ids, err, want)
		}
	}
}

// Listing the folders reads the file and counts the INBOX: every message, and
// as unread those the local read state does not mark read. The TUI lists
// folders before it loads the inbox, so counting only what was already cached
// showed INBOX (0) on a first open.
func TestMboxClientCountsInbox(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.mbox")
	body := "From x@example.com Tue Sep 01 09:00:00 2026\nSubject: one\nMessage-ID: <1@example.com>\n\na\n\n" +
		"From y@example.com Tue Sep 01 10:00:00 2026\nSubject: two\nMessage-ID: <2@example.com>\n\nb\n"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := &cfg_g.Config{AccountCache: cachedir.At(filepath.Join(dir, "cache"))}
	client := NewClient(cfg_acc.AccountConfig{Name: "m", Type: "mbox"}, cfg, path)

	items, err := client.GetLabelItems()
	if err != nil || len(items) != 1 || items[0].MessagesTotal != 2 || items[0].MessagesUnread != 2 {
		t.Fatalf("first listing: %+v, %v; want INBOX with 2 unread of 2", items, err)
	}
	ids, err := folderindex.IDs(cfg.AccountCache, "INBOX")
	if err != nil || len(ids) != 2 {
		t.Fatalf("the listing did not index the file: %v, %v", ids, err)
	}
	if err = cache.MarkIDsRead(cfg.AccountCache, ids[:1], nil); err != nil {
		t.Fatal(err)
	}
	items, err = client.GetLabelItems()
	if err != nil || len(items) != 1 || items[0].MessagesTotal != 2 || items[0].MessagesUnread != 1 {
		t.Errorf("after a read: %+v, %v; want INBOX with 1 unread of 2", items, err)
	}
	// The tree keeps the total only of a folder that holds messages
	// (IsLabel); without it the INBOX drew as (1/0).
	if tree := label.BuildLabelTree(items); len(tree) != 1 || tree[0].MessagesTotal != 2 {
		t.Errorf("the folder tree lost the INBOX total: %+v", tree)
	}

	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if items, err = client.GetLabelItems(); err != nil || len(items) != 1 || items[0].MessagesTotal != 0 {
		t.Errorf("a missing file: %+v, %v; want the INBOX listed without counts", items, err)
	}
}
