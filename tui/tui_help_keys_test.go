package tui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// Every description of every page can be scrolled into view at every width:
// two columns used to switch on at an inner width of 80 whatever the cells
// held, and a cell wider than its column was clipped at the frame for good.
func TestHelp_EveryDescriptionIsReachable(t *testing.T) {
	widths := []int{120}
	for w := 80; w <= 100; w += 2 {
		widths = append(widths, w)
	}
	for _, width := range widths {
		for i, sec := range helpSections {
			m := helpModel(t)
			m.width, m.height = width, 40
			m.showHelp, m.helpSection = true, i
			var seen strings.Builder
			for m.helpScroll = 0; m.helpScroll <= helpScrollBound(m); m.helpScroll++ {
				seen.WriteString(stripANSI(m.View()))
			}
			for _, row := range sec.rows {
				if !strings.Contains(seen.String(), row.desc) {
					t.Errorf("%dx40 %s: %q never shown", width, sec.name, row.desc)
				}
			}
		}
	}
}

// On a short terminal the section menu scrolls to keep its cursor in view.
func TestHelp_MenuCursorStaysVisible(t *testing.T) {
	m := helpModel(t)
	m.width, m.height = 80, 10
	m.showHelp, m.helpSection = true, -1
	last := helpSections[len(helpSections)-1].name
	m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	if !strings.Contains(m.View(), last) {
		t.Fatalf("End selected %s, which is not on screen", last)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyHome})
	for range helpSections[1:] {
		m.Update(tea.KeyMsg{Type: tea.KeyDown})
		if name := helpSections[m.helpCursor].name; !strings.Contains(m.View(), name) {
			t.Fatalf("cursor on %s, which is not on screen", name)
		}
	}
}

// Back from a page, the menu's cursor is on the page just left; ← on the
// menu closes help, as its hint says.
func TestHelp_MenuReturnAndLeft(t *testing.T) {
	m := helpModel(t)
	m.showHelp, m.helpSection = true, -1
	press(m, "4")
	if m.helpSection != 3 {
		t.Fatalf("4 opened section %d", m.helpSection)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if m.helpSection != -1 || m.helpCursor != 3 {
		t.Fatalf("back to menu: section %d cursor %d, want -1 and 3", m.helpSection, m.helpCursor)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if m.showHelp {
		t.Fatal("← on the menu did not close help")
	}
}

// Opening help over the folder tree, by key or by drawing it, and closing it
// again leaves the tree open: the render used to close it.
func TestHelp_LeavesTheFolderTreeOpen(t *testing.T) {
	m := helpModel(t)
	m.currentF = "INBOX"
	m.treeOpen = true
	m.Update(tea.KeyMsg{Type: tea.KeyF1})
	if !m.showHelp || helpSections[m.helpSection].name != "Folders" {
		t.Fatalf("F1 over the tree opened section %d", m.helpSection)
	}
	_ = m.View()
	m.Update(tea.KeyMsg{Type: tea.KeyF1})
	if m.showHelp || !m.treeOpen {
		t.Fatalf("after help: help %v tree %v", m.showHelp, m.treeOpen)
	}
}

// TestHelp_RowsNameBoundKeys fails when a help row names a key that no
// handler in the package answers to. It cannot tell whether the description
// is right (that "g" is group reply, not "first email"), only that the key
// exists; it caught "G", which nothing bound.
func TestHelp_RowsNameBoundKeys(t *testing.T) {
	bound := boundKeys(t)
	for _, sec := range helpSections {
		for _, row := range sec.rows {
			for _, k := range helpRowKeys(row.keys) {
				if !bound[k] {
					t.Errorf("%s: %q names %q, which no handler binds", sec.name, row.keys, k)
				}
			}
		}
	}
}

// helpRowKeys turns a row's key cell into key strings as bubbletea spells
// them. "Ctrl+a / u" is shorthand for Ctrl+a and Ctrl+u.
func helpRowKeys(cell string) []string {
	names := map[string]string{
		"enter": "enter", "esc": "esc", "space": " ", "tab": "tab",
		"↑": "up", "↓": "down", "←": "left", "→": "right",
		"shift+←": "shift+left", "shift+→": "shift+right",
		"pgup": "pgup", "pgdn": "pgdown", "home": "home", "end": "end",
	}
	skip := map[string]bool{"type": true, "alt": true, "[ro]": true}
	var keys []string
	ctrl := false
	for _, f := range strings.FieldsFunc(cell, func(r rune) bool { return r == '/' || r == ',' || r == ' ' }) {
		lower := strings.ToLower(f)
		switch {
		case skip[lower]:
		case names[lower] != "":
			keys = append(keys, names[lower])
		case strings.HasPrefix(lower, "ctrl+"):
			ctrl = true
			keys = append(keys, lower)
		case ctrl && len(f) == 1:
			keys = append(keys, "ctrl+"+lower)
		case len([]rune(f)) == 1:
			keys = append(keys, f) // a letter keeps its case: G is not g
		default:
			keys = append(keys, lower) // F5, Ctrl+C and the like
		}
	}
	return keys
}

// boundKeys collects every key the package's handlers compare against: the
// string literals in case clauses and == comparisons, and the tea.Key*
// constants, spelled as KeyMsg.String() spells them, and string constants,
// which is how a key named once (mouseToggleKey) is bound.
func boundKeys(t *testing.T) map[string]bool {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]bool{" ": true, "space": true}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		f, err := parser.ParseFile(fset, name, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			collectBoundKey(keys, n)
			return true
		})
	}
	// The list's and the message view's keys are named actions, bound
	// through the keymap table (tui_keymap.go) rather than in case clauses.
	for _, g := range keyTable.Groups {
		for _, b := range g.Bindings {
			for _, k := range b.Keys {
				keys[k] = true
			}
		}
	}
	return keys
}

