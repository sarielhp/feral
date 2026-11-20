package email

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListUnsubscribeURIs(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   []string
	}{
		{"empty", "", nil},
		{"one", "<https://list.example/u?t=1>", []string{"https://list.example/u?t=1"}},
		{"two in order", "<mailto:leave@list.example>, <https://list.example/u>",
			[]string{"mailto:leave@list.example", "https://list.example/u"}},
		{"no space after comma", "<mailto:a@list.example>,<https://list.example/u>",
			[]string{"mailto:a@list.example", "https://list.example/u"}},
		{"folded inside the brackets", "<https://list.example/\r\n  unsub?t=abc>", []string{"https://list.example/unsub?t=abc"}},
		{"whitespace around", "  < https://list.example/u >  ,\t<mailto:x@list.example>",
			[]string{"https://list.example/u", "mailto:x@list.example"}},
		{"comma inside a URI", "<https://list.example/u?a=1,2>", []string{"https://list.example/u?a=1,2"}},
		{"comment outside brackets", "(leave) <mailto:x@list.example> (or here)", []string{"mailto:x@list.example"}},
		{"unclosed", "<https://list.example/u", nil},
		{"bare URI without brackets", "https://list.example/u", nil},
		{"empty brackets", "<>, <mailto:x@list.example>", []string{"mailto:x@list.example"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ListUnsubscribeURIs(tt.header)
			if strings.Join(got, "|") != strings.Join(tt.want, "|") {
				t.Errorf("ListUnsubscribeURIs(%q) = %q, want %q", tt.header, got, tt.want)
			}
		})
	}
}

func TestPlanUnsubscribe(t *testing.T) {
	const post = "List-Unsubscribe=One-Click"
	tests := []struct {
		name, header, post string
		method             UnsubscribeMethod
		target             string
	}{
		{"no header", "", "", UnsubscribeNone, ""},
		{"one-click https", "<mailto:x@list.example>, <https://list.example/u>", post, UnsubscribeOneClick, "https://list.example/u"},
		{"one-click token case and space", "<https://list.example/u>", "  list-unsubscribe=one-click ", UnsubscribeOneClick, "https://list.example/u"},
		{"one-click needs https", "<http://list.example/u>", post, UnsubscribeBrowser, "http://list.example/u"},
		{"one-click needs https, mailto wins", "<http://list.example/u>, <mailto:x@list.example>", post, UnsubscribeMailto, "mailto:x@list.example"},
		{"wrong post value", "<https://list.example/u>", "List-Unsubscribe=Yes", UnsubscribeBrowser, "https://list.example/u"},
		{"mailto before link without one-click", "<https://list.example/u>, <mailto:x@list.example>", "", UnsubscribeMailto, "mailto:x@list.example"},
		{"first mailto that parses", "<mailto:a@x.example,b@x.example>, <mailto:c@list.example>", "", UnsubscribeMailto, "mailto:c@list.example"},
		{"bad mailto falls to link", "<mailto:not an address>, <https://list.example/u>", "", UnsubscribeBrowser, "https://list.example/u"},
		{"first link", "<https://one.example/u>, <https://two.example/u>", "", UnsubscribeBrowser, "https://one.example/u"},
		{"scheme is case-insensitive", "<HTTPS://list.example/u>", post, UnsubscribeOneClick, "HTTPS://list.example/u"},
		{"other schemes ignored", "<ftp://list.example/u>, <javascript:alert(1)>", "", UnsubscribeNone, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PlanUnsubscribe(tt.header, tt.post)
			if got.Method != tt.method || got.Target != tt.target {
				t.Errorf("PlanUnsubscribe(%q, %q) = %q %q, want %q %q", tt.header, tt.post, got.Method, got.Target, tt.method, tt.target)
			}
		})
	}
}

