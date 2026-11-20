package mbox

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/cache/folderindex"
	"github.com/sarielhp/feral/cache/msg"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
)

// mboxMessage renders one message with its From_ line. An empty mid leaves
// the Message-ID header out.
func mboxMessage(mid, subject, body string) string {
	var b strings.Builder
	b.WriteString("From sender@example.com Wed Sep 02 10:00:00 2026\n")
	b.WriteString("From: S <sender@example.com>\nSubject: " + subject + "\nDate: Wed, 02 Sep 2026 10:00:00 +0000\n")
	if mid != "" {
		b.WriteString("Message-ID: <" + mid + ">\n")
	}
	b.WriteString("\n" + body + "\n\n")
	return b.String()
}

// rewriteFixture writes an mbox of the given messages, reads it into a cache,
// and returns the path, the cache and the IDs in file order.
func rewriteFixture(t *testing.T, messages ...string) (string, cachedir.Account, []email.ID) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "box.mbox")
	if err := os.WriteFile(path, []byte(strings.Join(messages, "")), 0o600); err != nil {
		t.Fatal(err)
	}
	acct := cachedir.At(filepath.Join(dir, "cache"))
	ids, err := IngestMbox(path, acct, "box")
	if err != nil {
		t.Fatalf("IngestMbox: %v", err)
	}
	return path, acct, ids
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// reingest reads the file again and fails the test if that was not the
// metadata fast path's answer: same IDs as the folder index already holds.
func reingest(t *testing.T, path string, acct cachedir.Account) []email.ID {
	t.Helper()
	before, err := folderindex.IDs(acct, "INBOX")
	if err != nil {
		t.Fatal(err)
	}
	ids, err := IngestMbox(path, acct, "box")
	if err != nil {
		t.Fatalf("IngestMbox after rewrite: %v", err)
	}
	if !slices.Equal(ids, before) {
		t.Fatalf("reading the rewritten file changed the IDs: %v, index held %v", ids, before)
	}
	return ids
}

