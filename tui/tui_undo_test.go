package tui

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sarielhp/feral/bogofiltertest"
	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/cache/folderindex"
	"github.com/sarielhp/feral/cache/msg"
	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/label"
	"github.com/sarielhp/feral/mailclient"
	"github.com/sarielhp/feral/train"
)

// cachingMock downloads into the cache, as a backend's message store does: a
// message an action dropped from the cache comes back when it is read.
type cachingMock struct{ *mailclient.MockMailClient }

func (c cachingMock) GetRawMessage(id email.ID) ([]byte, error) {
	raw, err := c.MockMailClient.GetRawMessage(id)
	if err != nil {
		return nil, err
	}
	return raw, msg.Store(c.Cfg.AccountCache, id, raw, time.Now())
}

// FetchAndDownloadEmails lists the folder and downloads what is missing.
func (c cachingMock) FetchAndDownloadEmails(folder label.Folder, query string) ([]email.ID, error) {
	ids, err := c.MockMailClient.FetchAndDownloadEmails(folder, query)
	for _, id := range ids {
		if ok, _ := msg.Exists(c.Cfg.AccountCache, id); !ok && err == nil {
			_, err = c.GetRawMessage(id)
		}
	}
	return ids, err
}

// undoBox is an account "Home" holding m1..m3 in INBOX, behind the caching
// decorator, so the folder index moves as a real client's does.
type undoBox struct {
	m    *tuiModel
	mock *mailclient.MockMailClient
	acct cachedir.Account
}

func fixedFolder(name string) func(mailclient.MailClient) (string, error) {
	return func(mailclient.MailClient) (string, error) { return name, nil }
}

