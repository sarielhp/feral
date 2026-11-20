package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/feral/cfg_g"
)

func readSignatureFile(t *testing.T, session *Session, account string) (string, bool) {
	t.Helper()
	sig, set, err := cfg_g.ReadSignature(session.Config.ConfigDir, account)
	if err != nil {
		t.Fatal(err)
	}
	return sig, set
}

func TestAccountSignatureSetShowAndClear(t *testing.T) {
	session, _, buf := accountsSession(t)
	src := filepath.Join(t.TempDir(), "sig.txt")
	// A pasted delimiter and CRLF endings are normalized away.
	if err := os.WriteFile(src, []byte("-- \r\nBob Example\r\nExample Inc.\r\n\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := execAccounts(t, session, "account", "signature", "Work", "--file", src); err != nil {
		t.Fatal(err)
	}
	if sig, set := readSignatureFile(t, session, "work"); !set || sig != "Bob Example\nExample Inc.\n" {
		t.Errorf("signature.txt = %q (set %v), want the normalized text under the canonical name", sig, set)
	}
	if _, err := os.Stat(filepath.Join(cfg_g.AccountDir(session.Config.ConfigDir, "work"), cfg_g.SignatureFile)); err != nil {
		t.Errorf("the signature is not in the account's directory: %v", err)
	}

	buf.Reset()
	if err := execAccounts(t, session, "account", "signature", "work"); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "Bob Example\nExample Inc.\n" {
		t.Errorf("show printed %q", buf)
	}

	if err := execAccounts(t, session, "account", "signature", "work", "--clear"); err != nil {
		t.Fatal(err)
	}
	// Cleared is set to nothing, not removed: the TUI's ~/.signature
	// fallback must not sign it either.
	if sig, set := readSignatureFile(t, session, "work"); !set || sig != "" {
		t.Errorf("after --clear signature.txt = %q (set %v), want an empty file", sig, set)
	}
	buf.Reset()
	if err := execAccounts(t, session, "account", "signature", "work"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "has no signature") {
		t.Errorf("show after --clear printed %q", buf)
	}
}

func TestAccountSignatureRefusesUnknownAccountAndTwoSources(t *testing.T) {
	session, _, _ := accountsSession(t)
	if err := execAccounts(t, session, "account", "signature", "nosuch", "--clear"); err == nil {
		t.Error("an unknown account was given a signature")
	}
	if err := execAccounts(t, session, "account", "signature", "work", "--clear", "--edit"); err == nil {
		t.Error("--clear and --edit were accepted together")
	}
}

// --dry-run writes nothing; --read-only sets the signature, which is a local
// fact like the display name, not a record of a simulated server change.
func TestAccountSignatureModes(t *testing.T) {
	session, _, _ := accountsSession(t)
	err := execAccounts(t, session, "--dry-run", "account", "signature", "work", "--clear")
	if err == nil || !strings.Contains(err.Error(), "refused under --dry-run") {
		t.Errorf("--dry-run: err = %v, want a refusal", err)
	}
	if _, set := readSignatureFile(t, session, "work"); set {
		t.Error("--dry-run wrote signature.txt")
	}
	edited := false
	stubSignatureEditor(t, func(string) error { edited = true; return nil })
	if err := execAccounts(t, session, "--dry-run", "account", "signature", "work", "--edit"); err == nil {
		t.Error("--dry-run --edit was not refused")
	}
	if edited {
		t.Error("--dry-run started the editor before refusing")
	}

	if err := execAccounts(t, session, "--read-only", "account", "signature", "work", "--clear"); err != nil {
		t.Fatalf("--read-only: %v", err)
	}
	if _, set := readSignatureFile(t, session, "work"); !set {
		t.Error("--read-only did not set the signature")
	}
}

// stubSignatureEditor replaces the editor for the test with edit, which is
// given the file the editor would open.
func stubSignatureEditor(t *testing.T, edit func(path string) error) {
	t.Helper()
	saved := runSignatureEditor
	t.Cleanup(func() { runSignatureEditor = saved })
	runSignatureEditor = func(_ string, args []string) error { return edit(args[len(args)-1]) }
}

func TestAccountSignatureEdit(t *testing.T) {
	session, _, _ := accountsSession(t)
	session.Config.Editor = "true"
	var opened string
	stubSignatureEditor(t, func(path string) error {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		opened = string(data)
		return os.WriteFile(path, []byte("Bob\n"), 0o600)
	})
	if err := execAccounts(t, session, "account", "signature", "work", "--edit"); err != nil {
		t.Fatal(err)
	}
	if sig, _ := readSignatureFile(t, session, "work"); sig != "Bob\n" {
		t.Errorf("after --edit signature.txt = %q", sig)
	}
	if opened != "" {
		t.Errorf("the editor opened on %q, want an empty file for an account without a signature", opened)
	}

	stubSignatureEditor(t, func(path string) error {
		data, _ := os.ReadFile(path)
		opened = string(data)
		return errors.New("killed")
	})
	if err := execAccounts(t, session, "account", "signature", "work", "--edit"); err == nil {
		t.Error("a failed editor was taken as an edit")
	}
	if opened != "Bob\n" {
		t.Errorf("the editor opened on %q, want the current signature", opened)
	}
	if sig, _ := readSignatureFile(t, session, "work"); sig != "Bob\n" {
		t.Errorf("a failed editor changed the signature to %q", sig)
	}
}
