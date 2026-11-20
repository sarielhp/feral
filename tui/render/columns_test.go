package render

import (
	"strings"
	"testing"

	"github.com/sarielhp/feral/email"
)

func TestFormatSize(t *testing.T) {
	cases := map[int64]string{
		0:                 "0",
		834:               "834",
		1536:              "1.5K",
		34 * 1024:         "34K",
		5*1024*1024 + 1e5: "5.1M",
		120 * 1024 * 1024: "120M",
	}
	for n, want := range cases {
		if got := FormatSize(n); got != want {
			t.Errorf("FormatSize(%d) = %q, want %q", n, got, want)
		}
		if w := len(FormatSize(n)); w > sizeWidth {
			t.Errorf("FormatSize(%d) is %d cells, wider than the column", n, w)
		}
	}
}

// The subject starts exactly IndexHeaderWidth cells in, whatever the row
// carries, so the width the views reserve matches the row.
func TestEmailRowStyled_SubjectStartsAtHeaderWidth(t *testing.T) {
	base := email.Email{
		ID: "m1", Subject: "SUBJ", FromRaw: "Alice <alice@example.com>",
		FormattedDate: "11/19/26", Size: 34 * 1024,
	}
	collapsed := base
	collapsed.ThreadHasReplies, collapsed.ThreadCollapsed, collapsed.ThreadRepliesCount = true, true, 3
	icons := base
	icons.HasICS, icons.HasAttachment, icons.IsReplied = true, true, true
	unsized := base
	unsized.Size = 0
	rows := map[string]email.Email{"plain": base, "collapsed": collapsed, "icons": icons, "unsized": unsized}
	for name, e := range rows {
		for _, st := range []RowStyle{{}, {Highlight: true}, {Tagged: true}} {
			plain := EmailRowStyled(e, 120, 40, 1, st).Plain()
			if got, want := runeWidthBefore(plain, "SUBJ"), IndexHeaderWidth(120); got != want {
				t.Errorf("%s %+v: subject at cell %d, want %d: %q", name, st, got, want, plain)
			}
		}
	}
}

// runeWidthBefore is the display width of s before the first sub: the icons
// are two cells wide but more than two bytes long.
func runeWidthBefore(s, sub string) int {
	i := strings.Index(s, sub)
	if i < 0 {
		return -1
	}
	return Plain(s[:i]).Width()
}

// The columns come in mutt's index_format order: counter, thread count,
// flags, icons, date, sender, size, subject. A collapsed thread of four --
// the row and three replies -- shows 4.
func TestEmailRowStyled_MuttColumnOrder(t *testing.T) {
	e := email.Email{
		ID: "m1", Subject: "SUBJ", FromRaw: "Alice <alice@example.com>",
		FormattedDate: "11/19/26", Size: 34 * 1024,
		ThreadHasReplies: true, ThreadCollapsed: true, ThreadRepliesCount: 3,
		IsReplied: true,
	}
	plain := EmailRowStyled(e, 120, 40, 7, RowStyle{Tagged: true}).Plain()
	want := "   7  4 Nr*      11/19/26  Alice                  (34K)  SUBJ"
	if !strings.HasPrefix(plain, want) {
		t.Errorf("row\n got %q\nwant %q", plain, want)
	}
	if strings.Contains(plain, "[+") {
		t.Errorf("the thread count belongs in its column, not after the subject: %q", plain)
	}
}

// Each flag has its own cell, so a replied, unread message shows both, and a
// collapsed thread with unread hidden messages shows n.
func TestRenderFlags_EachFlagVisible(t *testing.T) {
	cases := []struct {
		name   string
		e      email.Email
		tagged bool
		want   string
	}{
		{"read", email.Email{IsRead: true}, false, "   "},
		{"unread", email.Email{}, false, "N  "},
		{"unread replied tagged", email.Email{IsReplied: true}, true, "Nr*"},
		{"hidden unread", email.Email{IsRead: true, ThreadHiddenUnread: true}, false, "n  "},
		{"own unread wins over hidden", email.Email{ThreadHiddenUnread: true}, false, "N  "},
		{"hidden unread replied", email.Email{IsRead: true, IsReplied: true, ThreadHiddenUnread: true}, false, "nr "},
	}
	for _, c := range cases {
		if got := renderFlags(c.e, false, c.tagged).Plain(); got != c.want {
			t.Errorf("%s: flags %q, want %q", c.name, got, c.want)
		}
	}
}

// A message whose size is unknown shows no size rather than "(0)".
func TestRenderSize_UnknownIsBlank(t *testing.T) {
	if got := renderSize(email.Email{}, false).Plain(); strings.TrimSpace(got) != "" || len(got) != sizeColumnWidth {
		t.Errorf("unknown size rendered %q", got)
	}
	if got := renderSize(email.Email{Size: 1536}, false).Plain(); got != "(1.5K)" {
		t.Errorf("size rendered %q", got)
	}
}

// The hidden count fits two cells however large the thread, and an expanded
// thread hides nothing.
func TestRenderHidden_FitsTwoCells(t *testing.T) {
	for _, n := range []int{1, 42, 99, 100, 5000} {
		e := email.Email{ThreadHasReplies: true, ThreadCollapsed: true, ThreadRepliesCount: n}
		if got := renderHidden(e, false).Width(); got != hiddenWidth {
			t.Errorf("%d replies: hidden column %d cells wide", n, got)
		}
	}
	if got := renderHidden(email.Email{ThreadHasReplies: true, ThreadRepliesCount: 3}, false).Plain(); got != "  " {
		t.Errorf("an expanded thread hides nothing, got %q", got)
	}
}

// A row never draws wider than it was given. A narrow pane -- the attachment
// split at a 100-column terminal is 50 cells -- drops the size column and
// keeps the subject at IndexHeaderWidth wherever there is room for it.
func TestEmailRowStyled_FitsNarrowPane(t *testing.T) {
	e := email.Email{
		ID: "m1", Subject: "SUBJ and a long subject that will not fit", FromRaw: "Alice <alice@example.com>",
		FormattedDate: "11/19/26", Size: 34 * 1024, HasAttachment: true,
	}
	for _, width := range []int{40, 50, 56, 72, 73, 120} {
		for _, st := range []RowStyle{{}, {Highlight: true}} {
			row := EmailRowStyled(e, width, width-IndexHeaderWidth(width), 1, st)
			if got := row.Width(); got > width {
				t.Errorf("width %d %+v: row is %d cells", width, st, got)
			}
			plain := row.Plain()
			if got, want := runeWidthBefore(plain, "SUBJ"), IndexHeaderWidth(width); width-want >= len("SUBJ") && got != want {
				t.Errorf("width %d: subject at cell %d, want %d: %q", width, got, want, plain)
			}
			if hasSize := strings.Contains(plain, "(34K)"); hasSize != sizeFits(width) {
				t.Errorf("width %d: size shown %v, want %v", width, hasSize, sizeFits(width))
			}
		}
	}
}

// The count is the whole thread, the collapsed row included: a thread of four
// messages used to show 3, the replies hidden behind it, which read as a bug.
func TestRenderHidden_CountsTheWholeThread(t *testing.T) {
	for replies, want := range map[int]string{1: " 2", 3: " 4", 98: "99", 99: ">>"} {
		e := email.Email{ThreadHasReplies: true, ThreadCollapsed: true, ThreadRepliesCount: replies}
		if got := renderHidden(e, false).Plain(); got != want {
			t.Errorf("%d replies: count %q, want %q", replies, got, want)
		}
	}
}
