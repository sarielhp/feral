package contactsui

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/sarielhp/feral/contacts"

	tea "github.com/charmbracelet/bubbletea"
)

// memStore is a Store in memory, counting saves.
type memStore struct {
	book    *contacts.Book
	history *contacts.HistoryBook
	saves   int
}

func (s *memStore) Load() (*contacts.Book, *contacts.HistoryBook, error) {
	data, _ := json.Marshal(s.book)
	var b contacts.Book
	_ = json.Unmarshal(data, &b)
	return &b, s.history, nil
}

func (s *memStore) Update(change func(*contacts.Book) error) (*contacts.Book, error) {
	b, _, _ := s.Load()
	if err := change(b); err != nil {
		return nil, err
	}
	s.book = b
	s.saves++
	b, _, err := s.Load()
	return b, err
}

func sampleBook() *contacts.Book {
	return &contacts.Book{Contacts: []contacts.Contact{
		{ID: "google:w:people/1", Name: "Alice Adams", Emails: []string{"alice@example.com"}, Source: "google:w"},
		{ID: "manual:bob@example.com", Name: "Bob Brown", Emails: []string{"bob@example.com"}, Source: "manual"},
		{ID: contacts.GroupID("mutt", "theory"), Name: "Theory", Emails: []string{"alice@example.com", "carol@example.com"},
			Nicknames: []string{"theory"}, Source: "mutt"},
	}}
}

func sampleHistory() *contacts.HistoryBook {
	h := contacts.NewHistoryBook()
	h.Entries = []contacts.ScoredContact{
		{Contact: contacts.Contact{Name: "Dan Day", Emails: []string{"dan@example.com"}}, Frequency: 7},
		{Contact: contacts.Contact{Emails: []string{"bob@example.com"}}, Frequency: 3},
	}
	return h
}

