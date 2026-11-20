package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/feral/app"
	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/mailclient"
)

// mboxSpliceAccounts is an mbox source holding two cached messages and a
// destination account with nothing in it.
func mboxSpliceAccounts(t *testing.T) (src, dst *mailclient.MockMailClient, downloadDir, configDir string) {
	t.Helper()
	tmp := t.TempDir()
	configDir = filepath.Join(tmp, "config")
	downloadDir = filepath.Join(tmp, "download")
	srcDir := filepath.Join(downloadDir, "acc1")
	dstDir := filepath.Join(downloadDir, "acc2")
	for _, d := range []string{configDir, srcDir, dstDir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	when := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
	ids := seedCachedBatch(t, srcDir, when, 2, func(i int) string {
		return "Message-ID: <spool-" + string(rune('0'+i)) + "@example.com>\nDate: Wed, 02 Sep 2026 10:00:00 +0000\nFrom: a@example.com\nSubject: spool\n\nbody"
	})
	src = archiveMock(srcDir, configDir, "INBOX", email.IDs(ids)...)
	src.BackendTypeVal = "mbox"
	src.InboxFolderVal = "INBOX"
	src.Cfg.SelectedAccount = "acc1"
	dst = &mailclient.MockMailClient{
		Labels: []string{"received"},
		Cfg:    &cfg_g.Config{SelectedAccount: "acc2", Paths: cfg_g.Paths{CacheRoot: dstDir, ConfigDir: configDir}, AccountCache: cachedir.At(dstDir)},
	}
	return src, dst, downloadDir, configDir
}

// A splice out of an mbox account copies into the other account and removes
// nothing from the file: --move acts as --copy, and says so, rather than
// reporting removals that never happened.
func TestSpliceFromMboxCopiesAndKeepsTheFile(t *testing.T) {
	src, dst, downloadDir, configDir := mboxSpliceAccounts(t)
	session := twoAccountSession(downloadDir, configDir, src, dst)
	var out, errOut bytes.Buffer
	session.Out, session.Err = &out, &errOut

	if err := InitCLI(session).Execute([]string{"label", "sort", "%acc1:INBOX", "-Y", "%acc2:received", "--move"}); err != nil {
		t.Fatalf("splice: %v", err)
	}
	if len(dst.UploadedEmails) != 2 {
		t.Errorf("uploaded %d message(s), want 2", len(dst.UploadedEmails))
	}
	if len(src.MovedEmails) != 0 || len(src.DeletedMessages) != 0 {
		t.Errorf("the mbox source was changed: moved %v, deleted %v", src.MovedEmails, src.DeletedMessages)
	}
	if !strings.Contains(errOut.String(), "--move acts as --copy") {
		t.Errorf("no word that --move became a copy: %q", errOut.String())
	}
	if strings.Contains(out.String(), "removed from source") {
		t.Errorf("a removal that never happened was reported:\n%s", out.String())
	}

	// A second run finds both already there and uploads nothing.
	if err := InitCLI(session).Execute([]string{"label", "sort", "%acc1:INBOX", "-Y", "%acc2:received", "--copy"}); err != nil {
		t.Fatalf("second splice: %v", err)
	}
	if len(dst.UploadedEmails) != 2 {
		t.Errorf("a rerun uploaded again: %d upload(s)", len(dst.UploadedEmails))
	}
}

// An mbox file is no destination, so a splice out of one must name another
// account.
func TestSpliceFromMboxNeedsADestinationAccount(t *testing.T) {
	src, dst, downloadDir, configDir := mboxSpliceAccounts(t)
	session := twoAccountSession(downloadDir, configDir, src, dst)
	session.Out, session.Err = &bytes.Buffer{}, &bytes.Buffer{}

	err := InitCLI(session).Execute([]string{"label", "sort", "%acc1:INBOX", "-f", "received", "--copy"})
	if !errors.Is(err, app.ErrUsage) {
		t.Fatalf("splice into the mbox itself: %v; want a usage error", err)
	}
	if len(dst.UploadedEmails) != 0 {
		t.Errorf("messages were uploaded despite the refusal")
	}
}

// A path names a local file to splice from, opened as a read-only view.
func TestSpliceFromAnMboxFilePath(t *testing.T) {
	src, dst, downloadDir, configDir := mboxSpliceAccounts(t)
	file := filepath.Join(t.TempDir(), "spool")
	if err := os.WriteFile(file, []byte("From a@example.com Wed Sep 02 10:00:00 2026\nSubject: x\n\nbody\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	session := twoAccountSession(downloadDir, configDir, src, dst)
	session.Out, session.Err = &bytes.Buffer{}, &bytes.Buffer{}
	var opened string
	session.Deps.Accounts = stubAccounts{
		OpenMboxFn: func(_ *cfg_g.Config, path string, _ cachedir.Account) (mailclient.MailClient, error) {
			opened = path
			return src, nil
		},
	}
	if err := InitCLI(session).Execute([]string{"label", "sort", file, "-Y", "%acc2:received", "--copy"}); err != nil {
		t.Fatalf("splice from a file: %v", err)
	}
	if opened != file {
		t.Errorf("opened %q, want %q", opened, file)
	}
	if len(dst.UploadedEmails) != 2 {
		t.Errorf("uploaded %d message(s), want 2", len(dst.UploadedEmails))
	}
}

func TestIsMboxFileArg(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "spool")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	for arg, want := range map[string]bool{
		file: true, "./spool": true, "spool": false, "%acc:spool": false, dir: false, "./missing": false,
	} {
		if got := isMboxFileArg(arg); got != want {
			t.Errorf("isMboxFileArg(%q) = %v, want %v", arg, got, want)
		}
	}
}
