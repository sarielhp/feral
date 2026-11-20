package email

import (
	"strings"
	"time"
)

// Email is the summary of one message: its identity, decoded display headers,
// read/reply/spam flags and its position in a thread.
type Email struct {
	ID      ID
	Account string
	// AccountBadge and AccountIndex are display-only: the tag a listing
	// that mixes accounts shows for Account, and the account's position in
	// the configuration, which gives it a stable colour.
	AccountBadge string
	AccountIndex int
	Subject      string
	FromEmail    string
	FromRaw      string
	// ToEmail and ToRaw are the recipient header. A listing of a folder the
	// owner wrote -- Sent or Drafts -- shows these where it would otherwise
	// show the sender, so the row names who the message went to rather than
	// the user's own address.
	ToEmail       string
	ToRaw         string
	CcEmail       string
	CcRaw         string
	EmailDate     time.Time
	FormattedDate string
	DateReceived  time.Time
	Size          int64
	IsSpam        bool
	IsPolitical   bool
	IsBlacklisted bool
	// IsAuthored marks a summary loaded from a folder the account's owner
	// wrote -- Sent or Drafts. It is folder context, not a fact of the
	// message, so it is never written to the parsed-summary cache: the same
	// message is authored in Sent and not in INBOX.
	IsAuthored    bool `json:"-"`
	IsRead        bool
	IsReplied     bool
	HasICS        bool
	HasAttachment bool
	MessageID     HeaderID
	// ListID is the mailing list the message came through, from List-Id
	// (RFC 2919): the identifier in angle brackets, lower-cased, or "". It
	// is never omitempty: a stored summary with no ListID key predates it,
	// and that is how the summary cache knows to parse the message again.
	ListID              string
	InReplyTo           string
	References          string
	ThreadDepth         int
	ThreadHasReplies    bool
	ThreadCollapsed     bool
	ThreadRepliesCount  int
	ThreadSenderSummary string
	ThreadCCSummary     string
	ThreadPrefix        string
	ThreadIndex         int
	// ThreadHiddenUnread marks a collapsed thread row whose hidden messages
	// include an unread one. It is view state, derived when the list is
	// built, so it is never written to the parsed-summary cache.
	ThreadHiddenUnread bool `json:"-"`
}

// ThreadNode is one message in a thread tree, together with its parent and its
// replies.
type ThreadNode struct {
	Email    *Email
	Children []*ThreadNode
	Parent   *ThreadNode
}

// HeaderID is one RFC 5322 message identifier, `<local@domain>`, as found in
// Message-ID and listed in In-Reply-To and References. It is not Email.ID,
// the backend's identifier for the stored message: the two are both strings,
// and code that compared one against the other matched by accident or not at
// all. Keeping them distinct types makes that comparison a compile error.
type HeaderID string

// Same reports whether two identifiers name the same message, ignoring case
// as every dedup comparison here always has.
func (h HeaderID) Same(o HeaderID) bool { return strings.EqualFold(string(h), string(o)) }

// Folded is h in the form Same compares, for use as a map key.
func (h HeaderID) Folded() HeaderID { return HeaderID(strings.ToLower(string(h))) }
