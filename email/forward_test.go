package email

import (
	"net/mail"
	"strings"
	"testing"
)

func TestFormatForwardSubject(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain subject", "Lunch on Friday", "Fwd: Lunch on Friday"},
		{"empty subject", "", "Fwd: "},
		{"whitespace subject", "   ", "Fwd: "},
		{"already forwarded", "Fwd: Lunch", "Fwd: Lunch"},
		{"already forwarded, short marker", "fw: Lunch", "fw: Lunch"},
		{"forward marker uppercase", "FWD: Lunch", "FWD: Lunch"},
		{"a reply subject is still forwarded", "Re: Lunch", "Fwd: Re: Lunch"},
		{"surrounding whitespace is trimmed", " Lunch ", "Fwd: Lunch"},
		{"forward marker with no space", "fwd:Lunch", "fwd:Lunch"},
	}
	for _, tt := range tests {
		if got := FormatForwardSubject(tt.in); got != tt.want {
			t.Errorf("%s: FormatForwardSubject(%q) = %q, want %q", tt.name, tt.in, got, tt.want)
		}
	}
}

func forwardTestMessage(t *testing.T) *mail.Message {
	t.Helper()
	raw := "From: Alice <alice@example.com>\r\n" +
		"To: Bob <bob@example.com>\r\n" +
		"Cc: Carol <carol@example.com>\r\n" +
		"Subject: Lunch on Friday\r\n" +
		"Date: Sun, 11 Oct 2026 12:00:00 +0000\r\n" +
		"\r\n" +
		"Are we still on for Friday?\r\n"
	msg, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	return msg
}

func TestFormatForward(t *testing.T) {
	got := FormatForward(forwardTestMessage(t), "Are we still on for Friday?\r\nSee you there.")
	want := "---------- Forwarded email ----------\n" +
		"From: Alice <alice@example.com>\n" +
		"Date: Sun, 11 Oct 2026 12:00:00 +0000\n" +
		"To: Bob <bob@example.com>\n" +
		"Cc: Carol <carol@example.com>\n" +
		"Subject: Lunch on Friday\n" +
		"\n" +
		"Are we still on for Friday?\nSee you there.\n" +
		"---------- End of forwarded email ----------\n"
	if got != want {
		t.Errorf("FormatForward =\n%q\nwant\n%q", got, want)
	}
}

func TestFormatForwardWithoutOptionalHeaders(t *testing.T) {
	raw := "From: Alice <alice@example.com>\r\nSubject: Lunch\r\n\r\nBody text.\r\n"
	msg, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}

	got := FormatForward(msg, "Body text.")
	for _, absent := range []string{"Date:", "To:", "Cc:"} {
		if strings.Contains(got, absent) {
			t.Errorf("a header the original lacks was invented: %s in\n%s", absent, got)
		}
	}
	for _, want := range []string{"From: Alice <alice@example.com>\n", "Subject: Lunch\n", "Body text.\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("forward block lost %q:\n%s", want, got)
		}
	}
}

func TestFormatForwardEmptyBody(t *testing.T) {
	got := FormatForward(forwardTestMessage(t), "")
	want := "---------- Forwarded email ----------\n" +
		"From: Alice <alice@example.com>\n" +
		"Date: Sun, 11 Oct 2026 12:00:00 +0000\n" +
		"To: Bob <bob@example.com>\n" +
		"Cc: Carol <carol@example.com>\n" +
		"Subject: Lunch on Friday\n" +
		"\n" +
		"---------- End of forwarded email ----------\n"
	if got != want {
		t.Errorf("FormatForward of an empty body =\n%q\nwant\n%q", got, want)
	}
}
