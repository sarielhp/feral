package runner

import (
	"bytes"
	"context"
	"net/mail"
	"strings"
	"testing"

	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/cli"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/pgp"
	"github.com/sarielhp/feral/report"
)

// signedSendService is a send service over one account, "work", whose
// signature is sig, printing what it would send to the returned buffer.
func signedSendService(t *testing.T, account cfg_acc.AccountConfig, sig string) (SendService, *cfg_g.Config, *bytes.Buffer) {
	t.Helper()
	dir := t.TempDir()
	if err := cfg_g.WriteSignature(dir, account.Name, sig); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	cfg := &cfg_g.Config{
		Paths:    cfg_g.Paths{ConfigDir: dir, CacheRoot: dir},
		Accounts: []cfg_acc.AccountConfig{account},
	}
	return SendService{Rep: report.Discard(), Out: &buf}, cfg, &buf
}

var workAccount = cfg_acc.AccountConfig{Name: "work", Username: "user@work.com", UserName: "Worker", Type: "jmap"}

// sentBody is the decoded text body of the message a dry run printed.
func sentBody(t *testing.T, out []byte) string {
	t.Helper()
	m, err := mail.ReadMessage(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("parsing the dry-run output: %v\n%s", err, out)
	}
	body, err := email.ExtractPlainBodyText(m)
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(body, "\r\n", "\n")
}

func TestSendAppendsTheAccountSignature(t *testing.T) {
	for _, sig := range []string{"Bob Example\nExample Inc.\n", "רועי הר-פלד\n"} {
		svc, cfg, buf := signedSendService(t, workAccount, sig)
		opts := cli.SendOptions{To: []string{"r@example.com"}, Subject: "s", Body: "Hello.\n", DryRun: true}
		if err := svc.Send(cfg, opts); err != nil {
			t.Fatal(err)
		}
		want := "Hello.\n\n-- \n" + sig
		if got := sentBody(t, buf.Bytes()); !strings.HasPrefix(got, want) {
			t.Errorf("body =\n%q\nwant it to start with\n%q", got, want)
		}
	}
}

func TestSendDoesNotSignTwice(t *testing.T) {
	svc, cfg, buf := signedSendService(t, workAccount, "Bob\n")
	opts := cli.SendOptions{To: []string{"r@example.com"}, Subject: "s", Body: "Hello.\n\n-- \nMine\n", DryRun: true}
	if err := svc.Send(cfg, opts); err != nil {
		t.Fatal(err)
	}
	if got := sentBody(t, buf.Bytes()); strings.Contains(got, "Bob") {
		t.Errorf("a body with its own signature was signed again:\n%q", got)
	}
}

// --raw sends the message it is given, byte for byte.
func TestSendRawIsNotSigned(t *testing.T) {
	svc, cfg, buf := signedSendService(t, workAccount, "Bob\n")
	raw := "From: user@work.com\r\nTo: r@example.com\r\nSubject: Raw\r\n\r\nPatch.\r\n"
	if err := svc.Send(cfg, cli.SendOptions{Raw: true, RawBytes: []byte(raw), DryRun: true}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "Bob") || strings.Contains(buf.String(), "-- ") {
		t.Errorf("a --raw message was signed:\n%s", buf)
	}
}

// The signature is part of the body, so a PGP signature covers it.
func TestSendSignatureIsInsideThePGPSignature(t *testing.T) {
	gpgKeyring(t)
	account := cfg_acc.AccountConfig{Name: "work", Username: "test@example.com", UserName: "Worker", Type: "jmap"}
	svc, cfg, buf := signedSendService(t, account, "Bob\n")
	opts := cli.SendOptions{To: []string{"r@example.com"}, Subject: "s", Body: "a signed body", DryRun: true, Sign: true}
	if err := svc.Send(cfg, opts); err != nil {
		t.Fatal(err)
	}
	result := pgp.Process(context.Background(), buf.Bytes(), pgp.Options{Verify: true})
	if result.Status == nil || !result.Status.ValidSig {
		t.Fatalf("status = %+v, want the message to verify", result.Status)
	}
	if body := sentBody(t, result.Raw); !strings.Contains(body, "a signed body\n\n-- \nBob") {
		t.Errorf("the verified part does not carry the signature:\n%q", body)
	}
}

