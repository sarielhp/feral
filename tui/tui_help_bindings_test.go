package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// helpShorthands are the cells the help writes shorter than the keymap
// would: the second key of browser and of the pane toggle, and Ctrl+ said
// once for two keys.
var helpShorthands = map[string]string{
	"w":          "w / W",
	"Ctrl+a / u": "Ctrl+a / Ctrl+u",
	"Tab / p":    "Tab / p / P",
}

// Each template is a row of its section (a shared one, of the sections that
// list it), names actions of its group, and
// with the default keys says what the literal cell says: the help and the
// keymap cannot drift apart.
func TestHelpBindings_TemplatesMatchTheDefaults(t *testing.T) {
	for name, tpl := range helpKeyTemplates {
		sec := helpSections[sectionIndex(name)]
		for cell, template := range tpl.cells {
			if !rowsHaveCell(sec.rows, cell) {
				if _, shared := sharedHelpCells[cell]; !shared {
					t.Errorf("%s: no row has the cell %q", name, cell)
				}
				continue
			}
			for _, match := range helpPlaceholder.FindAllStringSubmatch(template, -1) {
				if !defaultKeys.Has(tpl.group, match[1]) {
					t.Errorf("%s %q: %s has no action %q", name, cell, tpl.group, match[1])
				}
			}
			want := cell
			if short, ok := helpShorthands[cell]; ok {
				want = short
			}
			if got := expandHelpTemplate(defaultKeys, tpl.group, template); got != want {
				t.Errorf("%s: %q expands to %q with the defaults, want %q", name, cell, got, want)
			}
		}
	}
}

// A shared template is a row of the list's page or the message's.
func TestHelpBindings_SharedTemplatesAreUsed(t *testing.T) {
	for cell := range sharedHelpCells {
		if !rowsHaveCell(helpSections[sectionIndex("Messages")].rows, cell) &&
			!rowsHaveCell(helpSections[sectionIndex("Reading")].rows, cell) {
			t.Errorf("no row has the shared cell %q", cell)
		}
	}
}

func rowsHaveCell(rows []helpRow, cell string) bool {
	for _, r := range rows {
		if r.keys == cell {
			return true
		}
	}
	return false
}

// With the defaults the rows are the literal ones, untouched.
func TestHelpBindings_DefaultRowsAreTheLiteralOnes(t *testing.T) {
	m := newTuiModel(nil, "", nil)
	for i, sec := range helpSections {
		got := m.helpRows(i)
		if len(got) != len(sec.rows) {
			t.Fatalf("%s: %d rows, want %d", sec.name, len(got), len(sec.rows))
		}
		for j := range got {
			if got[j] != sec.rows[j] {
				t.Errorf("%s: row %d is %v, want %v", sec.name, j, got[j], sec.rows[j])
			}
		}
	}
}

// A moved key is what the help shows, on the page and in its search, and
// only in the group it was moved in.
func TestHelpBindings_ShowTheKeysInEffect(t *testing.T) {
	m := helpModel(t)
	m.width, m.height = 120, 60
	m.keyMap = rebound(t, `{"list": {"archive": "A", "tag_all": "ctrl+t"}, "message": {"delete": "Y"}}`)
	rows := map[string]map[string]string{}
	for _, name := range []string{"Messages", "Reading"} {
		rows[name] = map[string]string{}
		for _, r := range m.helpRows(sectionIndex(name)) {
			rows[name][r.desc] = r.keys
		}
	}
	checks := []struct{ section, desc, want string }{
		{"Messages", "Archive email", "A"},
		{"Messages", "Tag all / clear tags", "Ctrl+t / Ctrl+u"},
		{"Messages", "Delete / undo last move", "d / z"},
		{"Reading", "Archive / delete / spam", "E / Y / S"},
	}
	for _, c := range checks {
		if got := rows[c.section][c.desc]; got != c.want {
			t.Errorf("%s %q shows %q, want %q", c.section, c.desc, got, c.want)
		}
	}
	m.Update(tea.KeyMsg{Type: tea.KeyF1})
	if view := stripANSI(m.View()); !strings.Contains(view, "A   ") || !strings.Contains(view, "Archive email") {
		t.Errorf("the Messages page does not show A for archive:\n%s", view)
	}
	m.helpSection = -1
	m.helpFind = helpQuery{text: "archive"}
	if view := stripANSI(m.View()); !strings.Contains(view, "E / Y / S") {
		t.Errorf("searching the help does not show the moved key:\n%s", view)
	}
}
