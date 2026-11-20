package contacts

import (
	"errors"
	"fmt"
	"net/mail"
	"slices"
	"strings"
)

// The user's edits to the book. An imported contact -- from Google or mutt --
// is replaced on every re-import, so its edits cannot be written into the
// fields the source owns: they are kept as an overlay (Edits) on what the
// source said (Origin) and applied again after each import (carryUserEdits).
// A manual contact belongs to the user outright and is edited in place.

// SourceManual is the source of a contact the user made.
const SourceManual = "manual"

// Origin is what a contact's source last said about the fields the user can
// override.
type Origin struct {
	Name   string   `json:"name,omitempty"`
	Emails []string `json:"emails,omitempty"`
	Groups []string `json:"groups,omitempty"`
}

// Edits are the user's changes to an imported contact, kept as differences
// so that what the source changes later still shows through.
type Edits struct {
	Name         string   `json:"name,omitempty"`
	Primary      string   `json:"primary,omitempty"`
	AddEmails    []string `json:"add_emails,omitempty"`
	RemoveEmails []string `json:"remove_emails,omitempty"`
	AddGroups    []string `json:"add_groups,omitempty"`
	RemoveGroups []string `json:"remove_groups,omitempty"`
}

func (e *Edits) empty() bool {
	return e == nil || (e.Name == "" && e.Primary == "" && len(e.AddEmails) == 0 &&
		len(e.RemoveEmails) == 0 && len(e.AddGroups) == 0 && len(e.RemoveGroups) == 0)
}

// Change is the state the user wants a contact in. Edit turns it into
// direct changes for a manual contact and an overlay for an imported one.
type Change struct {
	Name    string
	Emails  []string
	Groups  []string
	Aliases []string
	Notes   string
}

// IsImported reports whether c comes from a source that re-imports it.
func (c Contact) IsImported() bool {
	return c.Source != "" && c.Source != SourceManual
}

// IsEdited reports whether the user changed fields of an imported contact.
func (c Contact) IsEdited() bool {
	return c.Origin != nil && !c.Edits.empty()
}

// SourceView is what the source says about c, before the user's edits.
func (c Contact) SourceView() Origin {
	if c.Origin != nil {
		return *c.Origin
	}
	return Origin{Name: c.Name, Emails: c.Emails, Groups: c.Groups}
}

// FindByID is the contact with id, or nil.
func FindByID(book *Book, id string) *Contact {
	if book == nil {
		return nil
	}
	for i := range book.Contacts {
		if book.Contacts[i].ID == id {
			return &book.Contacts[i]
		}
	}
	return nil
}

// Edit brings the contact with id to the state ch describes and returns it.
// A contact needs an address; a nickname another contact answers to is
// refused with *AliasTakenError.
func Edit(book *Book, id string, ch Change) (*Contact, error) {
	c := FindByID(book, id)
	if c == nil {
		return nil, fmt.Errorf("no contact %q", id)
	}
	emails, err := normalizeAddresses(ch.Emails)
	if err != nil {
		return nil, err
	}
	if len(emails) == 0 {
		return nil, errors.New("a contact needs at least one address")
	}
	aliases := cleanList(ch.Aliases)
	for _, a := range aliases {
		if holder := aliasHolderByID(book, a, id); holder != nil {
			return nil, &AliasTakenError{Alias: a, Holder: *holder}
		}
	}
	c.Aliases = aliases
	c.Notes = strings.TrimSpace(ch.Notes)
	name, groups := strings.TrimSpace(ch.Name), cleanList(ch.Groups)
	if !c.IsImported() {
		c.Name, c.Emails, c.Groups = name, emails, groups
		return c, nil
	}
	origin := c.SourceView()
	c.Origin = &origin
	c.Edits = editsFrom(origin, name, emails, groups)
	fold(c)
	return c, nil
}

// editsFrom is the overlay that turns origin into the wanted fields.
func editsFrom(origin Origin, name string, emails, groups []string) *Edits {
	e := &Edits{
		AddEmails:    missingFrom(emails, origin.Emails),
		RemoveEmails: missingFrom(origin.Emails, emails),
		AddGroups:    missingFrom(groups, origin.Groups),
		RemoveGroups: missingFrom(origin.Groups, groups),
	}
	if name != origin.Name {
		e.Name = name
	}
	natural := applyDiff(origin.Emails, e.RemoveEmails, e.AddEmails, "")
	if len(natural) > 0 && !strings.EqualFold(natural[0], emails[0]) {
		e.Primary = emails[0]
	}
	return e
}

