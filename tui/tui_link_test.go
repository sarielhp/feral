package tui

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/mailclient"
)

func TestDetailLongLinkFromRawMessage(t *testing.T) {
	t.Setenv("TERM", "xterm-kitty")
	target := "https://example.com/path/" + strings.Repeat("long-path/", 30) + "?token=original"
	for _, contentType := range []string{"text/plain", "text/html"} {
		t.Run(contentType, func(t *testing.T) {
			body := "Please visit " + target + " for details."
			if contentType == "text/html" {
				body = "<p>Please visit <a href=\"" + target + "\">" + target + "</a> for details.</p>"
			}
			m := newDetailModel(t, 80, 40, "INBOX", nil)
			m.client.(*mailclient.MockMailClient).RawMessages = map[email.ID][]byte{
				"link": []byte("From: sender@example.com\r\nSubject: Link\r\nContent-Type: " + contentType + "; charset=utf-8\r\n\r\n" + body),
			}
			loaded := m.loadDetailCmd("link")().(detailMsg)
			m.handleDetailLoaded(loaded)
			out := m.View()
			if !strings.Contains(out, "\x1b]8;;"+target+"\x1b\\") {
				t.Errorf("full hyperlink target lost: %q", out)
			}
			visible := email.StripEscapeSequences(out)
			if strings.Contains(visible, target) || !strings.Contains(visible, "...") {
				t.Errorf("link was not shortened: %q", visible)
			}
			visible = strings.Join(strings.Fields(visible), " ")
			if !strings.Contains(visible, "Please visit") || !strings.Contains(visible, "for details.") || strings.Contains(visible, "...for") {
				t.Errorf("link context lost: %q", visible)
			}
		})
	}
}

// TestDetailButtonTextIsTheLink reads an HTML sign-in message and clicks the
// button's own text. w3m rendered a button as the plain text "[1]Sign in",
// and the only link was a list of look-alike targets at the bottom; the
// anchor text must now carry the target, and the list must say what each
// target is.
func TestDetailButtonTextIsTheLink(t *testing.T) {
	if _, err := exec.LookPath("w3m"); err != nil {
		t.Skip("w3m not installed")
	}
	t.Setenv("TERM", "xterm-kitty")
	button := "https://login.example.com/magic-link#" + strings.Repeat("0a3ce7a9", 8) + ":c2lnbi1pbg"
	tracker := "https://click.example.net/ls/click?upn=" + strings.Repeat("u001.rFcAmKXLOm9u6wLRWHIUY-2B", 4)
	body := `<p>Click the button below to finish signing in.</p><p><a href="` + button + `">Sign in</a></p>` +
		`<p>Need a hand? Contact <a href="` + tracker + `">Example Support</a>.</p>`
	m := newDetailModel(t, 80, 40, "INBOX", nil)
	m.client.(*mailclient.MockMailClient).RawMessages = map[email.ID][]byte{
		"link": []byte("From: sender@example.com\r\nSubject: Link\r\nContent-Type: text/html; charset=utf-8\r\n\r\n" + body),
	}
	loaded := m.loadDetailCmd("link")().(detailMsg)
	m.handleDetailLoaded(loaded)
	out := m.View()

	screen := strings.Join(strings.Fields(email.StripEscapeSequences(out)), " ")
	for _, want := range []string{"Sign in[1]", "Example Support[2].", "[1]: https://login.example.com/", "\"Sign in\"", "\"Example Support\""} {
		if !strings.Contains(screen, want) {
			t.Errorf("screen lacks %q:\n%s", want, email.StripEscapeSequences(out))
		}
	}
	for _, tc := range []struct{ text, target string }{{"Sign in[1]", button}, {"Example Support[2]", tracker}} {
		col, row := findOnScreen(t, out, tc.text)
		for _, dx := range []int{0, len(tc.text) - 1} {
			if got := linkAt(out, col+dx, row); got != tc.target {
				t.Errorf("cell %d of %q links to %q, want %q", dx, tc.text, got, tc.target)
			}
		}
	}
}

// findOnScreen is the cell where text first appears in the rendered frame.
func findOnScreen(t *testing.T, frame, text string) (col, row int) {
	t.Helper()
	for row, line := range strings.Split(frame, "\n") {
		if before, _, ok := strings.Cut(email.StripEscapeSequences(line), text); ok {
			return len([]rune(before)), row
		}
	}
	t.Fatalf("%q is not on screen:\n%s", text, email.StripEscapeSequences(frame))
	return 0, 0
}
