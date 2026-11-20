package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/feral/cache/folderindex"
	"github.com/sarielhp/feral/cache/msg"
	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/mute"
)

// muteSession is a session over one account, "work", whose cache holds a
// reply in the thread rooted at <root@example.com>.
func muteSession(t *testing.T) (*Session, *bytes.Buffer) {
	t.Helper()
	dir := t.TempDir()
	config := &cfg_g.Config{
		Paths:    cfg_g.Paths{ConfigDir: dir, CacheRoot: dir},
		Accounts: []cfg_acc.AccountConfig{{Name: "work", Type: "jmap"}},
	}
	cache := config.AccountCacheFor(config.Accounts[0])
	raw := "From: list@example.com\r\nSubject: Re: noisy\r\nMessage-ID: <r2@example.com>\r\n" +
		"References: <root@example.com> <r1@example.com>\r\n\r\nbody\r\n"
	if err := msg.Store(cache, "reply1", []byte(raw), time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := folderindex.ReplaceAll(cache, "INBOX", nil); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	return &Session{Config: config, Out: &out}, &out
}

func mutedKeys(t *testing.T, session *Session) []string {
	t.Helper()
	path, err := mute.Path(session.Config.ConfigDir, "work")
	if err != nil {
		t.Fatal(err)
	}
	list, err := mute.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, th := range list.Threads {
		keys = append(keys, string(th.Key))
	}
	return keys
}

// mute records the thread's root in the account that holds the message;
// muted lists it; unmute by message or by the listed key removes it.
func TestMsgMuteUnmuteAndList(t *testing.T) {
	session, out := muteSession(t)
	app := InitCLI(session)
	app.Stdout = out
	if err := app.Execute([]string{"msg", "mute", "reply1"}); err != nil {
		t.Fatalf("msg mute: %v", err)
	}
	if got := mutedKeys(t, session); len(got) != 1 || got[0] != "<root@example.com>" {
		t.Fatalf("muted %v, want the thread root", got)
	}
	out.Reset()
	if err := app.Execute([]string{"msg", "muted"}); err != nil {
		t.Fatalf("msg muted: %v", err)
	}
	if !strings.Contains(out.String(), "<root@example.com>") || !strings.Contains(out.String(), "Re: noisy") {
		t.Errorf("msg muted printed %q", out.String())
	}
	if err := app.Execute([]string{"msg", "unmute", "reply1"}); err != nil {
		t.Fatalf("msg unmute by message: %v", err)
	}
	if got := mutedKeys(t, session); len(got) != 0 {
		t.Fatalf("still muted after unmute: %v", got)
	}
	if err := app.Execute([]string{"msg", "mute", "reply1"}); err != nil {
		t.Fatal(err)
	}
	if err := app.Execute([]string{"msg", "unmute", "root@example.com"}); err != nil {
		t.Fatalf("msg unmute by key: %v", err)
	}
	if got := mutedKeys(t, session); len(got) != 0 {
		t.Fatalf("still muted after unmute by key: %v", got)
	}
}

// --read-only refuses a mute, whose archiving it could not do; --dry-run says
// what it would do and writes nothing.
func TestMsgMuteReadOnlyAndDryRun(t *testing.T) {
	session, out := muteSession(t)
	session.Config.ReadOnly = true
	if err := muteByIDs(session, []string{"reply1"}); err == nil || !strings.Contains(err.Error(), "--read-only") {
		t.Fatalf("mute under --read-only = %v, want a refusal", err)
	}
	session.Config.ReadOnly, session.Config.DryRun = false, true
	if err := muteByIDs(session, []string{"reply1"}); err != nil {
		t.Fatalf("mute under --dry-run: %v", err)
	}
	if !strings.Contains(out.String(), "would have muted") {
		t.Errorf("--dry-run printed %q, want what it would do", out.String())
	}
	if got := mutedKeys(t, session); len(got) != 0 {
		t.Errorf("--dry-run wrote %v", got)
	}
}
