package contacts

import (
	"strings"
	"time"
)

// Contact represents a single contact or distribution list from any source.
type Contact struct {
	ID        string   `json:"id"`
	Name      string   `json:"name,omitempty"`
	Emails    []string `json:"emails,omitempty"`
	Nicknames []string `json:"nicknames,omitempty"`
	// Aliases are the nicknames the user gave this contact. Nicknames come
	// from the source and are replaced with it; Aliases are carried across
	// every re-import of the source (see UpdateSource).
	Aliases []string `json:"aliases,omitempty"`
	Groups  []string `json:"groups,omitempty"`
	Source  string   `json:"source,omitempty"`
	Notes   string   `json:"notes,omitempty"`
	// Origin and Edits are set only on an imported contact the user edited:
	// Origin is what the source last said, Edits what the user changed, and
	// Name, Emails and Groups above always hold the result of applying one to
	// the other (see fold), so a reader never needs to know about either.
	Origin *Origin `json:"origin,omitempty"`
	Edits  *Edits  `json:"edits,omitempty"`
}

// groupIDMarker is what a group alias's ID carries between its source and
// its key: "mutt:group:team".
const groupIDMarker = ":group:"

// GroupID is the ID of the group alias key imported from source.
func GroupID(source, key string) string {
	return source + groupIDMarker + key
}

// IsGroup reports whether c is a group alias, standing for all its
// addresses rather than one person.
func (c Contact) IsGroup() bool {
	return strings.Contains(c.ID, groupIDMarker)
}

// PrimaryEmail returns the first email address, or empty string.
func (c Contact) PrimaryEmail() string {
	if len(c.Emails) > 0 {
		return c.Emails[0]
	}
	return ""
}

// DisplayAlias returns the alias the user gave, else the source nickname, or
// an empty string.
func (c Contact) DisplayAlias() string {
	if len(c.Aliases) > 0 {
		return c.Aliases[0]
	}
	if len(c.Nicknames) > 0 {
		return c.Nicknames[0]
	}
	return ""
}

// AllAliases is every name the contact answers to besides its own: the
// user's aliases first, then the source nicknames.
func (c Contact) AllAliases() []string {
	if len(c.Aliases) == 0 {
		return c.Nicknames
	}
	if len(c.Nicknames) == 0 {
		return c.Aliases
	}
	all := make([]string, 0, len(c.Aliases)+len(c.Nicknames))
	return append(append(all, c.Aliases...), c.Nicknames...)
}

// Book represents the collection of contacts saved in local storage.
type Book struct {
	Contacts []Contact `json:"contacts"`
	// Hidden holds the imported contacts the user removed. A re-import keeps
	// them here instead of bringing them back; see Remove and Unhide.
	Hidden  []Contact `json:"hidden,omitempty"`
	Updated time.Time `json:"updated"`
}

// NewBook returns an empty Book initialized with current timestamp.
func NewBook() *Book {
	return &Book{
		Contacts: []Contact{},
		Updated:  time.Now().UTC(),
	}
}
