package email

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"unicode"
)

// Leaving a mailing list on the user's say-so: `msg unsub` and the TUI's X.
//
// A list names how to leave it in List-Unsubscribe (RFC 2369): URIs in angle
// brackets, the leftmost preferred. RFC 8058 adds one-click: with
// "List-Unsubscribe-Post: List-Unsubscribe=One-Click" and an https URI, a
// POST of that same token is the whole unsubscription. Every URI is chosen by
// whoever sent the message, so the POST goes through the SSRF-guarded client
// (unsubscribe.go), a mailto is reduced to one address, a subject and a body,
// and any other link is handed to the user's browser rather than fetched.
//
// Nothing here asks or decides whether to act: the caller confirms, and does
// not call CarryOut at all under --read-only or --dry-run.

// UnsubscribeMethod is how a message lets its reader leave the list.
type UnsubscribeMethod string

// The methods, in the order PlanUnsubscribe prefers them.
const (
	UnsubscribeNone     UnsubscribeMethod = ""
	UnsubscribeOneClick UnsubscribeMethod = "one-click"
	UnsubscribeMailto   UnsubscribeMethod = "mailto"
	UnsubscribeBrowser  UnsubscribeMethod = "browser"
)

// oneClickToken is the List-Unsubscribe-Post value RFC 8058 defines, and the
// body of the POST it asks for.
const oneClickToken = "List-Unsubscribe=One-Click"

// ErrNoListUnsubscribe reports a message that names no way to leave a list.
var ErrNoListUnsubscribe = errors.New("the message has no usable List-Unsubscribe header: it names no list to leave")

// MailtoDraft is the message a mailto: unsubscribe URI asks for.
type MailtoDraft struct {
	To      string
	Subject string
	Body    string
}

// UnsubscribePlan is what unsubscribing from one message would do.
type UnsubscribePlan struct {
	Method UnsubscribeMethod
	Target string      // the URI acted on
	Mailto MailtoDraft // set for UnsubscribeMailto
}

// Unsubscription is one message's plan with the addresses around it.
type Unsubscription struct {
	Plan   UnsubscribePlan
	Sender string // the list's From address, for the log
	From   string // the account's From header; the mailto is sent as it
}

// UnsubscribeEffects are the outward actions an unsubscription can take.
// The caller supplies them: the account sends, the user's browser opens, and
// tests reach none of the three.
type UnsubscribeEffects struct {
	PostOneClick func(url string) error
	OpenBrowser  func(url string) error
	SendMail     func(raw []byte) error
}

// ListUnsubscribeURIs returns the URIs of a List-Unsubscribe header in order.
// Each is what lies between one '<' and the next '>', with any whitespace
// removed: RFC 2369 lets a long URI be folded across lines, and a comma is
// legal inside one, so the header is not split on commas.
func ListUnsubscribeURIs(header string) []string {
	var out []string
	rest := header
	for {
		start := strings.IndexByte(rest, '<')
		if start < 0 {
			return out
		}
		end := strings.IndexByte(rest[start+1:], '>')
		if end < 0 {
			return out
		}
		if uri := strings.Join(strings.Fields(rest[start+1:start+1+end]), ""); uri != "" {
			out = append(out, uri)
		}
		rest = rest[start+end+2:]
	}
}

// PlanUnsubscribe chooses how to leave the list, from the List-Unsubscribe
// and List-Unsubscribe-Post headers: one-click when the message offers it on
// an https URI, else the first mailto that names one valid address, else the
// first web link, for the browser.
func PlanUnsubscribe(listUnsubscribe, listUnsubscribePost string) UnsubscribePlan {
	oneClick := strings.EqualFold(strings.TrimSpace(listUnsubscribePost), oneClickToken)
	var secure, web string
	var mailto *UnsubscribePlan
	for _, uri := range ListUnsubscribeURIs(listUnsubscribe) {
		u, err := url.Parse(uri)
		if err != nil {
			continue
		}
		switch u.Scheme {
		case "https":
			secure = firstOf(secure, uri)
			web = firstOf(web, uri)
		case "http":
			web = firstOf(web, uri)
		case "mailto":
			if draft, err := ParseMailto(uri); err == nil && mailto == nil {
				mailto = &UnsubscribePlan{Method: UnsubscribeMailto, Target: uri, Mailto: draft}
			}
		}
	}
	switch {
	case oneClick && secure != "":
		return UnsubscribePlan{Method: UnsubscribeOneClick, Target: secure}
	case mailto != nil:
		return *mailto
	case web != "":
		return UnsubscribePlan{Method: UnsubscribeBrowser, Target: web}
	}
	return UnsubscribePlan{}
}

