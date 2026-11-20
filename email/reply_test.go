package email

import (
	"net/mail"
	"slices"
	"strings"
	"testing"
)

// A group reply leaves out every one of the user's own addresses, not only the
// account's send address: my_emails and other accounts' addresses too, in To
// and Cc alike, whatever their case.
func TestBuildReplyDetails_GroupExcludesAllOwnAddresses(t *testing.T) {
	raw := "From: Alice <alice@example.com>\r\n" +
		"To: Me <me@example.com>, Bob <bob@example.com>, Alias <ME2@Other.example>\r\n" +
		"Cc: Work <me@work.example>, Carol <carol@example.com>\r\n" +
		"Subject: Hi\r\n\r\nBody\r\n"
	msg, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	own := OwnSet("me@example.com", "me2@other.example", "me@work.example")
	got := BuildReplyDetails(msg, own, true, false).Cc
	want := []string{`"Bob" <bob@example.com>`, `"Carol" <carol@example.com>`}
	if !slices.Equal(got, want) {
		t.Errorf("group reply Cc = %v, want %v", got, want)
	}
	if len(own) != 3 {
		t.Errorf("BuildReplyDetails changed the caller's own set: %v", own)
	}

	authored := strings.Replace(raw, "Cc: Work", "Cc: Bob2 <bob2@example.com>, Work", 1)
	msg, err = mail.ReadMessage(strings.NewReader(authored))
	if err != nil {
		t.Fatal(err)
	}
	got = BuildReplyDetails(msg, own, true, true).Cc
	want = []string{`"Bob2" <bob2@example.com>`, `"Carol" <carol@example.com>`}
	if !slices.Equal(got, want) {
		t.Errorf("authored group reply Cc = %v, want %v", got, want)
	}
}

func TestFormatReplySubject(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", "Re: "},
		{"   ", "Re: "},
		{"Hello world", "Re: Hello world"},
		{"Re: Hello world", "Re: Hello world"},
		{"re: Hello world", "re: Hello world"},
		{"RE: Hello world", "RE: Hello world"},
		{"  Re: Already replied  ", "Re: Already replied"},
	}

	for i, tc := range tests {
		got := formatReplySubject(tc.input)
		if got == tc.want {
			continue
		}
		t.Errorf("case %d: formatReplySubject(%q) = %q; want %q", i, tc.input, got, tc.want)
	}
}

func TestBuildGroupReplyCC(t *testing.T) {
	raw := "From: Alice <alice@example.com>\r\n" +
		"To: Bob <bob@example.com>, Me <me@example.com>\r\n" +
		"Cc: Charlie <charlie@example.com>, Alice <alice@example.com>, David <david@example.com>\r\n" +
		"\r\nBody\r\n"

	msg, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("failed to parse test message: %v", err)
	}

	cc := BuildGroupReplyCC(msg, "alice@example.com", OwnSet("me@example.com"))
	expected := []string{"\"Bob\" <bob@example.com>", "\"Charlie\" <charlie@example.com>", "\"David\" <david@example.com>"}
	if len(cc) != len(expected) {
		t.Fatalf("BuildGroupReplyCC returned %d addresses (%v); want %d (%v)", len(cc), cc, len(expected), expected)
	}
	for i, want := range expected {
		if cc[i] != want {
			t.Errorf("cc[%d] = %q; want %q", i, cc[i], want)
		}
	}
}

