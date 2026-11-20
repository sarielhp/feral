package contactsui

import (
	"errors"
	"fmt"

	"github.com/sarielhp/feral/contacts"

	tea "github.com/charmbracelet/bubbletea"
)

// ManagerOptions configure a Manager.
type ManagerOptions struct {
	// ReadOnly refuses every change; DryRun checks each change against the
	// book and reports it without saving.
	ReadOnly bool
	DryRun   bool
}

// mode is what the manager's keys act on.
type mode int

const (
	modeList    mode = iota
	modeSearch       // typing goes into the search
	modeDetail       // one contact, every field
	modeForm         // adding or editing
	modeConfirm      // a yes/no question
	modeHelp
)

// bookOp is a change to the book. It returns the ID of the contact to show
// afterwards and a line saying what it did.
type bookOp func(*contacts.Book) (id, done string, err error)

// confirmation is a question waiting for y or n.
type confirmation struct {
	prompt string
	op     bookOp
}

// Manager is the full-screen contacts manager.
type Manager struct {
	store   Store
	opts    ManagerOptions
	book    *contacts.Book
	history *contacts.HistoryBook

	list    *List
	view    view
	sortBy  sortKey
	mode    mode
	back    mode // where Esc returns to from confirm and form
	form    *form
	confirm *confirmation
	// detail is the ID of the contact the detail view shows.
	detail string

	status        string
	width, height int
	st            styles
}

// NewManager loads the book from store and opens the contacts list.
func NewManager(store Store, opts ManagerOptions) (*Manager, error) {
	m := &Manager{store: store, opts: opts, st: newStyles(), width: 80, height: 24}
	if err := m.reload(); err != nil {
		return nil, err
	}
	m.list = NewList(m.searchFor(viewContacts))
	return m, nil
}

// reload reads the book and history again.
func (m *Manager) reload() error {
	book, history, err := m.store.Load()
	if err != nil {
		return err
	}
	m.book, m.history = book, history
	return nil
}

// refresh reruns the list's search over the current book.
func (m *Manager) refresh() {
	m.list.SetSearch(m.searchFor(m.view))
}

// Init starts nothing.
func (m *Manager) Init() tea.Cmd { return nil }

// Update handles keys, the mouse and resizes.
func (m *Manager) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return m, dispatch(msg, &m.width, &m.height, m.mouse, m.key)
}

func (m *Manager) key(k tea.KeyMsg) tea.Cmd {
	if k.Type == tea.KeyCtrlC {
		return tea.Quit
	}
	// Fast typing and pastes arrive as one message of several runes; a
	// command letter among them must act, and what follows "/" must search.
	if k.Type == tea.KeyRunes && len(k.Runes) > 1 && !k.Paste {
		var cmds []tea.Cmd
		for _, r := range k.Runes {
			cmds = append(cmds, m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}))
		}
		return tea.Batch(cmds...)
	}
	switch m.mode {
	case modeSearch:
		m.keySearch(k)
	case modeDetail:
		return m.keyDetail(k)
	case modeForm:
		return m.keyForm(k)
	case modeConfirm:
		m.keyConfirm(k)
	case modeHelp:
		m.mode = modeList
	default:
		return m.keyList(k)
	}
	return nil
}

// keyList is the list's commands; letters are commands here, and / starts
// a search.
func (m *Manager) keyList(k tea.KeyMsg) tea.Cmd {
	m.status = ""
	switch k.Type {
	case tea.KeyEsc:
		if m.list.Query != "" {
			m.list.Query = ""
			m.list.Refresh()
			return nil
		}
		return tea.Quit
	case tea.KeyEnter:
		m.open()
		return nil
	case tea.KeyTab:
		m.switchView((m.view + 1) % viewCount)
		return nil
	case tea.KeyShiftTab:
		m.switchView((m.view + viewCount - 1) % viewCount)
		return nil
	case tea.KeyCtrlR:
		m.reloadAndRefresh("Reloaded")
		return nil
	case tea.KeyRunes:
		return m.keyListRune(k)
	}
	m.list.HandleKey(k, m.rows())
	return nil
}

func (m *Manager) keyListRune(k tea.KeyMsg) tea.Cmd {
	switch string(k.Runes) {
	case "q":
		return tea.Quit
	case "/":
		m.mode = modeSearch
	case "?":
		m.mode = modeHelp
	case "j":
		m.list.Move(1)
	case "k":
		m.list.Move(-1)
	case "g":
		m.list.Cursor = 0
	case "G":
		m.list.Move(len(m.list.Results))
	case "s":
		m.sortBy = (m.sortBy + 1) % sortCount
		m.refresh()
		m.status = "Sorted by " + m.sortBy.String()
	case "a":
		m.startAdd()
	case "N":
		m.startAddGroup()
	case "e":
		m.startEdit()
	case "d":
		m.askRemove()
	case "u":
		m.askUnhide()
	case "r":
		m.askRevert()
	case "m":
		m.askMerge()
	case "p":
		m.startPromote()
	case "x":
		m.list.ClearMarks()
	}
	return nil
}

