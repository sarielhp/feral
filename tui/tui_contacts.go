package tui

import (
	"fmt"
	"net/mail"
	"strings"

	"github.com/sarielhp/feral/contacts"
	"github.com/sarielhp/feral/contactsui"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/tui/render"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The contacts picker: a searchable list over the same ranking Tab-completion
// uses -- stored contacts and aliases merged with the history of addresses
// sent to and archived from -- that inserts the chosen people into a
// recipient field. F5 opens it on the compose screen, from the review screen
// (adding to To, Tab switching to Cc or Bcc) or while a recipient field is
// being edited (adding to that field).

const contactPickerLimit = 200

type contactPicker struct {
	// List is the search, results, cursor and marks, shared with
	// `contacts pick` and the contacts manager.
	*contactsui.List
	target composeField
	// editing is set when the picker was opened from a field being edited:
	// choices go into FieldInput and the edit continues.
	editing bool

	book    *contacts.Book
	history *contacts.HistoryBook
}

// openContactPicker opens the picker for the field being edited, or for To
// when opened from the review screen. Subject has no recipients to pick.
func (m *tuiModel) openContactPicker() {
	if m.compose == nil {
		return
	}
	target, editing := fieldTo, false
	switch m.compose.FieldEditing {
	case fieldTo, fieldCc, fieldBcc:
		target, editing = m.compose.FieldEditing, true
	case fieldSubject:
		return
	}
	book, err := contacts.LoadBook(contacts.DefaultPath(m.cfg))
	if err != nil {
		m.err = fmt.Errorf("contacts not loaded: %w", err)
		m.showError = true
	}
	history, _ := contacts.LoadHistory(contacts.DefaultHistoryPath(m.cfg))
	m.contactPick = &contactPicker{
		List:    contactsui.NewList(nil),
		target:  target,
		editing: editing,
		book:    book,
		history: history,
	}
	m.resetComposeSuggestions()
	m.refreshContactPicker()
}

func (m *tuiModel) refreshContactPicker() {
	p := m.contactPick
	var blacklist []string
	if m.cfg != nil {
		blacklist = m.cfg.Policy.Blacklist
	}
	p.SetSearch(contactsui.SuggestSearch(p.book, p.history, blacklist, contactPickerLimit))
}

// kContactPicker handles keys while the picker is open. Letters go to the
// search, so movement is on the arrow and page keys.
func (m *tuiModel) kContactPicker(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := m.contactPick
	switch k.Type {
	case tea.KeyEsc:
		m.contactPick = nil
		return m, nil
	case tea.KeyEnter:
		return m.insertPickedContacts()
	case tea.KeyTab:
		if !p.editing {
			p.target = nextRecipientField(p.target)
		}
		return m, nil
	case tea.KeyF6:
		return m.openAliasBrowser(true, "", "")
	}
	p.HandleKey(k, m.contactPickerRows())
	return m, nil
}

func nextRecipientField(f composeField) composeField {
	switch f {
	case fieldTo:
		return fieldCc
	case fieldCc:
		return fieldBcc
	}
	return fieldTo
}

// pickedRecipients is the marked contacts in the order they were marked, or
// the highlighted one when nothing is marked, as recipient-field entries.
func (p *contactPicker) pickedRecipients() []string {
	var picked []string
	for _, sc := range p.Picked() {
		if r := contactsui.Recipient(sc); r != "" {
			picked = append(picked, r)
		}
	}
	return picked
}

// insertPickedContacts adds the chosen recipients to the target field,
// skipping addresses already there, and closes the picker.
func (m *tuiModel) insertPickedContacts() (tea.Model, tea.Cmd) {
	p := m.contactPick
	m.contactPick = nil
	picked := p.pickedRecipients()
	if len(picked) == 0 || m.compose == nil {
		return m, nil
	}
	if p.editing {
		m.compose.FieldInput = appendRecipients(m.compose.FieldInput, picked)
		return m, nil
	}
	field := m.recipientField(p.target)
	*field = appendRecipients(*field, picked)
	return m, m.saveAndSyncDraftCmd()
}

func (m *tuiModel) recipientField(f composeField) *string {
	switch f {
	case fieldCc:
		return &m.compose.Cc
	case fieldBcc:
		return &m.compose.Bcc
	}
	return &m.compose.To
}

// appendRecipients adds each recipient whose address is not already in the
// comma-separated list. A trailing partial entry the user was typing -- text
// after the last comma that is not an address -- is replaced, since picking
// is how that entry gets finished.
func appendRecipients(list string, picked []string) string {
	entries := email.SplitAddressList(list)
	if n := len(entries); n > 0 && !hasAddress(entries[n-1]) {
		entries = entries[:n-1]
	}
	have := map[string]bool{}
	for _, e := range entries {
		have[addressKey(e)] = true
	}
	// A picked group is several recipients, each deduplicated on its own.
	for _, r := range picked {
		for _, one := range email.SplitAddressList(r) {
			key := addressKey(one)
			if key == "" || have[key] {
				continue
			}
			have[key] = true
			entries = append(entries, one)
		}
	}
	return strings.Join(entries, ", ")
}

func hasAddress(entry string) bool {
	return strings.Contains(entry, "@")
}

func addressKey(entry string) string {
	if a, err := mail.ParseAddress(entry); err == nil {
		return strings.ToLower(a.Address)
	}
	return strings.ToLower(strings.TrimSpace(entry))
}

func (m *tuiModel) contactPickerRows() int {
	return max(5, m.height-12)
}

func fieldLabel(f composeField) string {
	switch f {
	case fieldCc:
		return "Cc"
	case fieldBcc:
		return "Bcc"
	}
	return "To"
}

func renderContactPicker(m *tuiModel) string {
	t := m.theme.Theme()
	p := m.contactPick
	dialogWidth := min(100, max(20, m.width-4))
	if m.width > 0 && dialogWidth > m.width {
		dialogWidth = m.width
	}
	inner := max(10, dialogWidth-4)
	head := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(t.Get(render.ColorHighlightFg)))
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color(t.Get(render.ColorDate)))
	fg := lipgloss.NewStyle().Foreground(lipgloss.Color(t.Get(render.ColorFg)))

	lines := []string{
		head.Render(fmt.Sprintf("CONTACTS  → %s", fieldLabel(p.target))),
		"",
		fmt.Sprintf("Search: %s▏", p.Query),
		"",
	}
	lines = append(lines, contactRows(p, m.contactPickerRows(), inner, head, fg, dim)...)

	help := "type: search  ↑/↓: move  Space: mark  Enter: add  Tab: To/Cc/Bcc  F6: aliases  Esc: cancel"
	if p.editing {
		help = "type: search  ↑/↓: move  Space: mark  Enter: add  F6: aliases  Esc: cancel"
	}
	if n := p.MarkCount(); n > 0 {
		help = fmt.Sprintf("%d marked  │  %s", n, help)
	}
	lines = append(lines, "", dim.Render(markHints(truncateTo(help, inner),
		hint{"Enter: add", "enter"}, hint{"Tab: To/Cc/Bcc", "tab"}, hint{"Esc: cancel", "esc"})))

	dialog := lipgloss.NewStyle().
		Width(dialogWidth).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(t.Get(render.ColorBorder))).
		Padding(1, 2).
		Render(strings.Join(lines, "\n"))
	return lipgloss.NewStyle().Width(m.width).Height(max(10, m.height)).
		Align(lipgloss.Center, lipgloss.Center).Render(dialog)
}

// contactRows renders the visible window of results around the cursor.
func contactRows(p *contactPicker, rows, width int, sel, fg, dim lipgloss.Style) []string {
	if len(p.Results) == 0 {
		return []string{dim.Render("  No matching contacts.")}
	}
	start, end := p.Window(rows)
	var out []string
	for i := start; i < end; i++ {
		sc := p.Results[i]
		mark := "  "
		if p.IsMarked(sc) {
			mark = "✓ "
		}
		row := truncateTo(mark+contactLine(sc), width-2)
		if i == p.Cursor {
			out = append(out, rowZone(frameContacts, i, 0, sel.Render("> "+row)))
			continue
		}
		out = append(out, rowZone(frameContacts, i, 0, fg.Render("  "+row)))
	}
	if len(p.Results) > rows {
		out = append(out, dim.Render(fmt.Sprintf("  %d-%d of %d", start+1, end, len(p.Results))))
	}
	return out
}

func contactLine(sc contacts.ScoredContact) string {
	return contactsui.Line(sc)
}

func truncateTo(s string, width int) string {
	return contactsui.Truncate(s, width)
}
