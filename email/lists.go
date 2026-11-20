package email

import (
	"errors"
	"net/mail"
	"net/url"
	"strings"
	"unicode/utf8"
)

// Mailing-list headers. List-Id (RFC 2919) names the list a message was
// distributed through; List-Post (RFC 2369) says where a reply to the list
// goes. Both are written by the list server, which is to say by whoever sent
// the message, so they are parsed defensively and never trusted for anything
// but display and addressing.

// ErrNoListPost is a list reply to a message without List-Post: it did not
// come through a list that takes posts by mail, and a reply to "the list"
// has nowhere to go. It is never answered by addressing the sender instead.
var ErrNoListPost = errors.New("the message has no List-Post header, so there is no list address to reply to")

// ErrListClosed is a list reply to a message whose List-Post is NO: the list
// exists but does not accept posts (an announcement list).
var ErrListClosed = errors.New("the list does not accept posts (List-Post: NO)")

// ErrListPostNoMailto is a List-Post that offers no mailto: URI -- a list
// that takes posts only through a web form.
var ErrListPostNoMailto = errors.New("List-Post offers no mailto: address to reply to")

// listBadgeWidth caps the badge a list row shows, in characters.
const listBadgeWidth = 12

// ParseListID returns the list identifier of a List-Id header value -- the
// part in angle brackets, lower-cased -- or "" when the value names none.
// RFC 2919 writes it as an optional phrase followed by <list-label.namespace>;
// a value with no brackets is taken whole when it is a single dot-separated
// word, which is what a few list servers send.
func ParseListID(value string) string {
	value = strings.TrimSpace(StripEscapeSequences(value))
	if value == "" {
		return ""
	}
	if open := strings.LastIndexByte(value, '<'); open >= 0 {
		end := strings.IndexByte(value[open:], '>')
		if end < 0 {
			return ""
		}
		return cleanListID(value[open+1 : open+end])
	}
	if strings.ContainsAny(value, " \t\"(),;") {
		return ""
	}
	return cleanListID(value)
}

// cleanListID lower-cases id and refuses one that cannot be a list
// identifier: empty, with no dot, or carrying whitespace or controls.
func cleanListID(id string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" || !strings.Contains(id, ".") {
		return ""
	}
	for _, r := range id {
		if r <= ' ' || r == 0x7f {
			return ""
		}
	}
	return id
}

// ListBadge is the short tag a listing shows for a list: the list-label, the
// part of the identifier before its first dot, capped at listBadgeWidth
// characters. "linux-kernel.vger.kernel.org" shows as "linux-kernel".
func ListBadge(listID string) string {
	label, _, _ := strings.Cut(listID, ".")
	if utf8.RuneCountInString(label) <= listBadgeWidth {
		return label
	}
	return string([]rune(label)[:listBadgeWidth])
}

// ListReplyAddress is the address a reply to the list goes to, taken from the
// message's List-Post header: the first mailto: URI's address, without its
// query. It is the one place that choice is made, for the TUI's list reply and
// `msg reply --list` alike. A message with no List-Post is ErrNoListPost, a
// closed list ErrListClosed and a list reachable only on the web
// ErrListPostNoMailto; none of them falls back to the sender.
func ListReplyAddress(h mail.Header) (string, error) {
	value := strings.TrimSpace(h.Get("List-Post"))
	if value == "" {
		return "", ErrNoListPost
	}
	if strings.EqualFold(strings.TrimSpace(stripComments(value)), "NO") {
		return "", ErrListClosed
	}
	for _, uri := range bracketedURIs(value) {
		if addr, ok := mailtoAddress(uri); ok {
			return addr, nil
		}
	}
	return "", ErrListPostNoMailto
}

// stripComments removes RFC 5322 comments -- parenthesized text -- from a
// header value, so "NO (posting not allowed)" reads as NO.
func stripComments(value string) string {
	var b strings.Builder
	depth := 0
	for _, r := range value {
		switch {
		case r == '(':
			depth++
		case r == ')' && depth > 0:
			depth--
		case depth == 0:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// bracketedURIs is every <...> URI in an RFC 2369 header value, in order.
func bracketedURIs(value string) []string {
	var uris []string
	for {
		open := strings.IndexByte(value, '<')
		if open < 0 {
			return uris
		}
		end := strings.IndexByte(value[open:], '>')
		if end < 0 {
			return uris
		}
		uris = append(uris, strings.TrimSpace(value[open+1:open+end]))
		value = value[open+end+1:]
	}
}

// mailtoAddress is the single address of a mailto: URI, its query dropped,
// or false when uri is not a mailto: URI naming one valid address.
func mailtoAddress(uri string) (string, bool) {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "mailto" {
		return "", false
	}
	addr, err := mailtoURLAddress(u)
	return addr, err == nil
}

// BuildListReplyDetails is BuildReplyDetails for a reply to the list: the same
// subject, threading headers and quote, addressed to the list's posting
// address alone, with no Cc. It fails as ListReplyAddress does.
func BuildListReplyDetails(orig *mail.Message) (ReplyDetails, error) {
	if orig == nil {
		return ReplyDetails{}, ErrNoListPost
	}
	to, err := ListReplyAddress(orig.Header)
	if err != nil {
		return ReplyDetails{}, err
	}
	details := BuildReplyDetails(orig, nil, false, false)
	details.To = to
	details.Cc = nil
	return details, nil
}