func undoFixture(t *testing.T, backend string) *undoBox {
	t.Helper()
	// Every bogofilter call -- the spam mark's training, the reload's scan --
	// meets this one, never the user's database.
	bogofiltertest.Install(t, "#!/bin/sh\nwhile read -r _; do :; done\necho 'X-Bogosity: Spam, tests=bogofilter, spamicity=0.990000'\n")
	dir := t.TempDir()
	acct := cachedir.At(dir)
	ids := []email.ID{"m1", "m2", "m3"}
	raws := map[email.ID][]byte{}
	for i, id := range ids {
		date := time.Date(2026, 1, 1+i, 9, 0, 0, 0, time.UTC).Format(time.RFC1123Z)
		raws[id] = []byte(fmt.Sprintf("Message-ID: <%s@undo.example>\r\nDate: %s\r\nFrom: S <s@undo.example>\r\nSubject: Note %d\r\n\r\nBody.\r\n", id, date, i))
		if err := msg.Store(acct, id, raws[id], time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	if err := folderindex.ReplaceAll(acct, "INBOX", ids); err != nil {
		t.Fatal(err)
	}
	cfg := &cfg_g.Config{SelectedAccount: "Home", Accounts: []cfg_acc.AccountConfig{{Name: "Home", Type: backend}},
		AccountCache: acct, Paths: cfg_g.Paths{CacheRoot: dir, ConfigDir: t.TempDir()}, Policy: cfg_g.Policy{SpamFolder: "Spam"}}
	mock := &mailclient.MockMailClient{Cfg: cfg, RawMessages: raws, InboxFolderVal: "INBOX", BackendTypeVal: backend,
		Labels: []string{"INBOX", "Archive", "Trash", "Spam", "Projects"}, DownloadedIDs: map[string][]email.ID{"INBOX": ids}}
	client := mailclient.NewCheckingMailClient(cachingMock{mock}, "Home", nil)
	bk := &Backend{Folders: stubFolders{ArchiveFn: fixedFolder("Archive"), TrashFn: fixedFolder("Trash"), SpamFn: fixedFolder("Spam")}}
	m := newTuiModel(client, "INBOX", bk)
	m.cfg = cfg
	m.width, m.height = 100, 30
	m.mode = modeIndex
	m.currentF = "INBOX"
	m.loadCachedEmailsForFolder("INBOX")
	b := &undoBox{m: m, mock: mock, acct: acct}
	b.selectID(t, "m1")
	return b
}

func (b *undoBox) selectID(t *testing.T, id email.ID) {
	t.Helper()
	for i, e := range b.m.emails {
		if e.ID == id {
			b.m.eIdx, b.m.selectedID = i, id
			return
		}
	}
	t.Fatalf("%s is not in the list", id)
}

// settle runs cmd and every command its messages produce, leaving behind one
// that is still running after a moment -- a tick.
func settle(m *tuiModel, cmd tea.Cmd) {
	queue := []tea.Cmd{cmd}
	for i := 0; len(queue) > 0 && i < 60; i++ {
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		out := make(chan tea.Msg, 1)
		go func() { out <- c() }()
		var got tea.Msg
		select {
		case got = <-out:
		case <-time.After(300 * time.Millisecond):
			continue
		}
		switch got := got.(type) {
		case nil:
		case tea.BatchMsg:
			queue = append(queue, got...)
		default:
			_, next := m.Update(got)
			queue = append(queue, next)
		}
	}
}

// keys presses each key and settles what follows.
func (b *undoBox) keys(ks ...string) {
	for _, k := range ks {
		settle(b.m, press(b.m, k))
	}
}

func (b *undoBox) inbox(t *testing.T) []email.ID {
	t.Helper()
	ids, err := folderindex.IDs(b.acct, "INBOX")
	if err != nil {
		t.Fatal(err)
	}
	return ids
}

func sameIDs(a, b []email.ID) bool {
	a, b = slices.Clone(a), slices.Clone(b)
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(a, b)
}

// Each undoable action, one message and tagged: z moves what it moved back to
// INBOX, from the folder the action filed it in, and the folder index follows.
func TestUndo_EachAction(t *testing.T) {
	one, two := []email.ID{"m1"}, []email.ID{"m1", "m2"}
	tagged := func(ks ...string) func(*undoBox) {
		return func(b *undoBox) {
			b.m.taggedIDs = map[email.ID]bool{"m1": true, "m2": true}
			b.keys(ks...)
		}
	}
	tests := []struct {
		name  string
		act   func(*undoBox)
		ids   []email.ID
		from  string
		title string
	}{
		{"archive", func(b *undoBox) { b.keys("E") }, one, "Archive", "Archived 1"},
		{"delete to trash", func(b *undoBox) { b.keys("d") }, one, "Trash", "Deleted 1"},
		{"mark as spam", func(b *undoBox) { b.keys("S") }, one, "Spam", "Marked as spam 1"},
		{"archive tagged", tagged(";", "E"), two, "Archive", "Archived 2"},
		{"delete tagged", tagged(";", "d"), two, "Trash", "Deleted 2"},
		{"spam tagged", tagged(";", "S"), two, "Spam", "Marked as spam 2"},
		{"move tagged to a folder", func(b *undoBox) {
			b.m.taggedIDs = map[email.ID]bool{"m1": true, "m2": true}
			_, cmd := b.m.executeBulkMoveToFolder(label.FromServer("Projects"))
			settle(b.m, cmd)
		}, two, "Projects", "Moved 2"},
		{"move tagged by the prompt", func(b *undoBox) {
			b.m.taggedIDs = map[email.ID]bool{"m1": true, "m2": true}
			b.keys(";", "a")
			b.m.moveQuery = "Projects"
			b.keys("enter")
		}, two, "Projects", "Moved 2"},
	}
	// A tagged spam report goes through ReportSpam, which CheckingMailClient
	// does not reconcile: the index keeps the messages until the next listing.
	staleIndex := map[string]bool{"spam tagged": true}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := undoFixture(t, "jmap")
			tt.act(b)
			if len(b.m.undoStack) != 1 || !strings.Contains(b.m.saveStatus, "z to undo") {
				t.Fatalf("after the action: %d entries, status %q; want one, offered", len(b.m.undoStack), b.m.saveStatus)
			}
			for _, id := range tt.ids {
				if !staleIndex[tt.name] && slices.Contains(b.inbox(t), id) {
					t.Fatalf("%s is still indexed in INBOX after the action", id)
				}
			}
			moves := len(b.mock.MovedEmails)
			b.keys("z")
			if len(b.mock.MovedEmails) != moves+1 {
				t.Fatalf("z made %d server moves, want one", len(b.mock.MovedEmails)-moves)
			}
			back := b.mock.MovedEmails[moves]
			if !sameIDs(back.MessageIDs, tt.ids) || back.SourceLabel != tt.from || back.DestLabel != "INBOX" {
				t.Errorf("undo moved %v %s -> %s, want %v %s -> INBOX", back.MessageIDs, back.SourceLabel, back.DestLabel, tt.ids, tt.from)
			}
			if b.m.saveStatus != "Undone: "+tt.title || len(b.m.undoStack) != 0 {
				t.Errorf("status %q with %d entries left; want %q and none", b.m.saveStatus, len(b.m.undoStack), "Undone: "+tt.title)
			}
			for _, id := range tt.ids {
				if !slices.Contains(b.inbox(t), id) {
					t.Errorf("%s is not indexed in INBOX again", id)
				}
			}
		})
	}
}

