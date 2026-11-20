package contactsui

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/sarielhp/feral/app"
	"github.com/sarielhp/feral/contacts"
	"github.com/sarielhp/feral/email"
)

// The output formats of `contacts pick`. They are the contract other
// programs depend on, so a change here is a breaking change.
const (
	FormatRFC  = "rfc"  // one "Name <addr>" per line
	FormatAddr = "addr" // one bare address per line
	FormatJSON = "json" // an array of Choice
)

// Choice is one picked contact as `contacts pick --format=json` prints it.
type Choice struct {
	Name    string   `json:"name"`
	Email   string   `json:"email"`
	ID      string   `json:"id,omitempty"`
	Source  string   `json:"source,omitempty"`
	Groups  []string `json:"groups,omitempty"`
	IsGroup bool     `json:"is_group"`
	Members []Choice `json:"members,omitempty"`
}

// CheckFormat refuses a format WriteChoices does not know, as a usage error.
func CheckFormat(format string) error {
	switch format {
	case FormatRFC, FormatAddr, FormatJSON:
		return nil
	}
	return fmt.Errorf("%w: unknown format %q (want rfc, addr or json)", app.ErrUsage, format)
}

// WriteChoices prints picked to w in format. A group alias stands for its
// members, so rfc and addr always print the members; json prints the group
// with its members, or, with expandGroups, the members in its place. book
// supplies the members' names.
func WriteChoices(w io.Writer, picked []contacts.ScoredContact, format string, expandGroups bool, book *contacts.Book) error {
	choices := toChoices(picked, book)
	if format == FormatJSON {
		if expandGroups {
			choices = flatten(choices)
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(choices)
	}
	var b strings.Builder
	for _, c := range flatten(choices) {
		if format == FormatAddr {
			b.WriteString(c.Email + "\n")
			continue
		}
		b.WriteString(email.FormatAddress(c.Name, c.Email) + "\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func toChoices(picked []contacts.ScoredContact, book *contacts.Book) []Choice {
	out := make([]Choice, 0, len(picked))
	for _, sc := range picked {
		c := Choice{Name: sc.Name, Email: sc.PrimaryEmail(), ID: sc.ID, Source: sc.Source,
			Groups: sc.Groups, IsGroup: sc.IsGroup()}
		if c.IsGroup {
			c.Email = ""
			c.Members = members(sc.Emails, book)
		}
		out = append(out, c)
	}
	return out
}

// members are the group's addresses, named as the book names them.
func members(addrs []string, book *contacts.Book) []Choice {
	out := make([]Choice, 0, len(addrs))
	for _, a := range addrs {
		c := Choice{Email: a}
		if p := contacts.FindByAddress(book, a); p != nil {
			c.Name, c.ID, c.Source = p.Name, p.ID, p.Source
		}
		out = append(out, c)
	}
	return out
}

// flatten replaces each group by its members, dropping repeated addresses.
func flatten(choices []Choice) []Choice {
	seen := map[string]bool{}
	var out []Choice
	add := func(c Choice) {
		key := strings.ToLower(c.Email)
		if key == "" || seen[key] {
			return
		}
		seen[key] = true
		out = append(out, c)
	}
	for _, c := range choices {
		if !c.IsGroup {
			add(c)
			continue
		}
		for _, m := range c.Members {
			add(m)
		}
	}
	if out == nil {
		out = []Choice{}
	}
	return out
}
