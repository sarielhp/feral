package contacts

import (
	"errors"
	"fmt"
	"net/mail"
	"slices"
	"strings"
)

// AddAlias records nickname for addr in the book: the contact that already
// holds the address gains the nickname as one of its Aliases, or when no
// contact has it, a manual contact is created, keeping name as the display
// name when the address had one offered with it. Returns the affected contact
// and whether a new one was created; a nil contact means nothing was recorded.
// Nothing is written to disk; the caller saves the book.
func AddAlias(book *Book, addr, name, nickname string) (*Contact, bool) {
	address := normalizeAliasAddr(addr)
	nickname = strings.TrimSpace(nickname)
	if book == nil || address == "" || nickname == "" {
		return nil, false
	}

	for i := range book.Contacts {
		c := &book.Contacts[i]
		if !holdsPerson(c, address) {
			continue
		}
		if name != "" && strings.TrimSpace(c.Name) == "" {
			c.Name = strings.TrimSpace(name)
		}
		if !hasNickname(c, nickname) {
			c.Aliases = append(c.Aliases, nickname)
		}
		return c, false
	}

	book.Contacts = append(book.Contacts, Contact{
		ID:      "manual:" + address,
		Name:    strings.TrimSpace(name),
		Emails:  []string{address},
		Aliases: []string{nickname},
		Source:  "manual",
	})
	return &book.Contacts[len(book.Contacts)-1], true
}

// AliasTakenError is SetAlias refusing a nickname another contact already
// answers to, as an alias the user gave or a nickname its source carries.
type AliasTakenError struct {
	Alias  string
	Holder Contact
}

func (e *AliasTakenError) Error() string {
	who := e.Holder.Name
	if who == "" {
		who = e.Holder.PrimaryEmail()
	}
	return fmt.Sprintf("%q already names %s", e.Alias, who)
}

// SetAlias gives the contact holding addr the nickname newNick, replacing
// oldNick when it is one of the contact's own aliases: a rename, or, with
// newNick empty, a removal. With oldNick empty it adds, as AddAlias does. A
// nickname another contact answers to is refused with *AliasTakenError, so
// one alias never stands for two people. Returns the affected contact and
// whether it was created. Nothing is written to disk; the caller saves.
func SetAlias(book *Book, addr, name, oldNick, newNick string) (*Contact, bool, error) {
	address := normalizeAliasAddr(addr)
	oldNick, newNick = strings.TrimSpace(oldNick), strings.TrimSpace(newNick)
	if book == nil || address == "" {
		return nil, false, fmt.Errorf("%q is not an address", addr)
	}
	if newNick != "" {
		if holder := AliasHolder(book, newNick, address); holder != nil {
			return nil, false, &AliasTakenError{Alias: newNick, Holder: *holder}
		}
	}
	if oldNick == "" {
		if newNick == "" {
			return nil, false, errors.New("the alias is empty")
		}
		c, created := AddAlias(book, address, name, newNick)
		return c, created, nil
	}
	c := FindByAddress(book, address)
	i := -1
	if c != nil {
		i = slices.IndexFunc(c.Aliases, func(a string) bool { return strings.EqualFold(a, oldNick) })
	}
	if i < 0 {
		return nil, false, fmt.Errorf("%q is not an alias of %s", oldNick, address)
	}
	// In place, so the alias the lists show first stays first. A new name the
	// contact already answers to otherwise just drops the old one.
	others := *c
	others.Aliases = slices.Delete(slices.Clone(c.Aliases), i, i+1)
	if newNick == "" || hasNickname(&others, newNick) {
		c.Aliases = others.Aliases
		return c, false, nil
	}
	c.Aliases[i] = newNick
	return c, false, nil
}

// AliasHolder is the contact, other than the one holding exceptAddr, that
// answers to nickname, or nil when none does.
func AliasHolder(book *Book, nickname, exceptAddr string) *Contact {
	if book == nil {
		return nil
	}
	for i := range book.Contacts {
		c := &book.Contacts[i]
		if exceptAddr != "" && holdsPerson(c, normalizeAliasAddr(exceptAddr)) {
			continue
		}
		if hasNickname(c, nickname) {
			return c
		}
	}
	return nil
}

// FindByAddress is the first contact, not a group, holding addr, or nil.
func FindByAddress(book *Book, addr string) *Contact {
	address := normalizeAliasAddr(addr)
	if book == nil || address == "" {
		return nil
	}
	for i := range book.Contacts {
		if holdsPerson(&book.Contacts[i], address) {
			return &book.Contacts[i]
		}
	}
	return nil
}

// normalizeAliasAddr canonicalizes an address for matching: parsed and
// lowercased, or empty when it is not an address at all.
func normalizeAliasAddr(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ""
	}
	parsed, err := mail.ParseAddress(addr)
	if err != nil {
		return ""
	}
	return strings.ToLower(parsed.Address)
}

// holdsAddress reports whether contact carries email, case-insensitively.
func holdsAddress(c *Contact, email string) bool {
	for _, e := range c.Emails {
		if strings.EqualFold(e, email) {
			return true
		}
	}
	return false
}

// hasNickname reports whether contact already answers to nickname, as an
// alias or a source nickname.
func hasNickname(c *Contact, nickname string) bool {
	for _, n := range c.AllAliases() {
		if strings.EqualFold(n, nickname) {
			return true
		}
	}
	return false
}

// holdsPerson reports whether c is the person behind email: it carries the
// address and is not a group, which carries its members' addresses.
func holdsPerson(c *Contact, email string) bool {
	return !c.IsGroup() && holdsAddress(c, email)
}
