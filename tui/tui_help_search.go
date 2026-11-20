package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// Every help screen is searchable: "/" starts a query, each key typed
// shrinks the page to the rows that match it, Enter keeps the filter and
// gives the keys back to the page, and Esc clears it. The main help, the save
// dialogs' help and the attachment help share one query, since only one of
// them is open at a time, and every way of opening or closing one resets it.

// helpQuery is the search typed into a help screen: its text, and whether
// keys still go to it.
type helpQuery struct {
	text   string
	typing bool
}

// active reports whether the query narrows what the help shows: while it is
// being typed, or once kept with some text.
func (q helpQuery) active() bool {
	return q.typing || q.text != ""
}

// matches reports whether a row matches every word of the query, in its keys
// or its description, ignoring case: "save disk" finds "Save message (disk /
// label)".
func (q helpQuery) matches(row helpRow) bool {
	hay := strings.ToLower(row.keys + "  " + row.desc)
	for _, word := range strings.Fields(strings.ToLower(q.text)) {
		if !strings.Contains(hay, word) {
			return false
		}
	}
	return true
}

// filter is the rows that match the query, in their order.
func (q helpQuery) filter(rows []helpRow) []helpRow {
	if strings.TrimSpace(q.text) == "" {
		return rows
	}
	var out []helpRow
	for _, row := range rows {
		if q.matches(row) {
			out = append(out, row)
		}
	}
	return out
}

// typeKey edits the query with one key while it is being typed. Esc drops
// the search; Enter keeps it, or drops it when empty; Backspace on an empty
// query stops typing.
func (q *helpQuery) typeKey(k tea.KeyMsg) {
	s := k.String()
	switch {
	case k.Type == tea.KeyEsc:
		*q = helpQuery{}
	case k.Type == tea.KeyEnter:
		q.typing = false
	case k.Type == tea.KeyBackspace || s == "backspace" || s == "ctrl+h":
		if q.text == "" {
			q.typing = false
			return
		}
		q.text = dropLastRune(q.text)
	case k.Type == tea.KeySpace:
		q.text += " "
	case k.Type == tea.KeyRunes:
		q.text += string(k.Runes)
	}
}

// key gives a help screen's key to the search when the search wants it: every
// key while typing, "/" to start, and Esc to clear a kept filter. It reports
// whether the key was used, so the screen handles the rest.
func (q *helpQuery) key(k tea.KeyMsg) bool {
	switch {
	case q.typing:
		q.typeKey(k)
	case k.String() == "/":
		q.typing = true
	case k.Type == tea.KeyEsc && q.text != "":
		*q = helpQuery{}
	default:
		return false
	}
	return true
}

// prompt is the query as a help screen shows it, with a cursor while typing.
func (q helpQuery) prompt() string {
	p := "/" + q.text
	if q.typing {
		p += "▏"
	}
	return p
}

// smallHelpFooter is the last line of a one-page help: its usual way out, or
// the search and how to leave it.
func smallHelpFooter(q helpQuery, usual string) string {
	switch {
	case q.typing:
		return "  Search " + q.prompt() + "   Enter: keep   Esc: clear"
	case q.text != "":
		return "  Search " + q.prompt() + "   /: edit   Esc: clear"
	}
	return "  " + usual + "   /: search"
}

// noHelpMatch is the line a search that matches nothing shows, so the page is
// never an empty frame.
const noHelpMatch = "  No keys match the search"
