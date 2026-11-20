package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sarielhp/feral/app"
	"github.com/sarielhp/feral/cfg_g"
)

// --encrypt fails closed when any recipient lacks a key; the help used to say
// it encrypted "for every recipient with a key", as if the rest got plaintext.
func TestSendEncryptHelpSaysItFailsClosed(t *testing.T) {
	var buf bytes.Buffer
	cliApp := InitCLI(&Session{Config: &cfg_g.Config{}})
	cliApp.Stdout, cliApp.Stderr = &buf, &buf
	if err := cliApp.Execute([]string{"send", "-h"}); err != nil {
		t.Fatalf("send -h: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "--encrypt") {
		t.Fatalf("send -h does not list --encrypt:\n%s", out)
	}
	if strings.Contains(out, "every recipient with a key") || !strings.Contains(out, "fails unless all recipients have keys") {
		t.Errorf("--encrypt help misdescribes what happens to a recipient with no key:\n%s", out)
	}
}

func TestSplitCommaList(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"", nil},
		{"   ", nil},
		{"a@example.com", []string{"a@example.com"}},
		{"a@example.com, b@example.com", []string{"a@example.com", "b@example.com"}},
		{" a@example.com , , b@example.com ", []string{"a@example.com", "b@example.com"}},
	}

	for _, tc := range tests {
		got := splitCommaList(tc.input)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("splitCommaList(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestResolveSendBody_Inline(t *testing.T) {
	assertInlineBody(t, &flagSendBody, &flagSendInclude, resolveSendBody, "Hello world")
}

func TestResolveSendBody_IncludeFile(t *testing.T) {
	assertBodyFromFile(t, &flagSendBody, &flagSendInclude, resolveSendBody, "File content")
}

func TestResolveSendBody_MissingFile(t *testing.T) {
	flagSendBody = ""
	flagSendInclude = "/nonexistent/path/to/body.txt"
	defer func() { flagSendInclude = "" }()

	_, err := resolveSendBody()
	if err == nil {
		t.Fatalf("expected error for nonexistent file, got nil")
	}
}

func TestResolveSendBody_NoBodyReturnsUsageError(t *testing.T) {
	flagSendBody = ""
	flagSendInclude = ""

	_, err := resolveSendBody()
	if !errors.Is(err, app.ErrUsage) {
		t.Fatalf("expected ErrUsage, got %v", err)
	}
}

func TestRunSend_InvokesService(t *testing.T) {
	flagSendBody = "Test message"
	flagSendSubject = "Test Subject"
	flagSendCc = "cc@example.com"
	flagSendBcc = "bcc@example.com"
	flagSendAttach = nil
	defer func() {
		flagSendBody = ""
		flagSendSubject = ""
		flagSendCc = ""
		flagSendBcc = ""
	}()

	var calledWith SendOptions
	sendCalled := false

	session := dryRunSession("work")
	session.Deps.Send = stubSend{SendFn: func(_ *cfg_g.Config, opts SendOptions) error {
		sendCalled = true
		calledWith = opts
		return nil
	}}

	err := runSend(session, []string{"to@example.com"})
	if err != nil {
		t.Fatalf("runSend failed: %v", err)
	}

	if !sendCalled {
		t.Errorf("expected Send to be invoked")
	}
	if len(calledWith.To) != 1 || calledWith.To[0] != "to@example.com" {
		t.Errorf("unexpected To: %v", calledWith.To)
	}
	if calledWith.Subject != "Test Subject" {
		t.Errorf("unexpected Subject: %s", calledWith.Subject)
	}
	if calledWith.Body != "Test message" {
		t.Errorf("unexpected Body: %s", calledWith.Body)
	}
	if calledWith.Account != "work" {
		t.Errorf("unexpected Account: %s", calledWith.Account)
	}
	if !calledWith.DryRun {
		t.Errorf("expected DryRun true")
	}
}

// send has no review screen, so what `write` catches on the way to the editor
// send has to catch before mailing: an unquoted -a *.pdf leaves the other
// matches among the recipients, and a quoted pattern must arrive as files.
func TestRunSendExpandsAttachmentsAndRefusesFileRecipients(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	touch(t, dir, "a.pdf", "b.pdf")
	flagSendBody = "body"
	defer func() { flagSendBody = ""; flagSendAttach = nil }()

	var sent *SendOptions
	session := &Session{
		Config: &cfg_g.Config{Paths: cfg_g.Paths{ConfigDir: t.TempDir()}},
		Deps: Deps{Send: stubSend{SendFn: func(_ *cfg_g.Config, opts SendOptions) error {
			sent = &opts
			return nil
		}}},
	}

	flagSendAttach = []string{"a.pdf"}
	err := runSend(session, []string{"boss@example.com", "b.pdf"})
	if !errors.Is(err, app.ErrUsage) || sent != nil {
		t.Fatalf("a file among the recipients: err = %v, sent = %v; want a usage error and nothing sent", err, sent)
	}

	flagSendAttach = []string{"*.pdf"}
	if err := runSend(session, []string{"boss@example.com"}); err != nil {
		t.Fatal(err)
	}
	if want := []string{"a.pdf", "b.pdf"}; sent == nil || !reflect.DeepEqual(sent.Attachments, want) {
		t.Errorf("Attachments = %v, want %q", sent, want)
	}
}

func TestRunSendPassesPgpFlagsToTheService(t *testing.T) {
	flagSendBody = "Test message"
	flagSendSubject = "Test Subject"
	flagSendSign = true
	flagSendEncrypt = true
	flagSendPgpKey = "0xDEADBEEF"
	flagSendNoSig = true
	defer func() {
		flagSendBody, flagSendSubject = "", ""
		flagSendSign, flagSendEncrypt, flagSendPgpKey = false, false, ""
		flagSendNoSig = false
	}()

	var calledWith SendOptions
	session := &Session{
		Config: &cfg_g.Config{SelectedAccount: "work"},
		Deps: Deps{
			Send: stubSend{
				SendFn: func(_ *cfg_g.Config, opts SendOptions) error {
					calledWith = opts
					return nil
				},
			},
		},
	}

	if err := runSend(session, []string{"to@example.com"}); err != nil {
		t.Fatalf("runSend failed: %v", err)
	}
	if !calledWith.Sign || !calledWith.Encrypt {
		t.Errorf("Sign/Encrypt = %v/%v, want both set", calledWith.Sign, calledWith.Encrypt)
	}
	if !calledWith.NoSignature {
		t.Error("--no-signature did not reach SendOptions.NoSignature")
	}
	if calledWith.PgpKeyID != "0xDEADBEEF" {
		t.Errorf("PgpKeyID = %q", calledWith.PgpKeyID)
	}
}

// --raw means the message is already built. Signing it would sign a body this
// command cannot see the boundaries of, so it is refused rather than guessed at.
func TestRunSendRefusesSigningARawMessage(t *testing.T) {
	flagSendRaw = true
	flagSendSign = true
	defer func() { flagSendRaw, flagSendSign = false, false }()

	session := &Session{Config: &cfg_g.Config{}, Deps: Deps{Send: stubSend{}}}
	err := runSend(session, nil)
	if err == nil {
		t.Fatal("expected a usage error")
	}
	if !errors.Is(err, app.ErrUsage) {
		t.Fatalf("err = %v, want a usage error", err)
	}
}

// An omitted -s is filled in by the configured LLM from the body and the
// attachment names, and the line is printed; without an endpoint the send
// goes out with no subject, as before.
func TestRunSendWritesMissingSubjectWithLLM(t *testing.T) {
	var userMsg string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct{ Role, Content string } `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		userMsg = req.Messages[len(req.Messages)-1].Content
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"\"Agenda for Monday\""}}]}`))
	}))
	defer server.Close()

	dir := t.TempDir()
	agenda := filepath.Join(dir, "agenda.pdf")
	if err := os.WriteFile(agenda, []byte("%PDF"), 0o600); err != nil {
		t.Fatal(err)
	}
	flagSendBody = "Please find Monday's agenda attached."
	flagSendSubject = ""
	flagSendAttach = []string{agenda}
	defer func() { flagSendBody = ""; flagSendAttach = nil }()

	var calledWith SendOptions
	var out bytes.Buffer
	session := dryRunSession("work")
	session.Out = &out
	session.Config.LLM.Endpoint = server.URL
	session.Deps.Send = stubSend{SendFn: func(_ *cfg_g.Config, opts SendOptions) error {
		calledWith = opts
		return nil
	}}

	if err := runSend(session, []string{"to@example.com"}); err != nil {
		t.Fatalf("runSend: %v", err)
	}
	if calledWith.Subject != "Agenda for Monday" {
		t.Errorf("Subject = %q; want the model's line, unquoted", calledWith.Subject)
	}
	for _, want := range []string{"agenda.pdf", "Monday's agenda", "to@example.com"} {
		if !strings.Contains(userMsg, want) {
			t.Errorf("the model was not shown %q:\n%s", want, userMsg)
		}
	}
	if !strings.Contains(out.String(), "Agenda for Monday") {
		t.Errorf("the generated subject was not printed:\n%s", out.String())
	}

	// Off by config: the model is not asked and the subject stays empty.
	off := false
	session.Config.LLM.AutoSubject = &off
	userMsg = ""
	if err := runSend(session, []string{"to@example.com"}); err != nil {
		t.Fatalf("runSend: %v", err)
	}
	if calledWith.Subject != "" || userMsg != "" {
		t.Errorf("auto_subject off: Subject=%q asked=%v; want no call and no subject", calledWith.Subject, userMsg != "")
	}
}

