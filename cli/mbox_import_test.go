package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/mailclient"
)

// importSession is a session whose selected account is dst ("acc2", received
// folder "received"), with `mbox import` opening src for any file.
func importSession(t *testing.T) (session *Session, src, dst *mailclient.MockMailClient, file string, errOut *bytes.Buffer) {
	t.Helper()
	src, dst, downloadDir, configDir := mboxSpliceAccounts(t)
	dst.Labels = []string{"received", "projects", "INBOX"}
	dst.InboxFolderVal = "INBOX"
	dst.Cfg.ReceivedFolder = "received"
	file = filepath.Join(t.TempDir(), "mbox.fl")
	if err := os.WriteFile(file, []byte("From a@example.com Wed Sep 02 10:00:00 2026\nSubject: x\n\nbody\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	session = twoAccountSession(downloadDir, configDir, src, dst)
	session.Config.SelectedAccount = "acc2"
	errOut = &bytes.Buffer{}
	session.Out, session.Err = &bytes.Buffer{}, errOut
	session.Deps.Accounts = stubAccounts{
		OpenMboxFn: func(_ *cfg_g.Config, _ string, _ cachedir.Account) (mailclient.MailClient, error) { return src, nil },
	}
	return session, src, dst, file, errOut
}

func uploadedTo(dst *mailclient.MockMailClient) []string {
	var labels []string
	for _, u := range dst.UploadedEmails {
		labels = append(labels, u.TargetLabel)
	}
	return labels
}

// With no label, every message goes to the selected account's received
// folder; a second run finds them there and uploads nothing, and the file is
// left alone.
func TestMboxImportIntoTheReceivedFolder(t *testing.T) {
	session, src, dst, file, errOut := importSession(t)
	if err := InitCLI(session).Execute([]string{"mbox", "import", file}); err != nil {
		t.Fatalf("import: %v", err)
	}
	if got := uploadedTo(dst); len(got) != 2 || got[0] != "received" || got[1] != "received" {
		t.Errorf("uploaded to %v; want both into received", got)
	}
	if err := InitCLI(session).Execute([]string{"mbox", "import", file}); err != nil {
		t.Fatalf("second import: %v", err)
	}
	if len(dst.UploadedEmails) != 2 || !strings.Contains(errOut.String(), "2 already there") {
		t.Errorf("a rerun uploaded again (%d) or did not say so:\n%s", len(dst.UploadedEmails), errOut.String())
	}
	if len(src.MovedEmails) != 0 || len(src.DeletedMessages) != 0 {
		t.Errorf("the file was changed: moved %v, deleted %v", src.MovedEmails, src.DeletedMessages)
	}
}

// A label names the destination instead.
func TestMboxImportIntoALabel(t *testing.T) {
	session, _, dst, file, _ := importSession(t)
	if err := InitCLI(session).Execute([]string{"mbox", "import", file, "%acc2:projects"}); err != nil {
		t.Fatalf("import: %v", err)
	}
	if got := uploadedTo(dst); len(got) != 2 || got[0] != "projects" {
		t.Errorf("uploaded to %v; want projects", got)
	}
}

// An account with no received folder takes the import into its inbox.
func TestMboxImportWithoutAReceivedFolder(t *testing.T) {
	session, _, dst, file, _ := importSession(t)
	dst.Cfg.ReceivedFolder = ""
	if err := InitCLI(session).Execute([]string{"mbox", "import", file}); err != nil {
		t.Fatalf("import: %v", err)
	}
	if got := uploadedTo(dst); len(got) != 2 || got[0] != "INBOX" {
		t.Errorf("uploaded to %v; want INBOX", got)
	}
}

// Into a read-only (or dry-run) destination it reports and uploads nothing.
func TestMboxImportReadOnlyReportsOnly(t *testing.T) {
	session, _, dst, file, errOut := importSession(t)
	dst.Cfg.ReadOnly = true
	if err := InitCLI(session).Execute([]string{"mbox", "import", file}); err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(dst.UploadedEmails) != 0 || !strings.Contains(errOut.String(), "Would import 2") {
		t.Errorf("uploads %d; output:\n%s", len(dst.UploadedEmails), errOut.String())
	}
}

// A message that fails to upload fails the command, so a script notices.
func TestMboxImportFailureIsAnError(t *testing.T) {
	session, _, dst, file, _ := importSession(t)
	dst.FailOn = map[string]error{"UploadRawEmail": os.ErrPermission}
	if err := InitCLI(session).Execute([]string{"mbox", "import", file}); err == nil || !strings.Contains(err.Error(), "2 message(s) were not imported") {
		t.Errorf("err = %v; want the failures counted", err)
	}
}
