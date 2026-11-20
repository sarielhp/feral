package tui

import (
	"net/mail"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/sarielhp/feral/contacts"
	"github.com/sarielhp/feral/email"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// The aliases browser's keys, mouse and what it inserts.

// Every letter searches: q used to close the browser, so no name with a q in
// it could be searched for.
func TestAliasBrowser_QSearches(t *testing.T) {
	m := aliasModel(t)
	press(m, "f6")
	typeInto(m, "quinn")
	if m.aliasView == nil {
		t.Fatal("typing q closed the browser")
	}
	if m.aliasView.query != "quinn" {
		t.Fatalf("query = %q", m.aliasView.query)
	}
}

// Backspace in the naming stage removes a character, not a byte.
func TestAliasBrowser_NicknameBackspaceKeepsUTF8(t *testing.T) {
	m := aliasModel(t)
	press(m, "f6")
	b := m.aliasView
	b.adding, b.nickStage, b.nickname = true, true, "דנה"
	m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if b.nickname != "דנ" || !utf8.ValidString(b.nickname) {
		t.Fatalf("nickname = %q", b.nickname)
	}
}

// F1 opens help over the browser in compose as it does in the index, and
// closing help returns to the browser.
func TestAliasBrowser_F1InCompose(t *testing.T) {
	m := composeWithContacts(t)
	press(m, "f6")
	if m.aliasView == nil {
		t.Fatal("F6 did not open the browser from compose")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyF1})
	if !m.showHelp || helpSections[m.helpSection].name != "Contacts" {
		t.Fatalf("help = %v on section %d", m.showHelp, m.helpSection)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyF1})
	if m.showHelp || m.aliasView == nil {
		t.Fatalf("help %v browser %v after closing help", m.showHelp, m.aliasView)
	}
}

// The wheel moves the alias list, not the compose body hidden beneath it.
func TestAliasBrowser_WheelInCompose(t *testing.T) {
	m := composeWithContacts(t)
	press(m, "f6")
	if m.composeScreenShowing() {
		t.Fatal("compose reported as showing under the browser")
	}
	m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown})
	if m.aliasView.cursor != 1 {
		t.Fatalf("alias cursor = %d after wheel down", m.aliasView.cursor)
	}
}

// F6 opens from a recipient field being edited.
func TestAliasBrowser_OpensFromFieldEdit(t *testing.T) {
	m := composeWithContacts(t)
	press(m, "t")
	if m.compose.FieldEditing != fieldTo {
		t.Fatal("t did not start editing To")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyF6})
	if m.aliasView == nil {
		t.Fatal("F6 did not open the browser from the To field")
	}
}

// A name with a comma is quoted wherever a recipient is inserted, and the
// field still splits into the recipients it holds.
func TestRecipient_QuotedNameRoundTrips(t *testing.T) {
	sc := contacts.ScoredContact{Contact: contacts.Contact{
		ID: "g1", Name: "Doe, Alice", Emails: []string{"alice@example.com"},
	}}
	r := formatCompletionString(sc)
	if r != `"Doe, Alice" <alice@example.com>` {
		t.Fatalf("formatted = %q", r)
	}
	list := appendRecipients("bob@example.com", []string{r})
	list = appendRecipients(list, []string{"carol@example.com"})
	if got := email.SplitAddressList(list); len(got) != 3 {
		t.Fatalf("split %q into %q", list, got)
	}
	addrs, err := mail.ParseAddressList(list)
	if err != nil || len(addrs) != 3 || addrs[1].Name != "Doe, Alice" {
		t.Fatalf("parse %q: %+v, %v", list, addrs, err)
	}
	// Re-adding the quoted recipient is recognised as a duplicate.
	if again := appendRecipients(list, []string{r}); again != list {
		t.Fatalf("duplicate added: %q", again)
	}
}

// Tab completion keeps completing the entry after a quoted name, now that the
// entry start is found by the quote-aware scan.
func TestRecipient_CompletionAfterQuotedName(t *testing.T) {
	m := composeWithContacts(t)
	press(m, "t")
	m.compose.FieldInput = `"Doe, Alice" <a@x.com>, car`
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	want := `"Doe, Alice" <a@x.com>, Carol Chen <carol@example.com>`
	if m.compose.FieldInput != want {
		t.Fatalf("FieldInput = %q, want %q", m.compose.FieldInput, want)
	}
}

// List columns line up by display width: fmt pads by runes, which leaves a
// double-width CJK name one column too long per character.
func TestAliasBrowser_RowsAlignByDisplayWidth(t *testing.T) {
	latin := aliasListRow(contacts.Contact{Name: "Dana Diaz", Emails: []string{"d@x.com"}, Aliases: []string{"dd"}}, lipgloss.NewStyle())
	wide := aliasListRow(contacts.Contact{Name: "王小明", Emails: []string{"d@x.com"}, Aliases: []string{"小明"}}, lipgloss.NewStyle())
	col := func(row string) int {
		i := strings.Index(row, "d@x.com")
		if i < 0 {
			t.Fatalf("address missing from %q", row)
		}
		return runewidth.StringWidth(row[:i])
	}
	if col(latin) != col(wide) {
		t.Fatalf("address column %d vs %d:\n%q\n%q", col(latin), col(wide), latin, wide)
	}
}
