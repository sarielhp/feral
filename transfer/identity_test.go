package transfer

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/cache/msg"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/label"
	"github.com/sarielhp/feral/mailclient"
)

func TestSame(t *testing.T) {
	t1 := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)

	// Distinct Message-IDs must NEVER match, even if Subject and Date are identical
	e1 := &email.Email{
		MessageID: "<msg1@example.com>",
		Subject:   "Delivery Status Notification (Failure)",
		EmailDate: t1,
	}
	e2 := &email.Email{
		MessageID: "<msg2@example.com>",
		Subject:   "Delivery Status Notification (Failure)",
		EmailDate: t2,
	}
	if Same(e1, e2) {
		t.Errorf("expected emails with different Message-IDs to NOT match")
	}

	// Same Message-ID matches
	e3 := &email.Email{
		MessageID: "<msg1@example.com>",
		Subject:   "Different Subject",
	}
	if !Same(e1, e3) {
		t.Errorf("expected emails with same Message-ID to match")
	}

	// Fallback: one lacks Message-ID, but subjects and dates match
	e4 := &email.Email{
		Subject:   "Delivery Status Notification (Failure)",
		EmailDate: t1,
	}
	if !Same(e1, e4) {
		t.Errorf("expected fallback match when one Message-ID is missing")
	}

	// Zero dates must NOT match under fallback
	e5 := &email.Email{
		Subject: "Some Subject",
	}
	e6 := &email.Email{
		Subject: "Some Subject",
	}
	if Same(e5, e6) {
		t.Errorf("expected zero-date emails without Message-ID to NOT match")
	}
}

func TestSameAccount(t *testing.T) {
	// Same account cache despite different client wrapper instances
	c1 := &mailclient.MockMailClient{
		BackendTypeVal: "gmail",
		Cfg:            &cfg_g.Config{SelectedAccount: "", Paths: cfg_g.Paths{CacheRoot: "/cache/acc1"}, AccountCache: cachedir.At("/cache/acc1")},
	}
	c2 := &mailclient.MockMailClient{
		BackendTypeVal: "gmail",
		Cfg:            &cfg_g.Config{SelectedAccount: "", Paths: cfg_g.Paths{CacheRoot: "/cache/acc1"}, AccountCache: cachedir.At("/cache/acc1")},
	}
	if !SameAccount(c1, c2) {
		t.Errorf("expected the same account cache to be recognized as same account even if SelectedAccount is empty")
	}

	// Different DownloadDir
	c3 := &mailclient.MockMailClient{
		BackendTypeVal: "gmail",
		Cfg:            &cfg_g.Config{SelectedAccount: "other", Paths: cfg_g.Paths{CacheRoot: "/cache/acc2"}, AccountCache: cachedir.At("/cache/acc2")},
	}
	if SameAccount(c1, c3) {
		t.Errorf("expected different DownloadDir to not be same account")
	}

	// Empty DownloadDir, matching SelectedAccount
	c4 := &mailclient.MockMailClient{
		BackendTypeVal: "gmail",
		Cfg:            &cfg_g.Config{SelectedAccount: "work"},
	}
	c5 := &mailclient.MockMailClient{
		BackendTypeVal: "gmail",
		Cfg:            &cfg_g.Config{SelectedAccount: "WORK"},
	}
	if !SameAccount(c4, c5) {
		t.Errorf("expected matching SelectedAccount to be recognized as same account when DownloadDir is empty")
	}

	// Empty DownloadDir, different SelectedAccount
	c6 := &mailclient.MockMailClient{
		BackendTypeVal: "gmail",
		Cfg:            &cfg_g.Config{SelectedAccount: "personal"},
	}
	if SameAccount(c4, c6) {
		t.Errorf("expected different SelectedAccount to not be same account")
	}

	// Empty DownloadDir and SelectedAccount, matching Username
	c7 := &mailclient.MockMailClient{
		BackendTypeVal: "gmail",
		Cfg:            &cfg_g.Config{Credentials: cfg_g.Credentials{Username: "user@example.com"}},
	}
	c8 := &mailclient.MockMailClient{
		BackendTypeVal: "gmail",
		Cfg:            &cfg_g.Config{Credentials: cfg_g.Credentials{Username: "USER@example.com"}},
	}
	if !SameAccount(c7, c8) {
		t.Errorf("expected matching Username to be recognized as same account when DownloadDir and SelectedAccount are empty")
	}

	// Different BackendType
	c9 := &mailclient.MockMailClient{
		BackendTypeVal: "outlook",
		Cfg:            &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: "/cache/acc1"}, AccountCache: cachedir.At("/cache/acc1")},
	}
	if SameAccount(c1, c9) {
		t.Errorf("expected different BackendType to not be same account")
	}

	// Nil client checks
	if SameAccount(nil, c1) || SameAccount(c1, nil) || SameAccount(nil, nil) {
		t.Errorf("expected nil clients to not be same account")
	}
}

