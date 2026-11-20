package render

import (
	"fmt"
	"strings"

	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/label"

	"github.com/mattn/go-runewidth"
)

type styBuilder struct {
	segments []Segment
	curFg    ColorID
	curBg    ColorID
}

func newStyBuilder() *styBuilder {
	return &styBuilder{}
}

func (b *styBuilder) PushString(s string) *styBuilder {
	if s == "" {
		return b
	}
	if len(b.segments) > 0 {
		last := &b.segments[len(b.segments)-1]
		if last.Fg == b.curFg && last.Bg == b.curBg {
			last.Text += s
			return b
		}
	}
	b.segments = append(b.segments, Segment{Text: s, Fg: b.curFg, Bg: b.curBg})
	return b
}

func (b *styBuilder) PushSegment(s string, fg, bg ColorID) *styBuilder {
	b.curFg, b.curBg = fg, bg
	return b.PushString(s)
}

func (b *styBuilder) Build() ColoredString {
	return ColoredString{Segments: b.segments}
}

func joinCols(collections ...ColoredString) ColoredString {
	var all []Segment
	for _, cs := range collections {
		all = append(all, cs.Segments...)
	}
	return ColoredString{Segments: all}
}

func renderCounter(idx int, highlight bool) ColoredString {
	highlightStr := fmt.Sprintf("%4d", idx)
	if highlight {
		return Plain(highlightStr).WithFg(ColorHighlightFg).WithBg(ColorHighlightBg)
	}
	return Plain(highlightStr).WithFg(ColorDim)
}

func renderDate(date string, highlight bool) ColoredString {
	var dateCS ColoredString
	if highlight {
		dateCS = Plain(date).WithFg(ColorHighlightFg).WithBg(ColorHighlightBg)
	} else {
		dateCS = Plain(date).WithFg(ColorDate).WithBg(ColorDefault)
	}
	return dateCS.TruncateLeft(8).PadLeft(8)
}

func renderSender(e email.Email, highlight bool) ColoredString {
	from := email.BidiDisplay(e.SenderColumn(false))
	if e.ThreadHasReplies && e.ThreadCollapsed && e.ThreadSenderSummary != "" {
		from = e.ThreadSenderSummary
	}
	if from == "" {
		from = "(Unknown Sender)"
	}
	var senderCS ColoredString
	if highlight {
		senderCS = Plain(from).WithFg(ColorHighlightFg).WithBg(ColorHighlightBg)
	} else {
		senderCS = Plain(from).WithFg(ColorSender).WithBg(ColorDefault)
	}
	return senderCS.TruncateRight(20).PadRight(20)
}

func renderThreadSubjectPrefix(e email.Email, highlight bool) ColoredString {
	var prefix string
	if e.ThreadPrefix != "" {
		prefix = e.ThreadPrefix
	} else if e.ThreadDepth > 0 {
		prefix = strings.Repeat("  ", e.ThreadDepth-1) + "└─> "
	}
	if prefix == "" {
		return ColoredString{}
	}
	if highlight {
		return Plain(prefix).WithFg(ColorHighlightFg).WithBg(ColorHighlightBg)
	}
	return Plain(prefix).WithFg(ColorHighlightFg).WithBg(ColorDefault)
}

func renderSubject(e email.Email, subjWidth int, highlight bool) ColoredString {
	subjText := e.Subject
	if e.ThreadDepth > 0 || e.ThreadPrefix != "" {
		if cleaned := email.StripSubjectPrefix(subjText); cleaned != "" {
			subjText = cleaned
		}
	}
	subjText = email.BidiDisplay(subjText)

	prefixCS := renderThreadSubjectPrefix(e, highlight)
	var textCS ColoredString
	if highlight {
		textCS = Plain(subjText).WithFg(ColorHighlightFg).WithBg(ColorHighlightBg)
	} else {
		fgColor := ColorSubject
		if e.IsSpam || e.IsPolitical || e.IsBlacklisted {
			fgColor = getBadgeFg(e)
		}
		textCS = Plain(subjText).WithFg(fgColor).WithBg(ColorDefault)
	}

	subjCS := textCS
	if len(prefixCS.Segments) > 0 {
		subjCS = joinCols(prefixCS, textCS)
	}

	if badge := listBadgeText(e); badge != "" {
		subjCS = withBadge(subjCS, badge, ColorDate, highlight)
	}
	if e.IsSpam || e.IsPolitical || e.IsBlacklisted {
		subjCS = withBadge(subjCS, getBadgeText(e), getBadgeFg(e), highlight)
	}
	if e.Account != "" {
		// Padded to two cells, an emoji's width, so the subjects of rows with
		// a letter and rows with an emoji start in the same column.
		accBadge := runewidth.FillRight(AccountBadge(e), 2)
		subjCS = withBadge(subjCS, accBadge, AccountColor(e.AccountIndex), highlight)
	}

	return subjCS.TruncateRight(subjWidth)
}

