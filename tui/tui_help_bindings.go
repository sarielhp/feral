package tui

import (
	"maps"
	"regexp"
	"slices"

	"github.com/sarielhp/feral/keymap"
)

// The help shows the keys in effect. helpSections keeps the default key
// cells as written, short forms and all ("Ctrl+a / u"); a row whose keys are
// actions has a template here, which rebuilds the cell from the keymap --
// "{archive}" is archive's keys -- and only when one of its actions was
// moved, so the default help reads exactly as it always has.

// helpBindings are one help section's templates, keyed by the default cell,
// and the group whose keys they show.
type helpBindings struct {
	group string
	cells map[string]string
}

// sharedHelpCells are the rows of actions the list and a message share.
var sharedHelpCells = map[string]string{
	"w": "{browser}", "c": "{compose}", "r / g / G": "{reply} / {reply_all} / {reply_list}", "b": "{forward}",
	"E": "{archive}", "H": "{snooze}", "z": "{undo}", "X": "{unsubscribe}", "M": "{mute}", "N": "{toggle_read}", "x": "{mark_all_read}", "S": "{spam}",
	"U / u": "{unspam}", "B": "{block}", "s": "{save_to_folder}", "v": "{attachments}",
	"a": "{alias_sender}", "F5": "{spam_menu}", "F6": "{aliases}", "F7": "{saved_searches}",
	"L": "{lock_folder}", "|": "{pipe}",
}

// helpKeyTemplates are the templates of each section that shows actions.
var helpKeyTemplates = map[string]helpBindings{
	"Messages": {groupList, withCells(sharedHelpCells, map[string]string{
		"j / k, ↑ / ↓": "{down} / {up}, ↑ / ↓",
		"Space":        "{split_preview}",
		"e / Enter":    "{open} / Enter",
		"o / O":        "{sort} / {sort_reverse}",
		"t / T / ;":    "{tag} / {tag_pattern} / {tag_prefix}",
		"; v / x":      "{tag_prefix} v / x",
		"; a":          "{tag_prefix} a",
		"Ctrl+a / u":   "{tag_all} / {clear_tags}",
		"R":            "{refresh}",
		"d / z":        "{delete} / {undo}",
		"M / X":        "{mute} / {unsubscribe}",
	})},
	"Reading": {groupMessage, withCells(sharedHelpCells, map[string]string{
		"j / k, ↑ / ↓": "{scroll_down} / {scroll_up}, ↑ / ↓",
		"/":            "{search}",
		"E / d / S":    "{archive} / {delete} / {spam}",
		"Tab / p":      "Tab / {hide_pane}",
	})},
	"Search": {groupList, map[string]string{
		"/": "{search}", "=": "{filter_list}", "~": "{fuzzy_all}", "!": "{fuzzy_folder}", "Ctrl+f": "{server_search}", "+": "{load_more}",
	}},
	"Folders": {groupList, map[string]string{"L": "{lock_folder}", "R": "{refresh}"}},
	"Tools":   {groupList, map[string]string{"m / Alt": "{menu} / Alt", "D": "{deadlines}"}},
}

// withCells is shared and own in one map.
func withCells(shared, own map[string]string) map[string]string {
	out := maps.Clone(shared)
	maps.Copy(out, own)
	return out
}

var helpPlaceholder = regexp.MustCompile(`\{([a-z_]+)\}`)

// helpRows are section's rows as the keymap in effect binds them.
func (m *tuiModel) helpRows(section int) []helpRow {
	sec := helpSections[section]
	tpl, ok := helpKeyTemplates[sec.name]
	if !ok {
		return sec.rows
	}
	km := m.keys()
	var rows []helpRow
	for i, row := range sec.rows {
		cell, ok := tpl.cells[row.keys]
		if !ok || !templateMoved(km, tpl.group, cell) {
			continue
		}
		if rows == nil {
			rows = slices.Clone(sec.rows)
		}
		rows[i].keys = expandHelpTemplate(km, tpl.group, cell)
	}
	if rows == nil {
		return sec.rows
	}
	return rows
}

// templateMoved reports whether any action cell names was rebound.
func templateMoved(km *keymap.Keymap, group, cell string) bool {
	for _, match := range helpPlaceholder.FindAllStringSubmatch(cell, -1) {
		if km.Changed(group, match[1]) {
			return true
		}
	}
	return false
}

// expandHelpTemplate writes each action's keys into the cell.
func expandHelpTemplate(km *keymap.Keymap, group, cell string) string {
	return helpPlaceholder.ReplaceAllStringFunc(cell, func(p string) string {
		return keymap.DisplayKeys(km.Keys(group, p[1:len(p)-1]))
	})
}