// A failing endpoint is a warning, not a refused send.
func TestRunSendSubjectLLMFailureWarnsAndSendsWithoutOne(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "model not loaded", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	flagSendBody = "Quick note."
	flagSendSubject = ""
	flagSendAttach = nil
	defer func() { flagSendBody = "" }()

	sent := false
	var errOut bytes.Buffer
	session := dryRunSession("work")
	session.Err = &errOut
	session.Config.LLM.Endpoint = server.URL
	session.Deps.Send = stubSend{SendFn: func(_ *cfg_g.Config, opts SendOptions) error {
		sent = opts.Subject == ""
		return nil
	}}

	if err := runSend(session, []string{"to@example.com"}); err != nil {
		t.Fatalf("runSend: %v", err)
	}
	if !sent {
		t.Error("the message was not sent with an empty subject after the LLM failed")
	}
	if !strings.Contains(errOut.String(), "could not write one") || !strings.Contains(errOut.String(), "model not loaded") {
		t.Errorf("no warning naming the failure:\n%s", errOut.String())
	}
}

// `send -a report.pdf bob` needs no body: "See attachment." stands in, and a
// message without a file still asks for one.
func TestRunSend_AttachmentOnlyGetsANote(t *testing.T) {
	cases := []struct {
		attach []string
		want   string
	}{
		{[]string{"report.pdf"}, "See attachment.\n"},
		{[]string{"a.pdf", "b.pdf"}, "See attachments.\n"},
	}
	for _, tc := range cases {
		flagSendSubject, flagSendAttach = "Report", tc.attach
		var got SendOptions
		session := dryRunSession("work")
		session.Deps.Send = stubSend{SendFn: func(_ *cfg_g.Config, opts SendOptions) error {
			got = opts
			return nil
		}}
		if err := runSend(session, []string{"to@example.com"}); err != nil {
			t.Fatalf("runSend(%v): %v", tc.attach, err)
		}
		if got.Body != tc.want {
			t.Errorf("body with %v = %q, want %q", tc.attach, got.Body, tc.want)
		}
	}
	flagSendSubject, flagSendAttach = "", nil
	if err := runSend(dryRunSession("work"), []string{"to@example.com"}); !errors.Is(err, errNoSendBody) {
		t.Errorf("no body and no file: err = %v, want errNoSendBody", err)
	}
}
