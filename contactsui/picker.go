package contactsui

import (
	"fmt"
	"strings"

	"github.com/sarielhp/feral/contacts"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// PickerOptions configure a Picker.
type PickerOptions struct {
	Title     string
	Query     string   // the search to start with
	Preselect []string // addresses marked from the start
	Single    bool     // choose one contact; no marking
	Blacklist []string // addresses never offered
	Limit     int      // most results shown; 0 means 500
}

// PickedMsg ends an embedded Picker with the chosen contacts. A program
// embedding the Picker receives it from Update's command; standing alone,
// the Picker quits instead and Result reports the choice.
type PickedMsg struct{ Contacts []contacts.ScoredContact }

// CancelledMsg ends an embedded Picker with nothing chosen.
type CancelledMsg struct{}

// Picker is a full-screen contacts picker: search, mark, Enter to choose.
type Picker struct {
	List          *List
	opts          PickerOptions
	width, height int
	st            styles
	embedded      bool

	done      bool
	cancelled bool
	picked    []contacts.ScoredContact
}

// NewPicker builds a picker over book and history.
func NewPicker(book *contacts.Book, history *contacts.HistoryBook, opts PickerOptions) *Picker {
	if opts.Limit <= 0 {
		opts.Limit = 500
	}
	l := NewList(SuggestSearch(book, history, opts.Blacklist, opts.Limit))
	l.Single = opts.Single
	for _, sc := range preselected(book, opts.Preselect) {
		l.Mark(sc)
	}
	if opts.Query != "" {
		l.Type(opts.Query)
	}
	return &Picker{List: l, opts: opts, st: newStyles(), width: 80, height: 24}
}

// Embedded makes the picker report its end as a PickedMsg or CancelledMsg
// instead of quitting the program.
func (p *Picker) Embedded() *Picker {
	p.embedded = true
	return p
}

// preselected is the contact for each address: the book's, or a bare one.
func preselected(book *contacts.Book, addrs []string) []contacts.ScoredContact {
	var out []contacts.ScoredContact
	for _, entry := range addrs {
		if strings.TrimSpace(entry) == "" {
			continue
		}
		name, addr := splitAddress(entry)
		if c := contacts.FindByAddress(book, addr); c != nil {
			out = append(out, contacts.ScoredContact{Contact: *c})
			continue
		}
		out = append(out, contacts.ScoredContact{Contact: contacts.Contact{Name: name, Emails: []string{addr}}})
	}
	return out
}

// Result is what the picker ended with: the chosen contacts, or cancelled.
func (p *Picker) Result() ([]contacts.ScoredContact, bool) {
	return p.picked, p.cancelled
}

// Init starts nothing.
func (p *Picker) Init() tea.Cmd { return nil }

// Update handles keys, the mouse and resizes.
func (p *Picker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return p, dispatch(msg, &p.width, &p.height, p.mouse, p.key)
}

// dispatch hands msg to the screen: a resize sets width and height, and
// the mouse and keys go to their handlers. The Picker and the Manager share it.
func dispatch(msg tea.Msg, width, height *int, mouse func(tea.MouseMsg), key func(tea.KeyMsg) tea.Cmd) tea.Cmd {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		*width, *height = msg.Width, msg.Height
	case tea.MouseMsg:
		mouse(msg)
	case tea.KeyMsg:
		return key(msg)
	}
	return nil
}

func (p *Picker) key(k tea.KeyMsg) tea.Cmd {
	switch k.Type {
	case tea.KeyEsc, tea.KeyCtrlC:
		return p.finish(nil)
	case tea.KeyEnter:
		return p.finish(p.List.Picked())
	}
	p.List.HandleKey(k, p.rows())
	return nil
}

// finish ends the picker; choosing nothing is cancelling.
func (p *Picker) finish(picked []contacts.ScoredContact) tea.Cmd {
	p.done, p.picked, p.cancelled = true, picked, len(picked) == 0
	if p.embedded {
		if p.cancelled {
			return func() tea.Msg { return CancelledMsg{} }
		}
		return func() tea.Msg { return PickedMsg{Contacts: picked} }
	}
	return tea.Quit
}

// pickerHeaderLines is how many lines come before the first row.
const pickerHeaderLines = 4

func (p *Picker) mouse(msg tea.MouseMsg) {
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		p.List.Move(-3)
		return
	case tea.MouseButtonWheelDown:
		p.List.Move(3)
		return
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return
	}
	start, end := p.List.Window(p.rows())
	i := start + msg.Y - pickerHeaderLines
	if i < start || i >= end {
		return
	}
	if i == p.List.Cursor {
		p.List.ToggleMark()
		return
	}
	p.List.Cursor = i
}

func (p *Picker) rows() int {
	return max(3, p.height-pickerHeaderLines-3)
}

// View draws the picker.
func (p *Picker) View() string {
	if p.done {
		return ""
	}
	title := p.opts.Title
	if title == "" {
		title = "Pick contacts"
	}
	width := max(20, p.width)
	lines := []string{
		p.st.title.Render(title),
		"",
		fmt.Sprintf("Search: %s▏", p.List.Query),
		"",
	}
	lines = append(lines, renderRows(p.List, p.rows(), width, p.st)...)
	lines = append(lines, "", p.st.dim.Render(Truncate(p.help(), width)))
	return lipgloss.NewStyle().MaxWidth(width).Render(strings.Join(lines, "\n"))
}

func (p *Picker) help() string {
	if p.List.Single {
		return "type: search  ↑/↓ PgUp/PgDn: move  Enter: choose  Esc: cancel"
	}
	help := "type: search  ↑/↓ PgUp/PgDn: move  Space: mark  Enter: choose  Esc: cancel"
	if n := p.List.MarkCount(); n > 0 {
		help = fmt.Sprintf("%d marked  │  %s", n, help)
	}
	return help
}

func fmtRange(start, end, total int) string {
	return fmt.Sprintf("  %d-%d of %d", start+1, end, total)
}
