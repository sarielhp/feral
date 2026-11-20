package email

import (
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/bidi"
)

// A terminal draws cells in the order it receives them: neither kitty nor
// xterm applies the Unicode bidirectional algorithm, so a Hebrew or Arabic
// line sent in logical order appears mirrored. The visual order is computed
// here, one line at a time, before the text reaches the terminal.
//
// golang.org/x/text/unicode/bidi resolves a line into runs of alternating
// direction and stops there: Run.String returns a run "in its original
// order", and the embedding levels its rules computed are not exposed, so a
// BidiDisplay built on its runs alone was the identity. This file takes the
// runs, recovers the levels -- without explicit embedding characters UAX #9
// I1-I2 leave only levels 0-2, which the run direction and a run's leading
// digits determine -- and applies L2 (reversal), L3 (combining marks stay
// with their base) and L4 (bracket mirroring).

// Direction is the base direction of a paragraph.
type Direction int

const (
	// LeftToRight is the direction of a paragraph whose first strong
	// character is a left-to-right letter, and of one with no strong character.
	LeftToRight Direction = iota
	// RightToLeft is the direction of a paragraph whose first strong
	// character is a Hebrew or Arabic letter.
	RightToLeft
)

// BaseDirection is the direction of s's first strong character (UAX #9
// P2-P3); a line with none is left-to-right. A wrapped paragraph's lines
// should all take the direction of the paragraph, so compute it on the
// unwrapped line and pass it to BidiDisplayIn.
func BaseDirection(s string) Direction {
	for _, r := range s {
		p, _ := bidi.LookupRune(r)
		switch p.Class() {
		case bidi.L:
			return LeftToRight
		case bidi.R, bidi.AL:
			return RightToLeft
		}
	}
	return LeftToRight
}

// hasRTL reports whether s holds a right-to-left letter, the only case in
// which the visual order differs from the logical one. A direction mark
// alone (U+200F) does not count: without a letter there is nothing to reverse.
func hasRTL(s string) bool {
	for _, r := range s {
		if r < 0x0590 || !unicode.IsLetter(r) {
			continue
		}
		p, _ := bidi.LookupRune(r)
		if c := p.Class(); c == bidi.R || c == bidi.AL {
			return true
		}
	}
	return false
}

// BidiDisplay returns s in visual order for a terminal, taking the paragraph
// direction from its first strong character, and returns s unchanged when it
// holds no right-to-left letter.
func BidiDisplay(s string) string {
	return BidiDisplayIn(s, BaseDirection(s))
}

// BidiDisplayIn returns s in visual order for a paragraph of direction dir.
// Leading indentation and quote markers, and trailing whitespace, keep their
// place: the terminal lays every line out from the left margin, so a reversed
// line stays under its neighbours instead of moving its margin to the right.
func BidiDisplayIn(s string, dir Direction) string {
	if !hasRTL(s) {
		return s
	}
	body := strings.TrimLeft(s, " \t>")
	head := s[:len(s)-len(body)]
	tail := body[len(strings.TrimRight(body, " \t")):]
	body = body[:len(body)-len(tail)]

	atoms := splitAtoms(body)
	var shadow strings.Builder
	shadow.WriteRune(directionMark(dir))
	for _, a := range atoms {
		shadow.WriteRune(a.key)
	}
	var p bidi.Paragraph
	if _, err := p.SetString(shadow.String()); err != nil {
		return s
	}
	o, err := p.Order()
	if err != nil {
		return s
	}
	resolveLevels(atoms, &o, dir)
	reorder(atoms)

	var sb strings.Builder
	sb.WriteString(head)
	for _, a := range atoms {
		sb.WriteString(a.display())
	}
	sb.WriteString(tail)
	return sb.String()
}

// directionMark is the strong character that fixes a paragraph's direction
// when it opens the text (U+200E / U+200F); the mark is dropped from the output.
func directionMark(dir Direction) rune {
	if dir == RightToLeft {
		return '‏'
	}
	return '‎'
}

// atom is what the reordering moves as one unit: a character with the
// combining marks that follow it, or a whole OSC 8 hyperlink, kept together
// and ordered as left-to-right text. An escape sequence that is not a link
// has no width and travels with the character after it.
type atom struct {
	pre   string // escape sequences preceding the character
	key   rune   // the character the bidi algorithm sees; 'a' for a link
	body  string // the character and its combining marks, or the whole link
	level int
}

// display is the atom's text with its bracket mirrored when it sits in a
// right-to-left run (L4): a parenthesis that opened in logical order still
// opens visually.
func (a atom) display() string {
	m, ok := mirrored[a.key]
	if !ok || a.level%2 == 0 {
		return a.pre + a.body
	}
	return a.pre + string(m) + a.body[utf8.RuneLen(a.key):]
}

var mirrored = map[rune]rune{
	'(': ')', ')': '(', '[': ']', ']': '[', '{': '}', '}': '{',
	'<': '>', '>': '<', '«': '»', '»': '«', '‹': '›', '›': '‹',
}

