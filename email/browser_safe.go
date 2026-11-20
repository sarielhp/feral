package email

import (
	"regexp"
	"strings"
)

// browserPolicy is the Content-Security-Policy a message page opens under.
// It loads nothing from the network and runs no script: inline styles and
// images embedded as data: stay, so the message still looks like itself.
//
// A message opened in the browser used to be the sender's HTML verbatim, so
// its remote images -- tracking pixels among them -- loaded and told the
// sender when it was read and from where, and its scripts ran.
const browserPolicy = "default-src 'none'; img-src data: cid:; style-src 'unsafe-inline' data:; " +
	"font-src data:; media-src data:; form-action 'none'; base-uri 'none'"

var (
	// metaRefreshRE matches a meta refresh: a policy cannot stop a page from
	// navigating itself to a remote address, which leaks the open as a pixel
	// would.
	metaRefreshRE = regexp.MustCompile(`(?i)<meta[^>]*http-equiv\s*=\s*["']?\s*refresh[^>]*>`)
	doctypeRE     = regexp.MustCompile(`(?i)^\s*<!doctype[^>]*>`)
)

// ForBrowser prepares a message's HTML for a browser: the policy goes first,
// after any doctype (before it, the page would render in quirks mode), and
// meta refreshes are removed. Policies only add up, so one in the message
// cannot loosen this one.
func ForBrowser(htmlDoc string) string {
	htmlDoc = metaRefreshRE.ReplaceAllString(htmlDoc, "")
	meta := `<meta http-equiv="Content-Security-Policy" content="` + browserPolicy + `">` + "\n"
	if loc := doctypeRE.FindStringIndex(htmlDoc); loc != nil {
		return htmlDoc[:loc[1]] + "\n" + meta + strings.TrimLeft(htmlDoc[loc[1]:], "\n")
	}
	return meta + htmlDoc
}