func runes(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

var (
	enter = tea.KeyMsg{Type: tea.KeyEnter}
	esc   = tea.KeyMsg{Type: tea.KeyEsc}
	space = tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
	tab   = tea.KeyMsg{Type: tea.KeyTab}
)

func send(m tea.Model, keys ...tea.KeyMsg) {
	for _, k := range keys {
		m.Update(k)
	}
}

func TestList_MarksOutliveTheSearch(t *testing.T) {
	p := NewPicker(sampleBook(), sampleHistory(), PickerOptions{})
	send(p, runes("alice"), space)
	if p.List.MarkCount() != 1 {
		t.Fatalf("marked %d", p.List.MarkCount())
	}
	for range len("alice") {
		send(p, tea.KeyMsg{Type: tea.KeyBackspace})
	}
	send(p, runes("dan"), space, enter)
	picked, cancelled := p.Result()
	if cancelled || len(picked) != 2 || picked[0].PrimaryEmail() != "alice@example.com" || picked[1].PrimaryEmail() != "dan@example.com" {
		t.Fatalf("picked %+v cancelled %v", picked, cancelled)
	}
}

func TestPicker_EnterTakesCursorWhenNothingMarked(t *testing.T) {
	p := NewPicker(sampleBook(), sampleHistory(), PickerOptions{Query: "bob"})
	send(p, enter)
	picked, _ := p.Result()
	if len(picked) != 1 || picked[0].Name != "Bob Brown" {
		t.Fatalf("picked %+v", picked)
	}
}

func TestPicker_CancelAndEmpty(t *testing.T) {
	p := NewPicker(sampleBook(), nil, PickerOptions{})
	send(p, esc)
	if _, cancelled := p.Result(); !cancelled {
		t.Error("Esc did not cancel")
	}
	p = NewPicker(sampleBook(), nil, PickerOptions{Query: "nobody-matches-this"})
	send(p, enter)
	if _, cancelled := p.Result(); !cancelled {
		t.Error("Enter on an empty list did not count as cancelled")
	}
}

func TestPicker_PreselectAndSingle(t *testing.T) {
	p := NewPicker(sampleBook(), nil, PickerOptions{Preselect: []string{"Bob <BOB@example.com>", "new@example.org"}})
	if p.List.MarkCount() != 2 {
		t.Fatalf("preselected %d", p.List.MarkCount())
	}
	send(p, enter)
	picked, _ := p.Result()
	if picked[0].Name != "Bob Brown" || picked[1].PrimaryEmail() != "new@example.org" {
		t.Errorf("picked %+v", picked)
	}

	single := NewPicker(sampleBook(), nil, PickerOptions{Single: true})
	send(single, space)
	if single.List.MarkCount() != 0 {
		t.Error("marked in single mode")
	}
}

func TestPicker_Embedded(t *testing.T) {
	p := NewPicker(sampleBook(), nil, PickerOptions{Query: "bob"}).Embedded()
	_, cmd := p.Update(enter)
	if msg, ok := cmd().(PickedMsg); !ok || len(msg.Contacts) != 1 {
		t.Fatalf("embedded picker sent %#v", cmd())
	}
}

func TestWriteChoices(t *testing.T) {
	book := sampleBook()
	picked := []contacts.ScoredContact{{Contact: book.Contacts[1]}, {Contact: book.Contacts[2]}}
	tests := []struct {
		format string
		expand bool
		want   string
	}{
		{FormatRFC, false, "Bob Brown <bob@example.com>\nAlice Adams <alice@example.com>\ncarol@example.com\n"},
		{FormatAddr, false, "bob@example.com\nalice@example.com\ncarol@example.com\n"},
	}
	for _, tt := range tests {
		var b bytes.Buffer
		if err := WriteChoices(&b, picked, tt.format, tt.expand, book); err != nil {
			t.Fatal(err)
		}
		if b.String() != tt.want {
			t.Errorf("%s:\n got %q\nwant %q", tt.format, b.String(), tt.want)
		}
	}

	var b bytes.Buffer
	if err := WriteChoices(&b, picked, FormatJSON, false, book); err != nil {
		t.Fatal(err)
	}
	var got []Choice
	if err := json.Unmarshal(b.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !got[1].IsGroup || len(got[1].Members) != 2 || got[1].Members[0].Name != "Alice Adams" {
		t.Errorf("json = %s", b.String())
	}
	b.Reset()
	_ = WriteChoices(&b, picked, FormatJSON, true, book)
	_ = json.Unmarshal(b.Bytes(), &got)
	if len(got) != 3 {
		t.Errorf("expanded json has %d entries", len(got))
	}
	if err := CheckFormat("yaml"); err == nil {
		t.Error("an unknown format was accepted")
	}
}

func newTestManager(t *testing.T, opts ManagerOptions) (*Manager, *memStore) {
	t.Helper()
	store := &memStore{book: sampleBook(), history: sampleHistory()}
	m, err := NewManager(store, opts)
	if err != nil {
		t.Fatal(err)
	}
	return m, store
}

func typeText(m tea.Model, s string) {
	for _, r := range s {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

// selectContact searches for q and leaves search mode on the first match.
func selectContact(m *Manager, q string) {
	send(m, runes("/"))
	typeText(m, q)
	send(m, enter)
}

func TestManager_EditImportedKeepsOverlay(t *testing.T) {
	m, store := newTestManager(t, ManagerOptions{})
	selectContact(m, "alice adams")
	send(m, enter) // detail
	if m.mode != modeDetail || m.detail != "google:w:people/1" {
		t.Fatalf("mode %v detail %q", m.mode, m.detail)
	}
	send(m, runes("e"), tea.KeyMsg{Type: tea.KeyCtrlU})
	typeText(m, "Alice Q. Adams")
	send(m, tea.KeyMsg{Type: tea.KeyCtrlS})
	if m.mode != modeDetail || store.saves != 1 {
		t.Fatalf("mode %v saves %d status %q", m.mode, store.saves, m.status)
	}
	c := contacts.FindByID(store.book, "google:w:people/1")
	if c.Name != "Alice Q. Adams" || !c.IsEdited() || c.Origin.Name != "Alice Adams" {
		t.Fatalf("saved %+v", c)
	}
	if !strings.Contains(m.View(), "google:w says: Alice Adams") {
		t.Errorf("detail does not show the source's name:\n%s", m.View())
	}
}

func TestManager_AddDeleteAndHide(t *testing.T) {
	m, store := newTestManager(t, ManagerOptions{})
	send(m, runes("a"))
	typeText(m, "Eve")
	send(m, tab)
	typeText(m, "eve@example.com")
	send(m, tea.KeyMsg{Type: tea.KeyCtrlS})
	if contacts.FindByAddress(store.book, "eve@example.com") == nil {
		t.Fatalf("not added: %q", m.status)
	}
	send(m, esc) // back to the list
	selectContact(m, "eve")
	send(m, runes("d"), runes("y"))
	if contacts.FindByAddress(store.book, "eve@example.com") != nil || len(store.book.Hidden) != 0 {
		t.Fatalf("manual contact not deleted: %q", m.status)
	}
	send(m, esc)
	selectContact(m, "alice adams")
	send(m, runes("d"), runes("y"))
	if len(store.book.Hidden) != 1 {
		t.Fatalf("imported contact not hidden: %q", m.status)
	}
	send(m, esc, tab, tab) // Hidden list
	if m.view != viewHidden || len(m.list.Results) != 1 {
		t.Fatalf("view %v, %d hidden shown", m.view, len(m.list.Results))
	}
	send(m, runes("u"), runes("y"))
	if len(store.book.Hidden) != 0 {
		t.Errorf("unhide failed: %q", m.status)
	}
}

func TestManager_PromoteAndMerge(t *testing.T) {
	m, store := newTestManager(t, ManagerOptions{})
	send(m, tab) // Harvested: bob is a contact already, so only dan shows
	if len(m.list.Results) != 1 || m.list.Results[0].PrimaryEmail() != "dan@example.com" {
		t.Fatalf("harvested = %+v", m.list.Results)
	}
	send(m, enter, tea.KeyMsg{Type: tea.KeyCtrlS})
	if contacts.FindByAddress(store.book, "dan@example.com") == nil {
		t.Fatalf("not promoted: %q", m.status)
	}
	send(m, esc, tea.KeyMsg{Type: tea.KeyShiftTab}) // Contacts
	selectContact(m, "dan")
	send(m, space)
	send(m, esc) // clear the search; the mark stays
	selectContact(m, "bob")
	send(m, runes("m"), runes("y"))
	c := contacts.FindByID(store.book, "manual:bob@example.com")
	if c == nil || len(c.Emails) != 2 || contacts.FindByID(store.book, "manual:dan@example.com") != nil {
		t.Fatalf("merge: %+v status %q", c, m.status)
	}
}

func TestManager_ReadOnlyAndDryRun(t *testing.T) {
	for _, opts := range []ManagerOptions{{ReadOnly: true}, {ReadOnly: true, DryRun: true}} {
		m, store := newTestManager(t, opts)
		selectContact(m, "bob")
		send(m, runes("d"), runes("y"))
		if store.saves != 0 || contacts.FindByID(store.book, "manual:bob@example.com") == nil {
			t.Errorf("%+v: saved", opts)
		}
		if opts.DryRun && !strings.HasPrefix(m.status, "Dry run, not saved: Deleted Bob Brown") {
			t.Errorf("dry run status %q", m.status)
		}
		if !opts.DryRun && !strings.Contains(m.status, "Read-only") {
			t.Errorf("read-only status %q", m.status)
		}
	}
}

func TestManager_RefusedEditStaysInForm(t *testing.T) {
	m, store := newTestManager(t, ManagerOptions{})
	selectContact(m, "bob")
	send(m, runes("e"), tab, tea.KeyMsg{Type: tea.KeyCtrlU})
	typeText(m, "not an address")
	send(m, tea.KeyMsg{Type: tea.KeyCtrlS})
	if m.mode != modeForm || store.saves != 0 || !strings.HasPrefix(m.status, "Not saved") {
		t.Errorf("mode %v saves %d status %q", m.mode, store.saves, m.status)
	}
}

func TestLineInput(t *testing.T) {
	in := newLineInput("hello world")
	in.key(tea.KeyMsg{Type: tea.KeyCtrlW})
	in.key(tea.KeyMsg{Type: tea.KeyHome})
	in.key(runes(">"))
	if in.Value() != ">hello " {
		t.Errorf("value %q", in.Value())
	}
}

// Fast typing arrives as one message of several runes: "/bob" must start a
// search for bob, not be dropped as an unknown command.
func TestManager_MultiRuneMessage(t *testing.T) {
	m, _ := newTestManager(t, ManagerOptions{})
	send(m, runes("/bob"), enter)
	if m.list.Query != "bob" || len(m.list.Results) != 1 {
		t.Fatalf("query %q, %d results", m.list.Query, len(m.list.Results))
	}
}