// fold sets c's Name, Emails and Groups from its Origin and Edits, and drops
// both once the edits change nothing.
func fold(c *Contact) {
	if c.Origin == nil {
		return
	}
	o, e := c.Origin, c.Edits
	if e.empty() {
		c.Name, c.Emails, c.Groups = o.Name, slices.Clone(o.Emails), slices.Clone(o.Groups)
		c.Origin, c.Edits = nil, nil
		return
	}
	c.Name = o.Name
	if e.Name != "" {
		c.Name = e.Name
	}
	c.Emails = applyDiff(o.Emails, e.RemoveEmails, e.AddEmails, e.Primary)
	c.Groups = applyDiff(o.Groups, e.RemoveGroups, e.AddGroups, "")
}

// Revert drops the user's edits to the imported contact with id, so it shows
// what its source says again. Aliases and notes are the user's and stay.
func Revert(book *Book, id string) (*Contact, error) {
	c := FindByID(book, id)
	if c == nil {
		return nil, fmt.Errorf("no contact %q", id)
	}
	c.Edits = nil
	fold(c)
	return c, nil
}

// Remove takes the contact with id out of the book. A manual contact is
// deleted; an imported one moves to Hidden, since its source would bring it
// back. Reports whether it was hidden rather than deleted.
func Remove(book *Book, id string) (bool, error) {
	i := slices.IndexFunc(book.Contacts, func(c Contact) bool { return c.ID == id })
	if i < 0 {
		return false, fmt.Errorf("no contact %q", id)
	}
	c := book.Contacts[i]
	book.Contacts = slices.Delete(book.Contacts, i, i+1)
	if !c.IsImported() {
		return false, nil
	}
	book.Hidden = append(book.Hidden, c)
	return true, nil
}

// Unhide brings the hidden contact with id back into the book.
func Unhide(book *Book, id string) (*Contact, error) {
	i := slices.IndexFunc(book.Hidden, func(c Contact) bool { return c.ID == id })
	if i < 0 {
		return nil, fmt.Errorf("no hidden contact %q", id)
	}
	book.Contacts = append(book.Contacts, book.Hidden[i])
	book.Hidden = slices.Delete(book.Hidden, i, i+1)
	return &book.Contacts[len(book.Contacts)-1], nil
}

// AddContact makes a manual contact for a person. An address another
// contact already holds is refused, so one person is not listed twice.
func AddContact(book *Book, name string, emails []string) (*Contact, error) {
	addrs, err := normalizeAddresses(emails)
	if err != nil {
		return nil, err
	}
	if len(addrs) == 0 {
		return nil, errors.New("a contact needs at least one address")
	}
	for _, a := range addrs {
		if holder := FindByAddress(book, a); holder != nil {
			return nil, fmt.Errorf("%s is already a contact: %s", a, holder.Label())
		}
	}
	book.Contacts = append(book.Contacts, Contact{
		ID:     uniqueID(book, SourceManual+":"+addrs[0]),
		Name:   strings.TrimSpace(name),
		Emails: addrs,
		Source: SourceManual,
	})
	return &book.Contacts[len(book.Contacts)-1], nil
}

// AddGroup makes a manual group alias: a name standing for its members.
func AddGroup(book *Book, name string, members []string) (*Contact, error) {
	name = strings.TrimSpace(name)
	key := groupKey(name)
	if key == "" {
		return nil, errors.New("a group needs a name")
	}
	addrs, err := normalizeAddresses(members)
	if err != nil {
		return nil, err
	}
	if len(addrs) == 0 {
		return nil, errors.New("a group needs at least one member")
	}
	if holder := aliasHolderByID(book, key, ""); holder != nil {
		return nil, &AliasTakenError{Alias: key, Holder: *holder}
	}
	book.Contacts = append(book.Contacts, Contact{
		ID:      uniqueID(book, GroupID(SourceManual, key)),
		Name:    name,
		Emails:  addrs,
		Aliases: []string{key},
		Source:  SourceManual,
	})
	return &book.Contacts[len(book.Contacts)-1], nil
}

