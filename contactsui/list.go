package contactsui

import (
	"fmt"
	"strings"

	"github.com/sarielhp/feral/contacts"
	"github.com/sarielhp/feral/email"

	tea "github.com/charmbracelet/bubbletea"
)

// List is the state every contacts list shares -- the compose screen's F5
// picker, `contacts pick` and the manager: a search query, the results it
// gives, a cursor and the contacts marked so far. A mark outlives the search
// that showed it, so picking from several searches gathers them all.
type List struct {
	Query   string
	Results []contacts.ScoredContact
	Cursor  int
	// Single turns marking off: the contact under the cursor is the choice.
	Single bool

	search func(query string) []contacts.ScoredContact
	marks  []contacts.ScoredContact
	marked map[string]bool
}

// NewList returns a list whose results come from search, already run for
// the empty query.
func NewList(search func(query string) []contacts.ScoredContact) *List {
	l := &List{search: search, marked: map[string]bool{}}
	l.Refresh()
	return l
}

// Key identifies a contact across searches: a group by its ID, a person by
// address.
func Key(sc contacts.ScoredContact) string {
	if sc.IsGroup() {
		return sc.ID
	}
	return strings.ToLower(sc.PrimaryEmail())
}

// Refresh reruns the search, keeping the cursor in range.
func (l *List) Refresh() {
	if l.search != nil {
		l.Results = l.search(l.Query)
	}
	l.Cursor = min(max(0, l.Cursor), max(0, len(l.Results)-1))
}

// SetSearch replaces where results come from and reruns it.
func (l *List) SetSearch(search func(query string) []contacts.ScoredContact) {
	l.search = search
	l.Refresh()
}

// Current is the contact under the cursor.
func (l *List) Current() (contacts.ScoredContact, bool) {
	if l.Cursor < 0 || l.Cursor >= len(l.Results) {
		return contacts.ScoredContact{}, false
	}
	return l.Results[l.Cursor], true
}

// IsMarked reports whether sc is marked.
func (l *List) IsMarked(sc contacts.ScoredContact) bool {
	return l.marked[Key(sc)]
}

// MarkCount is how many contacts are marked.
func (l *List) MarkCount() int {
	return len(l.marks)
}

// Mark marks sc, unless it already is.
func (l *List) Mark(sc contacts.ScoredContact) {
	k := Key(sc)
	if k == "" || l.marked[k] {
		return
	}
	l.marked[k] = true
	l.marks = append(l.marks, sc)
}

// ClearMarks unmarks everything.
func (l *List) ClearMarks() {
	l.marks, l.marked = nil, map[string]bool{}
}

// Marks is the marked contacts in the order they were marked.
func (l *List) Marks() []contacts.ScoredContact {
	return l.marks
}

// ToggleMark marks or unmarks the contact under the cursor and moves on.
func (l *List) ToggleMark() {
	sc, ok := l.Current()
	if !ok || l.Single {
		return
	}
	k := Key(sc)
	if k == "" {
		return
	}
	if l.marked[k] {
		delete(l.marked, k)
		l.marks = removeKey(l.marks, k)
	} else {
		l.Mark(sc)
	}
	l.Cursor = min(len(l.Results)-1, l.Cursor+1)
}

func removeKey(list []contacts.ScoredContact, k string) []contacts.ScoredContact {
	out := list[:0]
	for _, sc := range list {
		if Key(sc) != k {
			out = append(out, sc)
		}
	}
	return out
}

// Picked is the marked contacts, or the one under the cursor when none are.
func (l *List) Picked() []contacts.ScoredContact {
	if len(l.marks) > 0 && !l.Single {
		return append([]contacts.ScoredContact(nil), l.marks...)
	}
	if sc, ok := l.Current(); ok {
		return []contacts.ScoredContact{sc}
	}
	return nil
}

// Move moves the cursor by delta, within the results.
func (l *List) Move(delta int) {
	l.Cursor = min(max(0, l.Cursor+delta), max(0, len(l.Results)-1))
}

// Type adds text to the query and searches again from the top.
func (l *List) Type(s string) {
	l.Query += s
	l.Cursor = 0
	l.Refresh()
}

// Backspace removes the last character of the query.
func (l *List) Backspace() {
	if l.Query == "" {
		return
	}
	r := []rune(l.Query)
	l.Query = string(r[:len(r)-1])
	l.Refresh()
}

// HandleKey applies the keys every list understands -- movement, marking
// with Space, and typing into the search -- and reports whether k was one.
// page is how many rows a PgUp or PgDn moves.
func (l *List) HandleKey(k tea.KeyMsg, page int) bool {
	switch k.Type {
	case tea.KeyUp:
		l.Move(-1)
	case tea.KeyDown:
		l.Move(1)
	case tea.KeyPgUp:
		l.Move(-page)
	case tea.KeyPgDown:
		l.Move(page)
	case tea.KeyHome:
		l.Cursor = 0
	case tea.KeyEnd:
		l.Move(len(l.Results))
	case tea.KeySpace:
		l.ToggleMark()
	case tea.KeyBackspace:
		l.Backspace()
	case tea.KeyRunes:
		l.Type(string(k.Runes))
	default:
		return false
	}
	return true
}

// Window is the range of results to show in rows lines, keeping the cursor
// on screen.
func (l *List) Window(rows int) (start, end int) {
	rows = max(1, rows)
	if l.Cursor >= rows {
		start = l.Cursor - rows + 1
	}
	return start, min(len(l.Results), start+rows)
}

// Recipient is how sc goes into an address field: "Name <addr>", or a
// group's members separated by commas.
func Recipient(sc contacts.ScoredContact) string {
	if sc.IsGroup() {
		return strings.Join(sc.Emails, ", ")
	}
	return email.FormatAddress(sc.Name, sc.PrimaryEmail())
}

// Line is the one-line summary of sc a list shows.
func Line(sc contacts.ScoredContact) string {
	name := sc.Name
	if name == "" {
		name = sc.DisplayAlias()
	}
	addr := sc.PrimaryEmail()
	if len(sc.Emails) > 1 && sc.IsGroup() {
		addr = fmt.Sprintf("group of %d", len(sc.Emails))
	}
	extra := ""
	if sc.Frequency > 0 {
		extra = fmt.Sprintf("  (%d×)", sc.Frequency)
	}
	return fmt.Sprintf("%-28s %s%s", Truncate(name, 28), addr, extra)
}

// Truncate shortens s to width runes, ending it with an ellipsis.
func Truncate(s string, width int) string {
	r := []rune(s)
	if width <= 1 || len(r) <= width {
		return s
	}
	return string(r[:width-1]) + "…"
}
