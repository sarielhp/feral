package watcher

import (
	"context"
	"errors"
	"os"
	"slices"
	"testing"

	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/mailclient"
	"github.com/sarielhp/feral/mute"
)

// muteFixture is an account whose watermark knows "a", whose thread rooted at
// <root@example.com> is muted, and into whose inbox a reply in that thread
// ("m") and an unrelated message ("b") have since arrived.
type muteFixture struct {
	deps     Deps
	rep      *recordingReporter
	notifier *fakeNotifier
	archived []email.ID
	client   *mailclient.MockMailClient
}

func newMuteFixture(t *testing.T, archiveErr error) *muteFixture {
	t.Helper()
	f := &muteFixture{rep: &recordingReporter{}, notifier: &fakeNotifier{}}
	reply := []byte("From: list@example.com\r\nSubject: Re: noisy\r\n" +
		"Date: Mon, 12 Oct 2026 10:00:00 +0000\r\nMessage-ID: <m@example.com>\r\n" +
		"References: <root@example.com> <r1@example.com>\r\n\r\nBody\r\n")
	f.client = newMock([]cfg_acc.MessageFolderRef{{MessageID: "a", Folder: "Inbox"}}, map[email.ID][]byte{
		"a": rawMessage("old", "alice@example.com"),
		"b": rawMessage("fresh", "bob@example.com"),
		"m": reply,
	})
	f.deps = testDeps(t, f.client, f.rep)
	f.deps.Config.ConfigDir = t.TempDir()
	f.deps.Archive = func(_ context.Context, account cfg_acc.AccountConfig, ids []email.ID) error {
		if account.Name != "work" {
			t.Errorf("archived in %q, want the polled account", account.Name)
		}
		f.archived = append(f.archived, ids...)
		return archiveErr
	}
	path, err := mute.Path(f.deps.Config.ConfigDir, "work")
	if err != nil {
		t.Fatal(err)
	}
	list := &mute.List{}
	list.Add(mute.Thread{Key: "<root@example.com>", Scheme: mute.SchemeHeaderRoot, MutedAt: fixedNow()})
	if err := list.Save(path); err != nil {
		t.Fatal(err)
	}
	f.poll(t) // first run: records "a"
	f.client.InboxIDs = []email.ID{"m", "b", "a"}
	return f
}

func (f *muteFixture) poll(t *testing.T) {
	t.Helper()
	if err := checkAccount(context.Background(), f.deps, Options{}, f.rep, f.notifier,
		f.deps.Config.Accounts[0], f.deps.Now); err != nil {
		t.Fatalf("checkAccount: %v", err)
	}
}

func (f *muteFixture) announced() []email.ID {
	ids := make([]email.ID, 0, len(f.notifier.got))
	for _, n := range f.notifier.got {
		ids = append(ids, n.ID)
	}
	slices.Sort(ids)
	return ids
}

// A new message in a muted thread is archived on arrival and not announced;
// the rest of the poll's mail is announced as ever, and the muted one is not
// archived again on the next poll.
func TestCheckAccountArchivesMutedThreads(t *testing.T) {
	f := newMuteFixture(t, nil)
	f.poll(t)
	if !slices.Equal(f.archived, []email.ID{"m"}) {
		t.Errorf("archived %v, want the muted thread's reply m", f.archived)
	}
	if got := f.announced(); !slices.Equal(got, []email.ID{"b"}) {
		t.Errorf("announced %v, want only b", got)
	}
	f.poll(t)
	if len(f.archived) != 1 {
		t.Errorf("a second poll archived again: %v", f.archived)
	}
}

// A muted message is silent only once it is archived: if the archive fails,
// or the session may not change the server, it is announced like any other.
// It must never be left in the inbox unannounced.
func TestCheckAccountAnnouncesMutedMailItCannotArchive(t *testing.T) {
	cases := map[string]func(f *muteFixture){
		"archive fails":     func(*muteFixture) {},
		"read-only":         func(f *muteFixture) { f.deps.Config.ReadOnly = true },
		"read-only account": func(f *muteFixture) { f.deps.Config.Accounts[0].ReadOnly = true },
		"dry-run":           func(f *muteFixture) { f.deps.Config.DryRun = true },
		"no archiver":       func(f *muteFixture) { f.deps.Archive = nil },
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			var archiveErr error
			if name == "archive fails" {
				archiveErr = errors.New("server said no")
			}
			f := newMuteFixture(t, archiveErr)
			setup(f)
			f.poll(t)
			if got := f.announced(); !slices.Equal(got, []email.ID{"b", "m"}) {
				t.Errorf("announced %v, want both b and m", got)
			}
			if name != "archive fails" && len(f.archived) != 0 {
				t.Errorf("archived %v in a session that may not change the server", f.archived)
			}
		})
	}
}

// A damaged muted-thread list is reported and mutes nothing: the poll still
// announces everything rather than failing the account.
func TestCheckAccountDamagedMuteListMutesNothing(t *testing.T) {
	f := newMuteFixture(t, nil)
	path, err := mute.Path(f.deps.Config.ConfigDir, "work")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{damaged"), 0o600); err != nil {
		t.Fatal(err)
	}
	f.poll(t)
	if len(f.archived) != 0 {
		t.Errorf("archived %v from a damaged list", f.archived)
	}
	if got := f.announced(); !slices.Equal(got, []email.ID{"b", "m"}) {
		t.Errorf("announced %v, want both", got)
	}
	if !f.rep.has("muted threads not applied") {
		t.Errorf("the damaged list was not reported: %v", f.rep.lines)
	}
}
