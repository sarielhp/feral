package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/feral/backend/mbox"
	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/mailclient"
	"github.com/sarielhp/feral/report"
	"github.com/sarielhp/feral/tui"
)

const mboxTestMsgA = "From: Alice <alice@example.com>\r\n" +
	"Subject: First\r\n" +
	"Message-ID: <first@example.com>\r\n" +
	"Date: Sun, 20 Sep 2026 10:00:00 +0000\r\n" +
	"Content-Type: text/plain\r\n" +
	"\r\n" +
	"First body.\r\n"

const mboxTestMsgB = "From: Bob <bob@example.com>\r\n" +
	"Subject: Second\r\n" +
	"Message-ID: <second@example.com>\r\n" +
	"Date: Sun, 20 Sep 2026 11:00:00 +0000\r\n" +
	"Content-Type: text/plain\r\n" +
	"\r\n" +
	"Second body.\r\n"

// ingestCount reads path back through the real mbox parser, which is the only
// proof that an appended entry is a well-formed one.
func ingestCount(t *testing.T, path string) int {
	t.Helper()
	ids, err := mbox.IngestMbox(path, cachedir.At(t.TempDir()), "MboxSaveTest")
	if err != nil {
		t.Fatalf("IngestMbox(%s) failed: %v", path, err)
	}
	return len(ids)
}

func TestAppendMboxEntries_CreatesThenAppends(t *testing.T) {
	path := filepath.Join(t.TempDir(), "corpus.mbox")

	n, err := AppendMboxEntries(path, [][]byte{[]byte(mboxTestMsgA)})
	if err != nil {
		t.Fatalf("AppendMboxEntries failed: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 written, got %d", n)
	}
	if got := ingestCount(t, path); got != 1 {
		t.Errorf("expected 1 message after the first append, got %d", got)
	}

	n, err = AppendMboxEntries(path, [][]byte{[]byte(mboxTestMsgB)})
	if err != nil {
		t.Fatalf("second AppendMboxEntries failed: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 written on the second append, got %d", n)
	}
	if got := ingestCount(t, path); got != 2 {
		t.Errorf("expected 2 messages after the second append, got %d", got)
	}
}

func TestAppendMboxEntries_EmptyWritesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.mbox")
	if _, err := AppendMboxEntries(path, nil); err != nil {
		t.Fatalf("AppendMboxEntries(nil) failed: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected no file for an empty append, got stat error %v", err)
	}
}

func TestAppendMboxEntries_SeparatesFromTailWithoutNewline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tail.mbox")
	// A complete first entry whose final newline was lost: appending to it must
	// not glue the second envelope onto the first body's last line.
	seed := "From alice@example.com Sun Sep 20 10:00:00 2026\n" + strings.TrimRight(mboxTestMsgA, "\n")
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatalf("seeding the file failed: %v", err)
	}

	if _, err := AppendMboxEntries(path, [][]byte{[]byte(mboxTestMsgB)}); err != nil {
		t.Fatalf("AppendMboxEntries failed: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the file failed: %v", err)
	}
	if !strings.Contains(string(data), "\nFrom bob@example.com ") {
		t.Errorf("expected the appended envelope to start on its own line, got:\n%s", data)
	}
	if got := ingestCount(t, path); got != 2 {
		t.Errorf("expected 2 messages, got %d", got)
	}
}

func TestAppendMboxEntries_EscapesFromLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "escape.mbox")
	raw := "From: Alice <alice@example.com>\r\n" +
		"Subject: Escaping\r\n" +
		"Message-ID: <escape@example.com>\r\n" +
		"Date: Sun, 20 Sep 2026 10:00:00 +0000\r\n" +
		"Content-Type: text/plain\r\n" +
		"\r\n" +
		"From the desk of Alice\r\n"

	if _, err := AppendMboxEntries(path, [][]byte{[]byte(raw)}); err != nil {
		t.Fatalf("AppendMboxEntries failed: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the file failed: %v", err)
	}
	if !strings.Contains(string(data), "\n>From the desk of Alice") {
		t.Errorf("expected the body's From line to be escaped, got:\n%s", data)
	}
	if got := ingestCount(t, path); got != 1 {
		t.Errorf("expected the escaped entry to parse back as 1 message, got %d", got)
	}
}