// Merge folds the contact dropID into keepID: keep gains drop's addresses,
// groups and aliases, and its notes are appended; drop is then removed (or
// hidden, when imported). Groups cannot be merged with people.
func Merge(book *Book, keepID, dropID string) (*Contact, error) {
	keep, drop := FindByID(book, keepID), FindByID(book, dropID)
	if keep == nil || drop == nil || keepID == dropID {
		return nil, errors.New("merge needs two different contacts")
	}
	if keep.IsGroup() != drop.IsGroup() {
		return nil, errors.New("a group cannot be merged with a person")
	}
	ch := Change{
		Name:    keep.Name,
		Emails:  unionFold(keep.Emails, drop.Emails),
		Groups:  unionFold(keep.Groups, drop.Groups),
		Aliases: unionFold(keep.Aliases, drop.Aliases),
		Notes:   joinNotes(keep.Notes, drop.Notes),
	}
	if ch.Name == "" {
		ch.Name = drop.Name
	}
	dropped := *drop
	if _, err := Remove(book, dropID); err != nil {
		return nil, err
	}
	merged, err := Edit(book, keepID, ch)
	if err != nil {
		// Put drop back, so a refused merge changes nothing.
		book.Contacts = append(book.Contacts, dropped)
		if dropped.IsImported() {
			book.Hidden = book.Hidden[:len(book.Hidden)-1]
		}
		return nil, err
	}
	return merged, nil
}

// Label is the name a contact is shown by: its name, else an alias, else its
// address.
func (c Contact) Label() string {
	if c.Name != "" {
		return c.Name
	}
	if a := c.DisplayAlias(); a != "" {
		return a
	}
	return c.PrimaryEmail()
}

// aliasHolderByID is the contact other than exceptID that answers to nick.
func aliasHolderByID(book *Book, nick, exceptID string) *Contact {
	for i := range book.Contacts {
		c := &book.Contacts[i]
		if c.ID != exceptID && hasNickname(c, nick) {
			return c
		}
	}
	return nil
}

// uniqueID is id, or id with a numeric suffix when a contact already has it.
func uniqueID(book *Book, id string) string {
	candidate := id
	for n := 2; FindByID(book, candidate) != nil; n++ {
		candidate = fmt.Sprintf("%s#%d", id, n)
	}
	return candidate
}

// normalizeAddresses parses each address, lowercased, dropping blanks and
// repeats. Each entry may itself be a comma-separated list.
func normalizeAddresses(in []string) ([]string, error) {
	var out []string
	for _, entry := range in {
		for part := range strings.SplitSeq(entry, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			a, err := mail.ParseAddress(part)
			if err != nil {
				return nil, fmt.Errorf("%q is not an address", part)
			}
			out = append(out, strings.ToLower(a.Address))
		}
	}
	return unionFold(out), nil
}

// cleanList trims each entry and drops blanks and case-insensitive repeats.
func cleanList(in []string) []string {
	var out []string
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return unionFold(out)
}

// unionFold concatenates lists, keeping the first of entries equal up to case.
func unionFold(lists ...[]string) []string {
	var out []string
	for _, l := range lists {
		for _, s := range l {
			if !containsFold(out, s) {
				out = append(out, s)
			}
		}
	}
	return out
}

func containsFold(list []string, s string) bool {
	return slices.ContainsFunc(list, func(x string) bool { return strings.EqualFold(x, s) })
}

// missingFrom is the entries of a not in b, up to case.
func missingFrom(a, b []string) []string {
	var out []string
	for _, s := range a {
		if !containsFold(b, s) {
			out = append(out, s)
		}
	}
	return out
}

// applyDiff is base without remove, plus add, with primary moved first.
func applyDiff(base, remove, add []string, primary string) []string {
	out := unionFold(missingFrom(base, remove), add)
	i := slices.IndexFunc(out, func(s string) bool { return strings.EqualFold(s, primary) })
	if i <= 0 {
		return out
	}
	// Read the entry before Delete clears the tail it leaves behind.
	first := out[i]
	return append([]string{first}, slices.Delete(out, i, i+1)...)
}

func joinNotes(a, b string) string {
	a, b = strings.TrimSpace(a), strings.TrimSpace(b)
	if a == "" || b == "" {
		return a + b
	}
	return a + "\n" + b
}

// groupKey is the alias a group is addressed by: its name, lowercased, with
// spaces as dashes.
func groupKey(name string) string {
	return strings.Join(strings.Fields(strings.ToLower(name)), "-")
}
