package contactsui

import (
	"fmt"

	"github.com/sarielhp/feral/contacts"
)

// openForm shows f, returning to the current mode when it is left.
func (m *Manager) openForm(f *form) {
	m.back = m.mode
	if m.back == modeForm || m.back == modeConfirm {
		m.back = modeList
	}
	m.form, m.mode = f, modeForm
}

// ask puts a yes/no question; y runs op.
func (m *Manager) ask(prompt string, op bookOp) {
	m.back = m.mode
	m.confirm, m.mode = &confirmation{prompt: prompt, op: op}, modeConfirm
}

func (m *Manager) startAdd() {
	m.openForm(newForm(formAdd, contacts.Contact{}))
}

// startAddGroup opens a new group whose members are the marked contacts.
func (m *Manager) startAddGroup() {
	var members []string
	for _, sc := range m.list.Marks() {
		members = append(members, sc.Emails...)
	}
	m.openForm(newForm(formAddGroup, contacts.Contact{Emails: members}))
}

// startPromote opens a new contact for the harvested address under the
// cursor.
func (m *Manager) startPromote() {
	sc, ok := m.list.Current()
	if m.view != viewHistory || !ok {
		m.status = "p makes a contact from the Harvested list (Tab to it)"
		return
	}
	m.openForm(newForm(formAdd, contacts.Contact{Name: sc.Name, Emails: sc.Emails}))
}

func (m *Manager) startEdit() {
	c, ok := m.selected()
	switch {
	case !ok:
		return
	case m.view == viewHistory:
		m.startPromote()
		return
	case m.view == viewHidden:
		m.status = "Unhide it first (u)"
		return
	}
	m.detail = c.ID
	m.openForm(newForm(formEdit, c))
}

// askRemove deletes a manual contact or hides an imported one.
func (m *Manager) askRemove() {
	c, ok := m.selected()
	if !ok || m.view != viewContacts {
		return
	}
	verb := "Delete"
	if c.IsImported() {
		verb = "Hide"
	}
	id := c.ID
	m.ask(fmt.Sprintf("%s %s? A hidden contact stays out of lists after every sync. (y/n)", verb, c.Label()),
		func(b *contacts.Book) (string, string, error) {
			hidden, err := contacts.Remove(b, id)
			if err != nil {
				return "", "", err
			}
			if hidden {
				return "", "Hid " + c.Label() + " (Tab to Hidden to bring it back)", nil
			}
			return "", "Deleted " + c.Label(), nil
		})
}

func (m *Manager) askUnhide() {
	c, ok := m.selected()
	if !ok || m.view != viewHidden {
		m.status = "u brings back a contact from the Hidden list"
		return
	}
	id := c.ID
	m.ask(fmt.Sprintf("Show %s again? (y/n)", c.Label()), func(b *contacts.Book) (string, string, error) {
		if _, err := contacts.Unhide(b, id); err != nil {
			return "", "", err
		}
		return "", "Restored " + c.Label(), nil
	})
}

func (m *Manager) askRevert() {
	c, ok := m.selected()
	if !ok || !c.IsEdited() {
		m.status = "Only an imported contact you edited can be reverted"
		return
	}
	id := c.ID
	m.ask(fmt.Sprintf("Drop your edits to %s and show what %s says? Aliases and notes stay. (y/n)", c.Label(), c.Source),
		func(b *contacts.Book) (string, string, error) {
			if _, err := contacts.Revert(b, id); err != nil {
				return "", "", err
			}
			return id, "Reverted " + c.Label() + " to its source", nil
		})
}

// askMerge folds the one marked contact into the one under the cursor.
func (m *Manager) askMerge() {
	marks := m.list.Marks()
	keep, ok := m.list.Current()
	if m.view != viewContacts || len(marks) != 1 || !ok || marks[0].ID == keep.ID {
		m.status = "To merge: mark one contact (Space), move to the one to keep, press m"
		return
	}
	drop := marks[0]
	keepID, dropID := keep.ID, drop.ID
	m.ask(fmt.Sprintf("Merge %s <%s> into %s <%s>? (y/n)", drop.Label(), drop.PrimaryEmail(), keep.Label(), keep.PrimaryEmail()),
		func(b *contacts.Book) (string, string, error) {
			c, err := contacts.Merge(b, keepID, dropID)
			if err != nil {
				return "", "", err
			}
			return keepID, fmt.Sprintf("Merged into %s (%d addresses)", c.Label(), len(c.Emails)), nil
		})
}
