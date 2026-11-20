package mute

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/fileformat"
)

var mutedAt = time.Date(2026, 11, 20, 3, 0, 0, 0, time.UTC)

func TestPathIsInTheAccountSettingsDirectory(t *testing.T) {
	dir := t.TempDir()
	got, err := Path(dir, "work")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(cfg_g.AccountDir(dir, "work"), FileName); got != want {
		t.Errorf("Path = %q, want %q", got, want)
	}
	// account rename moves the settings directory, and with it this file.
	if files := cfg_g.AccountFiles(dir, "work"); files[0] != filepath.Dir(got) {
		t.Errorf("the record is not under the directory a rename moves: %q vs %v", got, files)
	}
	if _, err := Path("", "work"); !errors.Is(err, ErrNoConfigDir) {
		t.Errorf("Path with no configuration directory = %v, want ErrNoConfigDir", err)
	}
}

func TestLoadMissingIsEmpty(t *testing.T) {
	list, err := Load(filepath.Join(t.TempDir(), FileName))
	if err != nil || len(list.Threads) != 0 {
		t.Fatalf("Load of a missing file = %+v, %v; want an empty list", list, err)
	}
}

// A damaged record is an error, never an empty list: the next save would
// write the empty list over every mute it held.
func TestLoadDamagedAndNewerAreErrors(t *testing.T) {
	dir := t.TempDir()
	damaged := filepath.Join(dir, "damaged.json")
	if err := os.WriteFile(damaged, []byte(`{"format":1,"threads":[`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(damaged); err == nil {
		t.Error("a damaged record loaded")
	}
	newer := filepath.Join(dir, "newer.json")
	if err := os.WriteFile(newer, []byte(`{"format":99,"threads":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var tooNew *fileformat.TooNewError
	if _, err := Load(newer); !errors.As(err, &tooNew) {
		t.Errorf("a record from a newer build = %v, want a TooNewError", err)
	}
}

func TestSaveLoadRoundTripCarriesFormat(t *testing.T) {
	path, err := Path(t.TempDir(), "work")
	if err != nil {
		t.Fatal(err)
	}
	list := &List{}
	if !list.Add(Thread{Key: "<root@example.com>", Scheme: SchemeHeaderRoot, Subject: "plan", MutedAt: mutedAt}) {
		t.Fatal("Add on an empty list reported no change")
	}
	if err = list.Save(path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var probe struct{ Format int }
	if err = json.Unmarshal(data, &probe); err != nil || probe.Format != fileformat.Current {
		t.Errorf("saved format = %d (%v), want %d", probe.Format, err, fileformat.Current)
	}
	back, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Threads) != 1 || back.Threads[0].Key != "<root@example.com>" || !back.Threads[0].MutedAt.Equal(mutedAt) {
		t.Errorf("round trip = %+v", back.Threads)
	}
}

// A reply belongs to a muted thread when any Message-ID it names is the
// thread's root: a trimmed References still names it, and so does an
// In-Reply-To alone. Case is not significant in a Message-ID's comparison
// here, as everywhere else in feral.
func TestMatch(t *testing.T) {
	list := &List{Threads: []Thread{{Key: "<Root@Example.com>", Scheme: SchemeHeaderRoot}}}
	tests := []struct {
		name string
		em   email.Email
		want bool
	}{
		{"root itself", email.Email{MessageID: "<root@example.com>"}, true},
		{"reply with references", email.Email{MessageID: "<r2@x>", References: "<root@example.com> <r1@x>", InReplyTo: "<r1@x>"}, true},
		{"trimmed references", email.Email{MessageID: "<r9@x>", References: "<root@example.com> <r8@x>"}, true},
		{"in-reply-to only", email.Email{MessageID: "<r1@x>", InReplyTo: "<root@example.com>"}, true},
		{"another thread", email.Email{MessageID: "<other@x>", References: "<elsewhere@x>"}, false},
		{"no ids at all", email.Email{Subject: "hello"}, false},
	}
	for _, tc := range tests {
		if _, got := list.Match(&tc.em); got != tc.want {
			t.Errorf("%s: Match = %v, want %v", tc.name, got, tc.want)
		}
	}
	other := &List{Threads: []Thread{{Key: "<root@example.com>", Scheme: "gmail-thread"}}}
	if _, got := other.Match(&email.Email{MessageID: "<root@example.com>"}); got {
		t.Error("an entry in a scheme this build does not know matched")
	}
	var none *List
	if _, got := none.Match(&email.Email{MessageID: "<root@example.com>"}); got {
		t.Error("a nil list matched")
	}
}

func TestThreadOfAndRemove(t *testing.T) {
	reply := &email.Email{Subject: "Re: plan", MessageID: "<r2@x>", References: "<root@x> <r1@x>"}
	thread, err := ThreadOf(reply, mutedAt)
	if err != nil {
		t.Fatal(err)
	}
	if thread.Key != "<root@x>" || thread.Scheme != SchemeHeaderRoot || thread.Subject != "Re: plan" {
		t.Errorf("ThreadOf = %+v, want the References root", thread)
	}
	if _, err := ThreadOf(&email.Email{Subject: "no ids"}, mutedAt); !errors.Is(err, ErrNoThreadID) {
		t.Errorf("ThreadOf a message with no IDs = %v, want ErrNoThreadID", err)
	}
	if _, err := ThreadOf(&email.Email{MessageID: "<>"}, mutedAt); !errors.Is(err, ErrNoThreadID) {
		t.Errorf("an empty <> Message-ID was taken as a thread: %v", err)
	}

	list := &List{}
	list.Add(thread)
	if list.Add(thread) {
		t.Error("a second Add of the same thread reported a change")
	}
	if list.Remove("<nothing@x>") {
		t.Error("Remove of a thread not muted reported a change")
	}
	if !list.Remove("<ROOT@x>") || len(list.Threads) != 0 {
		t.Errorf("Remove left %+v", list.Threads)
	}
	if list.Remove("") {
		t.Error("an empty key removed something")
	}
}
