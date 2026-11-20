package tui

import (
	"strings"
	"testing"

	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
)

// A message from one of the user's own addresses -- my_emails or an
// account's user_email -- is shown by its recipient, wherever it is filed.
func TestMarkOwnMail_ShowsRecipient(t *testing.T) {
	cfg := &cfg_g.Config{
		Accounts: []cfg_acc.AccountConfig{{Name: "work", UserEmail: "Me@Work.edu"}},
	}
	cfg.MyEmails = []string{"me@old.org"}
	m := &tuiModel{mode: modeIndex, cfg: cfg, rawEmails: []email.Email{
		{ID: "1", FromRaw: "Me <me@old.org>", ToRaw: "Bob <bob@x.com>"},
		{ID: "2", FromRaw: "me@work.edu", ToRaw: "Carol <carol@x.com>"},
		{ID: "3", FromRaw: "Dave <dave@x.com>", ToRaw: "me@old.org"},
	}}
	m.rebuildVisibleEmails()
	want := map[email.ID]bool{"1": true, "2": true, "3": false}
	for _, e := range m.emails {
		if e.IsAuthored != want[e.ID] {
			t.Errorf("message %s: IsAuthored = %v, want %v", e.ID, e.IsAuthored, want[e.ID])
		}
		shown := e.SenderColumn(false)
		if want[e.ID] && !strings.HasPrefix(shown, email.RecipientMarker) {
			t.Errorf("message %s shows %q, want the recipient", e.ID, shown)
		}
	}
}

func TestParseMyEmails(t *testing.T) {
	got, err := cfg_g.ParseMyEmails(" a@x.org, Me <b@y.org>,,A@X.org ")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "a@x.org,b@y.org" {
		t.Errorf("got %q", got)
	}
	if _, err := cfg_g.ParseMyEmails("not-an-address"); err == nil {
		t.Error("a non-address was accepted")
	}
}
