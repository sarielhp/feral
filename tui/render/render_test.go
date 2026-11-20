package render

import (
	"strings"
	"testing"

	"github.com/sarielhp/feral/email"
)

func TestRenderEmailRow_ColumnOrder(t *testing.T) {
	theme := NewDefaultTheme()
	e := email.Email{
		ID:            "msg100",
		Subject:       "Invoice Report",
		FromRaw:       "Alice <alice@example.com>",
		FormattedDate: "12:00",
		IsRead:        false,
		HasAttachment: true,
	}

	row := EmailRow(e, 80, 20, 1, theme, false, false)
	rowPlain := row.Plain()

	// Order must be counter, sep, nBadge, sep, date, sep, sender, sep, count, sep, subj
	// Counter is "   1", nBadge has "N" and "📎"
	counterIdx := strings.Index(rowPlain, "   1")
	badgeIdx := strings.Index(rowPlain, "N")
	attIdx := strings.Index(rowPlain, "📎")
	dateIdx := strings.Index(rowPlain, "12:00")
	senderIdx := strings.Index(rowPlain, "Alice")
	subjIdx := strings.Index(rowPlain, "Invoice Report")

	if counterIdx == -1 || badgeIdx == -1 || attIdx == -1 || dateIdx == -1 || senderIdx == -1 || subjIdx == -1 {
		t.Fatalf("missing expected column content in rendered row: %q", rowPlain)
	}

	if !(counterIdx < badgeIdx && badgeIdx < dateIdx && dateIdx < senderIdx && senderIdx < subjIdx) {
		t.Errorf("column order mismatch: counter=%d, badge=%d, date=%d, sender=%d, subj=%d. Row: %q",
			counterIdx, badgeIdx, dateIdx, senderIdx, subjIdx, rowPlain)
	}
}

func TestRenderEmailRowTagged(t *testing.T) {
	theme := NewDefaultTheme()
	e := email.Email{
		ID:            "msg100",
		Subject:       "Invoice Report",
		FromRaw:       "Alice <alice@example.com>",
		FormattedDate: "12:00",
		IsRead:        true,
	}

	rowUntagged := EmailRowTagged(e, 80, 20, 1, theme, false, false, false)
	rowTagged := EmailRowTagged(e, 80, 20, 1, theme, false, false, true)

	if strings.Contains(rowUntagged.Plain(), "*") {
		t.Errorf("untagged row should not contain '*', got %q", rowUntagged.Plain())
	}
	if !strings.Contains(rowTagged.Plain(), "*") {
		t.Errorf("tagged row must contain '*' in badge, got %q", rowTagged.Plain())
	}
}

func TestRenderEmailRow_AccountBadge(t *testing.T) {
	theme := NewDefaultTheme()
	e := email.Email{
		ID:            "msg200",
		Account:       "Work",
		Subject:       "Quarterly Review",
		FromRaw:       "Boss <boss@example.com>",
		FormattedDate: "13:00",
		IsRead:        true,
	}

	// No badge configured: the account's initial.
	row := EmailRow(e, 80, 40, 1, theme, false, false)
	if !strings.Contains(row.Plain(), "W  Quarterly Review") {
		t.Errorf("expected 'W  Quarterly Review' in row, got %q", row.Plain())
	}

	// A configured badge replaces it, and an emoji's double width does not
	// shift the row: it is exactly as wide as the one with a letter.
	e.AccountBadge = "💼"
	emojiRow := EmailRow(e, 80, 40, 1, theme, false, false)
	if !strings.Contains(emojiRow.Plain(), "💼 Quarterly Review") {
		t.Errorf("expected the configured badge in row, got %q", emojiRow.Plain())
	}
	if emojiRow.Width() != row.Width() {
		t.Errorf("emoji badge changed the row width: %d, want %d", emojiRow.Width(), row.Width())
	}
}

// Accounts are told apart by colour: each position in the configuration gets
// its own, and the palette repeats rather than running out.
func TestAccountColorDistinguishesAccounts(t *testing.T) {
	badgeFg := func(idx int) ColorID {
		e := email.Email{Account: "Work", AccountIndex: idx, Subject: "s"}
		for _, seg := range renderSubject(e, 40, false).Segments {
			if strings.TrimSpace(seg.Text) == "W" {
				return seg.Fg
			}
		}
		t.Fatalf("no badge segment for account %d", idx)
		return ColorDefault
	}
	seen := map[ColorID]bool{}
	for idx := range accountColors {
		fg := badgeFg(idx)
		if seen[fg] {
			t.Errorf("account %d reuses colour %d", idx, fg)
		}
		seen[fg] = true
		if NewDefaultTheme().Get(fg) == NewDefaultTheme().Get(ColorFg) && fg != ColorFg {
			t.Errorf("account colour %d has no theme entry", fg)
		}
	}
	if badgeFg(accountColors) != badgeFg(0) {
		t.Errorf("the palette should wrap after %d accounts", accountColors)
	}
}

// A row from a Sent folder shows the recipient, marked, in the sender column; a
// received row shows the sender.
func TestRenderSenderShowsRecipientForAuthoredMail(t *testing.T) {
	sent := email.Email{FromRaw: "Me <me@example.com>", ToRaw: "Bob <bob@example.com>", IsAuthored: true}
	got := renderSender(sent, false).Plain()
	if !strings.Contains(got, email.RecipientMarker+"Bob") {
		t.Errorf("authored sender column = %q; want the marked recipient", got)
	}
	if strings.Contains(got, "me@example.com") {
		t.Errorf("authored sender column = %q; still shows the sender", got)
	}

	received := email.Email{FromRaw: "Alice <alice@example.com>", ToRaw: "Me <me@example.com>"}
	if got := renderSender(received, false).Plain(); !strings.Contains(got, "Alice") {
		t.Errorf("received sender column = %q; want the sender", got)
	}
}

// A message from a list carries the list's tag ahead of its subject, inside
// the subject column, so the columns before it keep their place and a row
// still fits its width. A subject that already names the list is not tagged
// twice.
func TestRenderEmailRowListBadge(t *testing.T) {
	theme := NewDefaultTheme()
	plain := email.Email{ID: "m1", Subject: "release plan", FromRaw: "Alice <alice@example.com>", FormattedDate: "12:00"}
	listed := plain
	listed.ListID = "dev.lists.example.org"

	plainRow := EmailRow(plain, 80, 30, 1, theme, false, false).Plain()
	listedRow := EmailRow(listed, 80, 30, 1, theme, false, false).Plain()
	if !strings.Contains(listedRow, "[dev] release plan") {
		t.Errorf("list row = %q, want the [dev] tag before the subject", listedRow)
	}
	if strings.Contains(plainRow, "[dev]") {
		t.Errorf("a message from no list was tagged: %q", plainRow)
	}
	if strings.Index(listedRow, "[dev]") != strings.Index(plainRow, "release plan") {
		t.Errorf("the tag moved the subject column: %q vs %q", listedRow, plainRow)
	}
	for _, width := range []int{80, 60} {
		row := EmailRow(listed, width, 30, 1, theme, true, false)
		if got := row.Width(); got > width {
			t.Errorf("width %d: row is %d cells wide", width, got)
		}
	}

	tagged := listed
	tagged.Subject = "[DEV] release plan"
	if row := EmailRow(tagged, 80, 30, 1, theme, false, false).Plain(); strings.Count(strings.ToLower(row), "[dev]") != 1 {
		t.Errorf("a subject carrying the tag was tagged again: %q", row)
	}
}
