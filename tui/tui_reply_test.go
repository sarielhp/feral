package tui

import (
	"net/mail"
	"strings"
	"testing"

	"github.com/sarielhp/feral/email"
)

// A reply to a message the user wrote addresses the original recipients; a
// reply to received mail addresses the sender. The To header decides it only
// for authored mail, and a group reply's Cc differs accordingly: the original
// Cc for authored mail, the original's other recipients for received mail.
func TestReplyRecipients(t *testing.T) {
	authoredRaw := "From: Me <me@example.com>\r\n" +
		"To: Bob <bob@example.com>, Cara <cara@example.com>\r\n" +
		"Cc: Dave <dave@example.com>, Me <me@example.com>\r\n" +
		"Subject: Plan\r\n\r\nbody"
	authored, err := mail.ReadMessage(strings.NewReader(authoredRaw))
	if err != nil {
		t.Fatalf("parsing authored message: %v", err)
	}

	receivedRaw := "From: Alice <alice@example.com>\r\n" +
		"To: Me <me@example.com>, Bob <bob@example.com>\r\n" +
		"Cc: Carol <carol@example.com>\r\n" +
		"Subject: Hello\r\n\r\nbody"
	received, err := mail.ReadMessage(strings.NewReader(receivedRaw))
	if err != nil {
		t.Fatalf("parsing received message: %v", err)
	}

	to, cc := replyRecipients(authored, true, false, email.OwnSet("me@example.com"))
	if to != "Bob <bob@example.com>, Cara <cara@example.com>" {
		t.Errorf("authored single reply To = %q; want the original recipients", to)
	}
	if cc != nil {
		t.Errorf("authored single reply Cc = %v; want none", cc)
	}

	to, cc = replyRecipients(authored, true, true, email.OwnSet("me@example.com"))
	if to != "Bob <bob@example.com>, Cara <cara@example.com>" {
		t.Errorf("authored group reply To = %q", to)
	}
	if len(cc) != 1 || cc[0] != `"Dave" <dave@example.com>` {
		t.Errorf("authored group reply Cc = %v; want the original Cc minus our own address", cc)
	}

	to, cc = replyRecipients(received, false, false, email.OwnSet("me@example.com"))
	if to != "Alice <alice@example.com>" {
		t.Errorf("received single reply To = %q; want the sender", to)
	}
	if cc != nil {
		t.Errorf("received single reply Cc = %v; want none", cc)
	}

	to, cc = replyRecipients(received, false, true, email.OwnSet("me@example.com"))
	if to != "Alice <alice@example.com>" {
		t.Errorf("received group reply To = %q", to)
	}
	if len(cc) != 2 || cc[0] != `"Bob" <bob@example.com>` || cc[1] != `"Carol" <carol@example.com>` {
		t.Errorf("received group reply Cc = %v; want the original's other recipients", cc)
	}
}
