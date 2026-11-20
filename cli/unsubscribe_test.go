package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/sarielhp/feral/app"
	"github.com/sarielhp/feral/cfg_g"
)

// stubUnsub asks for the confirmation the way the real service does, and
// records whether it would have gone on.
type stubUnsub struct {
	got   *UnsubscribeOptions
	acted *bool
}

func (s stubUnsub) Unsubscribe(_ *cfg_g.Config, opts UnsubscribeOptions) error {
	*s.got = opts
	if err := opts.Confirm("Unsubscribe from news@list.example"); err != nil {
		return err
	}
	*s.acted = true
	return nil
}

func unsubSession(stdin string) (*Session, *UnsubscribeOptions, *bool, *bytes.Buffer) {
	got, acted := &UnsubscribeOptions{}, new(bool)
	var status bytes.Buffer
	session := &Session{
		Config: &cfg_g.Config{SelectedAccount: "work"},
		In:     strings.NewReader(stdin),
		Err:    &status,
		Deps:   Deps{Unsub: stubUnsub{got: got, acted: acted}},
	}
	return session, got, acted, &status
}

func TestMsgUnsubPassesTheMessageAndAccount(t *testing.T) {
	session, got, acted, _ := unsubSession("")
	if err := InitCLI(session).Execute([]string{"msg", "unsub", "--yes", "abc123de"}); err != nil {
		t.Fatalf("msg unsub --yes: %v", err)
	}
	if got.MessageID != "abc123de" || got.Account != "work" || !*acted {
		t.Errorf("got %+v, acted %v", *got, *acted)
	}
}

// Without --yes and without a terminal there is nobody to ask: it refuses
// and names the flag, and nothing is done.
func TestMsgUnsubRefusesWithoutATerminal(t *testing.T) {
	session, _, acted, _ := unsubSession("y\n")
	err := runUnsubscribe(session, "abc123de", false)
	if !errors.Is(err, app.ErrUsage) || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("err = %v, want a usage error naming --yes", err)
	}
	if *acted {
		t.Error("unsubscribed without a confirmation")
	}
}

func TestMsgUnsubDeclinedIsQuiet(t *testing.T) {
	session, _, acted, status := unsubSession("")
	session.Deps.Unsub = declineUnsub{}
	if err := runUnsubscribe(session, "abc123de", false); err != nil {
		t.Fatalf("a declined unsubscribe is not a failure: %v", err)
	}
	if *acted || !strings.Contains(status.String(), "Cancelled.") {
		t.Errorf("acted %v, status %q", *acted, status.String())
	}
}

type declineUnsub struct{}

func (declineUnsub) Unsubscribe(*cfg_g.Config, UnsubscribeOptions) error { return ErrDeclined }