func TestParseMailto(t *testing.T) {
	tests := []struct {
		name    string
		uri     string
		want    MailtoDraft
		wantErr bool
	}{
		{"plain", "mailto:leave@list.example", MailtoDraft{To: "leave@list.example", Subject: "unsubscribe", Body: "unsubscribe"}, false},
		{"subject and body", "mailto:leave@list.example?subject=Remove%20me&body=unsubscribe%20me%0D%0Aplease",
			MailtoDraft{To: "leave@list.example", Subject: "Remove me", Body: "unsubscribe me\nplease"}, false},
		{"field names are case-insensitive", "mailto:leave@list.example?SUBJECT=go",
			MailtoDraft{To: "leave@list.example", Subject: "go", Body: "unsubscribe"}, false},
		{"cc and bcc ignored", "mailto:leave@list.example?cc=victim@other.example&bcc=b@other.example&to=c@other.example",
			MailtoDraft{To: "leave@list.example", Subject: "unsubscribe", Body: "unsubscribe"}, false},
		{"header injection in subject", "mailto:leave@list.example?subject=hi%0D%0ABcc:%20victim@other.example",
			MailtoDraft{To: "leave@list.example", Subject: "hiBcc: victim@other.example", Body: "unsubscribe"}, false},
		{"escape stripped from body", "mailto:leave@list.example?body=a%1B%5B2Jb",
			MailtoDraft{To: "leave@list.example", Subject: "unsubscribe", Body: "a[2Jb"}, false},
		{"encoded address", "mailto:leave%2Bme@list.example", MailtoDraft{To: "leave+me@list.example", Subject: "unsubscribe", Body: "unsubscribe"}, false},
		{"two addresses refused", "mailto:a@list.example,b@list.example", MailtoDraft{}, true},
		{"line break in address refused", "mailto:a@list.example%0D%0ABcc:b@other.example", MailtoDraft{}, true},
		{"no address", "mailto:?subject=x", MailtoDraft{}, true},
		{"not mailto", "https://list.example/u", MailtoDraft{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseMailto(tt.uri)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseMailto(%q) error = %v, wantErr %v", tt.uri, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ParseMailto(%q) = %+v, want %+v", tt.uri, got, tt.want)
			}
		})
	}
}

// The mailto draft becomes a message from the account to the list's one
// address, with the subject and body the URI asked for and nothing else.
func TestUnsubscriptionCarryOutMailtoBuildsTheDraft(t *testing.T) {
	dir := t.TempDir()
	raw := []byte("From: News <news@list.example>\r\nList-Unsubscribe: <mailto:leave@list.example?subject=unsub%20now&cc=x@other.example>\r\n\r\nhi\r\n")
	u, err := PrepareUnsubscribe(raw)
	if err != nil {
		t.Fatal(err)
	}
	u.From = "Me <me@home.example>"
	var sent []byte
	fx := UnsubscribeEffects{SendMail: func(b []byte) error { sent = b; return nil }}
	if carryErr := u.CarryOut(fx, dir); carryErr != nil {
		t.Fatalf("CarryOut: %v", carryErr)
	}
	msg, err := mail.ReadMessage(strings.NewReader(string(sent)))
	if err != nil {
		t.Fatalf("sent message does not parse: %v\n%s", err, sent)
	}
	checks := map[string]string{"From": "Me <me@home.example>", "To": "leave@list.example", "Subject": "unsub now", "Cc": "", "Bcc": ""}
	for k, want := range checks {
		if got := msg.Header.Get(k); got != want {
			t.Errorf("header %s = %q, want %q", k, got, want)
		}
	}
	body, _ := io.ReadAll(msg.Body)
	if !strings.Contains(string(body), "unsubscribe") {
		t.Errorf("body = %q, want the default command", body)
	}
	entries := readUnsubLog(t, dir)
	if len(entries) != 1 || entries[0].Method != "mailto" || !entries[0].Success || entries[0].Sender != "news@list.example" {
		t.Errorf("log = %+v, want one successful mailto entry for news@list.example", entries)
	}
}

func TestPrepareUnsubscribeWithoutHeader(t *testing.T) {
	_, err := PrepareUnsubscribe([]byte("From: a@b.example\r\nSubject: hi\r\n\r\nbody\r\n"))
	if !errors.Is(err, ErrNoListUnsubscribe) {
		t.Fatalf("err = %v, want ErrNoListUnsubscribe", err)
	}
}

// A failed one-click is logged as a failure and nothing else is tried: the
// mailto beside it was not what the user confirmed.
func TestUnsubscriptionCarryOutDoesNotFallBack(t *testing.T) {
	dir := t.TempDir()
	raw := []byte("From: n@list.example\r\nList-Unsubscribe: <https://list.example/u>, <mailto:x@list.example>\r\n" +
		"List-Unsubscribe-Post: List-Unsubscribe=One-Click\r\n\r\nhi\r\n")
	u, err := PrepareUnsubscribe(raw)
	if err != nil {
		t.Fatal(err)
	}
	u.From = "me@home.example"
	sent, opened := false, false
	fx := UnsubscribeEffects{
		PostOneClick: func(string) error { return errors.New("HTTP unsubscribe returned status 500") },
		SendMail:     func([]byte) error { sent = true; return nil },
		OpenBrowser:  func(string) error { opened = true; return nil },
	}
	if err := u.CarryOut(fx, dir); err == nil {
		t.Fatal("CarryOut succeeded after the POST failed")
	}
	if sent || opened {
		t.Errorf("a failed one-click went on to another method (sent %v, opened %v)", sent, opened)
	}
	entries := readUnsubLog(t, dir)
	if len(entries) != 1 || entries[0].Success || entries[0].Method != "one-click" {
		t.Errorf("log = %+v, want one failed one-click entry", entries)
	}
}

