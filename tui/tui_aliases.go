package tui

import (
	"fmt"
	"net/mail"
	"slices"
	"sort"
	"strings"
	"unicode"

	"github.com/mattn/go-runewidth"
	"github.com/sarielhp/feral/contacts"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/tui/render"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The aliases browser: the saved contacts book — nicknames for email
// addresses, the same record Tab completion and the F5 picker rank — as a
// searchable overlay. F6 opens it from the index, the reader, the compose
// screen and the picker over it. Ctrl+n starts a new alias: it walks from an
// address that already exists — a contact in the book, a harvested history
// entry, or, in the reader, the sender of the open message — to a nickname,
// which is saved into contacts.json for every account. Enter opens one
// entry in a window of its own, with the actions on the person: writing to
// them, or reading their latest mail.
//
// An alias is a fact the user authored, kept in a local file, so read-only
// mode records it: read-only forbids touching the server and recording what
// did not happen, and neither applies. A dry run reports instead of writing.

const aliasBrowserLimit = 200

// aliasLastEmailsLimit is how many of a person's newest messages the entry
// window asks the server for.
const aliasLastEmailsLimit = 10

type aliasBrowser struct {
	// The list stage: browsing and searching what is saved.
	query   string
	results []contacts.Contact
	cursor  int
	// book is nil when contacts.json could not be read; adding is then
	// refused, so a damaged file is never overwritten.
	book *contacts.Book

	// Adding an alias runs in two stages: picking the address, then naming it.
	adding     bool
	addQuery   string
	addResults []contacts.ScoredContact
	addCursor  int
	picked     contacts.Contact
	nickStage  bool
	nickname   string
	// renaming is the alias the naming stage replaces: the picked contact's
	// own alias, edited rather than joined by a second one. Empty adds.
	renaming string
	// history is the harvested address history, read once when the add
	// flow starts rather than on every key.
	history *contacts.HistoryBook

	// freshAddr is the sender of the message on screen, which leads the
	// candidate list when the new-alias flow starts from a message.
	freshAddr string
	freshName string

	// fromPicker returns to the open contacts picker on close; Enter then
	// inserts the highlighted alias as a recipient.
	fromPicker bool

	// quick is the one-step flow the a key starts: naming the sender of the
	// message on screen. Esc and a successful save close the overlay.
	quick bool

	// detail is the entry Enter opened from the list: a window over one
	// contact with the actions on the person. A copy, so refreshing the
	// list under it cannot change what the window shows.
	detail *contacts.Contact

	status string
}

// openAliasBrowser opens the aliases overlay over the current screen.
// fromPicker returns to the open contacts picker on close; seedAddr, when
// set, is the sender of the open message, offered first in the new-alias flow.
func (m *tuiModel) openAliasBrowser(fromPicker bool, seedAddr, seedName string) (tea.Model, tea.Cmd) {
	book, err := contacts.LoadBook(contacts.DefaultPath(m.cfg))
	m.aliasView = &aliasBrowser{
		book:       book,
		fromPicker: fromPicker,
		freshAddr:  seedAddr,
		freshName:  seedName,
	}
	if err != nil {
		m.aliasView.status = fmt.Sprintf("Contacts not loaded: %v", err)
	}
	m.refreshAliasBrowser()
	return m, nil
}

// aliasCurrentSender is the a key: the alias overlay opened straight on the
// naming stage for the sender of the message on screen, so an alias is a key,
// a name and Enter. An address already in the book keeps its saved name.
func (m *tuiModel) aliasCurrentSender() (tea.Model, tea.Cmd) {
	if len(m.emails) == 0 {
		// An empty list — often an account that failed to log in — has no
		// sender; a opens the account switcher there, as it always has.
		m.openAccountOverlay()
		return m, nil
	}
	addr, name := m.currentSender()
	if addr == "" {
		m.saveStatus = "No message to alias the sender of"
		return m, nil
	}
	m.openAliasBrowser(false, addr, name)
	b := m.aliasView
	b.quick = true
	m.startAddAlias()
	if !b.adding {
		return m, nil // the book could not be read; status says so
	}
	want := parsedAddress(addr)
	for i, sc := range b.addResults {
		if strings.EqualFold(parsedAddress(sc.PrimaryEmail()), want) {
			b.addCursor = i
			m.pickAliasAddress()
			return m, nil
		}
	}
	m.beginAliasNaming(aliasSenderContact(*b))
	return m, nil
}

// currentSender is the open message's sender address and display name, or
// empty strings when there is nothing open.
func (m *tuiModel) currentSender() (string, string) {
	id := m.currentEmailID()
	if id == "" {
		return "", ""
	}
	for _, em := range m.emails {
		if em.ID == id {
			return senderOf(em)
		}
	}
	for _, em := range m.rawEmails {
		if em.ID == id {
			return senderOf(em)
		}
	}
	return "", ""
}

// senderOf is a message's address, and the display name its raw header gave
// when it has one.
func senderOf(e email.Email) (string, string) {
	name := ""
	if parsed, err := mail.ParseAddress(e.FromRaw); err == nil {
		name = parsed.Name
	}
	return e.FromEmail, name
}

// refreshAliasBrowser re-filters and re-sorts the visible aliases.
func (m *tuiModel) refreshAliasBrowser() {
	b := m.aliasView
	// Search hands back the book's own slice for an empty query; sorting
	// that in place would reorder the book.
	b.results = slices.Clone(contacts.Search(b.book, b.query))
	sortAliasContacts(b.results)
	if b.cursor >= len(b.results) {
		b.cursor = max(0, len(b.results)-1)
	}
}

// sortAliasContacts orders contacts for reading: the ones carrying an alias
// first, each side alphabetical.
func sortAliasContacts(cs []contacts.Contact) {
	sort.SliceStable(cs, func(i, j int) bool {
		oi, oj := aliasOrderOf(cs[i]), aliasOrderOf(cs[j])
		if oi.aliased != oj.aliased {
			return oi.aliased
		}
		return oi.name < oj.name
	})
}

// aliasOrderOf is a contact's sort key.
type aliasOrder struct {
	aliased bool
	name    string
}

func aliasOrderOf(c contacts.Contact) aliasOrder {
	name := c.DisplayAlias()
	if name == "" {
		name = c.Name
	}
	return aliasOrder{aliased: len(c.AllAliases()) > 0, name: strings.ToLower(name)}
}

// startAddAlias begins the two-stage new-alias flow, refused when the book
// could not be read, since saving would replace the damaged file.
func (m *tuiModel) startAddAlias() (tea.Model, tea.Cmd) {
	b := m.aliasView
	if b.book == nil {
		b.status = "Cannot add aliases: contacts.json could not be read"
		return m, nil
	}
	b.adding, b.nickStage = true, false
	b.addQuery, b.addCursor, b.nickname, b.status = "", 0, "", ""
	b.renaming = ""
	b.history, _ = contacts.LoadHistory(contacts.DefaultHistoryPath(m.cfg))
	m.refreshAliasCandidates()
	return m, nil
}

// refreshAliasCandidates re-scores and re-filters the address list the new
// alias is being attached to: contacts and harvested history, ranked the way
// the picker ranks, without group aliases. The sender of the message the
// browser was opened from leads the list when it fits the query.
func (m *tuiModel) refreshAliasCandidates() {
	b := m.aliasView
	var blacklist []string
	if m.cfg != nil {
		blacklist = m.cfg.Policy.Blacklist
	}
	scored := contacts.Suggest(b.book, b.history, blacklist, b.addQuery, aliasBrowserLimit)
	filtered := make([]contacts.ScoredContact, 0, len(scored)+1)
	for _, sc := range scored {
		if !sc.IsGroup() {
			filtered = append(filtered, sc)
		}
	}
	b.addResults = prependAliasSender(*b, filtered)
	b.addCursor = max(0, min(b.addCursor, len(b.addResults)-1))
}

// prependAliasSender puts the message's sender first, when what the flow was
// seeded with matches the candidate query and is not already in candidates.
func prependAliasSender(b aliasBrowser, candidates []contacts.ScoredContact) []contacts.ScoredContact {
	if b.freshAddr == "" || !aliasSenderMatches(b) {
		return candidates
	}
	for _, sc := range candidates {
		if strings.EqualFold(sc.PrimaryEmail(), b.freshAddr) {
			return candidates
		}
	}
	sender := contacts.ScoredContact{Contact: aliasSenderContact(b)}
	return append([]contacts.ScoredContact{sender}, candidates...)
}

// aliasSenderMatches reports whether the flow's seeded sender fits the query.
func aliasSenderMatches(b aliasBrowser) bool {
	query := strings.ToLower(strings.TrimSpace(b.addQuery))
	if query == "" {
		return true
	}
	target := strings.ToLower(b.freshAddr + " " + b.freshName)
	return strings.Contains(target, query)
}

// aliasSenderContact is the seeded sender as a candidate: the address and the
// name the message showed, marked so the list can say where it came from.
func aliasSenderContact(b aliasBrowser) contacts.Contact {
	return contacts.Contact{
		ID:     "alias:sender",
		Name:   strings.TrimSpace(b.freshName), // already parsed by currentSender
		Emails: []string{strings.ToLower(parsedAddress(b.freshAddr))},
		Source: "this message",
	}
}

// parsedAddress returns addr parsed through a bare "Name <addr>" form, or the
// trimmed input when it is not one.
func parsedAddress(addr string) string {
	parsed, err := mail.ParseAddress(strings.TrimSpace(addr))
	if err != nil {
		return strings.TrimSpace(addr)
	}
	return parsed.Address
}

// pickAliasAddress promotes the highlighted candidate to the naming stage.
func (m *tuiModel) pickAliasAddress() {
	b := m.aliasView
	if b.addCursor < 0 || b.addCursor >= len(b.addResults) {
		return
	}
	m.beginAliasNaming(b.addResults[b.addCursor].Contact)
}

// beginAliasNaming opens the naming stage for c. When the book's contact for
// that address already has an alias of the user's, the stage edits it --
// starting from it, and saving replaces it -- so naming someone twice renames
// instead of piling up aliases; otherwise it starts from a suggestion.
func (m *tuiModel) beginAliasNaming(c contacts.Contact) {
	b := m.aliasView
	b.picked, b.nickStage, b.renaming, b.status = c, true, "", ""
	if held := contacts.FindByAddress(b.book, c.PrimaryEmail()); held != nil && len(held.Aliases) > 0 {
		b.renaming = held.Aliases[0]
		b.nickname = b.renaming
		return
	}
	b.nickname = suggestAliasNickname(c)
}

// editEntryAlias is e in an entry's window: the naming stage over that
// person, editing their alias, or adding one when they have none. A group's
// name comes from its source, so it is not edited here.
func (m *tuiModel) editEntryAlias() (tea.Model, tea.Cmd) {
	b := m.aliasView
	switch {
	case b.book == nil:
		b.status = "Cannot edit aliases: contacts.json could not be read"
	case b.detail.IsGroup():
		b.status = "A group's name comes from its source; edit it there."
	case b.detail.PrimaryEmail() == "":
		b.status = "This entry has no address to name."
	default:
		m.beginAliasNaming(*b.detail)
	}
	return m, nil
}

// suggestAliasNickname is where the naming stage starts from: the display
// name collapsed to alias tail, else the address's local part.
func suggestAliasNickname(c contacts.Contact) string {
	name := strings.TrimSpace(c.Name)
	if name == "" {
		if local, _, found := strings.Cut(c.PrimaryEmail(), "@"); found {
			return local
		}
		return ""
	}
	return strings.Map(func(r rune) rune {
		if r == ' ' || r == '_' || r == '.' {
			return '-'
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' {
			return unicode.ToLower(r)
		}
		return -1
	}, name)
}

// commitAliasNickname saves the nickname for the picked address into the
// contacts book and back to contacts.json.
func (m *tuiModel) commitAliasNickname() (tea.Model, tea.Cmd) {
	b := m.aliasView
	nick := strings.TrimSpace(b.nickname)
	if nick == "" && b.renaming == "" {
		b.status = "Type a name for the alias first"
		return m, nil
	}
	if nick == b.renaming {
		b.status = fmt.Sprintf("Alias %q unchanged", nick)
		return m.leaveAliasNaming()
	}
	addr := strings.ToLower(parsedAddress(b.picked.PrimaryEmail()))
	var isNew bool
	setAlias := func(book *contacts.Book) error {
		var err error
		_, isNew, err = contacts.SetAlias(book, addr, b.picked.Name, b.renaming, nick)
		return err
	}
	done := aliasChangeSummary(b.renaming, nick, addr)
	// The change is applied to the book as it is on disk now, under its lock:
	// an import or sync since the browser opened must not be overwritten by
	// the copy it loaded. A dry run checks it on a copy and saves nothing.
	path := contacts.DefaultPath(m.cfg)
	var book *contacts.Book
	var err error
	if m.isDryRun() {
		if book, err = contacts.LoadBook(path); err == nil {
			err = setAlias(book)
		}
	} else {
		book, err = contacts.Update(path, setAlias)
	}
	if err != nil {
		// A collision keeps the stage open, so the name can be changed.
		b.status = fmt.Sprintf("Alias not saved: %v", err)
		return m, nil
	}
	if m.isDryRun() {
		b.status = "Dry run, not saved: " + done
		return m, nil
	}
	b.book = book
	b.status = done
	if isNew {
		b.status += " (new contact)"
	}
	return m.leaveAliasNaming()
}

// aliasChangeSummary says what saving the naming stage did: added, renamed
// or, for an edit emptied out, removed.
func aliasChangeSummary(old, nick, addr string) string {
	switch {
	case old == "":
		return fmt.Sprintf("Alias %q saved for %s", nick, addr)
	case nick == "":
		return fmt.Sprintf("Alias %q removed from %s", old, addr)
	default:
		return fmt.Sprintf("Alias %q renamed to %q for %s", old, nick, addr)
	}
}

// leaveAliasNaming ends the naming stage after a save: the quick flow closes
// the overlay, an entry's window shows the entry as it now is, and the add
// flow returns to the list.
func (m *tuiModel) leaveAliasNaming() (tea.Model, tea.Cmd) {
	b := m.aliasView
	if b.quick {
		m.saveStatus = b.status
		return m.closeAliasBrowser()
	}
	b.nickStage, b.renaming = false, ""
	if b.detail != nil {
		if held := contacts.FindByAddress(b.book, b.detail.PrimaryEmail()); held != nil {
			entry := *held
			b.detail = &entry
		}
		m.refreshAliasBrowser()
		return m, nil
	}
	b.adding = false
	b.query = ""
	m.refreshAliasBrowser()
	return m, nil
}

// closeAliasBrowser leaves the overlay, reloading the picker's book when the
// browser was opened from it, since saving may have changed what it shows.
func (m *tuiModel) closeAliasBrowser() (tea.Model, tea.Cmd) {
	m.aliasView = nil
	if m.contactPick == nil {
		return m, nil
	}
	if book, err := contacts.LoadBook(contacts.DefaultPath(m.cfg)); err == nil {
		m.contactPick.book = book
		m.refreshContactPicker()
	}
	return m, nil
}

// insertAliasRecipient adds the highlighted alias to the picker's target
// recipient field, the way a picked contact is added, and closes both.
func (m *tuiModel) insertAliasRecipient() (tea.Model, tea.Cmd) {
	b := m.aliasView
	p := m.contactPick
	if b.cursor < 0 || b.cursor >= len(b.results) || p == nil {
		return m, nil
	}
	// The alias is how the contact was found, not what the recipient sees:
	// the field gets the name and address, as from the picker or Tab.
	recipient := formatCompletionString(contacts.ScoredContact{Contact: b.results[b.cursor]})
	if recipient == "" {
		return m, nil
	}
	m.aliasView = nil
	m.contactPick = nil
	if p.editing {
		m.compose.FieldInput = appendRecipients(m.compose.FieldInput, []string{recipient})
		return m, nil
	}
	field := m.recipientField(p.target)
	*field = appendRecipients(*field, []string{recipient})
	return m, m.saveAndSyncDraftCmd()
}

// kAliasBrowser handles keys while the aliases overlay is open, dispatching
// by stage: an open entry, naming, picking an address, or browsing the list.
// F6 always closes.
func (m *tuiModel) kAliasBrowser(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if k.Type == tea.KeyF6 {
		return m.closeAliasBrowser()
	}
	if k.Type == tea.KeyF1 {
		// Help opens over the browser from every screen; compose sends every
		// key here first, so it is answered here rather than by handleKey.
		m.openHelp()
		return m, nil
	}
	// Naming leads: it opens over the add flow and over an entry's window,
	// and Esc returns to whichever it came from.
	switch {
	case m.aliasView.nickStage:
		return m.kAliasNick(k)
	case m.aliasView.detail != nil:
		return m.kAliasDetail(k)
	case m.aliasView.adding:
		return m.kAliasPick(k)
	default:
		return m.kAliasList(k)
	}
}

// kAliasList is the browsing stage: letters search, Enter inserts when the
// browser was opened over the contacts picker and opens the entry otherwise,
// Ctrl+n starts a new alias.
func (m *tuiModel) kAliasList(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	b := m.aliasView
	switch k.String() {
	case "esc":
		return m.closeAliasBrowser()
	case "enter":
		if b.fromPicker {
			return m.insertAliasRecipient()
		}
		if b.cursor >= 0 && b.cursor < len(b.results) {
			entry := b.results[b.cursor]
			b.detail = &entry
			b.status = ""
		}
	case "ctrl+n":
		return m.startAddAlias()
	case "up":
		b.cursor = max(0, b.cursor-1)
	case "down":
		b.cursor = min(len(b.results)-1, b.cursor+1)
	case "pgup":
		b.cursor = max(0, b.cursor-m.aliasBrowserRows())
	case "pgdown":
		b.cursor = min(len(b.results)-1, b.cursor+m.aliasBrowserRows())
	case "home":
		b.cursor = 0
	case "end":
		b.cursor = max(0, len(b.results)-1)
	case "backspace":
		m.retreatAliasQuery()
	default:
		m.extendAliasQuery(k)
	}
	settleAliasCursor(&b.cursor, len(b.results))
	return m, nil
}

// kAliasPick is the address stage of a new alias: letters search the
// candidates, Enter promotes one to the naming stage.
func (m *tuiModel) kAliasPick(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	b := m.aliasView
	switch k.String() {
	case "esc":
		b.adding = false
		b.status = ""
	case "enter":
		m.pickAliasAddress()
	case "up":
		b.addCursor = max(0, b.addCursor-1)
	case "down":
		b.addCursor = min(len(b.addResults)-1, b.addCursor+1)
	case "pgup":
		b.addCursor = max(0, b.addCursor-m.aliasBrowserRows())
	case "pgdown":
		b.addCursor = min(len(b.addResults)-1, b.addCursor+m.aliasBrowserRows())
	case "home":
		b.addCursor = 0
	case "end":
		b.addCursor = max(0, len(b.addResults)-1)
	case "backspace":
		if b.addQuery != "" {
			r := []rune(b.addQuery)
			b.addQuery = string(r[:len(r)-1])
			m.refreshAliasCandidates()
		}
	default:
		if len(k.Runes) > 0 {
			b.addQuery += string(k.Runes)
			b.addCursor = 0
			m.refreshAliasCandidates()
		}
	}
	settleAliasCursor(&b.addCursor, len(b.addResults))
	return m, nil
}

// kAliasNick is the naming stage of a new alias: a one-line input that Enter
// saves through, Esc abandons.
func (m *tuiModel) kAliasNick(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	b := m.aliasView
	switch k.String() {
	case "esc":
		if b.quick {
			return m.closeAliasBrowser()
		}
		b.nickStage, b.renaming = false, ""
		b.status = ""
	case "enter":
		return m.commitAliasNickname()
	case "backspace":
		if r := []rune(b.nickname); len(r) > 0 {
			b.nickname = string(r[:len(r)-1])
		}
	case "space":
		b.nickname += " "
	default:
		if len(k.Runes) > 0 {
			b.nickname += string(k.Runes)
		}
	}
	return m, nil
}

// kAliasDetail is the open entry's window: c writes to the person, l reads
// their latest mail, e edits their alias, Enter and Esc return to the list.
func (m *tuiModel) kAliasDetail(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	b := m.aliasView
	switch k.String() {
	case "esc", "q", "enter":
		b.detail = nil
		b.status = ""
	case "c":
		return m.aliasComposeToEntry()
	case "l":
		return m.aliasLatestEmails()
	case "e":
		return m.editEntryAlias()
	}
	return m, nil
}

// aliasComposeToEntry writes to the entry's person: over a compose session
// they join its recipients, everywhere else a new compose opens with them
// already in To.
func (m *tuiModel) aliasComposeToEntry() (tea.Model, tea.Cmd) {
	b := m.aliasView
	if b == nil || b.detail == nil {
		return m, nil
	}
	recipient := formatCompletionString(contacts.ScoredContact{Contact: *b.detail})
	if recipient == "" {
		b.status = "This entry has no address to write to."
		return m, nil
	}
	m.aliasView = nil
	if m.mode != modeCompose || m.compose == nil {
		return m, m.composeCmd(recipient)
	}
	// A field is being edited: the person joins what it holds, the way a
	// picker insert does, rather than a field the user is not editing.
	if m.compose.FieldEditing != fieldNone {
		m.compose.FieldInput = appendRecipients(m.compose.FieldInput, []string{recipient})
		return m, nil
	}
	m.compose.To = appendRecipients(m.compose.To, []string{recipient})
	return m, m.saveAndSyncDraftCmd()
}

// aliasLatestEmails reads the person's latest mail into the search view: the
// one query every backend reads — from:<address> — asked of the server with
// a limit, so it stays the newest few messages. The results land on the
// message list, so the screen the browser was opened over is left behind:
// the reader for the list, a compose session saved as its draft.
func (m *tuiModel) aliasLatestEmails() (tea.Model, tea.Cmd) {
	b := m.aliasView
	if b == nil || b.detail == nil {
		return m, nil
	}
	entry := *b.detail
	if entry.IsGroup() {
		b.status = "A group stands for several people; write to it instead."
		return m, nil
	}
	addr := strings.TrimSpace(entry.PrimaryEmail())
	if addr == "" {
		b.status = "This entry has no address to read mail from."
		return m, nil
	}
	m.aliasView = nil
	var cmds []tea.Cmd
	if m.mode == modeCompose && m.compose != nil {
		_, cmd := m.saveDraftAndLeave(false)
		cmds = append(cmds, cmd)
	}
	m.showList()
	_, cmd := m.startRemoteSearch("from:"+addr, aliasLastEmailsLimit)
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

// extendAliasQuery appends what was typed, spaces included, to the search.
func (m *tuiModel) extendAliasQuery(k tea.KeyMsg) {
	b := m.aliasView
	switch k.Type {
	case tea.KeySpace:
		b.query += " "
		b.cursor = 0
		m.refreshAliasBrowser()
	case tea.KeyRunes:
		b.query += string(k.Runes)
		b.cursor = 0
		m.refreshAliasBrowser()
	}
}

// retreatAliasQuery drops the query's last character.
func (m *tuiModel) retreatAliasQuery() {
	b := m.aliasView
	if b.query != "" {
		r := []rune(b.query)
		b.query = string(r[:len(r)-1])
		m.refreshAliasBrowser()
	}
}

// settleAliasCursor clamps a list cursor into range after a move.
func settleAliasCursor(cursor *int, count int) {
	*cursor = max(0, min(count-1, *cursor))
}

func (m *tuiModel) aliasBrowserRows() int {
	return max(5, m.height-12)
}

// aliasBrowserRowCount is the clickable rows of the overlay's list, for
// clicks: candidates when one is being picked, otherwise the saved aliases.
// An open entry has no rows: its actions are keys.
func (m *tuiModel) aliasBrowserRowCount() int {
	if m.aliasView == nil || m.aliasView.nickStage || m.aliasView.detail != nil {
		return 0
	}
	if m.aliasView.adding {
		return len(m.aliasView.addResults)
	}
	return len(m.aliasView.results)
}

// clickAliasRow selects a row; a click on the row already under the cursor
// presses what the row's key would: choose the address, or open the entry.
func (m *tuiModel) clickAliasRow(i int) (tea.Model, tea.Cmd) {
	if m.aliasView == nil || m.aliasView.nickStage || m.aliasView.detail != nil {
		return m, nil
	}
	if m.aliasView.adding {
		return m.selectOrPress(&m.aliasView.addCursor, i, tea.KeyMsg{Type: tea.KeyEnter})
	}
	return m.selectOrPress(&m.aliasView.cursor, i, tea.KeyMsg{Type: tea.KeyEnter})
}

// renderAliasBrowser is the overlay: a stage-dependent dialog, drawn the way
// the contacts picker is drawn — centred, filling the terminal.
func renderAliasBrowser(m *tuiModel) string {
	t := m.theme.Theme()
	b := m.aliasView
	dialogWidth := min(100, max(20, m.width-4))
	if m.width > 0 && dialogWidth > m.width {
		dialogWidth = m.width
	}
	inner := max(10, dialogWidth-4)
	head := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(t.Get(render.ColorHighlightFg)))
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color(t.Get(render.ColorDate)))
	fg := lipgloss.NewStyle().Foreground(lipgloss.Color(t.Get(render.ColorFg)))

	lines := renderAliasStage(m, *b, head, fg, dim, inner)
	if b.status != "" {
		lines = append(lines, "", head.Render(truncateTo("  "+b.status, inner)))
	}
	hints := aliasHints(*b)
	lines = append(lines, "", dim.Render(markHints(truncateTo(hints, inner),
		hint{"Enter", "enter"}, hint{"Esc", "esc"})))

	dialog := lipgloss.NewStyle().
		Width(dialogWidth).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(t.Get(render.ColorBorder))).
		Padding(1, 2).
		Render(strings.Join(lines, "\n"))
	return lipgloss.NewStyle().Width(m.width).Height(max(10, m.height)).
		Align(lipgloss.Center, lipgloss.Center).Render(dialog)
}

