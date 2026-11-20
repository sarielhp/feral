package contactsui

import (
	"fmt"
	"strings"

	"github.com/sarielhp/feral/contacts"

	tea "github.com/charmbracelet/bubbletea"
)

// formKind is what saving a form does.
type formKind int

const (
	formEdit     formKind = iota // change an existing contact
	formAdd                      // a new person
	formAddGroup                 // a new group alias
)

// The fields of the form, in order.
const (
	fieldName = iota
	fieldEmails
	fieldAliases
	fieldGroups
	fieldNotes
	fieldCount
)

// form edits one contact: a line per field, lists comma-separated.
type form struct {
	kind   formKind
	id     string // the contact edited, for formEdit
	group  bool
	inputs [fieldCount]lineInput
	focus  int
	// source is what the source says, shown under a field the user changed.
	source *contacts.Origin
}

func newForm(kind formKind, c contacts.Contact) *form {
	f := &form{kind: kind, id: c.ID, group: kind == formAddGroup || c.IsGroup()}
	values := [fieldCount]string{
		c.Name,
		strings.Join(c.Emails, ", "),
		strings.Join(c.Aliases, ", "),
		strings.Join(c.Groups, ", "),
		c.Notes,
	}
	for i := range f.inputs {
		f.inputs[i] = newLineInput(values[i])
	}
	if c.IsImported() {
		src := c.SourceView()
		f.source = &src
	}
	f.setFocus(fieldName)
	return f
}

func (f *form) label(i int) string {
	labels := [fieldCount]string{"Name", "Emails", "Aliases", "Groups", "Notes"}
	if f.group && i == fieldEmails {
		return "Members"
	}
	return labels[i]
}

func (f *form) title() string {
	switch f.kind {
	case formAdd:
		return "NEW CONTACT"
	case formAddGroup:
		return "NEW GROUP"
	}
	return "EDIT CONTACT"
}

func (f *form) setFocus(i int) {
	f.focus = (i + fieldCount) % fieldCount
}

// key handles a key in the form and reports whether to save it.
func (f *form) key(k tea.KeyMsg) bool {
	switch k.Type {
	case tea.KeyTab, tea.KeyDown:
		f.setFocus(f.focus + 1)
		return false
	case tea.KeyShiftTab, tea.KeyUp:
		f.setFocus(f.focus - 1)
		return false
	case tea.KeyCtrlS:
		return true
	case tea.KeyEnter:
		if f.focus == fieldCount-1 {
			return true
		}
		f.setFocus(f.focus + 1)
		return false
	}
	f.inputs[f.focus].key(k)
	return false
}

func (f *form) value(i int) string {
	return strings.TrimSpace(f.inputs[i].Value())
}

// list is the comma-separated entries of field i.
func (f *form) list(i int) []string {
	var out []string
	for part := range strings.SplitSeq(f.value(i), ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func (f *form) change() contacts.Change {
	return contacts.Change{
		Name:    f.value(fieldName),
		Emails:  f.list(fieldEmails),
		Aliases: f.list(fieldAliases),
		Groups:  f.list(fieldGroups),
		Notes:   f.value(fieldNotes),
	}
}

// op is what saving the form does to the book, and what it reports.
func (f *form) op() bookOp {
	ch := f.change()
	switch f.kind {
	case formAdd:
		return func(b *contacts.Book) (string, string, error) {
			c, err := contacts.AddContact(b, ch.Name, ch.Emails)
			if err != nil {
				return "", "", err
			}
			_, err = contacts.Edit(b, c.ID, ch)
			return c.ID, "Added " + c.Label(), err
		}
	case formAddGroup:
		return func(b *contacts.Book) (string, string, error) {
			c, err := contacts.AddGroup(b, ch.Name, ch.Emails)
			if err != nil {
				return "", "", err
			}
			ch.Aliases = append(c.Aliases, ch.Aliases...)
			_, err = contacts.Edit(b, c.ID, ch)
			return c.ID, fmt.Sprintf("Added group %s (%d members)", c.Label(), len(c.Emails)), err
		}
	}
	id := f.id
	return func(b *contacts.Book) (string, string, error) {
		c, err := contacts.Edit(b, id, ch)
		if err != nil {
			return "", "", err
		}
		return id, "Saved " + c.Label(), nil
	}
}

// sourceHint is the source's value for field i when the user's differs,
// for the form to show beneath it.
func (f *form) sourceHint(i int) string {
	if f.source == nil {
		return ""
	}
	var src string
	switch i {
	case fieldName:
		src = f.source.Name
	case fieldEmails:
		src = strings.Join(f.source.Emails, ", ")
	case fieldGroups:
		src = strings.Join(f.source.Groups, ", ")
	default:
		return ""
	}
	if src == f.value(i) {
		return ""
	}
	return src
}
