package contacts

import (
	"fmt"
	"slices"
	"strings"
)

// A re-import replaces every contact of its source. What the user added --
// aliases, notes, and the overlay of edits -- is not the source's to drop:
// it moves to the contact that now stands for the same person, found by ID
// first (a Google resource name or a mutt alias key) and by a shared address
// after. A contact the user hid stays hidden.

// matcher finds, in a list of contacts, the one an old contact became.
type matcher struct {
	byID   map[string]int
	byAddr map[string]int
}

func newMatcher(book []Contact) *matcher {
	m := &matcher{byID: map[string]int{}, byAddr: map[string]int{}}
	for i := range book {
		m.index(book, i)
	}
	return m
}

func (m *matcher) index(book []Contact, i int) {
	if _, taken := m.byID[book[i].ID]; !taken {
		m.byID[book[i].ID] = i
	}
	for _, e := range book[i].Emails {
		key := strings.ToLower(strings.TrimSpace(e))
		if _, taken := m.byAddr[key]; !taken && key != "" {
			m.byAddr[key] = i
		}
	}
}

// find is the index of the contact old became, or -1.
func (m *matcher) find(old Contact) int {
	if i, ok := m.byID[old.ID]; ok {
		return i
	}
	for _, e := range slices.Concat(old.Emails, old.SourceView().Emails) {
		if i, ok := m.byAddr[strings.ToLower(strings.TrimSpace(e))]; ok {
			return i
		}
	}
	return -1
}

// hasUserData reports whether the user added anything to c.
func hasUserData(c Contact) bool {
	return len(c.Aliases) > 0 || c.Notes != "" || c.IsEdited()
}

// carryUserEdits moves the user's additions from the replaced contacts onto
// the contacts that now stand for them, creating a manual contact for one no
// contact stands for any more, so a re-import never loses what the user did.
func carryUserEdits(book []Contact, replaced []Contact) []Contact {
	m := newMatcher(book)
	for _, old := range replaced {
		if !hasUserData(old) || old.PrimaryEmail() == "" {
			continue
		}
		if i := m.find(old); i >= 0 {
			applyUserData(&book[i], old)
			continue
		}
		book = append(book, orphanedContact(book, old))
		m.index(book, len(book)-1)
	}
	return book
}

// applyUserData gives c the aliases, notes and edits the user made to old.
func applyUserData(c *Contact, old Contact) {
	for _, a := range old.Aliases {
		if !hasNickname(c, a) {
			c.Aliases = append(c.Aliases, a)
		}
	}
	if c.Notes == "" {
		c.Notes = old.Notes
	}
	if !old.IsEdited() || !c.IsImported() || c.IsEdited() {
		return
	}
	origin := c.SourceView()
	edits := *old.Edits
	c.Origin, c.Edits = &origin, &edits
	fold(c)
}

// orphanedContact is the manual contact that keeps what the user gave old
// once its source no longer lists it.
func orphanedContact(book []Contact, old Contact) Contact {
	emails := make([]string, 0, len(old.Emails))
	for _, e := range old.Emails {
		emails = append(emails, strings.ToLower(strings.TrimSpace(e)))
	}
	id := SourceManual + ":" + emails[0]
	for n := 2; slices.ContainsFunc(book, func(c Contact) bool { return c.ID == id }); n++ {
		id = fmt.Sprintf("%s:%s#%d", SourceManual, emails[0], n)
	}
	return Contact{
		ID:      id,
		Name:    old.Name,
		Emails:  emails,
		Groups:  slices.Clone(old.Groups),
		Aliases: slices.Clone(old.Aliases),
		Notes:   old.Notes,
		Source:  SourceManual,
	}
}

// keepHidden moves out of book the contacts that the hidden ones became,
// with the user's additions carried, and returns the new book and hidden
// list. A hidden contact its source no longer lists is dropped.
func keepHidden(book []Contact, hidden []Contact) ([]Contact, []Contact) {
	m := newMatcher(book)
	hide := map[int]bool{}
	var still []Contact
	for _, old := range hidden {
		i := m.find(old)
		if i < 0 || hide[i] {
			continue
		}
		applyUserData(&book[i], old)
		hide[i] = true
		still = append(still, book[i])
	}
	var shown []Contact
	for i, c := range book {
		if !hide[i] {
			shown = append(shown, c)
		}
	}
	return shown, still
}