// withBadge puts badge, in fg, and a space ahead of subj, on the highlight
// background when the row is selected.
func withBadge(subj ColoredString, badge string, fg ColorID, highlight bool) ColoredString {
	bg, spaceFg := ColorDefault, ColorDefault
	if highlight {
		bg, spaceFg = ColorHighlightBg, ColorHighlightFg
	}
	return joinCols(Plain(badge).WithFg(fg).WithBg(bg), Plain(" ").WithFg(spaceFg).WithBg(bg), subj)
}

// listBadgeText is a row's mailing-list tag, "[label]" (email.ListBadge), or
// "" for a message from no list -- and for one whose subject already carries
// the tag, as most lists put it there themselves. It sits inside the subject
// column, which is truncated to its width, so the columns before it do not
// move.
func listBadgeText(e email.Email) string {
	label := email.ListBadge(e.ListID)
	if label == "" {
		return ""
	}
	badge := "[" + label + "]"
	if strings.Contains(strings.ToLower(e.Subject), strings.ToLower(badge)) {
		return ""
	}
	return badge
}

// AccountBadge is the tag a row shows for its account: the configured badge,
// or else the first letter of the account name, upper-cased. The colour, not
// the letter, is what tells two accounts with the same initial apart.
func AccountBadge(e email.Email) string {
	if e.AccountBadge != "" {
		return e.AccountBadge
	}
	for _, r := range e.Account {
		return strings.ToUpper(string(r))
	}
	return ""
}

// The index row follows mutt's index_format column order:
//
//	counter  hidden  flags  icons  date  sender  (size)  subject
//
// hidden is how many messages a collapsed thread hides (mutt's %2M), flags
// are mutt's %Z letters, and the size is mutt's (%4c).
const (
	hiddenWidth = 2
	flagsWidth  = 3
	iconsWidth  = 4
	sizeWidth   = 4
	// sizeColumnWidth is the size with its parentheses.
	sizeColumnWidth = sizeWidth + 2
	// indexHeaderWidth is every column before the subject, each followed by
	// its separator: one space inside the counter-to-icons block, two after.
	indexHeaderWidth = 4 + 1 + hiddenWidth + 1 + flagsWidth + 1 + iconsWidth + 1 +
		8 + 2 + 20 + 2 + sizeColumnWidth + 2
	// minSubjectWidth is the least a row leaves its subject before it gives
	// up the size column instead.
	minSubjectWidth = 16
)

// sizeFits reports whether a row totalWidth cells wide has room for the size
// column and still a usable subject. A narrow pane -- the attachment split,
// or the folder menu on a small terminal -- drops the size rather than
// overflowing into its neighbour.
func sizeFits(totalWidth int) bool {
	return totalWidth-indexHeaderWidth >= minSubjectWidth
}

// IndexHeaderWidth is the number of cells a row totalWidth cells wide spends
// before its subject.
func IndexHeaderWidth(totalWidth int) int {
	if sizeFits(totalWidth) {
		return indexHeaderWidth
	}
	return indexHeaderWidth - sizeColumnWidth - 2
}

// RowStyle is how a row is drawn, beyond the message it shows.
type RowStyle struct {
	Highlight bool
	Ghosted   bool
	Tagged    bool
}

// FormatSize renders a byte count in at most four cells, as mutt's index
// does: 834, 1.2K, 34K, 5.6M, 120M.
func FormatSize(n int64) string {
	const k, m, g = 1024, 1024 * 1024, 1024 * 1024 * 1024
	switch {
	case n < 0:
		return "?"
	case n < 1000:
		return fmt.Sprintf("%d", n)
	case n < 10*k:
		return fmt.Sprintf("%.1fK", float64(n)/k)
	case n < 1000*k:
		return fmt.Sprintf("%dK", (n+k/2)/k)
	case n < 10*m:
		return fmt.Sprintf("%.1fM", float64(n)/m)
	case n < 1000*m:
		return fmt.Sprintf("%dM", (n+m/2)/m)
	default:
		return fmt.Sprintf("%.1fG", float64(n)/g)
	}
}