// keySearch types into the search; Enter keeps it, Esc drops it.
func (m *Manager) keySearch(k tea.KeyMsg) {
	switch k.Type {
	case tea.KeyEnter:
		m.mode = modeList
	case tea.KeyEsc:
		m.mode = modeList
		m.list.Query = ""
		m.list.Refresh()
	case tea.KeyRunes, tea.KeySpace:
		m.list.Type(string(k.Runes))
	default:
		m.list.HandleKey(k, m.rows())
	}
}

func (m *Manager) keyDetail(k tea.KeyMsg) tea.Cmd {
	m.status = ""
	switch {
	case k.Type == tea.KeyEsc || k.String() == "q":
		m.mode = modeList
	case k.Type == tea.KeyEnter || k.String() == "e":
		m.startEdit()
	case k.String() == "d":
		m.askRemove()
	case k.String() == "u":
		m.askUnhide()
	case k.String() == "r":
		m.askRevert()
	}
	return nil
}

func (m *Manager) keyForm(k tea.KeyMsg) tea.Cmd {
	if k.Type == tea.KeyEsc {
		m.form, m.mode = nil, m.back
		m.status = "Not saved"
		return nil
	}
	if !m.form.key(k) {
		return nil
	}
	if m.apply(m.form.op()) {
		m.form = nil
		m.mode = modeDetail
		m.leaveMissingDetail()
	}
	return nil
}

func (m *Manager) keyConfirm(k tea.KeyMsg) {
	c := m.confirm
	m.confirm = nil
	if k.String() == "y" || k.String() == "Y" {
		m.apply(c.op)
	} else {
		m.status = "Cancelled"
	}
	m.mode = m.back
	m.leaveMissingDetail()
}

// leaveMissingDetail returns to the list when the contact the detail view
// would show is gone -- deleted, hidden, or only added in a dry run.
func (m *Manager) leaveMissingDetail() {
	if _, ok := m.detailContact(); m.mode == modeDetail && !ok {
		m.mode = modeList
	}
}

// open shows the contact under the cursor, or, in the harvested list,
// starts making it a contact.
func (m *Manager) open() {
	if m.view == viewHistory {
		m.startPromote()
		return
	}
	sc, ok := m.list.Current()
	if !ok {
		return
	}
	m.detail, m.mode = sc.ID, modeDetail
}

func (m *Manager) switchView(v view) {
	m.view = v
	m.list.Query, m.list.Cursor = "", 0
	m.list.ClearMarks()
	m.refresh()
}

// selected is the contact a command acts on: the detail view's, else the
// one under the cursor.
func (m *Manager) selected() (contacts.Contact, bool) {
	if m.mode == modeDetail {
		return m.detailContact()
	}
	sc, ok := m.list.Current()
	return sc.Contact, ok
}

// detailContact is the contact the detail view shows, looked up afresh.
func (m *Manager) detailContact() (contacts.Contact, bool) {
	if c := contacts.FindByID(m.book, m.detail); c != nil {
		return *c, true
	}
	for _, c := range m.book.Hidden {
		if c.ID == m.detail {
			return c, true
		}
	}
	return contacts.Contact{}, false
}

// apply runs op against the book on disk, under its lock, and shows the
// result; it reports whether op succeeded. Read-only refuses; a dry run runs
// op on a copy and saves nothing.
func (m *Manager) apply(op bookOp) bool {
	if m.opts.ReadOnly && !m.opts.DryRun {
		m.status = "Read-only: nothing is saved"
		return false
	}
	var id, done string
	run := func(b *contacts.Book) error {
		var err error
		id, done, err = op(b)
		return err
	}
	if m.opts.DryRun {
		book, _, err := m.store.Load()
		if err == nil {
			err = run(book)
		}
		return m.report(err, "Dry run, not saved: "+done, id)
	}
	book, err := m.store.Update(run)
	if err == nil {
		m.book = book
	}
	return m.report(err, done, id)
}

// report shows what apply did and moves the list to the affected contact.
func (m *Manager) report(err error, done, id string) bool {
	if err != nil {
		var taken *contacts.AliasTakenError
		if errors.As(err, &taken) {
			m.status = fmt.Sprintf("Not saved: %v", err)
			return false
		}
		m.status = "Not saved: " + err.Error()
		return false
	}
	m.status = done
	m.detail = id
	m.list.ClearMarks()
	m.refresh()
	m.moveTo(id)
	return true
}

// moveTo puts the cursor on the contact with id, when it is listed.
func (m *Manager) moveTo(id string) {
	for i, sc := range m.list.Results {
		if sc.ID == id {
			m.list.Cursor = i
			return
		}
	}
}

func (m *Manager) reloadAndRefresh(done string) {
	if err := m.reload(); err != nil {
		m.status = "Reload failed: " + err.Error()
		return
	}
	m.refresh()
	m.status = done
}

// rows is how many list rows fit on screen.
func (m *Manager) rows() int {
	return max(3, m.height-managerHeaderLines-3)
}