func firstOf(have, candidate string) string {
	if have != "" {
		return have
	}
	return candidate
}

// ParseMailto reads a mailto: URI (RFC 6068) into the message it asks for.
// Only the address, subject and body are honoured: a cc, bcc or second
// address would send the opt-out somewhere other than the list, so a URI
// naming more than one recipient is refused and other fields are ignored.
// An empty subject or body becomes "unsubscribe", the command list servers
// read.
func ParseMailto(uri string) (MailtoDraft, error) {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "mailto" {
		return MailtoDraft{}, fmt.Errorf("not a mailto URI: %q", uri)
	}
	to, err := mailtoURLAddress(u)
	if err != nil {
		return MailtoDraft{}, err
	}
	draft := MailtoDraft{To: to, Subject: "unsubscribe", Body: "unsubscribe"}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return MailtoDraft{}, fmt.Errorf("mailto URI %q: %w", uri, err)
	}
	for key, values := range query {
		value := strings.TrimSpace(strings.Join(values, " "))
		switch {
		case value == "":
		case strings.EqualFold(key, "subject"):
			draft.Subject = printable(SanitizeHeaderValue(value), false)
		case strings.EqualFold(key, "body"):
			draft.Body = printable(value, true)
		}
	}
	return draft, nil
}

// mailtoURLAddress is the one address a parsed mailto URI names: one address,
// escapes decoded, no line break. Both the unsubscribe and the list-reply
// paths read a mailto through it.
func mailtoURLAddress(u *url.URL) (string, error) {
	raw := u.Opaque
	if raw == "" {
		raw = u.Path
	}
	decoded, err := url.PathUnescape(raw)
	if err != nil {
		return "", fmt.Errorf("mailto address %q: %w", raw, err)
	}
	if strings.ContainsAny(decoded, "\r\n") {
		return "", fmt.Errorf("mailto address %q contains a line break", decoded)
	}
	parts := strings.Split(decoded, ",")
	if len(parts) != 1 {
		return "", fmt.Errorf("mailto names %d addresses; an unsubscribe goes to one", len(parts))
	}
	addr, err := mail.ParseAddress(strings.TrimSpace(parts[0]))
	if err != nil {
		return "", fmt.Errorf("mailto address %q: %w", parts[0], err)
	}
	return addr.Address, nil
}