// A browser link is handed to the opener, never fetched.
func TestUnsubscriptionCarryOutBrowser(t *testing.T) {
	var opened string
	u := Unsubscription{Plan: PlanUnsubscribe("<https://list.example/u>", ""), Sender: "n@list.example"}
	fx := UnsubscribeEffects{
		OpenBrowser:  func(s string) error { opened = s; return nil },
		PostOneClick: func(string) error { t.Error("a browser link was fetched"); return nil },
	}
	if err := u.CarryOut(fx, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if opened != "https://list.example/u" {
		t.Errorf("opened %q", opened)
	}
}

func TestUnsubscriptionPreviewStripsControls(t *testing.T) {
	u := Unsubscription{Plan: PlanUnsubscribe("<https://list.example/u\x1b]8;;x>", ""), Sender: "evil\x1b[2J@list.example"}
	if p := u.Preview(); strings.ContainsRune(p, '\x1b') {
		t.Errorf("preview carries an escape: %q", p)
	}
}

func readUnsubLog(t *testing.T, dir string) []UnsubscriptionEntry {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "unsubscriptions.json"))
	if err != nil {
		t.Fatalf("reading the log: %v", err)
	}
	var entries []UnsubscriptionEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatal(err)
	}
	return entries
}

func TestOneClickPostSuccess(t *testing.T) {
	var method, body, ctype string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, ctype = r.Method, r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	if err := postOneClickWithClient(newSafeHTTPClient(true), ts.URL+"/u?t=1", true); err != nil {
		t.Fatalf("one-click POST: %v", err)
	}
	if method != http.MethodPost || body != "List-Unsubscribe=One-Click" || ctype != "application/x-www-form-urlencoded" {
		t.Errorf("server saw %s %q (%s), want POST of the one-click token", method, body, ctype)
	}
}

func TestOneClickPostNon2xx(t *testing.T) {
	for _, code := range []int{http.StatusNotFound, http.StatusInternalServerError} {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(code) }))
		err := postOneClickWithClient(newSafeHTTPClient(true), ts.URL, true)
		ts.Close()
		if err == nil || !strings.Contains(err.Error(), http.StatusText(code)) {
			t.Errorf("status %d: err = %v, want a failure naming it", code, err)
		}
	}
}

// A redirect chain stops at five hops.
func TestOneClickPostRedirectCap(t *testing.T) {
	hops := 0
	var ts *httptest.Server
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hops++
		http.Redirect(w, r, ts.URL+"/again", http.StatusTemporaryRedirect)
	}))
	defer ts.Close()

	err := postOneClickWithClient(newSafeHTTPClient(true), ts.URL, true)
	if err == nil || !strings.Contains(err.Error(), "stopped after 5 redirects") {
		t.Fatalf("err = %v, want the redirect cap", err)
	}
	if hops != 5 {
		t.Errorf("server saw %d requests, want 5", hops)
	}
}

// The production entry point refuses loopback, private and metadata targets,
// and anything that is not https.
func TestExecuteOneClickRefusesTargets(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("the guard let a request through")
	}))
	defer ts.Close()

	targets := []struct{ url, want string }{
		{ts.URL, "https"},
		{"https://127.0.0.1:1/u", "blocked"},
		{"https://localhost/u", "blocked"},
		{"https://10.0.0.1/u", "blocked"},
		{"https://192.168.1.1/u", "blocked"},
		{"https://169.254.169.254/latest/meta-data/", "blocked"},
		{"http://list.example/u", "https"},
	}
	for _, tt := range targets {
		err := ExecuteOneClickUnsubscribe(tt.url)
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("ExecuteOneClickUnsubscribe(%q) = %v, want an error containing %q", tt.url, err, tt.want)
		}
	}
	// The dialer, not only the URL check, refuses loopback: a test-mode
	// client that allows the URL still may not connect when it is built
	// without the loopback allowance.
	err := postOneClickWithClient(newSafeHTTPClient(false), ts.URL, true)
	if err == nil || !strings.Contains(err.Error(), "blocked") {
		t.Errorf("dialer allowed loopback: %v", err)
	}
}

// A one-click redirect may lead only to https.
func TestHTTPSOnlyRedirects(t *testing.T) {
	c := httpsOnlyRedirects(newSafeHTTPClient(false))
	plain := &http.Request{URL: &url.URL{Scheme: "http", Host: "list.example"}}
	if err := c.CheckRedirect(plain, nil); err == nil {
		t.Error("a redirect to http was allowed")
	}
	secure := &http.Request{URL: &url.URL{Scheme: "https", Host: "list.example"}}
	if err := c.CheckRedirect(secure, nil); err != nil {
		t.Errorf("a redirect to https was refused: %v", err)
	}
	private := &http.Request{URL: &url.URL{Scheme: "https", Host: "10.0.0.1"}}
	if err := c.CheckRedirect(private, nil); err == nil {
		t.Error("the wrapped client lost the private-address check")
	}
}
