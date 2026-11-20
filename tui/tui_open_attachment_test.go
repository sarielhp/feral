package tui

import (
	"testing"
	"time"

	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/mailclient"
)

// An HTML attachment -- a message's own HTML body is one -- opens in the
// configured browser, as w opens a message; it used to go to the desktop
// opener, so one page opened in two different browsers. Anything else still
// goes to the desktop opener.
func TestOpenAttachmentFile_HTMLUsesConfiguredBrowser(t *testing.T) {
	browserCalls := make(chan [2]string, 2)
	systemCalls := make(chan string, 2)
	prevB, prevS := openBrowserFile, openSystemFile
	openBrowserFile = func(path, preferred string) error { browserCalls <- [2]string{path, preferred}; return nil }
	openSystemFile = func(path string) { systemCalls <- path }
	t.Cleanup(func() { openBrowserFile, openSystemFile = prevB, prevS })

	m := newTuiModel(&mailclient.MockMailClient{Cfg: &cfg_g.Config{Prefs: cfg_g.Prefs{Browser: "my-browser"}}}, "INBOX", nil)
	m.openAttachmentFile(email.Attachment{Filename: "message_body.html", ContentType: "text/html"}, "/tmp/x.html")
	m.openAttachmentFile(email.Attachment{Filename: "report.pdf", ContentType: "application/pdf"}, "/tmp/r.pdf")

	select {
	case got := <-browserCalls:
		if got != [2]string{"/tmp/x.html", "my-browser"} {
			t.Errorf("browser opened %v, want /tmp/x.html with my-browser", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the HTML attachment did not reach the configured browser")
	}
	select {
	case got := <-systemCalls:
		if got != "/tmp/r.pdf" {
			t.Errorf("desktop opener got %q, want the PDF", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the PDF did not reach the desktop opener")
	}
}