// cell is text in fg, or in the highlight colours on the cursor row. It keeps
// a segment even for empty text, so padding it takes the row's colours.
func cell(text string, fg ColorID, highlight bool) ColoredString {
	if highlight {
		return ColoredString{Segments: []Segment{{Text: text, Fg: ColorHighlightFg, Bg: ColorHighlightBg}}}
	}
	return ColoredString{Segments: []Segment{{Text: text, Fg: fg, Bg: ColorDefault}}}
}

// renderSize is "(34K)", right-aligned. A message whose size is unknown --
// not yet downloaded -- shows nothing rather than a false "(0)".
func renderSize(e email.Email, highlight bool) ColoredString {
	text := ""
	if e.Size > 0 {
		text = "(" + FormatSize(e.Size) + ")"
	}
	return cell(text, ColorDim, highlight).PadLeft(sizeColumnWidth)
}

// renderHidden is how many messages a collapsed thread holds, the row itself
// included -- a thread of four shows 4, not the three mutt's %M counts as
// hidden behind it, which read as a miscount; ">>" past two digits.
func renderHidden(e email.Email, highlight bool) ColoredString {
	text := ""
	if e.ThreadHasReplies && e.ThreadCollapsed {
		total := e.ThreadRepliesCount + 1
		text = fmt.Sprintf("%2d", total)
		if total > 99 {
			text = ">>"
		}
	}
	return cell(text, ColorBorder, highlight).PadLeft(hiddenWidth)
}

// renderFlags is mutt's flag letters, each in a cell of its own so none hides
// another: N for an unread message, or n for a collapsed thread whose hidden
// messages include an unread one; r for replied; * for tagged.
func renderFlags(e email.Email, highlight, tagged bool) ColoredString {
	read := " "
	switch {
	case !e.IsRead:
		read = "N"
	case e.ThreadHiddenUnread:
		read = "n"
	}
	replied, tag := " ", " "
	if e.IsReplied {
		replied = "r"
	}
	if tagged {
		tag = "*"
	}
	return joinCols(
		cell(read, ColorDate, highlight),
		cell(replied, ColorHighlightFg, highlight),
		cell(tag, ColorHighlightFg, highlight),
	)
}

// renderIcons is the calendar-invitation and attachment icons, two cells each.
func renderIcons(e email.Email, highlight bool) ColoredString {
	var icons []ColoredString
	if e.HasICS {
		icons = append(icons, cell("⏰", ColorHighlightFg, highlight))
	}
	if e.HasAttachment {
		icons = append(icons, cell("📎", ColorHighlightFg, highlight))
	}
	return joinCols(icons...).TruncateRight(iconsWidth).PadRight(iconsWidth)
}

// EmailRow renders one message-list row without the tagged marker; it is
// EmailRowTagged with tagged false.
func EmailRow(e email.Email, totalWidth, subjWidth, idx int, t *Theme, highlight bool, ghosted bool) ColoredString {
	return EmailRowTagged(e, totalWidth, subjWidth, idx, t, highlight, ghosted, false)
}

// EmailRowTagged renders one message-list row. The theme argument is unused:
// colours are resolved when the result is rendered.
func EmailRowTagged(e email.Email, totalWidth, subjWidth, idx int, _ *Theme, highlight, ghosted, tagged bool) ColoredString {
	return EmailRowStyled(e, totalWidth, subjWidth, idx, RowStyle{
		Highlight: highlight, Ghosted: ghosted, Tagged: tagged,
	})
}

