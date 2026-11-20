package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/sarielhp/feral/cfg_g"
)

func TestResolveComposeBody_Inline(t *testing.T) {
	assertInlineBody(t, &flagComposeBody, &flagComposeInclude, resolveComposeBody, "Initial compose body")
}

func TestResolveComposeBody_IncludeFile(t *testing.T) {
	assertBodyFromFile(t, &flagComposeBody, &flagComposeInclude, resolveComposeBody, "Template body from file")
}

func TestResolveComposeBody_MissingFile(t *testing.T) {
	flagComposeBody = ""
	flagComposeInclude = "/nonexistent/path/to/compose_body.txt"
	defer func() { flagComposeInclude = "" }()

	_, err := resolveComposeBody()
	if err == nil {
		t.Fatalf("expected error for nonexistent file, got nil")
	}
}

func TestRunCompose_PassesOptionsToTUI(t *testing.T) {
	var capturedOpts ComposeOptions
	called := false

	session := &Session{
		Config: &cfg_g.Config{
			SelectedAccount: "work",
		},
		Deps: Deps{
			TUI: stubTUI{
				ComposeFn: func(_ *cfg_g.Config, opts ComposeOptions) error {
					called = true
					capturedOpts = opts
					return nil
				},
			},
		},
	}

	flagComposeSubject = "Discussion"
	flagComposeCc = "dev1@example.com, dev2@example.com"
	flagComposeBcc = "archive@example.com"
	flagComposeAttach = []string{"spec.pdf"}
	flagComposeBody = "Let's review the specs."
	defer func() {
		flagComposeSubject = ""
		flagComposeCc = ""
		flagComposeBcc = ""
		flagComposeAttach = nil
		flagComposeBody = ""
	}()

	recipients := []string{"boss@example.com"}
	err := runCompose(session, recipients)
	if err != nil {
		t.Fatalf("runCompose failed: %v", err)
	}
	if !called {
		t.Fatalf("expected TUI.Compose to be called")
	}

	if !reflect.DeepEqual(capturedOpts.To, recipients) {
		t.Errorf("To = %v, want %v", capturedOpts.To, recipients)
	}
	if capturedOpts.Subject != "Discussion" {
		t.Errorf("Subject = %q, want 'Discussion'", capturedOpts.Subject)
	}
	if capturedOpts.Account != "work" {
		t.Errorf("Account = %q, want 'work'", capturedOpts.Account)
	}
	if len(capturedOpts.Cc) != 2 || capturedOpts.Cc[0] != "dev1@example.com" || capturedOpts.Cc[1] != "dev2@example.com" {
		t.Errorf("Cc = %v, want [dev1@example.com dev2@example.com]", capturedOpts.Cc)
	}
	if len(capturedOpts.Bcc) != 1 || capturedOpts.Bcc[0] != "archive@example.com" {
		t.Errorf("Bcc = %v, want [archive@example.com]", capturedOpts.Bcc)
	}
	if len(capturedOpts.Attachments) != 1 || capturedOpts.Attachments[0] != "spec.pdf" {
		t.Errorf("Attachments = %v, want [spec.pdf]", capturedOpts.Attachments)
	}
	if capturedOpts.Body != "Let's review the specs." {
		t.Errorf("Body = %q, want 'Let\\'s review the specs.'", capturedOpts.Body)
	}
}

// `write boss < notes.txt` starts the message with the file as its body; -i
// and --body still win, and a resumed draft keeps its own.
func TestResolveComposeBody_RedirectedStdin(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(path, []byte("from stdin\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	withStdinFile := func(fn func()) {
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		old := os.Stdin
		defer func() { os.Stdin = old }()
		os.Stdin = f
		fn()
	}
	cases := []struct {
		name, body, resume, want string
	}{
		{name: "stdin", want: "from stdin\n"},
		{name: "--body wins", body: "inline", want: "inline"},
		{name: "--resume ignores stdin", resume: "2026/11/12/1", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			flagComposeBody, flagComposeResume = tc.body, tc.resume
			defer func() { flagComposeBody, flagComposeResume = "", "" }()
			withStdinFile(func() {
				got, err := resolveComposeBody()
				if err != nil || got != tc.want {
					t.Fatalf("body = %q, %v; want %q", got, err, tc.want)
				}
			})
		})
	}
}
