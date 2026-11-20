// Package keymap is the TUI's configurable keys: which key runs which named
// action on which group of screens, the defaults the TUI declares, and the
// overrides a user writes in keys.json.
//
// It knows nothing about what an action does. The TUI declares its actions
// and their default keys as a Table and dispatches on the action name Action
// returns, so this package is a leaf the command line can read as well (feral
// utils keys prints Table.JSON).
//
// A keys file that names an action or group the table does not have, a key
// that is not a key, a key the TUI keeps for itself, or a key two actions of
// one group would share is refused, with the line that caused it: a keymap
// that half applies is worse than none, because the user cannot tell which
// half.
package keymap

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sarielhp/feral/fileformat"
)

// FileName is the keys file, in the configuration directory.
const FileName = "keys.json"

// Binding is one action and its default keys, spelled as bubbletea's
// KeyMsg.String() spells them: "E", "ctrl+f", "f5", " ".
type Binding struct {
	Action string
	Keys   []string
}

// Group is the actions of one group of screens, in the order a keys file
// lists them.
type Group struct {
	Name     string
	Bindings []Binding
}

// Table is what a keys file may change, and what it may not.
type Table struct {
	Groups []Group
	// Reserved are the keys no action may take, each with what it does:
	// the global keys, and the keys a screen handles before its actions.
	Reserved map[string]string
	// ReservedPrefixes reserve every key that starts with one, such as
	// "alt+", which opens the menu whatever the letter.
	ReservedPrefixes map[string]string
}

// Keymap is the bindings in effect: the table's defaults with a keys file's
// overrides applied.
type Keymap struct {
	keys     map[string]map[string][]string // group -> action -> keys
	actions  map[string]map[string]string   // group -> key -> action
	defaults map[string]map[string][]string
}

// Default is the keymap the table declares.
func Default(t *Table) *Keymap {
	km := &Keymap{
		keys:     map[string]map[string][]string{},
		actions:  map[string]map[string]string{},
		defaults: map[string]map[string][]string{},
	}
	for _, g := range t.Groups {
		km.keys[g.Name] = map[string][]string{}
		km.defaults[g.Name] = map[string][]string{}
		for _, b := range g.Bindings {
			km.keys[g.Name][b.Action] = b.Keys
			km.defaults[g.Name][b.Action] = b.Keys
		}
	}
	km.index()
	return km
}

// index rebuilds the key -> action lookup from the bindings.
func (km *Keymap) index() {
	for group, byAction := range km.keys {
		km.actions[group] = map[string]string{}
		for action, keys := range byAction {
			for _, k := range keys {
				km.actions[group][k] = action
			}
		}
	}
}

// Action is the action key runs in group, or "" when it runs none.
func (km *Keymap) Action(group, key string) string {
	return km.actions[group][key]
}

// Keys are the keys that run action in group.
func (km *Keymap) Keys(group, action string) []string {
	return km.keys[group][action]
}

// Changed reports whether action in group runs on other keys than its
// defaults.
func (km *Keymap) Changed(group, action string) bool {
	return !slices.Equal(km.keys[group][action], km.defaults[group][action])
}

// Has reports whether group declares action.
func (km *Keymap) Has(group, action string) bool {
	_, ok := km.defaults[group][action]
	return ok
}

// has reports whether group declares action.
func (t *Table) has(group, action string) bool {
	for _, g := range t.Groups {
		if g.Name == group {
			return slices.ContainsFunc(g.Bindings, func(b Binding) bool { return b.Action == action })
		}
	}
	return false
}

// Check refuses a table whose defaults a keys file could not have produced:
// a key misspelled, reserved, or shared by two actions of a group, or an
// action declared twice. It is for the TUI's own tests.
func (t *Table) Check() error {
	for _, g := range t.Groups {
		owner := map[string]string{}
		for _, b := range g.Bindings {
			if _, dup := owner["action:"+b.Action]; dup || b.Action == "" {
				return fmt.Errorf("%s: action %q declared twice or unnamed", g.Name, b.Action)
			}
			owner["action:"+b.Action] = b.Action
			if err := t.checkKeys(b.Keys); err != nil {
				return fmt.Errorf("%s.%s: %w", g.Name, b.Action, err)
			}
			for _, k := range b.Keys {
				if other, taken := owner[k]; taken {
					return fmt.Errorf("%s.%s: %q is also %s's key", g.Name, b.Action, k, other)
				}
				owner[k] = b.Action
			}
		}
	}
	return nil
}

// checkKeys refuses keys that are not keys in their spelling, are reserved,
// or repeat, and an empty list: every action needs a key.
func (t *Table) checkKeys(keys []string) error {
	if len(keys) == 0 {
		return fmt.Errorf("no key: every action needs one")
	}
	for i, k := range keys {
		n, err := Normalize(k)
		if err != nil {
			return err
		}
		if n != k {
			return fmt.Errorf("%q is spelled %q", k, n)
		}
		if what := t.reserved(k); what != "" {
			return fmt.Errorf("%q is not configurable: it is %s", k, what)
		}
		if slices.Contains(keys[:i], k) {
			return fmt.Errorf("%q is listed twice", k)
		}
	}
	return nil
}

