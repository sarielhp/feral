package email

import "testing"

// The expectations are visual order for a terminal that draws cells left to
// right in the order it receives them: a Hebrew word reads correctly when its
// letters arrive last-to-first.
func TestBidiDisplay(t *testing.T) {
	link := FormatOSC8Link("https://example.com/x", "https://example.com/x")
	// A case with forced set uses BidiDisplayIn with dir; the others let
	// BidiDisplay take the direction from the first strong character.
	tests := []struct {
		name, in, want string
		forced         bool
		dir            Direction
	}{
		{name: "empty", in: "", want: ""},
		{name: "ascii unchanged", in: "hello world", want: "hello world"},
		{name: "hebrew word", in: "שלום", want: "םולש"},
		{name: "hebrew words reverse as a line", in: "שלום עולם", want: "םלוע םולש"},
		{name: "hebrew inside english keeps its place", in: "abc שלום def", want: "abc םולש def"},
		{name: "english after hebrew moves to the left", in: "שלום abc", want: "abc םולש"},
		{name: "english then hebrew then number", in: "abc שלום 123", want: "abc 123 םולש"},
		{name: "hebrew number english", in: "שלום 123 abc", want: "abc 123 םולש"},
		{name: "decimal inside hebrew stays a number", in: "סעיף 3.3 מצורפת", want: "תפרוצמ 3.3 ףיעס"},
		{name: "two numbers keep their order within a hebrew line", in: "שלום 12 34", want: "34 12 םולש"},
		{name: "percent travels with its digits", in: "הנחה 50% היום", want: "םויה 50% החנה"},
		{name: "trailing period ends the sentence on the left", in: "שלום.", want: ".םולש"},
		{name: "quote prefix stays at the margin", in: "> שלום עולם", want: "> םלוע םולש"},
		{name: "nested quote prefix stays at the margin", in: ">> > שלום", want: ">> > םולש"},
		{name: "indentation stays at the margin", in: "  שלום", want: "  םולש"},
		{name: "trailing whitespace stays at the end", in: "שלום  ", want: "םולש  "},
		{name: "parentheses are mirrored", in: "שלום (עולם)", want: "(םלוע) םולש"},
		{name: "angle brackets are mirrored", in: "מאת <x@y.com>:", want: ":<x@y.com> תאמ"},
		{name: "combining marks follow their letter", in: "שָׁלוֹם", want: "םוֹלשָׁ"},
		{name: "hyperlink is one left-to-right unit", in: "ראו " + link + " כאן", want: "ןאכ " + link + " ואר"},
		{name: "hyperlink in english line with hebrew", in: "see " + link + " שלום", want: "see " + link + " םולש"},
		{name: "arabic", in: "مرحبا بالعالم", want: "ملاعلاب ابحرم"},
		{name: "direction mark alone is not reordered", in: "\u200f<x@y.com>", want: "\u200f<x@y.com>"},
		{name: "hebrew after a mark", in: "מאת Haim \u200f<a@b.c>:", want: ":<a@b.c>\u200f Haim תאמ"},
		{name: "hebrew-first line forced left-to-right", in: "שלום abc", forced: true, dir: LeftToRight, want: "םולש abc"},
		{name: "english-first line forced right-to-left", in: "abc שלום", forced: true, dir: RightToLeft, want: "םולש abc"},
		{name: "number after hebrew in a forced left-to-right line", in: "שלום 123 abc", forced: true, dir: LeftToRight, want: "123 םולש abc"},
		{name: "quote prefix stays under the forced direction", in: "> abc שלום", forced: true, dir: RightToLeft, want: "> םולש abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BidiDisplay(tt.in)
			if tt.forced {
				got = BidiDisplayIn(tt.in, tt.dir)
			}
			if got != tt.want {
				t.Errorf("%q (forced=%v dir=%v)\n got %q\nwant %q", tt.in, tt.forced, tt.dir, got, tt.want)
			}
		})
	}
}

func TestBaseDirection(t *testing.T) {
	tests := []struct {
		in   string
		want Direction
	}{
		{"", LeftToRight},
		{"123 - !", LeftToRight},
		{"hello שלום", LeftToRight},
		{"שלום hello", RightToLeft},
		{"> 12 שלום", RightToLeft},
		{"\u200fabc", RightToLeft},
		{"مرحبا", RightToLeft},
	}
	for _, tt := range tests {
		got := BaseDirection(tt.in)
		if got != tt.want {
			t.Errorf("BaseDirection(%q) = %v, want %v", tt.in, got, tt.want)
		}
		// BidiDisplay is BidiDisplayIn under the direction BaseDirection finds.
		if shown, forced := BidiDisplay(tt.in), BidiDisplayIn(tt.in, got); shown != forced {
			t.Errorf("BidiDisplay(%q) = %q but BidiDisplayIn with %v gives %q", tt.in, shown, got, forced)
		}
	}
}

// A reordered line is a permutation of its input, brackets taken up to
// mirroring: nothing is lost or invented, whatever the mix of scripts,
// digits, escapes and marks.
func TestBidiDisplayPreservesCharacters(t *testing.T) {
	inputs := []string{
		"שלום עולם 123 abc (x) [y] {z} <w>",
		"\x1b[1mשלום\x1b[0m world",
		FormatOSC8Link("https://a.b/", "https://a.b/") + " שלום",
		"שָׁלוֹם עֲלֵיכֶם",
		"\x1b]8;;unterminated שלום",
	}
	for _, in := range inputs {
		got := BidiDisplay(in)
		if sortRunes(got) != sortRunes(in) {
			t.Errorf("BidiDisplay(%q) = %q: not a permutation of the input", in, got)
		}
	}
}

func sortRunes(s string) string {
	rs := []rune(s)
	for i, r := range rs {
		if m, ok := mirrored[r]; ok && m < r {
			rs[i] = m
		}
	}
	for i := 1; i < len(rs); i++ {
		for j := i; j > 0 && rs[j-1] > rs[j]; j-- {
			rs[j-1], rs[j] = rs[j], rs[j-1]
		}
	}
	return string(rs)
}