// renderAliasStage is each stage's header, search or prompt and rows.
func renderAliasStage(m *tuiModel, b aliasBrowser, head, fg, dim lipgloss.Style, inner int) []string {
	switch {
	case b.nickStage && b.renaming != "":
		return []string{
			head.Render("ALIASES  ▸  edit alias"),
			"",
			fg.Render(fmt.Sprintf("  For:    %s", aliasTargetLine(b.picked))),
			dim.Render(fmt.Sprintf("  Was:    %s", b.renaming)),
			fmt.Sprintf("  Alias:  %s▏", b.nickname),
		}
	case b.nickStage:
		return []string{
			head.Render("ALIASES  ▸  name it"),
			"",
			fg.Render(fmt.Sprintf("  For:    %s", aliasTargetLine(b.picked))),
			fmt.Sprintf("  Alias:  %s▏", b.nickname),
		}
	case b.detail != nil:
		return renderAliasDetail(b, head, fg, dim, inner)
	case b.adding:
		lines := []string{
			head.Render("ALIASES  ▸  pick an address"),
			"",
			fmt.Sprintf("Search: %s▏    %s", b.addQuery, dim.Render(fmt.Sprintf("%d candidates", len(b.addResults)))),
			"",
		}
		if len(b.addResults) == 0 {
			return append(lines, dim.Render("  No address matches. Keep typing, or Esc to go back."))
		}
		return append(lines, aliasRows(frameAliases, aliasCandidateRow, b.addResults, b.addCursor, m.aliasBrowserRows(), inner, head, fg, dim)...)
	default:
		lines := []string{
			head.Render("ALIASES"),
			"",
			fmt.Sprintf("Search: %s▏    %s", b.query, dim.Render(fmt.Sprintf("%d contacts", len(b.results)))),
			"",
		}
		if len(b.results) == 0 {
			return append(lines, dim.Render("  No matching entries. Ctrl+n adds an alias."))
		}
		return append(lines, aliasRows(frameAliases, aliasListRow, b.results, b.cursor, m.aliasBrowserRows(), inner, head, fg, dim)...)
	}
}