// reserved is what a reserved key does, or "" when the key is free.
func (t *Table) reserved(k string) string {
	if what, ok := t.Reserved[k]; ok {
		return what
	}
	for prefix, what := range t.ReservedPrefixes {
		if strings.HasPrefix(k, prefix) {
			return what
		}
	}
	return ""
}

// JSON is the table's defaults as a keys file, ready to edit: every group
// and action, in the table's order.
func (t *Table) JSON() []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "{\n  \"format\": %d", fileformat.Current)
	for _, g := range t.Groups {
		fmt.Fprintf(&b, ",\n  %s: {", quote(g.Name))
		for i, bd := range g.Bindings {
			if i > 0 {
				b.WriteString(",")
			}
			fmt.Fprintf(&b, "\n    %s: %s", quote(bd.Action), keysJSON(bd.Keys))
		}
		b.WriteString("\n  }")
	}
	b.WriteString("\n}\n")
	return b.Bytes()
}

// keysJSON is one key as a string, several as a list. The space bar is
// written "space", which reads better than " " and parses back the same.
func keysJSON(keys []string) string {
	spelled := make([]string, len(keys))
	for i, k := range keys {
		if k == " " {
			k = "space"
		}
		spelled[i] = quote(k)
	}
	if len(spelled) == 1 {
		return spelled[0]
	}
	return "[" + strings.Join(spelled, ", ") + "]"
}

func quote(s string) string {
	out, err := json.Marshal(s)
	if err != nil {
		return `""` // a Go string always marshals
	}
	return string(out)
}

// keyNames are the named keys bubbletea reports: "enter", "ctrl+a", "f5".
var keyNames = func() map[string]bool {
	names := map[string]bool{}
	for kt := tea.KeyType(-256); kt <= 256; kt++ {
		if s := kt.String(); s != "" && s != "runes" && utf8.RuneCountInString(s) > 1 {
			names[s] = true
		}
	}
	return names
}()

// keyAliases are spellings a person may write for a named key.
var keyAliases = map[string]string{
	"space": " ", "pgdn": "pgdown", "pagedown": "pgdown", "pageup": "pgup",
	"escape": "esc", "return": "enter", "del": "delete",
}

// Normalize spells key the way bubbletea reports it: a single character as
// it is (G is not g), a named key in lower case ("Ctrl+F" is "ctrl+f"),
// "space" as " ". It refuses what is not a key.
func Normalize(key string) (string, error) {
	if utf8.RuneCountInString(key) == 1 {
		r, _ := utf8.DecodeRuneInString(key)
		if !unicode.IsPrint(r) {
			return "", fmt.Errorf("%q is not a printable key", key)
		}
		return key, nil
	}
	lower := strings.ToLower(key)
	if alias, ok := keyAliases[lower]; ok {
		return alias, nil
	}
	if rest, ok := strings.CutPrefix(lower, "alt+"); ok && rest != "" {
		inner, err := Normalize(key[len("alt+"):])
		if err != nil {
			return "", err
		}
		return "alt+" + inner, nil
	}
	if keyNames[lower] {
		return lower, nil
	}
	return "", fmt.Errorf("%q is not a key: write one character, or a name such as ctrl+f, f5 or space", key)
}

// displayNames are the named keys as the help shows them.
var displayNames = map[string]string{
	" ": "Space", "enter": "Enter", "esc": "Esc", "tab": "Tab", "backspace": "Backspace",
	"delete": "Delete", "insert": "Insert", "up": "↑", "down": "↓", "left": "←", "right": "→",
	"pgup": "PgUp", "pgdown": "PgDn", "home": "Home", "end": "End",
	"shift+left": "Shift+←", "shift+right": "Shift+→", "shift+tab": "Shift+Tab",
}

// Display is key as the help writes it: "Ctrl+f", "F5", "Space", "E".
func Display(key string) string {
	if name, ok := displayNames[key]; ok {
		return name
	}
	if rest, ok := strings.CutPrefix(key, "ctrl+"); ok {
		return "Ctrl+" + rest
	}
	if rest, ok := strings.CutPrefix(key, "alt+"); ok {
		return "Alt+" + Display(rest)
	}
	if len(key) > 1 && key[0] == 'f' && strings.Trim(key[1:], "0123456789") == "" {
		return "F" + key[1:]
	}
	return key
}

// DisplayKeys is an action's keys as the help writes them, joined by " / ".
func DisplayKeys(keys []string) string {
	shown := make([]string, len(keys))
	for i, k := range keys {
		shown[i] = Display(k)
	}
	return strings.Join(shown, " / ")
}