func mboxTestConfig(t *testing.T, cacheDir string) *cfg_g.Config {
	t.Helper()
	return &cfg_g.Config{
		Paths:        cfg_g.Paths{ConfigDir: t.TempDir(), CacheRoot: cacheDir},
		AccountCache: cachedir.At(cacheDir),
	}
}

func TestAppendClientFolderToMbox_AppendsEveryMessage(t *testing.T) {
	config := mboxTestConfig(t, t.TempDir())
	mock := &mailclient.MockMailClient{
		Cfg:           config,
		Labels:        []string{"INBOX"},
		DownloadedIDs: map[string][]email.ID{"INBOX": {"a1", "b1"}},
		RawMessages: map[email.ID][]byte{
			"a1": []byte(mboxTestMsgA),
			"b1": []byte(mboxTestMsgB),
		},
	}
	path := filepath.Join(t.TempDir(), "folder.mbox")

	n, err := appendClientFolderToMbox(config, mock, "INBOX", path, report.Discard())
	if err != nil {
		t.Fatalf("appendClientFolderToMbox failed: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 messages appended, got %d", n)
	}
	if got := ingestCount(t, path); got != 2 {
		t.Errorf("expected 2 messages in the file, got %d", got)
	}
}

func TestAppendClientFolderToMbox_DryRunWritesNothing(t *testing.T) {
	config := mboxTestConfig(t, t.TempDir())
	config.DryRun = true
	mock := &mailclient.MockMailClient{
		Cfg:           config,
		Labels:        []string{"INBOX"},
		DownloadedIDs: map[string][]email.ID{"INBOX": {"a1"}},
		RawMessages:   map[email.ID][]byte{"a1": []byte(mboxTestMsgA)},
	}
	path := filepath.Join(t.TempDir(), "dry.mbox")

	n, err := appendClientFolderToMbox(config, mock, "INBOX", path, report.Discard())
	if err != nil {
		t.Fatalf("appendClientFolderToMbox failed: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 message counted, got %d", n)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected no file under dry run, got stat error %v", err)
	}
}

func TestAppendClientFolderToMbox_SkipsUnreadable(t *testing.T) {
	config := mboxTestConfig(t, t.TempDir())
	mock := &mailclient.MockMailClient{
		Cfg:           config,
		Labels:        []string{"INBOX"},
		DownloadedIDs: map[string][]email.ID{"INBOX": {"a1", "gone"}},
		RawMessages:   map[email.ID][]byte{"a1": []byte(mboxTestMsgA)},
	}
	path := filepath.Join(t.TempDir(), "partial.mbox")

	n, err := appendClientFolderToMbox(config, mock, "INBOX", path, report.Discard())
	if err != nil {
		t.Fatalf("appendClientFolderToMbox failed: %v", err)
	}
	if n != 1 {
		t.Errorf("expected only the readable message to be appended, got %d", n)
	}
	if got := ingestCount(t, path); got != 1 {
		t.Errorf("expected 1 message in the file, got %d", got)
	}
}

// The TUI's write path: a set tagged in a unified inbox arrives as one source
// per account, and each is read through its own client.
func TestMboxService_AppendMessagesToMboxReadsEverySource(t *testing.T) {
	config := mboxTestConfig(t, t.TempDir())
	work := &mailclient.MockMailClient{
		Cfg:         config,
		RawMessages: map[email.ID][]byte{"a1": []byte(mboxTestMsgA)},
	}
	personal := &mailclient.MockMailClient{
		Cfg:         config,
		RawMessages: map[email.ID][]byte{"b1": []byte(mboxTestMsgB)},
	}
	path := filepath.Join(t.TempDir(), "tagged.mbox")

	n, err := MboxService{}.AppendMessagesToMbox(path, []tui.MboxSource{
		{Client: work, IDs: []email.ID{"a1"}},
		{Client: personal, IDs: []email.ID{"b1"}},
	})
	if err != nil {
		t.Fatalf("AppendMessagesToMbox failed: %v", err)
	}
	if n != 2 {
		t.Errorf("appended %d message(s), want 2", n)
	}
	if got := ingestCount(t, path); got != 2 {
		t.Errorf("expected 2 messages in the file, got %d", got)
	}
}