// renderAliasDetail is the open entry's window: what the contact is, then
// the actions on the person.
func renderAliasDetail(b aliasBrowser, head, fg, dim lipgloss.Style, inner int) []string {
	c := *b.detail
	name := c.Name
	if name == "" {
		name = c.PrimaryEmail()
	}
	field := func(label, value string) string {
		return "  " + dim.Render(fitTo(label, 10)) + " " + fg.Render(truncateTo(value, inner-13))
	}
	lines := []string{
		head.Render("ALIASES  ▸  entry"),
		"",
		field("Name:", name),
	}
	if c.IsGroup() {
		lines = append(lines, field("Members:", strings.Join(c.Emails, ", ")))
	} else {
		lines = append(lines, field("Address:", c.PrimaryEmail()))
		if rest := otherEmails(c); len(rest) > 0 {
			lines = append(lines, field("Emails:", strings.Join(rest, ", ")))
		}
	}
	if aliases := c.AllAliases(); len(aliases) > 0 {
		lines = append(lines, field("Aliases:", strings.Join(aliases, ", ")))
	}
	if c.Source != "" {
		lines = append(lines, field("Source:", c.Source))
	}
	if c.Notes != "" {
		lines = append(lines, field("Notes:", c.Notes))
	}

	lines = append(lines, "",
		fmt.Sprintf("  %s   write to %s", head.Render("c"), truncateTo(name, 40)))
	if !c.IsGroup() && c.PrimaryEmail() != "" {
		lines = append(lines, fmt.Sprintf("  %s   the latest %d emails from %s",
			head.Render("l"), aliasLastEmailsLimit, truncateTo(c.PrimaryEmail(), 40)))
	}
	if !c.IsGroup() && c.PrimaryEmail() != "" {
		verb := "add an alias"
		if len(c.Aliases) > 0 {
			verb = "edit the alias " + truncateTo(c.Aliases[0], 30)
		}
		lines = append(lines, fmt.Sprintf("  %s   %s", head.Render("e"), verb))
	}
	lines = append(lines, fmt.Sprintf("  %s  back to the list", head.Render("Enter")))
	return lines
}