func TestBuildReplyDetails_Single(t *testing.T) {
	raw := "From: Alice <alice@example.com>\r\n" +
		"Subject: Important Update\r\n" +
		"Message-ID: <msg123@example.com>\r\n" +
		"Date: Sun, 20 Sep 2026 12:00:00 +0000\r\n" +
		"Content-Type: text/plain\r\n" +
		"\r\n" +
		"Here is the plan for tomorrow.\nLet me know what you think.\n"

	msg, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("failed to parse message: %v", err)
	}

	details := BuildReplyDetails(msg, OwnSet("me@example.com"), false, false)

	if details.To != "Alice <alice@example.com>" {
		t.Errorf("To = %q; want Alice <alice@example.com>", details.To)
	}
	if len(details.Cc) != 0 {
		t.Errorf("Cc length = %d; want 0", len(details.Cc))
	}
	if details.Subject != "Re: Important Update" {
		t.Errorf("Subject = %q; want Re: Important Update", details.Subject)
	}
	if details.InReplyTo != "<msg123@example.com>" {
		t.Errorf("InReplyTo = %q; want <msg123@example.com>", details.InReplyTo)
	}
	if details.References != "<msg123@example.com>" {
		t.Errorf("References = %q; want <msg123@example.com>", details.References)
	}
	if !strings.Contains(details.QuotedBody, "On Sun, 20 Sep 2026 12:00:00 +0000, Alice <alice@example.com> wrote:") {
		t.Errorf("QuotedBody missing attribution header: %q", details.QuotedBody)
	}
	if !strings.Contains(details.QuotedBody, "> Here is the plan for tomorrow.") {
		t.Errorf("QuotedBody missing quoted line: %q", details.QuotedBody)
	}
}

func TestBuildReplyDetails_ReplyToAndReferences(t *testing.T) {
	raw := "From: Alice <alice@example.com>\r\n" +
		"Reply-To: Support Team <support@example.com>\r\n" +
		"Subject: Re: Existing Discussion\r\n" +
		"Message-ID: <msg456@example.com>\r\n" +
		"References: <root123@example.com>\r\n" +
		"Content-Type: text/plain\r\n" +
		"\r\n" +
		"Please reply to this ticket."

	msg, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("failed to parse message: %v", err)
	}

	details := BuildReplyDetails(msg, OwnSet("me@example.com"), false, false)

	if details.To != "Support Team <support@example.com>" {
		t.Errorf("To = %q; want Support Team <support@example.com>", details.To)
	}
	if details.Subject != "Re: Existing Discussion" {
		t.Errorf("Subject = %q; want Re: Existing Discussion", details.Subject)
	}
	if details.InReplyTo != "<msg456@example.com>" {
		t.Errorf("InReplyTo = %q; want <msg456@example.com>", details.InReplyTo)
	}
	if details.References != "<root123@example.com> <msg456@example.com>" {
		t.Errorf("References = %q; want '<root123@example.com> <msg456@example.com>'", details.References)
	}
}

// Replying to a message the user wrote addresses the people it was sent to,
// not the user's own address: the original To becomes the reply's To, and a
// group reply's Cc is the original Cc rather than a copy of the recipients.
func TestBuildReplyDetails_Authored(t *testing.T) {
	raw := "From: Me <me@example.com>\r\n" +
		"To: Bob <bob@example.com>, Cara <cara@example.com>\r\n" +
		"Cc: Dave <dave@example.com>, Me <me@example.com>\r\n" +
		"Subject: The plan\r\n" +
		"Message-ID: <sent1@example.com>\r\n" +
		"Content-Type: text/plain\r\n" +
		"\r\nHere is the plan."

	msg, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("failed to parse message: %v", err)
	}

	single := BuildReplyDetails(msg, OwnSet("me@example.com"), false, true)
	if single.To != "Bob <bob@example.com>, Cara <cara@example.com>" {
		t.Errorf("authored reply To = %q; want the original recipients", single.To)
	}
	if len(single.Cc) != 0 {
		t.Errorf("authored single reply Cc = %v; want none", single.Cc)
	}
	if single.Subject != "Re: The plan" {
		t.Errorf("authored reply Subject = %q", single.Subject)
	}

	group := BuildReplyDetails(msg, OwnSet("me@example.com"), true, true)
	if len(group.Cc) != 1 || group.Cc[0] != `"Dave" <dave@example.com>` {
		t.Errorf("authored group reply Cc = %v; want just the original Cc, minus our own address", group.Cc)
	}

	// The same message answered as if received still goes to its From.
	if got := BuildReplyDetails(msg, OwnSet("me@example.com"), false, false).To; got != "Me <me@example.com>" {
		t.Errorf("non-authored reply To = %q; want the From", got)
	}
}

func TestBuildReplyDetails_Nil(t *testing.T) {
	details := BuildReplyDetails(nil, OwnSet("me@example.com"), false, false)
	if details.To != "" || details.Subject != "" {
		t.Errorf("BuildReplyDetails(nil) returned non-empty details: %+v", details)
	}
}
