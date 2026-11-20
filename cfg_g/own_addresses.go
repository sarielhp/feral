package cfg_g

import (
	"fmt"
	"net/mail"
	"slices"
	"strings"
)

// OwnAddresses is the set of addresses that are the user's own, lower-cased:
// the global my_emails list, plus every account's user_email and any username
// that is itself an address. A message from one of them is mail the user
// wrote, wherever it is filed.
func (c *Config) OwnAddresses() map[string]bool {
	own := map[string]bool{}
	for _, a := range c.OwnAddressSources() {
		own[a.Address] = true
	}
	return own
}

// Where an own address comes from: the my_emails list, or an account's send
// address or login.
const (
	OwnFromMyEmails  = "my_emails"
	OwnFromUserEmail = "user_email"
	OwnFromUsername  = "username"
)

// OwnSource is one place an own address is configured: Field is one of the
// OwnFrom* names and Account the account's name, empty for my_emails.
type OwnSource struct {
	Field   string
	Account string
}

// OwnAddress is one of the user's own addresses, lower-cased, with every
// place it is configured.
type OwnAddress struct {
	Address string
	Sources []OwnSource
}

// OwnAddressSources is what OwnAddresses is made of, kept in the order it was
// found -- my_emails first, then account by account -- each address once with
// all its sources, so `config emails` can say where each one is set.
func (c *Config) OwnAddressSources() []OwnAddress {
	if c == nil {
		return nil
	}
	var out []OwnAddress
	index := map[string]int{}
	add := func(addr string, src OwnSource) {
		addr = strings.ToLower(strings.TrimSpace(addr))
		if !strings.Contains(addr, "@") {
			return
		}
		i, seen := index[addr]
		if !seen {
			i = len(out)
			index[addr] = i
			out = append(out, OwnAddress{Address: addr})
		}
		if !slices.Contains(out[i].Sources, src) {
			out[i].Sources = append(out[i].Sources, src)
		}
	}
	for _, a := range c.MyEmails {
		add(a, OwnSource{Field: OwnFromMyEmails})
	}
	for _, acc := range c.Accounts {
		add(acc.UserEmail, OwnSource{Field: OwnFromUserEmail, Account: acc.Name})
		add(acc.Username, OwnSource{Field: OwnFromUsername, Account: acc.Name})
	}
	return out
}

// ParseMyEmails splits a comma-separated list of addresses, as
// `config set my_emails` takes it, keeping the bare address of each and
// dropping blanks and repeats; an entry that is not an address is an error.
func ParseMyEmails(value string) ([]string, error) {
	out := []string{}
	seen := map[string]bool{}
	for _, p := range strings.Split(value, ",") {
		if p = strings.TrimSpace(p); p == "" {
			continue
		}
		addr, err := mail.ParseAddress(p)
		if err != nil {
			return nil, fmt.Errorf("not an email address: %q", p)
		}
		key := strings.ToLower(addr.Address)
		if !seen[key] {
			seen[key] = true
			out = append(out, addr.Address)
		}
	}
	return out, nil
}
