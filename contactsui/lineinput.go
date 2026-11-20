package contactsui

import (
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
)

// lineInput is a one-line text field with a cursor: the editing keys of a
// shell line (arrows, Home/End, Ctrl-A/E/U/K/W, Backspace, Delete).
type lineInput struct {
	text   []rune
	cursor int
}

func newLineInput(s string) lineInput {
	r := []rune(s)
	return lineInput{text: r, cursor: len(r)}
}

func (in *lineInput) Value() string { return string(in.text) }

// key edits the field and reports whether k was an editing key.
func (in *lineInput) key(k tea.KeyMsg) bool {
	switch k.Type {
	case tea.KeyLeft:
		in.cursor = max(0, in.cursor-1)
	case tea.KeyRight:
		in.cursor = min(len(in.text), in.cursor+1)
	case tea.KeyHome, tea.KeyCtrlA:
		in.cursor = 0
	case tea.KeyEnd, tea.KeyCtrlE:
		in.cursor = len(in.text)
	case tea.KeyBackspace:
		if in.cursor > 0 {
			in.text = append(in.text[:in.cursor-1], in.text[in.cursor:]...)
			in.cursor--
		}
	case tea.KeyDelete:
		if in.cursor < len(in.text) {
			in.text = append(in.text[:in.cursor], in.text[in.cursor+1:]...)
		}
	case tea.KeyCtrlU:
		in.text, in.cursor = append([]rune(nil), in.text[in.cursor:]...), 0
	case tea.KeyCtrlK:
		in.text = in.text[:in.cursor]
	case tea.KeyCtrlW:
		in.deleteWord()
	case tea.KeyRunes, tea.KeySpace:
		in.insert(k.Runes)
	default:
		return false
	}
	return true
}

func (in *lineInput) insert(r []rune) {
	tail := append([]rune(nil), in.text[in.cursor:]...)
	in.text = append(append(in.text[:in.cursor], r...), tail...)
	in.cursor += len(r)
}

// deleteWord removes the word before the cursor and the spaces after it.
func (in *lineInput) deleteWord() {
	i := in.cursor
	for i > 0 && unicode.IsSpace(in.text[i-1]) {
		i--
	}
	for i > 0 && !unicode.IsSpace(in.text[i-1]) {
		i--
	}
	in.text = append(in.text[:i], in.text[in.cursor:]...)
	in.cursor = i
}

// view is the text with the cursor drawn as a bar when focused.
func (in *lineInput) view(focused bool) string {
	if !focused {
		return string(in.text)
	}
	var b strings.Builder
	b.WriteString(string(in.text[:in.cursor]))
	b.WriteString("▏")
	b.WriteString(string(in.text[in.cursor:]))
	return b.String()
}