// The list changes under the stack -- a refresh brings new mail, the cursor
// moves, another folder is opened -- and z still undoes the latest action
// first, moving each message back to where it was, not to the folder on
// screen; the restored rows come back when that folder is listed.
func TestUndo_AfterTheListChanged(t *testing.T) {
	b := undoFixture(t, "jmap")
	b.keys("E")
	b.selectID(t, "m3")
	b.keys("d")
	b.mock.DownloadedIDs["INBOX"] = []email.ID{"m2"}
	settle(b.m, b.m.loadEmailsCmd("INBOX"))
	b.m.currentF = "Projects"
	b.m.loadCachedEmailsForFolder("Projects")

	moves := len(b.mock.MovedEmails)
	b.keys("z")
	if last := b.mock.MovedEmails[moves]; !sameIDs(last.MessageIDs, []email.ID{"m3"}) || last.SourceLabel != "Trash" || last.DestLabel != "INBOX" {
		t.Fatalf("first z moved %+v; want m3 back from Trash", last)
	}
	b.keys("z")
	if last := b.mock.MovedEmails[moves+1]; !sameIDs(last.MessageIDs, []email.ID{"m1"}) || last.SourceLabel != "Archive" || last.DestLabel != "INBOX" {
		t.Fatalf("second z moved %+v; want m1 back from Archive", last)
	}
	b.keys("z")
	if b.m.saveStatus != "Nothing to undo" {
		t.Errorf("a third z says %q", b.m.saveStatus)
	}
	b.m.currentF = "INBOX"
	b.mock.DownloadedIDs["INBOX"] = []email.ID{"m1", "m2", "m3"}
	settle(b.m, b.m.loadEmailsCmd("INBOX"))
	if len(b.m.emails) != 3 {
		t.Errorf("INBOX lists %d messages after both undos, want 3", len(b.m.emails))
	}
}

// Delete drops the cached copy and, with its record, the user's spam
// override; undo downloads the message again and puts the override back.
func TestUndo_DeleteRestoresTheOverride(t *testing.T) {
	b := undoFixture(t, "jmap")
	if err := msg.SetOverride(b.acct, "m1", msg.OverrideHam); err != nil {
		t.Fatal(err)
	}
	b.keys("d")
	if _, err := msg.GetInfo(b.acct, "m1"); err == nil {
		t.Fatal("delete kept the cached record; this test needs it gone")
	}
	b.keys("z")
	info, err := msg.GetInfo(b.acct, "m1")
	if err != nil || info.Override != msg.OverrideHam {
		t.Fatalf("after undo: %+v, %v; want the ham override back", info, err)
	}
}

// What cannot be moved back stands on the stack, so z says why instead of
// reaching past it to an older action -- and the next z reaches that one.
func TestUndo_FinalEntries(t *testing.T) {
	t.Run("outlook default IDs", func(t *testing.T) {
		b := undoFixture(t, "outlook")
		b.keys("E")
		if !strings.Contains(b.m.saveStatus, "cannot be undone") {
			t.Fatalf("status %q; an Outlook default ID changes on the move", b.m.saveStatus)
		}
		moves := len(b.mock.MovedEmails)
		b.keys("z")
		if len(b.mock.MovedEmails) != moves || !strings.HasPrefix(b.m.saveStatus, "Cannot undo Archived") {
			t.Errorf("z moved %d and says %q", len(b.mock.MovedEmails)-moves, b.m.saveStatus)
		}
	})
	t.Run("blocked sender over an archive", func(t *testing.T) {
		b := undoFixture(t, "jmap")
		b.selectID(t, "m2")
		b.keys("E")
		b.m.Update(blockDoneMsg{id: "m1", senderEmail: "s@undo.example"})
		if !strings.Contains(b.m.saveStatus, "cannot be undone: blocking is a rule change") {
			t.Fatalf("status %q", b.m.saveStatus)
		}
		moves := len(b.mock.MovedEmails)
		b.keys("z")
		if len(b.mock.MovedEmails) != moves || b.m.saveStatus != "Cannot undo Blocked a sender: blocking is a rule change" {
			t.Fatalf("z moved %d and says %q", len(b.mock.MovedEmails)-moves, b.m.saveStatus)
		}
		b.keys("z")
		if b.m.saveStatus != "Undone: Archived 1" {
			t.Errorf("the second z says %q; want the archive beneath undone", b.m.saveStatus)
		}
	})
	t.Run("move to another account", func(t *testing.T) {
		m, calls, _ := accountMoveFixture(t)
		press(m, ";")
		press(m, "a")
		settle(m, press(m, "enter"))
		if len(*calls) != 1 || !strings.Contains(m.saveStatus, "cannot be undone: a move to another account is not undone") {
			t.Fatalf("%d transfers, status %q", len(*calls), m.saveStatus)
		}
		press(m, "z")
		if !strings.HasPrefix(m.saveStatus, "Cannot undo Moved") {
			t.Errorf("z says %q", m.saveStatus)
		}
	})
}