func TestInDestIndexMatching(t *testing.T) {
	tempDir := t.TempDir()
	fixedDate := time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)

	eml1 := "Message-ID: <dest1@example.com>\nDate: Fri, 18 Sep 2026 10:00:00 +0000\nSubject: Project Alpha\n\nbody"
	eml2 := "Date: Fri, 18 Sep 2026 10:00:00 +0000\nSubject: Fallback Topic\n\nbody"

	_ = msg.Store(cachedir.At(tempDir), "dest_001", []byte(eml1), fixedDate)
	_ = msg.Store(cachedir.At(tempDir), "dest_002", []byte(eml2), fixedDate)

	mockClient := &mailclient.MockMailClient{
		Labels:        []string{"Archive"},
		DownloadedIDs: map[string][]email.ID{"Archive": {"dest_001", "dest_002"}},
		Cfg:           &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: tempDir}, AccountCache: cachedir.At(tempDir)},
	}

	destCache := make(FolderCache)

	// 1. Authoritative match by Message-ID
	srcMatchID := &email.Email{
		MessageID: "<dest1@example.com>",
		Subject:   "Completely Different Subject",
		EmailDate: fixedDate,
	}
	if !InDest(mockClient, label.FromServer("Archive"), "src_001", srcMatchID, destCache) {
		t.Error("expected match by authoritative Message-ID")
	}

	// 2. Authoritative mismatch: both have Message-ID, but differ (even if Subject and Date match!)
	srcMismatchID := &email.Email{
		MessageID: "<different@example.com>",
		Subject:   "Project Alpha",
		EmailDate: fixedDate,
	}
	if InDest(mockClient, label.FromServer("Archive"), "src_002", srcMismatchID, destCache) {
		t.Error("expected mismatch when both possess differing Message-IDs despite same Subject and Date")
	}

	// 3. Fallback heuristic match when dest lacks Message-ID
	srcFallbackMatch := &email.Email{
		MessageID: "<src_has_id@example.com>",
		Subject:   "Fallback Topic",
		EmailDate: fixedDate,
	}
	if !InDest(mockClient, label.FromServer("Archive"), "src_003", srcFallbackMatch, destCache) {
		t.Error("expected fallback match when dest lacks Message-ID and Subject+Date match")
	}

	// 4. Mismatch when Subject and Date don't match any dest email
	srcNoMatch := &email.Email{
		Subject:   "Unrelated Topic",
		EmailDate: fixedDate,
	}
	if InDest(mockClient, label.FromServer("Archive"), "src_004", srcNoMatch, destCache) {
		t.Error("expected no match for unrelated message")
	}

	// 5. Index caching verification: remove files from disk to prove lookups use in-memory index
	_ = os.Remove(filepath.Join(tempDir, "dest_001.eml"))
	_ = os.Remove(filepath.Join(tempDir, "dest_002.eml"))
	if !InDest(mockClient, label.FromServer("Archive"), "src_001", srcMatchID, destCache) {
		t.Error("expected match from cached index even after deleting disk files")
	}
}