func replyFixture(t *testing.T, sig string) (SendService, *cfg_g.Config, *bytes.Buffer) {
	t.Helper()
	svc, cfg, buf := signedSendService(t, workAccount, sig)
	rawMsg := "From: Boss <boss@company.com>\r\nTo: Worker <user@work.com>\r\nSubject: Review\r\n" +
		"Message-ID: <rev1@company.com>\r\nDate: Sun, 20 Sep 2026 10:00:00 +0000\r\n" +
		"Content-Type: text/plain\r\n\r\nPlease review.\r\n-- \r\nThe Boss\r\n"
	setupTestCachedMessage(t, cfg.CacheRoot, "work", "msg001", []byte(rawMsg))
	return svc, cfg, buf
}

func TestHeadlessReplySignsAboveTheQuote(t *testing.T) {
	svc, cfg, buf := replyFixture(t, "Bob\n")
	opts := cli.ReplyOptions{MessageID: "msg001", Headless: true, DryRun: true, Body: "Done."}
	if err := svc.Reply(cfg, opts); err != nil {
		t.Fatal(err)
	}
	got := sentBody(t, buf.Bytes())
	sig := strings.Index(got, "Done.\n\n-- \nBob\n\n")
	quote := strings.Index(got, "> Please review.")
	if sig != 0 || quote < 0 || quote < sig {
		t.Errorf("want the text, then the signature, then the quote:\n%q", got)
	}
	if strings.Count(got, "\n-- \n") != 1 {
		t.Errorf("the original's signature, quoted, must not read as a delimiter:\n%q", got)
	}
}

// The editor of an interactive reply gets the user's text and the quote
// apart, so the compose screen can put the signature between them.
func TestInteractiveReplyHandsTheQuoteApart(t *testing.T) {
	svc, cfg, _ := replyFixture(t, "Bob\n")
	var got cli.ComposeOptions
	svc.TUI = stubLauncher{composeFn: func(_ *cfg_g.Config, opts cli.ComposeOptions) error { got = opts; return nil }}
	if err := svc.Reply(cfg, cli.ReplyOptions{MessageID: "msg001", Body: "Done."}); err != nil {
		t.Fatal(err)
	}
	if got.Body != "Done." || !strings.Contains(got.Quote, "> Please review.") {
		t.Errorf("Body = %q, Quote = %q", got.Body, got.Quote)
	}
	if strings.Contains(got.Body+got.Quote, "Bob") {
		t.Error("the runner signed a reply the compose screen will sign")
	}
}

// --no-signature is how a script that pipes mail through msg send, or sends a
// headless reply, keeps the account's signature off.
func TestNoSignatureLeavesTheSignatureOff(t *testing.T) {
	svc, cfg, buf := signedSendService(t, workAccount, "Bob\n")
	opts := cli.SendOptions{To: []string{"r@example.com"}, Subject: "s", Body: "Hello.\n", DryRun: true, NoSignature: true}
	if err := svc.Send(cfg, opts); err != nil {
		t.Fatal(err)
	}
	if got := sentBody(t, buf.Bytes()); strings.Contains(got, "-- \n") || strings.Contains(got, "Bob") {
		t.Errorf("send --no-signature carries the signature:\n%q", got)
	}

	svc, cfg, buf = replyFixture(t, "Bob\n")
	reply := cli.ReplyOptions{MessageID: "msg001", Headless: true, DryRun: true, Body: "Done.", NoSignature: true}
	if err := svc.Reply(cfg, reply); err != nil {
		t.Fatal(err)
	}
	got := sentBody(t, buf.Bytes())
	if !strings.HasPrefix(got, "Done.\n") || strings.Contains(got, "\n-- \nBob") {
		t.Errorf("reply --no-signature carries the signature:\n%q", got)
	}
	if !strings.Contains(got, "> Please review.") {
		t.Errorf("reply --no-signature lost the quote:\n%q", got)
	}
}
