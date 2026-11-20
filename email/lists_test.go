package email

import (
	"errors"
	"net/mail"
	"strings"
	"testing"
)

func TestParseListID(t *testing.T) {
	tests := []struct {
		name, value, want string
	}{
		{"phrase and id", `Linux Kernel Mailing List <linux-kernel.vger.kernel.org>`, "linux-kernel.vger.kernel.org"},
		{"bare bracketed id", `<golang-nuts.googlegroups.com>`, "golang-nuts.googlegroups.com"},
		{"no brackets", `announce.example.org`, "announce.example.org"},
		{"quoted phrase", `"Dev, the list" <dev.lists.example.org>`, "dev.lists.example.org"},
		{"encoded phrase", `=?UTF-8?B?16jXqdeZ157XlA==?= <hebrew.lists.example.org>`, "hebrew.lists.example.org"},
		{"upper case folded", `<Dev.Lists.Example.ORG>`, "dev.lists.example.org"},
		{"empty", "", ""},
		{"unclosed bracket", `List <dev.example.org`, ""},
		{"no dot", `<localonly>`, ""},
		{"words without brackets", `some list name`, ""},
		{"escape sequence stripped", "<dev.\x1b[31mexample.org>", "dev.example.org"},
	}
	for _, tc := range tests {
		got := ParseListID(tc.value)
		if got == tc.want {
			continue
		}
		t.Errorf("%s: ParseListID(%q) = %q, want %q", tc.name, tc.value, got, tc.want)
		if tc.want != "" && ListBadge(got) != ListBadge(tc.want) {
			t.Errorf("%s: the badge would read %q, not %q", tc.name, ListBadge(got), ListBadge(tc.want))
		}
	}
}

func TestListBadge(t *testing.T) {
	tests := map[string]string{
		"linux-kernel.vger.kernel.org":          "linux-kernel",
		"dev.lists.example.org":                 "dev",
		"averyveryverylonglistname.example.org": "averyveryver",
		"":                                      "",
		"rshimon-ha-ivri.lists.example.org":     "rshimon-ha-i",
		"עברית.example.org":                     "עברית",
	}
	for id, want := range tests {
		if got := ListBadge(id); got != want {
			t.Errorf("ListBadge(%q) = %q, want %q", id, got, want)
		}
	}
}

func headerWith(listPost string) mail.Header {
	h := mail.Header{}
	if listPost != "" {
		h["List-Post"] = []string{listPost}
	}
	return h
}

func TestListReplyAddress(t *testing.T) {
	tests := []struct {
		name, value, want string
		err               error
	}{
		{"plain mailto", `<mailto:dev@lists.example.org>`, "dev@lists.example.org", nil},
		{"comment after uri", `<mailto:dev@lists.example.org> (Postings are moderated)`, "dev@lists.example.org", nil},
		{"query dropped", `<mailto:dev@lists.example.org?subject=list%20post>`, "dev@lists.example.org", nil},
		{"escaped address", `<mailto:dev%2Bpost@lists.example.org>`, "dev+post@lists.example.org", nil},
		{"web first, mailto second", `<https://lists.example.org/post>, <mailto:dev@lists.example.org>`, "dev@lists.example.org", nil},
		{"upper-case scheme", `<MAILTO:dev@lists.example.org>`, "dev@lists.example.org", nil},
		{"closed list", `NO`, "", ErrListClosed},
		{"closed list with comment", `NO (posting not allowed on this list)`, "", ErrListClosed},
		{"closed list lower case", `no`, "", ErrListClosed},
		{"no header", "", "", ErrNoListPost},
		{"web only", `<https://lists.example.org/post>`, "", ErrListPostNoMailto},
		{"two addresses in one uri", `<mailto:a@example.org,b@example.org>`, "", ErrListPostNoMailto},
		{"not an address", `<mailto:not an address>`, "", ErrListPostNoMailto},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ListReplyAddress(headerWith(tc.value))
			if !errors.Is(err, tc.err) {
				t.Fatalf("ListReplyAddress(%q) error = %v, want %v", tc.value, err, tc.err)
			}
			if got != tc.want {
				t.Errorf("ListReplyAddress(%q) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}

const listMessage = "From: Alice <alice@example.com>\r\n" +
	"To: dev@lists.example.org\r\n" +
	"Cc: carol@example.com\r\n" +
	"Reply-To: alice-private@example.com\r\n" +
	"Subject: [dev] release plan\r\n" +
	"Message-ID: <plan@example.com>\r\n" +
	"References: <root@example.com>\r\n" +
	"List-Id: Developers <dev.lists.example.org>\r\n" +
	"List-Post: <mailto:dev@lists.example.org>\r\n" +
	"\r\n" +
	"the plan\r\n"

// A list reply goes to the list alone: not to the Reply-To the sender set,
// not to the Cc, and with the same threading headers as any reply.
func TestBuildListReplyDetails(t *testing.T) {
	orig, err := mail.ReadMessage(strings.NewReader(listMessage))
	if err != nil {
		t.Fatal(err)
	}
	details, err := BuildListReplyDetails(orig)
	if err != nil {
		t.Fatalf("BuildListReplyDetails: %v", err)
	}
	if details.To != "dev@lists.example.org" {
		t.Errorf("To = %q, want the list address", details.To)
	}
	if len(details.Cc) != 0 {
		t.Errorf("Cc = %v, want none", details.Cc)
	}
	if details.Subject != "Re: [dev] release plan" {
		t.Errorf("Subject = %q", details.Subject)
	}
	if details.InReplyTo != "<plan@example.com>" || details.References != "<root@example.com> <plan@example.com>" {
		t.Errorf("threading headers = %q / %q", details.InReplyTo, details.References)
	}
}

// A message from no list, or from a closed one, is refused rather than
// answered by mailing the sender.
func TestBuildListReplyDetailsRefuses(t *testing.T) {
	noList := strings.Replace(listMessage, "List-Post: <mailto:dev@lists.example.org>\r\n", "", 1)
	closed := strings.Replace(listMessage, "<mailto:dev@lists.example.org>", "NO", 1)
	for name, tc := range map[string]struct {
		raw  string
		want error
	}{
		"no List-Post": {noList, ErrNoListPost},
		"closed list":  {closed, ErrListClosed},
	} {
		t.Run(name, func(t *testing.T) {
			orig, err := mail.ReadMessage(strings.NewReader(tc.raw))
			if err != nil {
				t.Fatal(err)
			}
			details, err := BuildListReplyDetails(orig)
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
			if details.To != "" {
				t.Errorf("To = %q on a refused list reply", details.To)
			}
		})
	}
	if _, err := BuildListReplyDetails(nil); !errors.Is(err, ErrNoListPost) {
		t.Errorf("nil message: error = %v", err)
	}
}

func TestParseReaderReadsListID(t *testing.T) {
	em := ParseReader(strings.NewReader(listMessage), "id1", "")
	if em == nil {
		t.Fatal("ParseReader returned nil")
	}
	if em.ListID != "dev.lists.example.org" {
		t.Errorf("ListID = %q", em.ListID)
	}
	plain := ParseReader(strings.NewReader("From: a@example.com\r\nSubject: hi\r\n\r\nbody\r\n"), "id2", "")
	if plain == nil || plain.ListID != "" {
		t.Errorf("a message with no List-Id has ListID %q", plain.ListID)
	}
}