// otherEmails is a contact's addresses besides its primary one.
func otherEmails(c contacts.Contact) []string {
	var rest []string
	for _, e := range c.Emails {
		if e != c.PrimaryEmail() {
			rest = append(rest, e)
		}
	}
	return rest
}

// aliasTargetLine is a contact as the naming stage shows it.
func aliasTargetLine(c contacts.Contact) string {
	name := c.Name
	if name == "" {
		name = c.PrimaryEmail()
	}
	return truncateTo(fmt.Sprintf("%s  <%s>", name, c.PrimaryEmail()), 90)
}

// aliasHints is the stage's navigation hint line.
func aliasHints(b aliasBrowser) string {
	switch {
	case b.nickStage && b.renaming != "":
		if b.quick {
			return "Enter: save (empty removes it)   Esc: cancel"
		}
		return "Enter: save (empty removes it)   Esc: back"
	case b.nickStage && b.quick:
		return "Enter: save   Esc: cancel"
	case b.nickStage:
		return "Enter: save   Esc: back"
	case b.detail != nil:
		if b.detail.IsGroup() || b.detail.PrimaryEmail() == "" {
			return "c: write to them   Enter: back   Esc: back"
		}
		return "c: write   l: latest mail   e: alias   Enter: back   Esc: back"
	case b.adding:
		return "type: search   ↑/↓: move   Enter: choose   Esc: back"
	default:
		if b.fromPicker {
			return "type: search   ↑/↓: move   Enter: add   Ctrl+n: new alias   Esc: back to picker"
		}
		return "type: search   ↑/↓: move   Ctrl+n: new alias   Esc: close   F6: close"
	}
}

