package tui

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// Editing an alias: e in an entry's window, and the add flow reaching a
// person who already has one. A name another contact answers to is refused.

// openBobEntry opens Bob's entry window from the aliases list.
func openBobEntry(t *testing.T, m *tuiModel) {
	t.Helper()
	press(m, "f6")
	typeInto(m, "bob")
	press(m, "enter")
	if m.aliasView == nil || m.aliasView.detail == nil || m.aliasView.detail.PrimaryEmail() != "bob@example.com" {
		t.Fatalf("Bob's entry not open: %+v", m.aliasView)
	}
}

// retypeNickname replaces what the naming stage holds with s.
func retypeNickname(m *tuiModel, s string) {
	for range []rune(m.aliasView.nickname) {
		m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	}
	typeInto(m, s)
}

func TestAliasEdit_FromEntryAddsRenamesAndRemoves(t *testing.T) {
	m := aliasModel(t)
	openBobEntry(t, m)

	// No alias yet: e adds one, starting from the suggestion.
	press(m, "e")
	b := m.aliasView
	if !b.nickStage || b.renaming != "" || b.nickname != "bob-brown" {
		t.Fatalf("e on an unaliased entry = stage %v renaming %q nick %q", b.nickStage, b.renaming, b.nickname)
	}
	press(m, "enter")
	if got := bobAliases(t, m); !slices.Equal(got, []string{"bob-brown"}) {
		t.Fatalf("aliases after add = %v", got)
	}
	if b.nickStage || b.detail == nil || !slices.Equal(b.detail.Aliases, []string{"bob-brown"}) {
		t.Fatalf("after save: stage %v, entry %+v; want the refreshed entry window", b.nickStage, b.detail)
	}

	// Now e edits it: the stage starts from the alias, and saving replaces it.
	press(m, "e")
	if b.renaming != "bob-brown" || b.nickname != "bob-brown" {
		t.Fatalf("e on an aliased entry = renaming %q nick %q", b.renaming, b.nickname)
	}
	if !strings.Contains(m.View(), "edit alias") {
		t.Fatal("the naming stage does not say it is editing")
	}
	retypeNickname(m, "bb")
	press(m, "enter")
	if got := bobAliases(t, m); !slices.Equal(got, []string{"bb"}) {
		t.Fatalf("aliases after rename = %v, want [bb]", got)
	}
	if !strings.Contains(b.status, `"bob-brown" renamed to "bb"`) {
		t.Fatalf("status = %q", b.status)
	}

	// Emptied out, the edit removes the alias.
	press(m, "e")
	retypeNickname(m, "")
	press(m, "enter")
	if got := bobAliases(t, m); len(got) != 0 {
		t.Fatalf("aliases after removal = %v", got)
	}
	if !strings.Contains(b.status, "removed") {
		t.Fatalf("status = %q", b.status)
	}
}

// A name another contact answers to -- here Alice's source nickname -- is
// refused, the file is untouched, and the stage stays open to change it.
func TestAliasEdit_CollisionIsRefused(t *testing.T) {
	m := aliasModel(t)
	openBobEntry(t, m)
	press(m, "e")
	retypeNickname(m, "ALI")
	before := bookBytes(t, m)
	press(m, "enter")
	b := m.aliasView
	if !b.nickStage {
		t.Fatal("a refused name closed the naming stage")
	}
	if !strings.Contains(b.status, "already names Alice Adams") {
		t.Fatalf("status = %q, want the collision named", b.status)
	}
	if !bytes.Equal(bookBytes(t, m), before) {
		t.Fatal("contacts.json written despite the collision")
	}
	// Esc goes back to the entry, not the list.
	press(m, "esc")
	if b.nickStage || b.detail == nil {
		t.Fatalf("esc from editing: stage %v entry %v", b.nickStage, b.detail)
	}
}

// Naming, through the add flow, a person who already has an alias edits that
// alias rather than piling a second one on.
func TestAliasEdit_AddFlowEditsAnExistingAlias(t *testing.T) {
	m := aliasModel(t)
	press(m, "f6")
	saveBobAlias(t, m) // bob-brown
	press(m, "ctrl+n")
	typeInto(m, "bob")
	press(m, "enter")
	b := m.aliasView
	if b.renaming != "bob-brown" || b.nickname != "bob-brown" {
		t.Fatalf("add flow on an aliased person = renaming %q nick %q", b.renaming, b.nickname)
	}
	retypeNickname(m, "bobby")
	press(m, "enter")
	if got := bobAliases(t, m); !slices.Equal(got, []string{"bobby"}) {
		t.Fatalf("aliases = %v, want [bobby]", got)
	}
}

// Under --dry-run an edit is reported and not written.
func TestAliasEdit_DryRunWritesNothing(t *testing.T) {
	m := aliasModel(t)
	press(m, "f6")
	saveBobAlias(t, m)
	m.cfg.DryRun = true
	before := bookBytes(t, m)
	press(m, "esc")
	openBobEntry(t, m)
	press(m, "e")
	retypeNickname(m, "bb")
	press(m, "enter")
	if !bytes.Equal(bookBytes(t, m), before) {
		t.Fatal("a dry run rewrote contacts.json")
	}
	if !strings.Contains(m.aliasView.status, "Dry run") {
		t.Fatalf("status = %q", m.aliasView.status)
	}
}