// printable drops control characters, which in sender-chosen text could
// move the cursor or rewrite the terminal the preview is printed on. A body
// keeps its line breaks and tabs.
func printable(s string, multiline bool) string {
	return strings.Map(func(r rune) rune {
		if multiline && (r == '\n' || r == '\t') {
			return r
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.ReplaceAll(s, "\r\n", "\n"))
}

// Message is the draft as a message from from.
func (d MailtoDraft) Message(from string) *ComposeMessage {
	return &ComposeMessage{From: from, To: d.To, Subject: d.Subject, Body: d.Body}
}

// PrepareUnsubscribe reads the headers of a raw message into an
// Unsubscription; From is left for the caller, who knows the account.
func PrepareUnsubscribe(raw []byte) (Unsubscription, error) {
	parsed, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return Unsubscription{}, fmt.Errorf("reading the message headers: %w", err)
	}
	h := parsed.Header
	plan := PlanUnsubscribe(h.Get("List-Unsubscribe"), h.Get("List-Unsubscribe-Post"))
	if plan.Method == UnsubscribeNone {
		return Unsubscription{}, ErrNoListUnsubscribe
	}
	sender := ParseEmailAddress(h.Get("From"))
	if sender == "" {
		sender = printable(h.Get("From"), false)
	}
	return Unsubscription{Plan: plan, Sender: sender}, nil
}

// Summary is one line naming what CarryOut would do.
func (u Unsubscription) Summary() string {
	target := printable(u.Plan.Target, false)
	switch u.Plan.Method {
	case UnsubscribeOneClick:
		return "one-click unsubscribe (RFC 8058): POST to " + target
	case UnsubscribeMailto:
		return "send an unsubscribe message to " + printable(u.Plan.Mailto.To, false)
	case UnsubscribeBrowser:
		return "open " + target + " in the browser (feral does not fetch it)"
	}
	return "nothing: " + ErrNoListUnsubscribe.Error()
}

// Preview is what the user confirms: the summary and, for a mailto, the
// message itself.
func (u Unsubscription) Preview() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Unsubscribe from %s\n", printable(u.Sender, false))
	fmt.Fprintf(&b, "  by: %s\n", u.Summary())
	if u.Plan.Method != UnsubscribeMailto {
		return b.String()
	}
	d := u.Plan.Mailto
	fmt.Fprintf(&b, "\n  From:    %s\n  To:      %s\n  Subject: %s\n\n", printable(u.From, false), printable(d.To, false), d.Subject)
	for _, line := range strings.Split(d.Body, "\n") {
		fmt.Fprintf(&b, "  %s\n", line)
	}
	return b.String()
}

// CarryOut performs a confirmed unsubscription and records it in the
// unsubscription log in configDir. A failed attempt is not followed by
// another method: that would be a second action nobody confirmed.
func (u Unsubscription) CarryOut(fx UnsubscribeEffects, configDir string) error {
	err := u.perform(fx)
	if errors.Is(err, ErrNoListUnsubscribe) {
		return err
	}
	LogUnsubscription(configDir, u.Sender, string(u.Plan.Method), u.Plan.Target, err)
	return err
}

func (u Unsubscription) perform(fx UnsubscribeEffects) error {
	switch u.Plan.Method {
	case UnsubscribeOneClick:
		if fx.PostOneClick == nil {
			return errors.New("one-click unsubscribe is not available here")
		}
		return fx.PostOneClick(u.Plan.Target)
	case UnsubscribeMailto:
		if fx.SendMail == nil {
			return errors.New("sending mail is not available here")
		}
		if u.From == "" {
			return errors.New("the account has no address to send the unsubscribe message from")
		}
		raw, err := u.Plan.Mailto.Message(u.From).BuildRFC822()
		if err != nil {
			return fmt.Errorf("building the unsubscribe message: %w", err)
		}
		return fx.SendMail(raw)
	case UnsubscribeBrowser:
		if fx.OpenBrowser == nil {
			return errors.New("no browser is available here")
		}
		return fx.OpenBrowser(u.Plan.Target)
	}
	return ErrNoListUnsubscribe
}

// ExecuteOneClickUnsubscribe sends the RFC 8058 one-click POST to an https
// URL through the SSRF-guarded client. A redirect may lead only to another
// https URL: the token in the URL is the user's subscription.
func ExecuteOneClickUnsubscribe(urlStr string) error {
	if u, err := url.Parse(urlStr); err != nil || u.Scheme != "https" {
		return fmt.Errorf("one-click unsubscribe needs an https URL, not %q", urlStr)
	}
	return postOneClickWithClient(httpsOnlyRedirects(defaultUnsubClient), urlStr, false)
}

func postOneClickWithClient(client *http.Client, urlStr string, allowLoopback bool) error {
	return sendUnsubscribeRequest(client, http.MethodPost, urlStr, oneClickToken, allowLoopback, 300)
}

// httpsOnlyRedirects is base refusing a redirect to anything but https, on
// top of its own checks.
func httpsOnlyRedirects(base *http.Client) *http.Client {
	c := *base
	check := base.CheckRedirect
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" {
			return fmt.Errorf("refusing a redirect from https to %s", req.URL.Scheme)
		}
		if check == nil {
			return nil
		}
		return check(req, via)
	}
	return &c
}