func collectBoundKey(keys map[string]bool, n ast.Node) {
	switch x := n.(type) {
	case *ast.CaseClause:
		for _, e := range x.List {
			addKeyLiteral(keys, e)
		}
	case *ast.BinaryExpr:
		if x.Op == token.EQL {
			addKeyLiteral(keys, x.X)
			addKeyLiteral(keys, x.Y)
		}
	case *ast.ValueSpec:
		// A key named by a constant, such as mouseToggleKey.
		for _, v := range x.Values {
			addKeyLiteral(keys, v)
		}
	case *ast.SelectorExpr:
		if id, ok := x.X.(*ast.Ident); ok && id.Name == "tea" && strings.HasPrefix(x.Sel.Name, "Key") {
			keys[tea.KeyMsg{Type: teaKeyTypes[x.Sel.Name]}.String()] = true
		}
	}
}

func addKeyLiteral(keys map[string]bool, e ast.Expr) {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return
	}
	if s, err := strconv.Unquote(lit.Value); err == nil {
		keys[s] = true
	}
}

// teaKeyTypes are the tea.Key constants the handlers use by name.
var teaKeyTypes = map[string]tea.KeyType{
	"KeyF1": tea.KeyF1, "KeyF2": tea.KeyF2, "KeyF3": tea.KeyF3, "KeyF4": tea.KeyF4,
	"KeyF5": tea.KeyF5, "KeyF6": tea.KeyF6, "KeyF9": tea.KeyF9,
	"KeyEnter": tea.KeyEnter, "KeyEsc": tea.KeyEsc, "KeyTab": tea.KeyTab,
	"KeyUp": tea.KeyUp, "KeyDown": tea.KeyDown, "KeyLeft": tea.KeyLeft, "KeyRight": tea.KeyRight,
	"KeyHome": tea.KeyHome, "KeyEnd": tea.KeyEnd, "KeyPgUp": tea.KeyPgUp, "KeyPgDown": tea.KeyPgDown,
	"KeyCtrlC": tea.KeyCtrlC, "KeySpace": tea.KeySpace, "KeyBackspace": tea.KeyBackspace,
}
