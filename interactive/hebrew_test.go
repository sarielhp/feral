package interactive

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestHebrewBodyArrivesInVisualOrder opens a plain-text Hebrew message and
// checks what the terminal received. kitty draws cells in the order they
// arrive and applies no bidi algorithm of its own, so a Hebrew line is
// legible only when its letters arrive last-to-first; a line that reached
// the terminal in logical order reads mirrored.
func TestHebrewBodyArrivesInVisualOrder(t *testing.T) {
	fixture, err := filepath.Abs(filepath.Join("..", "testdata", "interactive", "hebrew.eml"))
	if err != nil {
		t.Fatal(err)
	}
	s := start(t, "tui", fixture)
	s.WaitFor("Hebrew sample")
	s.Key("enter")
	screen := s.WaitFor("Hello from a left-to-right line.")

	for _, want := range []string{
		"Hebrew sample הקידב",        // the subject, reordered in the list and the header
		".הקידב תעדוה וז ,םלוע םולש", // the first line, period on the left
		"> .ףרוצמ 3.3 ףיעס",          // a quoted line: marker at the margin, the number intact
	} {
		if !strings.Contains(screen, want) {
			t.Errorf("terminal did not receive %q:\n%s", want, screen)
		}
	}
	for _, mirrored := range []string{"שלום עולם", "סעיף 3.3"} {
		if strings.Contains(screen, mirrored) {
			t.Errorf("terminal received %q in logical order, which kitty draws mirrored:\n%s", mirrored, screen)
		}
	}
}
