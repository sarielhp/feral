package email

import "testing"

// A thread's root is the first References entry, else In-Reply-To, else the
// message's own Message-ID; an empty <> names nothing.
func TestThreadRoot(t *testing.T) {
	tests := []struct {
		name string
		em   *Email
		want HeaderID
	}{
		{"references first", &Email{MessageID: "<c@x>", InReplyTo: "<b@x>", References: "<a@x> <b@x>"}, "<a@x>"},
		{"in-reply-to alone", &Email{MessageID: "<c@x>", InReplyTo: "<b@x>"}, "<b@x>"},
		{"a thread's first message", &Email{MessageID: "<a@x>"}, "<a@x>"},
		{"empty references skipped", &Email{MessageID: "<c@x>", References: "<>"}, "<c@x>"},
		{"nothing", &Email{Subject: "hi"}, ""},
		{"nil", nil, ""},
	}
	for _, tc := range tests {
		if got := ThreadRoot(tc.em); got != tc.want {
			t.Errorf("%s: ThreadRoot = %q, want %q", tc.name, got, tc.want)
		}
	}
	ids := ThreadIDs(&Email{MessageID: "<c@x>", InReplyTo: "<b@x>", References: "<a@x> <b@x>"})
	if len(ids) != 4 || ids[0] != "<a@x>" || ids[3] != "<c@x>" {
		t.Errorf("ThreadIDs = %v", ids)
	}
	if ThreadIDs(nil) != nil {
		t.Error("ThreadIDs(nil) named IDs")
	}
}