// Under --read-only every move is refused before anything is staged, so
// there is no simulation to undo and z says so; and an entry recorded before
// the session became read-only is kept, not acted on.
func TestUndo_ReadOnly(t *testing.T) {
	b := undoFixture(t, "jmap")
	b.m.cfg.ReadOnly = true
	b.keys("E")
	b.keys("z")
	if len(b.mock.MovedEmails) != 0 || len(b.m.undoStack) != 0 || b.m.saveStatus != "Nothing to undo" {
		t.Fatalf("read-only: %d moves, %d entries, status %q", len(b.mock.MovedEmails), len(b.m.undoStack), b.m.saveStatus)
	}

	b = undoFixture(t, "jmap")
	b.keys("E")
	b.m.cfg.ReadOnly = true
	b.keys("z")
	if len(b.mock.MovedEmails) != 1 || len(b.m.undoStack) != 1 || !b.m.showError {
		t.Errorf("z under read-only: %d moves, %d entries, error shown %v; want refused and kept",
			len(b.mock.MovedEmails), len(b.m.undoStack), b.m.showError)
	}
}

// The stack keeps the last ten, and only a message's latest action is
// undoable: a newer one, or an unspam, makes an older entry forget it.
func TestUndo_StackBounds(t *testing.T) {
	b := undoFixture(t, "jmap")
	m := b.m
	batch := func(id email.ID) []undoBatch {
		return []undoBatch{{account: "Home", from: label.FromServer("Archive"), to: label.FromServer("INBOX"), ids: []email.ID{id}}}
	}
	for i := 0; i < undoStackDepth+2; i++ {
		m.recordUndo(undoEntry{verb: "Archived", batches: batch(email.ID(fmt.Sprintf("x%d", i)))})
	}
	if len(m.undoStack) != undoStackDepth || m.undoStack[0].batches[0].ids[0] != "x2" {
		t.Fatalf("stack holds %d, oldest %v; want the last %d", len(m.undoStack), m.undoStack[0].batches[0].ids, undoStackDepth)
	}
	m.recordUndo(undoEntry{verb: "Moved", batches: batch("x5")})
	for _, e := range m.undoStack[:len(m.undoStack)-1] {
		if slices.Contains(e.batches[0].ids, "x5") {
			t.Fatalf("an older entry still holds x5, which a newer action moved")
		}
	}
	m.undoStack = nil
	m.recordUndo(undoEntry{verb: "Archived", batches: batch("m1")})
	m.unspamEmailByID("m1")
	if len(m.undoStack) != 0 {
		t.Errorf("unspam moved m1 to the inbox, but %d entries still hold it", len(m.undoStack))
	}
}

// Undoing a spam mark takes back its training as well as its move: each
// registration the mark made is unregistered, and the ledger forgets the
// message, so a later mark trains it again. It is not trained as ham.
func TestUndo_SpamMarkUntrains(t *testing.T) {
	b := undoFixture(t, "jmap")
	filter := bogofiltertest.InstallLearning(t)
	ledger := train.MessageIDPath(b.m.cfg.ConfigDir)
	b.keys("S")
	if filter.Count(t, "-s") != 1 || !train.OpenOrEmpty(ledger).Has("m1") {
		t.Fatalf("the mark registered %d times and recorded m1 = %v; want 1 and true",
			filter.Count(t, "-s"), train.OpenOrEmpty(ledger).Has("m1"))
	}
	b.keys("z")
	if b.m.showError {
		t.Fatalf("undo failed: %v", b.m.err)
	}
	if got := filter.Count(t, "-S"); got != 1 {
		t.Errorf("unregistrations = %d, want 1", got)
	}
	if got := filter.Count(t, "-n"); got != 0 {
		t.Errorf("undo trained m1 as ham %d times", got)
	}
	if train.OpenOrEmpty(ledger).Has("m1") {
		t.Error("the ledger still records m1 as trained")
	}
	if info, err := msg.GetInfo(b.acct, "m1"); err == nil && info.Override == msg.OverrideSpam {
		t.Error("m1 is back in the inbox still marked as spam")
	}
	if !strings.HasPrefix(b.m.saveStatus, "Undone:") {
		t.Errorf("status = %q, want the undo reported", b.m.saveStatus)
	}
}