// The survivors are copied byte for byte, From_ lines and >From quoting
// included, and the cache follows without a re-parse.
func TestDeleteFromMboxKeepsTheRestVerbatim(t *testing.T) {
	a := mboxMessage("a@example.com", "a", "first")
	b := mboxMessage("b@example.com", "b", ">From quoted line\nsecond")
	c := mboxMessage("c@example.com", "c", "third")
	path, acct, ids := rewriteFixture(t, a, b, c)

	if err := deleteFromMbox(path, acct, []email.ID{ids[1]}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if got := readFile(t, path); got != a+c {
		t.Errorf("file after delete:\n%q\nwant\n%q", got, a+c)
	}
	if got := reingest(t, path, acct); !slices.Equal(got, []email.ID{ids[0], ids[2]}) {
		t.Errorf("IDs %v, want %v", got, []email.ID{ids[0], ids[2]})
	}
	if ok, _ := msg.Exists(acct, ids[1]); ok {
		t.Error("the deleted message is still cached")
	}

	if err := deleteFromMbox(path, acct, []email.ID{ids[0], ids[2]}); err != nil {
		t.Fatalf("delete the rest: %v", err)
	}
	if got := readFile(t, path); got != "" {
		t.Errorf("file not empty: %q", got)
	}
}

// Deleting the first of two copies of one Message-ID leaves the second with
// the ID it had: a re-parse would have renamed it, which is why the cache is
// updated rather than rebuilt.
func TestDeleteFromMboxDuplicateMessageIDs(t *testing.T) {
	first := mboxMessage("dup@example.com", "dup", "copy one")
	second := mboxMessage("dup@example.com", "dup", "copy two")
	other := mboxMessage("x@example.com", "x", "other")
	path, acct, ids := rewriteFixture(t, first, second, other)
	if ids[1] != ids[0]+"-1" {
		t.Fatalf("test premise: duplicate IDs %v", ids)
	}

	if err := deleteFromMbox(path, acct, []email.ID{ids[0]}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if got := readFile(t, path); got != second+other {
		t.Errorf("wrong copy deleted; file:\n%s", got)
	}
	if got := reingest(t, path, acct); !slices.Equal(got, ids[1:]) {
		t.Errorf("IDs %v, want %v", got, ids[1:])
	}
	if err := deleteFromMbox(path, acct, []email.ID{ids[1]}); err != nil {
		t.Fatalf("delete the second copy: %v", err)
	}
	if got := readFile(t, path); got != other {
		t.Errorf("file:\n%s", got)
	}
}

// A message with no Message-ID has a content-derived ID and deletes like any.
func TestDeleteFromMboxMessageWithoutMessageID(t *testing.T) {
	a := mboxMessage("", "no id", "anonymous")
	b := mboxMessage("b@example.com", "b", "named")
	path, acct, ids := rewriteFixture(t, a, b)
	if !strings.HasPrefix(string(ids[0]), "mbox-") {
		t.Fatalf("test premise: %v", ids)
	}
	if err := deleteFromMbox(path, acct, []email.ID{ids[0]}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if got := readFile(t, path); got != b {
		t.Errorf("file:\n%s", got)
	}
}

// Mail delivered after the cache read the file stops the rewrite: the file is
// left exactly as the delivery made it.
func TestDeleteFromMboxRefusesAChangedFile(t *testing.T) {
	a := mboxMessage("a@example.com", "a", "first")
	path, acct, ids := rewriteFixture(t, a)
	late := mboxMessage("late@example.com", "late", "delivered during")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString(late)
	_ = f.Close()

	err = deleteFromMbox(path, acct, []email.ID{ids[0]})
	if !errors.Is(err, ErrMboxChanged) {
		t.Fatalf("delete after a delivery: %v; want ErrMboxChanged", err)
	}
	if got := readFile(t, path); got != a+late {
		t.Errorf("the file was changed:\n%s", got)
	}
}

// A dotlock held by a delivery agent is waited for, and the rewrite gives up
// rather than proceed without it.
func TestDeleteFromMboxWaitsForTheDotlock(t *testing.T) {
	a := mboxMessage("a@example.com", "a", "first")
	path, acct, ids := rewriteFixture(t, a)
	if err := os.WriteFile(path+".lock", []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := lockTimeout
	lockTimeout = 200 * time.Millisecond
	defer func() { lockTimeout = old }()

	if err := deleteFromMbox(path, acct, []email.ID{ids[0]}); err == nil || !strings.Contains(err.Error(), "dotlock") {
		t.Fatalf("delete under a held dotlock: %v", err)
	}
	if got := readFile(t, path); got != a {
		t.Errorf("the file was changed under someone else's lock")
	}
	if _, err := os.Stat(path + ".lock"); err != nil {
		t.Errorf("the other holder's dotlock was removed: %v", err)
	}
}

// A crash before the rename leaves the old file; the next read forgets the
// marker and the temporary file and changes nothing.
func TestDeleteFromMboxCrashBeforeRename(t *testing.T) {
	a := mboxMessage("a@example.com", "a", "first")
	b := mboxMessage("b@example.com", "b", "second")
	path, acct, ids := rewriteFixture(t, a, b)
	crashHook = func(point string) error {
		if point == "before-rename" {
			return errors.New("crash")
		}
		return nil
	}
	defer func() { crashHook = nil }()

	if err := deleteFromMbox(path, acct, []email.ID{ids[0]}); err == nil {
		t.Fatal("the simulated crash did not stop the rewrite")
	}
	crashHook = nil
	if got := readFile(t, path); got != a+b {
		t.Errorf("the file changed before the rename")
	}
	got, err := IngestMbox(path, acct, "box")
	if err != nil || !slices.Equal(got, ids) {
		t.Errorf("after settling: %v, %v; want %v", got, err, ids)
	}
	if entries, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".box.mbox.feral-*")); len(entries) != 0 {
		t.Errorf("temporary file left behind: %v", entries)
	}
	if _, err := os.Stat(filepath.Join(acct.Path(), rewriteMarkerFile)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("marker left behind: %v", err)
	}
}

// A crash after the new file is in place but before the cache heard of it is
// settled from the marker, not by a re-parse: the surviving duplicate keeps
// its ID.
func TestDeleteFromMboxCrashAfterRename(t *testing.T) {
	first := mboxMessage("dup@example.com", "dup", "copy one")
	second := mboxMessage("dup@example.com", "dup", "copy two")
	path, acct, ids := rewriteFixture(t, first, second)
	crashHook = func(point string) error {
		if point == "after-write" {
			return errors.New("crash")
		}
		return nil
	}
	defer func() { crashHook = nil }()

	if err := deleteFromMbox(path, acct, []email.ID{ids[0]}); err == nil {
		t.Fatal("the simulated crash did not stop the rewrite")
	}
	crashHook = nil
	if got := readFile(t, path); got != second {
		t.Fatalf("file after the crash:\n%s", got)
	}
	got, err := IngestMbox(path, acct, "box")
	if err != nil || !slices.Equal(got, ids[1:]) {
		t.Errorf("after settling: %v, %v; want %v (the second copy keeps its ID)", got, err, ids[1:])
	}
}

// Mail appended after the first read (and read by the append path) is placed
// correctly when an earlier message is deleted.
func TestDeleteFromMboxAfterAppend(t *testing.T) {
	a := mboxMessage("a@example.com", "a", "first")
	b := mboxMessage("b@example.com", "b", "second")
	path, acct, _ := rewriteFixture(t, a, b)
	c := mboxMessage("a@example.com", "a again", "a duplicate, appended")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString(c)
	_ = f.Close()
	ids, err := IngestMbox(path, acct, "box")
	if err != nil || len(ids) != 3 {
		t.Fatalf("append read: %v, %v", ids, err)
	}

	if err := deleteFromMbox(path, acct, []email.ID{ids[0]}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if got := readFile(t, path); got != b+c {
		t.Errorf("file:\n%s", got)
	}
	if got := reingest(t, path, acct); !slices.Equal(got, ids[1:]) {
		t.Errorf("IDs %v, want %v", got, ids[1:])
	}
}

// A message whose bytes no longer match its cached copy is not deleted, and
// nothing else is either.
func TestDeleteFromMboxRefusesAMismatch(t *testing.T) {
	a := mboxMessage("a@example.com", "a", "first")
	b := mboxMessage("b@example.com", "b", "second")
	path, acct, ids := rewriteFixture(t, a, b)
	if err := msg.Store(acct, ids[0], []byte("Subject: something else\n\nbody\n"), time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := deleteFromMbox(path, acct, []email.ID{ids[0], ids[1]}); err == nil {
		t.Fatal("a message that does not match its cached copy was deleted")
	}
	if got := readFile(t, path); got != a+b {
		t.Errorf("the file was changed")
	}
}

// Where the directory is not writable -- a spool directory owned by root --
// the file is compacted in place, after a backup, with the fcntl lock alone.
func TestDeleteFromMboxInPlace(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	a := mboxMessage("a@example.com", "a", "first")
	b := mboxMessage("b@example.com", "b", "second")
	path, acct, ids := rewriteFixture(t, a, b)
	dir := filepath.Dir(path)
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(dir, 0o755) }()

	if err := deleteFromMbox(path, acct, []email.ID{ids[0]}); err != nil {
		t.Fatalf("in-place delete: %v", err)
	}
	if got := readFile(t, path); got != b {
		t.Errorf("file:\n%s", got)
	}
	if got := readFile(t, filepath.Join(acct.Path(), "mbox.bak")); got != a+b {
		t.Errorf("backup holds %q", got)
	}
	if got := reingest(t, path, acct); !slices.Equal(got, ids[1:]) {
		t.Errorf("IDs %v, want %v", got, ids[1:])
	}
}

// The client deletes through the rewrite, and refuses a single .eml file.
func TestClientDeleteMessages(t *testing.T) {
	a := mboxMessage("a@example.com", "a", "first")
	b := mboxMessage("b@example.com", "b", "second")
	path, acct, ids := rewriteFixture(t, a, b)
	c := &client{mboxPath: path, localCfg: nil}
	c.localCfg = cfgFor(acct)
	if err := c.DeleteMessages(ids[:1], c.InboxFolder()); err != nil {
		t.Fatalf("DeleteMessages: %v", err)
	}
	if got := readFile(t, path); got != b {
		t.Errorf("file:\n%s", got)
	}
	eml := &client{mboxPath: filepath.Join(t.TempDir(), "one.eml"), localCfg: cfgFor(acct)}
	if err := eml.DeleteMessages(ids[1:], eml.InboxFolder()); !errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("DeleteMessages on an .eml: %v", err)
	}
}

func cfgFor(acct cachedir.Account) *cfg_g.Config {
	return &cfg_g.Config{AccountCache: acct}
}

// A configured path that is a symlink -- ~/mbox pointing at the spool --
// rewrites the file it points at, and leaves the link a link: renaming over
// the link would detach the account from the file delivery writes to.
func TestDeleteFromMboxFollowsASymlink(t *testing.T) {
	a := mboxMessage("a@example.com", "a", "first")
	b := mboxMessage("b@example.com", "b", "second")
	dir := t.TempDir()
	realPath := filepath.Join(dir, "realPath.mbox")
	if err := os.WriteFile(realPath, []byte(a+b), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.mbox")
	if err := os.Symlink(realPath, link); err != nil {
		t.Fatal(err)
	}
	acct := cachedir.At(filepath.Join(dir, "cache"))
	ids, err := IngestMbox(link, acct, "link")
	if err != nil {
		t.Fatal(err)
	}
	if err := deleteFromMbox(link, acct, ids[:1]); err != nil {
		t.Fatalf("delete through a link: %v", err)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("the link was replaced by a file: %v", err)
	}
	if got := readFile(t, realPath); got != b {
		t.Errorf("the realPath file holds:\n%s", got)
	}
	if _, err := os.Stat(realPath + ".lock"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("dotlock left behind: %v", err)
	}
}

// Deleting what is already gone is not an error: it is how a retry, or
// journal recovery, finds a removal that already happened.
func TestDeleteFromMboxTwice(t *testing.T) {
	a := mboxMessage("a@example.com", "a", "first")
	b := mboxMessage("b@example.com", "b", "second")
	path, acct, ids := rewriteFixture(t, a, b)
	for i := range 2 {
		if err := deleteFromMbox(path, acct, ids[:1]); err != nil {
			t.Fatalf("delete #%d: %v", i+1, err)
		}
	}
	if got := readFile(t, path); got != b {
		t.Errorf("file:\n%s", got)
	}
}

// After a crash between the new file and the cache, a retry of the same
// delete settles the rewrite and then finds nothing left to do.
func TestDeleteFromMboxRetryAfterCrash(t *testing.T) {
	a := mboxMessage("a@example.com", "a", "first")
	b := mboxMessage("b@example.com", "b", "second")
	path, acct, ids := rewriteFixture(t, a, b)
	crashHook = func(point string) error {
		if point == "after-write" {
			return errors.New("crash")
		}
		return nil
	}
	err := deleteFromMbox(path, acct, ids[:1])
	crashHook = nil
	if err == nil {
		t.Fatal("the simulated crash did not stop the rewrite")
	}
	if err := deleteFromMbox(path, acct, ids[:1]); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if got := reingest(t, path, acct); !slices.Equal(got, ids[1:]) {
		t.Errorf("IDs %v, want %v", got, ids[1:])
	}
}

// An in-place rewrite cut short leaves a file that is neither the old one nor
// the new one. Reading it would take the damage for mail and a later rewrite
// would overwrite the only good copy, so both refuse and name the backup.
func TestDeleteFromMboxInterruptedInPlaceFailsClosed(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	a := mboxMessage("a@example.com", "a", strings.Repeat("first ", 30000))
	b := mboxMessage("b@example.com", "b", strings.Repeat("second ", 30000))
	path, acct, ids := rewriteFixture(t, a, b)
	dir := filepath.Dir(path)
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(dir, 0o755) }()
	crashHook = func(point string) error {
		if point == "mid-compaction" {
			return errors.New("crash")
		}
		return nil
	}
	err := deleteFromMbox(path, acct, ids[:1])
	crashHook = nil
	if err == nil {
		t.Fatal("the simulated crash did not stop the rewrite")
	}
	backup := filepath.Join(acct.Path(), "mbox.bak")
	if _, err := IngestMbox(path, acct, "box"); err == nil || !strings.Contains(err.Error(), backup) {
		t.Errorf("reading the damaged file: %v; want a refusal naming %s", err, backup)
	}
	if err := deleteFromMbox(path, acct, ids[1:]); err == nil || !strings.Contains(err.Error(), backup) {
		t.Errorf("rewriting the damaged file: %v; want a refusal naming %s", err, backup)
	}
	if got := readFile(t, backup); got != a+b {
		t.Error("the backup no longer holds the file as it was")
	}
}

// The locks are realPath to other processes: while a rewrite is under way the
// dotlock exists and another process's fcntl lock attempt is refused.
func TestDeleteFromMboxHoldsLocksOtherProcessesSee(t *testing.T) {
	a := mboxMessage("a@example.com", "a", "first")
	b := mboxMessage("b@example.com", "b", "second")
	path, acct, ids := rewriteFixture(t, a, b)
	probed := false
	crashHook = func(point string) error {
		if point != "before-rename" {
			return nil
		}
		probed = true
		if _, err := os.Stat(path + ".lock"); err != nil {
			t.Errorf("no dotlock during the rewrite: %v", err)
		}
		cmd := exec.Command(os.Args[0], "-test.run=^TestFcntlProbeHelper$")
		cmd.Env = append(os.Environ(), "FERAL_FCNTL_PROBE="+path)
		out, err := cmd.CombinedOutput()
		if err != nil || !strings.Contains(string(out), "probe: refused") {
			t.Errorf("another process was not refused the fcntl lock: %v\n%s", err, out)
		}
		return nil
	}
	defer func() { crashHook = nil }()
	if err := deleteFromMbox(path, acct, ids[:1]); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if !probed {
		t.Fatal("the probe never ran")
	}
}

// TestFcntlProbeHelper is the other process of the test above: it tries to
// take the fcntl write lock and says whether it was refused.
func TestFcntlProbeHelper(t *testing.T) {
	path := os.Getenv("FERAL_FCNTL_PROBE")
	if path == "" {
		t.Skip("helper process only")
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	lk := syscall.Flock_t{Type: syscall.F_WRLCK}
	err = syscall.FcntlFlock(f.Fd(), syscall.F_SETLK, &lk)
	if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EACCES) {
		fmt.Println("probe: refused")
		return
	}
	fmt.Println("probe: granted", err)
}