// aliasWindow holds what a visible window of alias rows needs, folded once so
// the list and candidate rows share it.
type aliasWindow struct {
	rows, inner   int
	cursor        int
	head, fg, dim lipgloss.Style
}

// aliasRows renders the visible window of a list-stage table, whatever its
// row format: browsing, or the new-alias candidates.
func aliasRows[C any](kind string, row func(C, lipgloss.Style) string, items []C, cursor, rows, inner int, head, fg, dim lipgloss.Style) []string {
	w := aliasWindow{rows: rows, inner: inner - 2, cursor: cursor, head: head, fg: fg, dim: dim}
	start := 0
	if w.cursor >= w.rows {
		start = w.cursor - w.rows + 1
	}
	end := min(len(items), start+w.rows)
	var out []string
	for i := start; i < end; i++ {
		line := truncateTo(row(items[i], fg), w.inner)
		if i == w.cursor {
			out = append(out, rowZone(kind, i, 0, w.head.Render("> "+line)))
			continue
		}
		out = append(out, rowZone(kind, i, 0, w.fg.Render("  "+line)))
	}
	if len(items) > w.rows {
		out = append(out, w.dim.Render(fmt.Sprintf("  %d-%d of %d", start+1, end, len(items))))
	}
	return out
}

// aliasListRow is one contact of the browsing list: its alias, name, address,
// and where it came from.
func aliasListRow(c contacts.Contact, _ lipgloss.Style) string {
	alias := c.DisplayAlias()
	if alias == "" {
		alias = "—"
	}
	name := c.Name
	if name == "" {
		name = "—"
	}
	// Columns by display width, not bytes, so a Hebrew or CJK name does not
	// push the address column out of line.
	return fitTo(alias, 12) + " " + fitTo(name, 22) + " " + fitTo(c.PrimaryEmail(), 28) + " " + c.Source
}

// aliasCandidateRow is one address the new-alias flow offers, with how often
// it was seen: the picker's line, plus the message sender's origin tag.
func aliasCandidateRow(sc contacts.ScoredContact, _ lipgloss.Style) string {
	line := contactLine(sc)
	if sc.Source == "this message" {
		line += "   ← this message's sender"
	}
	return line
}

// fitTo is s cut and padded to exactly width display columns.
func fitTo(s string, width int) string {
	return padTo(runewidth.Truncate(s, width, "…"), width)
}