// EmailRowStyled renders one message-list row in mutt's column order --
// counter, hidden count, flags, icons, date, sender, size and subject --
// padding it to totalWidth when highlighted and striking it through when
// ghosted.
func EmailRowStyled(e email.Email, totalWidth, subjWidth, idx int, st RowStyle) ColoredString {
	highlight := st.Highlight
	sp := cell(" ", ColorDefault, highlight)
	sep := cell("  ", ColorDefault, highlight)

	cols := []ColoredString{
		renderCounter(idx, highlight), sp,
		renderHidden(e, highlight), sp,
		renderFlags(e, highlight, st.Tagged), sp,
		renderIcons(e, highlight), sp,
		renderDate(e.FormattedDate, highlight), sep,
		renderSender(e, highlight), sep,
	}
	if sizeFits(totalWidth) {
		cols = append(cols, renderSize(e, highlight), sep)
	}
	row := joinCols(append(cols, renderSubject(e, subjWidth, highlight))...)
	if st.Ghosted {
		for i := range row.Segments {
			row.Segments[i].Strike = true
			row.Segments[i].Fg = ColorDim
		}
	}
	if highlight {
		row = row.PadRight(totalWidth)
	}
	return row.TruncateTo(totalWidth)
}

func getBadgeText(e email.Email) string {
	if e.IsBlacklisted {
		return "[BLACKLIST]"
	}
	if e.IsPolitical {
		return "[POLI]"
	}
	if e.IsSpam {
		return "[SPAM]"
	}
	return ""
}

func getBadgeFg(e email.Email) ColorID {
	if e.IsBlacklisted {
		return ColorSpam
	}
	if e.IsPolitical {
		return ColorPoli
	}
	if e.IsSpam {
		return ColorSpam
	}
	return ColorSubject
}

// EndOfMsgGreen returns the centred "End of message" bar, width cells wide, on
// the message-end background.
func EndOfMsgGreen(width int, t *Theme) string {
	const label = " < End of message > "
	b := newStyBuilder()
	pad := max(0, (width-len(label))/2)
	b.PushSegment(strings.Repeat(" ", pad), ColorDefault, ColorMsgEndBg)
	b.PushSegment(label, ColorDefault, ColorMsgEndBg)
	b.PushSegment(strings.Repeat(" ", width-len(label)-pad), ColorDefault, ColorMsgEndBg)
	return b.Build().Render(t)
}

// MakeLabelLine renders one folder-tree row with its frame, expand arrow, name
// and message counts, highlighting and padding it when it is the cursor row.
func MakeLabelLine(e label.TreeEntry, isCursor bool, t *Theme, width int) string {
	var segs []Segment
	if isCursor {
		segs = append(segs, Segment{Text: "  ", Fg: ColorHighlightFg, Bg: ColorHighlightBg})
	} else {
		segs = append(segs, Segment{Text: "  ", Fg: ColorDefault, Bg: ColorDefault})
	}
	frame := e.Prefix
	if frame != "" {
		var fg, bg ColorID = ColorBorder, ColorDefault
		if isCursor {
			fg, bg = ColorHighlightFg, ColorHighlightBg
		}
		segs = append(segs, Segment{Text: frame, Fg: fg, Bg: bg})
	}
	var arrow string
	if e.Node != nil && len(e.Node.Children) > 0 {
		if e.Node.Expanded {
			arrow = "[-] "
		} else {
			arrow = "[+] "
		}
	}
	if arrow != "" {
		if isCursor {
			segs = append(segs, Segment{Text: arrow, Fg: ColorHighlightFg, Bg: ColorHighlightBg})
		} else {
			segs = append(segs, Segment{Text: arrow, Fg: ColorDefault, Bg: ColorDefault})
		}
	}
	folderText := e.Text
	if e.Node != nil && len(e.Node.Children) > 0 {
		folderText += fmt.Sprintf(" [%d]", label.CountAllDescendants(e.Node))
	}
	if isCursor {
		segs = append(segs, Segment{Text: folderText, Fg: ColorHighlightFg, Bg: ColorHighlightBg})
	} else {
		segs = append(segs, Segment{Text: folderText, Fg: ColorDefault, Bg: ColorDefault})
	}
	var count string
	if e.Node != nil {
		if e.Node.MessagesUnread > 0 {
			count = fmt.Sprintf(" (%d/%d)", e.Node.MessagesUnread, e.Node.MessagesTotal)
		} else {
			count = fmt.Sprintf(" (%d)", e.Node.MessagesTotal)
		}
	}
	if count != "" {
		if isCursor {
			segs = append(segs, Segment{Text: count, Fg: ColorHighlightFg, Bg: ColorHighlightBg})
		} else {
			segs = append(segs, Segment{Text: count, Fg: ColorSubject, Bg: ColorDefault})
		}
	}
	row := ColoredString{Segments: segs}
	if isCursor {
		row = row.PadRight(max(0, width-2))
	}
	return row.Render(t)
}