var (
	osc8LinkAtStart = regexp.MustCompile(`^\x1b\]8;;[^\x1b\x07]*(?:\x1b\\|\x07).*?\x1b\]8;;(?:\x1b\\|\x07)`)
	oscAtStart      = regexp.MustCompile(`^\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)`)
	csiAtStart      = regexp.MustCompile(`^\x1b\[[0-?]*[ -/]*[@-~]`)
)

// splitAtoms cuts s into the units the reordering moves.
func splitAtoms(s string) []atom {
	var atoms []atom
	pending := "" // escapes waiting for the character they precede
	for s != "" {
		if s[0] == 0x1b {
			if m := osc8LinkAtStart.FindString(s); m != "" {
				atoms = append(atoms, atom{pre: pending, key: 'a', body: m})
				pending, s = "", s[len(m):]
				continue
			}
			n := escapeLen(s)
			pending, s = pending+s[:n], s[n:]
			continue
		}
		r, size := utf8.DecodeRuneInString(s)
		s = s[size:]
		if p, _ := bidi.LookupRune(r); p.Class() == bidi.NSM && len(atoms) > 0 && pending == "" {
			atoms[len(atoms)-1].body += string(r)
			continue
		}
		atoms = append(atoms, atom{pre: pending, key: r, body: string(r)})
		pending = ""
	}
	if pending == "" {
		return atoms
	}
	if len(atoms) == 0 {
		return []atom{{pre: pending}}
	}
	atoms[len(atoms)-1].body += pending
	return atoms
}

// escapeLen is the length of the escape sequence that opens s: an OSC up to
// its BEL or ST terminator, a CSI up to its final byte, or a two-byte escape.
func escapeLen(s string) int {
	if m := oscAtStart.FindString(s); m != "" {
		return len(m)
	}
	if m := csiAtStart.FindString(s); m != "" {
		return len(m)
	}
	return min(2, len(s))
}

// resolveLevels gives every atom its embedding level from the runs x/text
// computed over the shadow text, whose first rune is the direction mark. In a
// right-to-left paragraph, right-to-left text is level 1 and everything else
// level 2. In a left-to-right paragraph, right-to-left text is level 1,
// left-to-right text level 0, and digits following right-to-left text level
// 2; those digits are the only even-level characters a right-to-left run can
// be followed by, so a left-to-right run after one opens with a level-2
// cluster of digits and continues at level 0.
func resolveLevels(atoms []atom, o *bidi.Ordering, base Direction) {
	for i := 0; i < o.NumRuns(); i++ {
		run := o.Run(i)
		start, end := run.Pos()
		lo, hi := max(start-1, 0), end // atoms[lo:hi]; shadow position 0 is the mark
		if run.Direction() == bidi.RightToLeft {
			setLevel(atoms[lo:hi], 1)
			continue
		}
		if base == RightToLeft {
			setLevel(atoms[lo:hi], 2)
			continue
		}
		n := 0
		if start > 0 {
			n = numberClusterLen(atoms[lo:hi])
		}
		setLevel(atoms[lo:lo+n], 2)
		setLevel(atoms[lo+n:hi], 0)
	}
}

func setLevel(atoms []atom, level int) {
	for i := range atoms {
		atoms[i].level = level
	}
}

// numberClusterLen is the length of the run of digits that opens atoms,
// including the separators and terminators W4-W5 attach to digits: "1,000",
// "3.3", "$5", "50%".
func numberClusterLen(atoms []atom) int {
	classes := make([]bidi.Class, len(atoms))
	for i, a := range atoms {
		p, _ := bidi.LookupRune(a.key)
		classes[i] = p.Class()
	}
	isNumber := func(c bidi.Class) bool { return c == bidi.EN || c == bidi.AN }
	for i := 1; i+1 < len(classes); i++ { // W4: a single separator between two digits
		if (classes[i] == bidi.ES || classes[i] == bidi.CS) && isNumber(classes[i-1]) && classes[i-1] == classes[i+1] {
			classes[i] = classes[i-1]
		}
	}
	for i := 1; i < len(classes); i++ { // W5: a terminator after a digit
		if classes[i] == bidi.ET && classes[i-1] == bidi.EN {
			classes[i] = bidi.EN
		}
	}
	for i := len(classes) - 2; i >= 0; i-- { // W5: a terminator before a digit
		if classes[i] == bidi.ET && classes[i+1] == bidi.EN {
			classes[i] = bidi.EN
		}
	}
	n := 0
	for n < len(classes) && isNumber(classes[n]) {
		n++
	}
	return n
}

// reorder applies L2: from the highest level down to 1, every maximal
// sequence of atoms at that level or above is reversed.
func reorder(atoms []atom) {
	for lvl := 2; lvl >= 1; lvl-- {
		for i := 0; i < len(atoms); {
			if atoms[i].level < lvl {
				i++
				continue
			}
			j := i
			for j < len(atoms) && atoms[j].level >= lvl {
				j++
			}
			slices.Reverse(atoms[i:j])
			i = j
		}
	}
}
