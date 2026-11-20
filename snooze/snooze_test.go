package snooze

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/feral/cache"
	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/cache/meta"
	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/fileformat"
	"github.com/sarielhp/feral/mailclient"
)

var (
	t0    = time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)
	later = t0.Add(2 * time.Hour)
)

func newClient(t *testing.T, root, account string) *mailclient.MockMailClient {
	t.Helper()
	return &mailclient.MockMailClient{
		Labels: []string{"INBOX", "Archive", "Projects/Snoozed"},
		Cfg: &cfg_g.Config{
			Paths:           cfg_g.Paths{CacheRoot: root},
			SelectedAccount: cfg_g.AccountName(account),
			AccountCache:    cachedir.ForAccount(root, account),
		},
		BackendTypeVal: "jmap",
	}
}

func request(root string) Request {
	return Request{Root: root, Account: "work", Until: later, Now: t0}
}

func load(t *testing.T, root string) []Entry {
	t.Helper()
	f, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	return f.Entries
}

func TestLoadMissingIsEmptyDamagedAndNewerAreErrors(t *testing.T) {
	root := t.TempDir()
	if got := load(t, root); len(got) != 0 {
		t.Fatalf("missing file: %v", got)
	}
	if err := os.WriteFile(Path(root), []byte(`{"entries": [`), 0o600); err != nil {
		t.Fatal(err)
	}
	if f, err := Load(root); err == nil {
		t.Fatalf("damaged file read as %+v; want an error", f)
	}
	if err := os.WriteFile(Path(root), []byte(`{"format": 2, "entries": []}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var tooNew *fileformat.TooNewError
	if _, err := Load(root); !errors.As(err, &tooNew) {
		t.Fatalf("newer format: %v; want TooNewError", err)
	}
}

func TestSnoozeCreatesFolderMovesAndRecords(t *testing.T) {
	root := t.TempDir()
	c := newClient(t, root, "work")
	done, err := Snooze(c, request(root), []Item{{ID: "m1", Folder: "INBOX", Subject: "Lunch", From: "a@example.com"}})
	if err != nil || len(done) != 1 {
		t.Fatalf("Snooze = %v, %v", done, err)
	}
	// Projects/Snoozed only ends in the name: a folder of the user's own.
	if len(c.EnsuredLabels) != 1 || c.EnsuredLabels[0] != FolderName {
		t.Errorf("ensured %v; want [%s]", c.EnsuredLabels, FolderName)
	}
	if len(c.MovedEmails) != 1 || c.MovedEmails[0].SourceLabel != "INBOX" || c.MovedEmails[0].DestLabel != FolderName {
		t.Errorf("moves %+v", c.MovedEmails)
	}
	got := load(t, root)
	want := Entry{Account: "work", ID: "m1", Until: later, FromFolder: "INBOX", Folder: FolderName,
		Subject: "Lunch", From: "a@example.com", SnoozedAt: t0}
	if len(got) != 1 || !got[0].Until.Equal(want.Until) || got[0].ID != want.ID || got[0].FromFolder != want.FromFolder ||
		got[0].Folder != want.Folder || got[0].Account != want.Account || got[0].Subject != want.Subject {
		t.Errorf("record %+v; want %+v", got, want)
	}
	raw, _ := os.ReadFile(Path(root))
	if !strings.Contains(string(raw), `"format": 1`) {
		t.Errorf("record carries no format: %s", raw)
	}
}

func TestSnoozeUsesExistingFolderInAnyCase(t *testing.T) {
	root := t.TempDir()
	c := newClient(t, root, "work")
	c.Labels = append(c.Labels, "snoozed")
	if _, err := Snooze(c, request(root), []Item{{ID: "m1", Folder: "INBOX"}}); err != nil {
		t.Fatal(err)
	}
	if len(c.EnsuredLabels) != 0 || c.MovedEmails[0].DestLabel != "snoozed" {
		t.Errorf("ensured %v, moved to %q; want the existing folder", c.EnsuredLabels, c.MovedEmails[0].DestLabel)
	}
}

func TestSnoozeFailedMoveLeavesNoRecord(t *testing.T) {
	root := t.TempDir()
	c := newClient(t, root, "work")
	c.FailOnMessage = map[email.ID]error{"bad": errors.New("server said no")}
	done, err := Snooze(c, request(root), []Item{{ID: "bad", Folder: "INBOX"}, {ID: "ok", Folder: "INBOX"}})
	if err == nil || !strings.Contains(err.Error(), "server said no") {
		t.Fatalf("err = %v", err)
	}
	if len(done) != 1 || done[0].ID != "ok" {
		t.Errorf("done = %+v", done)
	}
	if got := load(t, root); len(got) != 1 || got[0].ID != "ok" {
		t.Errorf("record = %+v; want only ok", got)
	}
}

// Nothing moves when the record cannot be read, the account cannot really
// move it, or the time is not in the future.
func TestSnoozeRefusesBeforeMoving(t *testing.T) {
	cases := map[string]func(t *testing.T, root string, c *mailclient.MockMailClient, req *Request){
		"damaged record": func(t *testing.T, root string, _ *mailclient.MockMailClient, _ *Request) {
			if err := os.WriteFile(Path(root), []byte("{"), 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"read-only": func(_ *testing.T, _ string, c *mailclient.MockMailClient, _ *Request) { c.Cfg.ReadOnly = true },
		"dry-run":   func(_ *testing.T, _ string, c *mailclient.MockMailClient, _ *Request) { c.Cfg.DryRun = true },
		"mbox":      func(_ *testing.T, _ string, c *mailclient.MockMailClient, _ *Request) { c.BackendTypeVal = "mbox" },
		"past":      func(_ *testing.T, _ string, _ *mailclient.MockMailClient, req *Request) { req.Until = t0 },
		"no account": func(_ *testing.T, _ string, _ *mailclient.MockMailClient, req *Request) {
			req.Account = ""
		},
		"no download directory": func(_ *testing.T, _ string, _ *mailclient.MockMailClient, req *Request) {
			req.Root = ""
		},
		"outlook default ids": func(t *testing.T, _ string, c *mailclient.MockMailClient, _ *Request) {
			if err := meta.Write(c.Cfg.AccountCache, meta.Manifest{Version: meta.CurrentVersion, Backend: "outlook",
				IDScheme: meta.SchemeGraphDefault}); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			c := newClient(t, root, "work")
			req := request(root)
			setup(t, root, c, &req)
			if _, err := Snooze(c, req, []Item{{ID: "m1", Folder: "INBOX"}}); err == nil {
				t.Fatal("snoozed; want a refusal")
			}
			if len(c.MovedEmails) != 0 || len(c.EnsuredLabels) != 0 {
				t.Errorf("moved %+v, created %v before refusing", c.MovedEmails, c.EnsuredLabels)
			}
		})
	}
}

// A message snoozed again changes its time and keeps where it came from.
func TestSnoozeAgainKeepsFromFolder(t *testing.T) {
	root := t.TempDir()
	c := newClient(t, root, "work")
	if _, err := Snooze(c, request(root), []Item{{ID: "m1", Folder: "INBOX"}}); err != nil {
		t.Fatal(err)
	}
	req := request(root)
	req.Until = later.Add(24 * time.Hour)
	if _, err := Snooze(c, req, []Item{{ID: "m1", Folder: FolderName}}); err != nil {
		t.Fatal(err)
	}
	got := load(t, root)
	if len(got) != 1 || got[0].FromFolder != "INBOX" || !got[0].Until.Equal(req.Until) {
		t.Errorf("record = %+v", got)
	}
	if len(c.MovedEmails) != 1 {
		t.Errorf("moved again: %+v", c.MovedEmails)
	}
}

// A message with a record that was moved out of the snooze folder by hand
// and is snoozed again from there: a failed move puts the old record back
// rather than leaving none, and a move that works records the new origin.
func TestSnoozeAgainFromAnotherFolder(t *testing.T) {
	root := t.TempDir()
	c := newClient(t, root, "work")
	if _, err := Snooze(c, request(root), []Item{{ID: "m1", Folder: "INBOX"}}); err != nil {
		t.Fatal(err)
	}
	req := request(root)
	req.Until = later.Add(time.Hour)
	c.FailOnMessage = map[email.ID]error{"m1": errors.New("server said no")}
	if _, err := Snooze(c, req, []Item{{ID: "m1", Folder: "Lists"}}); err == nil {
		t.Fatal("a failed move reported success")
	}
	got := load(t, root)
	if len(got) != 1 || got[0].FromFolder != "INBOX" || !got[0].Until.Equal(later) {
		t.Fatalf("record after a failed re-snooze = %+v; want the first one back", got)
	}
	c.FailOnMessage = nil
	if _, err := Snooze(c, req, []Item{{ID: "m1", Folder: "Lists"}}); err != nil {
		t.Fatal(err)
	}
	if got = load(t, root); len(got) != 1 || got[0].FromFolder != "Lists" || !got[0].Until.Equal(req.Until) {
		t.Errorf("record = %+v; want from Lists", got)
	}
}

type warnings struct{ lines []string }

func (w *warnings) Infof(string, ...any)    {}
func (w *warnings) Successf(string, ...any) {}
func (w *warnings) Errorf(string, ...any)   {}
func (w *warnings) Printf(string, ...any)   {}
func (w *warnings) Progress(int, int, string) {
}
func (w *warnings) Warnf(f string, a ...any) { w.lines = append(w.lines, fmt.Sprintf(f, a...)) }

func seed(t *testing.T, root string, entries ...Entry) {
	t.Helper()
	for _, e := range entries {
		if _, err := Put(root, e, false); err != nil {
			t.Fatal(err)
		}
	}
}

func waker(root string, c *mailclient.MockMailClient, rep *warnings, accounts ...string) *Waker {
	w := &Waker{Root: root, Reporter: rep,
		ClientFor: func(cfg_acc.AccountConfig) (WakeClient, error) { return c, nil }}
	for _, a := range accounts {
		w.Accounts = append(w.Accounts, cfg_acc.AccountConfig{Name: a})
	}
	return w
}

// Only what is due wakes: marked unread, moved to the inbox, recorded unread
// locally, and dropped from the record.
func TestWakeDueByTheClock(t *testing.T) {
	root := t.TempDir()
	c := newClient(t, root, "work")
	seed(t, root,
		Entry{Account: "work", ID: "soon", Until: t0.Add(time.Hour), Folder: FolderName, FromFolder: "INBOX"},
		Entry{Account: "work", ID: "late", Until: t0.Add(3 * time.Hour), Folder: FolderName, FromFolder: "INBOX"})
	if err := cache.MarkIDsRead(c.Cfg.AccountCache, []email.ID{"soon"}, nil); err != nil {
		t.Fatal(err)
	}
	rep := &warnings{}
	w := waker(root, c, rep, "work")

	if woken, err := w.Due(context.Background(), t0); err != nil || len(woken) != 0 {
		t.Fatalf("at t0: woke %+v, %v", woken, err)
	}
	woken, err := w.Due(context.Background(), t0.Add(time.Hour))
	if err != nil || len(woken) != 1 || woken[0].ID != "soon" {
		t.Fatalf("at t0+1h: woke %+v, %v", woken, err)
	}
	// MoveEmail into the inbox, never MoveToInbox: that is the unspam path.
	if fmt.Sprint(c.MarkedUnread) != "[soon]" || fmt.Sprint(c.MovedEmails) != "[{[soon] Snoozed Inbox}]" || len(c.MovedToInbox) != 0 {
		t.Errorf("unread %v, moved %+v, unspam path %v; want soon moved from Snoozed to the inbox",
			c.MarkedUnread, c.MovedEmails, c.MovedToInbox)
	}
	if cache.LoadReadState(c.Cfg.AccountCache)["soon"] {
		t.Error("woken message is still read in the local cache")
	}
	if got := load(t, root); len(got) != 1 || got[0].ID != "late" {
		t.Errorf("record = %+v; want only late", got)
	}
	if len(rep.lines) != 0 {
		t.Errorf("warnings: %v", rep.lines)
	}
}

// A message whose account is gone, or that the server no longer has, keeps
// its entry and is reported -- once, however many polls fail.
func TestWakeKeepsWhatItCannotWake(t *testing.T) {
	root := t.TempDir()
	c := newClient(t, root, "work")
	c.FailOnMessage = map[email.ID]error{"gone": errors.New("not found")}
	seed(t, root,
		Entry{Account: "work", ID: "gone", Until: t0, Folder: FolderName, Subject: "deleted"},
		Entry{Account: "home", ID: "orphan", Until: t0, Folder: FolderName, Subject: "no account"})
	rep := &warnings{}
	w := waker(root, c, rep, "work")
	for range 3 {
		if woken, err := w.Due(context.Background(), later); err != nil || len(woken) != 0 {
			t.Fatalf("woke %+v, %v", woken, err)
		}
	}
	if got := load(t, root); len(got) != 2 {
		t.Errorf("record = %+v; want both kept", got)
	}
	if len(rep.lines) != 2 || !strings.Contains(strings.Join(rep.lines, "\n"), "no longer configured") {
		t.Errorf("warnings = %q; want one per entry", rep.lines)
	}
}

func TestWakeOnlyNamedAccount(t *testing.T) {
	root := t.TempDir()
	c := newClient(t, root, "work")
	seed(t, root, Entry{Account: "home", ID: "h", Until: t0, Folder: FolderName},
		Entry{Account: "work", ID: "w", Until: t0, Folder: FolderName})
	w := waker(root, c, &warnings{}, "work", "home")
	w.Only = "work"
	woken, err := w.Due(context.Background(), later)
	if err != nil || len(woken) != 1 || woken[0].ID != "w" {
		t.Fatalf("woke %+v, %v", woken, err)
	}
}

func TestWakeRefusesReadOnlyAccount(t *testing.T) {
	root := t.TempDir()
	c := newClient(t, root, "work")
	c.Cfg.ReadOnly = true
	seed(t, root, Entry{Account: "work", ID: "m", Until: t0, Folder: FolderName})
	woken, err := waker(root, c, &warnings{}, "work").Wake(context.Background(), load(t, root))
	if err == nil || len(woken) != 0 || len(c.MovedEmails) != 0 {
		t.Fatalf("woke %+v, %v, moved %v", woken, err, c.MovedEmails)
	}
	if len(load(t, root)) != 1 {
		t.Error("entry dropped")
	}
}

func TestDueDamagedRecordIsAnError(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(Path(root), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := waker(root, nil, &warnings{}).Due(context.Background(), later); err == nil {
		t.Fatal("damaged record woke nothing and said nothing")
	}
}

func TestRelabelIsIdempotent(t *testing.T) {
	root := t.TempDir()
	if err := Relabel(root, "old", "new"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(Path(root)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("relabel of no record wrote one: %v", err)
	}
	seed(t, root, Entry{Account: "old", ID: "a", Until: t0}, Entry{Account: "other", ID: "b", Until: t0})
	for range 2 {
		if err := Relabel(root, "old", "new"); err != nil {
			t.Fatal(err)
		}
	}
	got := load(t, root)
	if got[0].Account != "new" || got[1].Account != "other" {
		t.Errorf("record = %+v", got)
	}
}

func TestSelect(t *testing.T) {
	entries := []Entry{{Account: "a", ID: "id-one"}, {Account: "b", ID: "id-two"}}
	short := string(email.ComputeShortID("id-two"))
	matched, unknown := Select(entries, []string{"id-one", short, "id-one", "nope"})
	if len(matched) != 2 || matched[0].ID != "id-one" || matched[1].ID != "id-two" {
		t.Errorf("matched %+v", matched)
	}
	if fmt.Sprint(unknown) != "[nope]" {
		t.Errorf("unknown %v", unknown)
	}
}

func TestPathIsInDownloadDir(t *testing.T) {
	if got := Path("/d"); got != filepath.Join("/d", "snooze.json") {
		t.Errorf("Path = %s", got)
	}
}
