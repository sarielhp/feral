package tui

import (
	"testing"

	"github.com/sarielhp/feral/cache"
	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/mailclient"
)

const replyFixtureRaw = "From: Boss <boss@work.example>\r\nTo: me@work.example\r\n" +
	"Subject: Job\r\nMessage-ID: <w1@work.example>\r\n\r\nPlease reply.\r\n"

// Replying from the unified inbox fetches the original through the account
// that holds it. It used to go through the active client, which does not have
// another account's message, and the reply failed.
func TestReplyUnifiedFetchesThroughOwnAccount(t *testing.T) {
	m, personal, work := unifiedFixture()
	work.RawMessages = map[email.ID][]byte{"w1": []byte(replyFixtureRaw)}
	work.Cfg.AccountCache = cachedir.At(t.TempDir())

	cmd := m.replyCmd("w1", replyToSender)
	if cmd == nil {
		t.Fatal("expected a command")
	}
	if fin, ok := cmd().(composeEditorFinishedMsg); ok && fin.err != nil {
		t.Fatalf("reply to another account's message failed: %v", fin.err)
	}
	if len(personal.RawMessages) != 0 {
		t.Error("the active account was given a message it does not hold")
	}
}

// A reply to a message whose account cannot be reached is refused, rather
// than fetched through some other account.
func TestReplyUnifiedUnknownAccountRefused(t *testing.T) {
	m, _, _ := unifiedFixture()
	fin, ok := m.replyCmd("b1", replyToSender)().(composeEditorFinishedMsg)
	if !ok || fin.err == nil {
		t.Fatalf("expected a refusal, got %#v", fin)
	}
}

// The replied flag goes to the cache of the account the original belongs to,
// whichever account is active when the send completes -- after an F3 switch,
// or after the From account was changed in compose.
func TestRepliedFlagGoesToOriginalAccount(t *testing.T) {
	active := cachedir.At(t.TempDir())
	original := cachedir.At(t.TempDir())
	m := newTuiModel(&mailclient.MockMailClient{Cfg: &cfg_g.Config{AccountCache: active}}, "INBOX", nil)
	m.rawEmails = []email.Email{{ID: "orig"}}
	m.rebuildVisibleEmails()

	m.handleEmailSent(emailSentMsg{targetID: "orig", repliedCache: original, session: "s"})

	if !cache.LoadRepliedState(original)["orig"] {
		t.Error("the replied flag is missing from the original's account")
	}
	if cache.LoadRepliedState(active)["orig"] {
		t.Error("the replied flag was filed under the active account")
	}
	if !visibleRow(t, m, "orig").IsReplied {
		t.Error("the row on screen does not show r")
	}
}

// The send records where the flag goes when it is dispatched: the session's
// original account, or for a session that does not know it, the account that
// sends -- never the one active when the result comes back.
func TestSendCapturesRepliedCache(t *testing.T) {
	sending := cachedir.At(t.TempDir())
	original := cachedir.At(t.TempDir())
	for _, c := range []struct {
		name  string
		state cachedir.Account
		want  cachedir.Account
	}{
		{"reply", original, original},
		{"resumed draft", cachedir.Account{}, sending},
	} {
		m := newTuiModel(&mailclient.MockMailClient{Cfg: &cfg_g.Config{AccountCache: sending}}, "INBOX", nil)
		m.compose = &composeState{
			OriginalID: "orig", OriginalCache: c.state,
			From: "me@example.com", To: "you@example.com", Subject: "s", Body: "b",
		}
		_, cmd := m.executeSendComposeEmail()
		if cmd == nil {
			t.Fatalf("%s: no send command", c.name)
		}
		var sent *emailSentMsg
		for _, msg := range runCmd(cmd) {
			if sm, ok := msg.(emailSentMsg); ok {
				sent = &sm
			}
		}
		if sent == nil {
			t.Fatalf("%s: the send produced no emailSentMsg", c.name)
		}
		if sent.repliedCache != c.want {
			t.Errorf("%s: replied cache %v, want %v", c.name, sent.repliedCache, c.want)
		}
	}
}
