package contactsui

import (
	"github.com/sarielhp/feral/tui/render"

	"github.com/charmbracelet/lipgloss"
)

// styles are the colours of the mail TUI's theme, so the contacts screens
// look like the rest of feral.
type styles struct {
	title, sel, fg, dim, warn, border, label lipgloss.Style
	borderColor                              lipgloss.Color
}

func newStyles() styles {
	t := render.NewDefaultTheme()
	c := func(id render.ColorID) lipgloss.Color { return lipgloss.Color(t.Get(id)) }
	return styles{
		title:       lipgloss.NewStyle().Bold(true).Foreground(c(render.ColorHighlightFg)),
		sel:         lipgloss.NewStyle().Bold(true).Foreground(c(render.ColorHighlightFg)),
		fg:          lipgloss.NewStyle().Foreground(c(render.ColorFg)),
		dim:         lipgloss.NewStyle().Foreground(c(render.ColorDate)),
		warn:        lipgloss.NewStyle().Foreground(c(render.ColorSpam)),
		label:       lipgloss.NewStyle().Foreground(c(render.ColorSender)),
		border:      lipgloss.NewStyle().Foreground(c(render.ColorBorder)),
		borderColor: c(render.ColorBorder),
	}
}

// renderRows draws the visible window of l, with "> " at the cursor and a
// check mark on marked rows.
func renderRows(l *List, rows, width int, st styles) []string {
	if len(l.Results) == 0 {
		return []string{st.dim.Render("  No matching contacts.")}
	}
	start, end := l.Window(rows)
	out := make([]string, 0, end-start+1)
	for i := start; i < end; i++ {
		sc := l.Results[i]
		mark := "  "
		if l.IsMarked(sc) {
			mark = "✓ "
		}
		row := Truncate(mark+Line(sc), width-2)
		if i == l.Cursor {
			out = append(out, st.sel.Render("> "+row))
			continue
		}
		out = append(out, st.fg.Render("  "+row))
	}
	if len(l.Results) > rows {
		out = append(out, st.dim.Render(fmtRange(start, end, len(l.Results))))
	}
	return out
}
