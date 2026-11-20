package contactsui

import (
	"cmp"
	"slices"
	"strings"

	"github.com/sarielhp/feral/contacts"
)

// view is which set of contacts the manager lists.
type view int

const (
	viewContacts view = iota // the book
	viewHistory              // harvested addresses that are not contacts yet
	viewHidden               // imported contacts the user removed
	viewCount
)

func (v view) String() string {
	switch v {
	case viewHistory:
		return "Harvested"
	case viewHidden:
		return "Hidden"
	}
	return "Contacts"
}

// sortKey is how the contacts view is ordered.
type sortKey int

const (
	sortName sortKey = iota
	sortFrequency
	sortSource
	sortCount
)

func (s sortKey) String() string {
	switch s {
	case sortFrequency:
		return "most used"
	case sortSource:
		return "source"
	}
	return "name"
}

// searchFor is the search that fills the list for v.
func (m *Manager) searchFor(v view) func(string) []contacts.ScoredContact {
	switch v {
	case viewHistory:
		return m.searchHistory
	case viewHidden:
		return m.searchHidden
	}
	return m.searchContacts
}

// searchContacts is the book's contacts matching query, with how often each
// was written to or archived from, in the chosen order.
func (m *Manager) searchContacts(query string) []contacts.ScoredContact {
	use := usage(m.history)
	var out []contacts.ScoredContact
	for _, c := range contacts.Search(m.book, query) {
		sc := contacts.ScoredContact{Contact: c}
		for _, e := range c.Emails {
			sc.Frequency += use[strings.ToLower(e)]
		}
		out = append(out, sc)
	}
	sortContacts(out, m.sortBy)
	return out
}

// searchHistory is the harvested addresses no contact holds, most used first.
func (m *Manager) searchHistory(query string) []contacts.ScoredContact {
	if m.history == nil {
		return nil
	}
	q := strings.ToLower(strings.TrimSpace(query))
	var out []contacts.ScoredContact
	for _, h := range m.history.Entries {
		addr := h.PrimaryEmail()
		if addr == "" || contacts.FindByAddress(m.book, addr) != nil {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(h.Name+" "+addr), q) {
			continue
		}
		out = append(out, h)
	}
	slices.SortStableFunc(out, func(a, b contacts.ScoredContact) int {
		return cmp.Or(cmp.Compare(b.Frequency, a.Frequency), b.LastSeen.Compare(a.LastSeen))
	})
	return out
}

// searchHidden is the hidden contacts matching query.
func (m *Manager) searchHidden(query string) []contacts.ScoredContact {
	if m.book == nil {
		return nil
	}
	found := contacts.Search(&contacts.Book{Contacts: m.book.Hidden}, query)
	out := make([]contacts.ScoredContact, 0, len(found))
	for _, c := range found {
		out = append(out, contacts.ScoredContact{Contact: c})
	}
	sortContacts(out, sortName)
	return out
}

// usage is how often each address appears in the history.
func usage(history *contacts.HistoryBook) map[string]int {
	use := map[string]int{}
	if history == nil {
		return use
	}
	for _, h := range history.Entries {
		use[strings.ToLower(h.PrimaryEmail())] += h.Frequency
	}
	return use
}

func sortContacts(list []contacts.ScoredContact, by sortKey) {
	byName := func(a, b contacts.ScoredContact) int {
		return cmp.Compare(strings.ToLower(a.Label()), strings.ToLower(b.Label()))
	}
	slices.SortStableFunc(list, func(a, b contacts.ScoredContact) int {
		switch by {
		case sortFrequency:
			return cmp.Or(cmp.Compare(b.Frequency, a.Frequency), byName(a, b))
		case sortSource:
			return cmp.Or(cmp.Compare(a.Source, b.Source), byName(a, b))
		}
		return byName(a, b)
	})
}
