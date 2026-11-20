package tui

import (
	"net/mail"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/mailclient"
)

// signatureModel is a compose model whose accounts keep their files in a
// configuration directory of the test's own; each signature in sigs is
// written for the account it is keyed by.
func signatureModel(t *testing.T, sigs map[string]string) *tuiModel {
	t.Helper()
	m, _, _ := setupTestComposeModel(t, false)
	m.cfg.ConfigDir = t.TempDir()
	for name, sig := range sigs {
		if err := cfg_g.WriteSignature(m.cfg.ConfigDir, name, sig); err != nil {
			t.Fatal(err)
		}
	}
	return m
}

func readTemplate(t *testing.T, path string, err error) string {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func origMessage(t *testing.T, body string) *mail.Message {
	t.Helper()
	raw := "From: Alice <alice@example.com>\r\nTo: user@personal.com\r\nSubject: Plans\r\n" +
		"Date: Sun, 30 Aug 2026 12:00:00 +0000\r\nMessage-ID: <o1@example.com>\r\n\r\n" + body
	msg, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	return msg
}

func TestComposeTemplateIsSignedByTheSendingAccount(t *testing.T) {
	m := signatureModel(t, map[string]string{"Personal": "Bob at home\n", "Work": "Bob at work\n"})
	path, baseline, err := m.createComposeBodyTemplate("Personal")
	if got := readTemplate(t, path, err); got != "\n\n-- \nBob at home\n" || baseline != got {
		t.Errorf("template = %q, baseline = %q", got, baseline)
	}
	path, _, err = m.createComposeBodyTemplate("Work")
	if got := readTemplate(t, path, err); got != "\n\n-- \nBob at work\n" {
		t.Errorf("Work's template = %q", got)
	}
}

func TestReplyTemplateSignsAboveTheQuote(t *testing.T) {
	m := signatureModel(t, map[string]string{"Personal": "Bob\n"})
	path, baseline, err := m.createReplyBodyTemplate("Personal", origMessage(t, "Lunch?\r\n-- \r\nAlice\r\n"))
	got := readTemplate(t, path, err)
	if !strings.HasPrefix(got, "\n\n-- \nBob\n\nOn ") || !strings.Contains(got, "> Lunch?\n> -- \n> Alice\n") {
		t.Errorf("reply template =\n%q", got)
	}
	if baseline != got {
		t.Error("an untouched reply template must read as unmodified")
	}
}

// The CLI's `msg reply` hands the text and the quote apart; the signature
// goes between them, and the template is no baseline since text was given.
func TestStandaloneReplyTemplateSignsBetweenTextAndQuote(t *testing.T) {
	m := signatureModel(t, map[string]string{"Personal": "Bob\n"})
	path, baseline, err := m.createComposeBodyTemplateWithContent("Personal", "Done.", "On Mon, Alice wrote:\n> Lunch?\n")
	if got := readTemplate(t, path, err); got != "Done.\n\n-- \nBob\n\nOn Mon, Alice wrote:\n> Lunch?\n" || baseline != "" {
		t.Errorf("template = %q, baseline = %q", got, baseline)
	}
	// Text that brings its own signature is not signed again.
	path, _, err = m.createComposeBodyTemplateWithContent("Personal", "Notes\n-- \nMine\n", "")
	if got := readTemplate(t, path, err); strings.Contains(got, "Bob") {
		t.Errorf("a signed text was signed again: %q", got)
	}
}

// A forward carries the original sender's text, signature and all; the
// forwarder's signature still goes under it.
func TestForwardTemplateSignsUnderTheForwardedText(t *testing.T) {
	m := signatureModel(t, map[string]string{"Personal": "Bob\n"})
	orig := origMessage(t, "")
	path, _, err := m.createForwardBodyTemplate("Personal", orig, "Lunch?\n-- \nAlice\n")
	got := readTemplate(t, path, err)
	if !strings.Contains(got, "Lunch?\n-- \nAlice\n") || !strings.HasSuffix(got, "\n-- \nBob\n") {
		t.Errorf("forward template =\n%q", got)
	}
}

// ~/.signature signs an account with no signature of its own, as it did
// before signatures were per account; one set to nothing does not fall back.
func TestHomeSignatureIsTheFallback(t *testing.T) {
	m := signatureModel(t, map[string]string{"Work": ""})
	// A HOME of its own: the package's shared scratch HOME must not carry a
	// signature into the other tests.
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.WriteFile(filepath.Join(home, ".signature"), []byte("Legacy\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := m.composeSignature("Personal"); got != "Legacy\n" {
		t.Errorf("Personal, with no signature.txt, signs with %q", got)
	}
	if got := m.composeSignature("Work"); got != "" {
		t.Errorf("Work, whose signature was cleared, signs with %q", got)
	}
}

// A resumed draft already carries its signature: neither the review screen
// nor a re-edit adds another.
func TestResumedDraftIsNotSignedAgain(t *testing.T) {
	m := signatureModel(t, map[string]string{"Personal": "Bob\n"})
	body := email.SignBody("Draft text", "Bob\n", "")
	raw, err := (&email.ComposeMessage{From: "user@personal.com", To: "x@example.com", Subject: "s", Body: body}).BuildRFC822()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "draft.eml")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	m.resumeDraftCmd(path)
	if m.compose == nil || strings.Count(m.compose.Body, "-- \nBob") != 1 {
		t.Fatalf("resumed body = %+v", m.compose)
	}
	m.width, m.height = 100, 40
	if view := m.View(); !strings.Contains(view, "Bob") {
		t.Errorf("the review screen does not show the signature:\n%s", view)
	}

	var edited string
	previous := runProgram
	t.Cleanup(func() { runProgram = previous })
	runProgram = func(c *exec.Cmd, fn tea.ExecCallback) tea.Cmd {
		data, _ := os.ReadFile(c.Args[len(c.Args)-1])
		edited = string(data)
		return func() tea.Msg { return fn(nil) }
	}
	if cmd := m.reEditBodyCmd(); cmd == nil {
		t.Fatal("no editor was started")
	}
	if strings.Count(edited, "-- \nBob") != 1 {
		t.Errorf("the re-edit opened on %q", edited)
	}
}

// The signature is found through the client's per-account copy of the
// config as through the session's, whichever names the directory.
func TestSignatureFoundThroughEitherConfig(t *testing.T) {
	m := signatureModel(t, map[string]string{"Personal": "Bob\n"})
	clientCfg := *m.cfg
	m.client = &mailclient.MockMailClient{Cfg: &clientCfg, BackendTypeVal: "gmail"}
	m.cfg = &cfg_g.Config{Accounts: clientCfg.Accounts}
	if got := m.composeSignature("Personal"); got != "Bob\n" {
		t.Errorf("through the client's config: %q", got)
	}
	m.cfg, clientCfg.ConfigDir = &cfg_g.Config{Paths: cfg_g.Paths{ConfigDir: clientCfg.ConfigDir}}, ""
	if got := m.composeSignature("Personal"); got != "Bob\n" {
		t.Errorf("through the session's config: %q", got)
	}
}

func TestSwitchingAccountSwapsTheSignature(t *testing.T) {
	m := signatureModel(t, map[string]string{"Personal": "Bob at home\n", "Work": "Bob at work\n"})
	quote := "On Mon, Alice wrote:\n> Lunch?\n"
	m.compose = &composeState{AccountIndex: 0, AccountName: "Personal", To: "x@example.com", Subject: "s",
		Body: email.SignBody("Yes.", "Bob at home\n", quote)}
	m.switchComposeAccountTo(1)
	if want := "Yes.\n\n-- \nBob at work\n\n" + quote; m.compose.Body != want {
		t.Errorf("after the switch body =\n%q\nwant\n%q", m.compose.Body, want)
	}

	// A signature the user edited is theirs: the switch leaves it.
	m.compose.Body = "Yes.\n\n-- \nBob at work, mostly\n"
	m.switchComposeAccountTo(0)
	if m.compose.Body != "Yes.\n\n-- \nBob at work, mostly\n" {
		t.Errorf("an edited signature was replaced: %q", m.compose.Body)
	}
}
