package runner

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/feral/cache/dirlock"
	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/outbox"
	"github.com/sarielhp/feral/report"
)

func TestOutboxServiceListAndRemove(t *testing.T) {
	tmpDir := t.TempDir()
	config := &cfg_g.Config{
		Paths: cfg_g.Paths{
			CacheRoot: tmpDir,
		},
	}

	obDir := outbox.Dir(tmpDir)
	e, err := outbox.Enqueue(obDir, "work", []string{"bob@example.com"}, "Hello", []byte("RFC822 body"))
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	svc := OutboxService{
		Rep: report.Discard(),
		Out: &bytes.Buffer{},
	}

	list, err := svc.List(config)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 1 || list[0].ID != e.ID {
		t.Fatalf("unexpected list: %+v", list)
	}

	if removeErr := svc.Remove(config, e.ID); removeErr != nil {
		t.Fatalf("Remove failed: %v", removeErr)
	}

	listAfter, err := svc.List(config)
	if err != nil {
		t.Fatalf("List after remove failed: %v", err)
	}
	if len(listAfter) != 0 {
		t.Fatalf("expected 0 items, got %d", len(listAfter))
	}
}

func TestOutboxServiceFlushUnconfiguredAccount(t *testing.T) {
	tmpDir := t.TempDir()
	config := &cfg_g.Config{
		Paths: cfg_g.Paths{
			CacheRoot: tmpDir,
		},
		Accounts: []cfg_acc.AccountConfig{
			{Name: "other", Type: "mbox"},
		},
	}

	obDir := outbox.Dir(tmpDir)
	e, err := outbox.Enqueue(obDir, "unknown-acc", []string{"bob@example.com"}, "Unreachable", []byte("RFC822 body"))
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	svc := OutboxService{
		Rep: report.Discard(),
		Out: &bytes.Buffer{},
	}

	results, err := svc.Flush(config)
	if err != nil {
		t.Fatalf("Flush returned unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Success {
		t.Errorf("expected failure for unconfigured account")
	}

	// Verify attempt was recorded
	item, _, err := outbox.Get(obDir, e.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if item.Attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", item.Attempts)
	}
}

func TestOutboxFlushRefusedUnderReadOnlyAndDryRun(t *testing.T) {
	for _, cfg := range []cfg_g.Config{{ReadOnly: true}, {DryRun: true}} {
		tmpDir := t.TempDir()
		cfg.CacheRoot = tmpDir
		obDir := outbox.Dir(tmpDir)
		e, err := outbox.Enqueue(obDir, "acc", []string{"bob@example.com"}, "S", []byte("body"))
		if err != nil {
			t.Fatal(err)
		}
		svc := OutboxService{Rep: report.Discard()}
		if _, err = svc.Flush(&cfg); !errors.Is(err, ErrFlushRefused) {
			t.Errorf("read-only %v dry-run %v: want ErrFlushRefused, got %v", cfg.ReadOnly, cfg.DryRun, err)
		}
		item, _, err := outbox.Get(obDir, e.ID)
		if err != nil || item.Attempts != 0 {
			t.Errorf("a refused flush must not record an attempt: %+v %v", item, err)
		}
	}
}

// A flush holds the outbox lock, so a second flush waits rather than reading
// the same entries and sending them twice.
func TestOutboxFlushWaitsForTheOutboxLock(t *testing.T) {
	tmpDir := t.TempDir()
	config := &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: tmpDir}}
	release, err := dirlock.Guard(outbox.Dir(tmpDir))
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		_, _ = OutboxService{Rep: report.Discard()}.Flush(config)
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("flush ran while another holder had the outbox lock")
	case <-time.After(100 * time.Millisecond):
	}
	release()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("flush did not proceed after the lock was released")
	}
}

// A flush sends only what is due. The clock is injected: at 07:59 the entry
// due at 08:00 is left alone, untouched; at 08:00 it is attempted (it fails,
// as its account is not configured, and the attempt is recorded -- the proof
// that the flush tried to send it).
func TestOutboxFlushSendsOnlyWhatIsDue(t *testing.T) {
	tmpDir := t.TempDir()
	config := &cfg_g.Config{
		Paths:    cfg_g.Paths{CacheRoot: tmpDir},
		Accounts: []cfg_acc.AccountConfig{{Name: "other", Type: "mbox"}},
	}
	obDir := outbox.Dir(tmpDir)
	due := time.Date(2026, 11, 21, 8, 0, 0, 0, time.Local)
	e, err := outbox.EnqueueEntry(obDir, outbox.Entry{Account: "unknown-acc", Subject: "Later", SendAfter: due}, []byte("body"))
	if err != nil {
		t.Fatal(err)
	}
	clock := due.Add(-time.Minute)
	svc := OutboxService{Rep: report.Discard(), Now: func() time.Time { return clock }}

	results, err := svc.Flush(config)
	if err != nil || len(results) != 0 {
		t.Fatalf("a flush before the due time = %+v, %v; want nothing attempted", results, err)
	}
	if item, _, getErr := outbox.Get(obDir, e.ID); getErr != nil || item.Attempts != 0 {
		t.Fatalf("an entry not yet due was touched: %+v %v", item, getErr)
	}

	clock = due
	results, err = svc.Flush(config)
	if err != nil || len(results) != 1 || results[0].ID != e.ID || results[0].Success {
		t.Fatalf("a flush at the due time = %+v, %v; want the entry attempted", results, err)
	}
	if item, _, getErr := outbox.Get(obDir, e.ID); getErr != nil || item.Attempts != 1 {
		t.Errorf("the due entry's attempt was not recorded: %+v %v", item, getErr)
	}
}

// An entry a newer build wrote is reported as a failed flush whatever its due
// time says, and is never attempted.
func TestOutboxFlushRefusesANewerFormatEntry(t *testing.T) {
	tmpDir := t.TempDir()
	config := &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: tmpDir}}
	home := filepath.Join(outbox.Dir(tmpDir), "scheduled")
	if err := os.MkdirAll(home, 0700); err != nil {
		t.Fatal(err)
	}
	meta := `{"id":"f3","account":"work","format":3,"send_after":"2099-01-01T00:00:00Z"}`
	if err := os.WriteFile(filepath.Join(home, "f3.json"), []byte(meta), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "f3.eml"), []byte("body"), 0600); err != nil {
		t.Fatal(err)
	}
	results, err := OutboxService{Rep: report.Discard()}.Flush(config)
	if err != nil || len(results) != 1 || results[0].Success || !strings.Contains(results[0].Err, "newer feral") {
		t.Fatalf("Flush = %+v, %v; want the format-3 entry refused", results, err)
	}
	if data, _ := os.ReadFile(filepath.Join(home, "f3.json")); string(data) != meta {
		t.Errorf("the refused entry was rewritten: %s", data)
	}
}
