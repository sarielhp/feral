package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/label"
	"github.com/sarielhp/feral/mailclient"
)

// acrossCall records one MoveAcross call.
type acrossCall struct {
	src, dst     string
	source, dest string
	ids          []email.ID
}

// accountMoveFixture is an mbox account "Local" with two tagged messages,
// beside a server account "Home"; MoveAcross records its calls and moves
// everything except the IDs in refuse.
func accountMoveFixture(t *testing.T, refuse ...email.ID) (*tuiModel, *[]acrossCall, *mailclient.MockMailClient) {
	t.Helper()
	accounts := []cfg_acc.AccountConfig{{Name: "Local", Type: "mbox", Path: "/nonexistent"}, {Name: "Home", Type: "gmail"}}
	src := &mailclient.MockMailClient{BackendTypeVal: "mbox", InboxFolderVal: "INBOX",
		Cfg: &cfg_g.Config{SelectedAccount: "Local", Accounts: accounts, AccountCache: cachedir.At(t.TempDir())}}
	home := &mailclient.MockMailClient{BackendTypeVal: "gmail", InboxFolderVal: "INBOX", Labels: []string{"INBOX", "receipts"},
		Cfg: &cfg_g.Config{SelectedAccount: "Home", Accounts: accounts, AccountCache: cachedir.At(t.TempDir())}}
	calls := new([]acrossCall)
	bk := &Backend{
		Clients: stubClients{ForAccountFn: func(_ *cfg_g.Config, acc cfg_acc.AccountConfig) (mailclient.MailClient, error) {
			if acc.Name == "Home" {
				return home, nil
			}
			return nil, errors.New("unexpected account " + acc.Name)
		}},
		Transfer: stubTransfer{MoveAcrossFn: func(s, d mailclient.MailClient, source label.Folder, ids []email.ID, dest label.Folder) ([]email.ID, error) {
			*calls = append(*calls, acrossCall{src: string(s.Config().SelectedAccount), dst: string(d.Config().SelectedAccount),
				source: source.String(), dest: dest.String(), ids: ids})
			var moved []email.ID
			for _, id := range ids {
				if !slicesContains(refuse, id) {
					moved = append(moved, id)
				}
			}
			if len(moved) < len(ids) {
				return moved, errors.New("left in the source")
			}
			return moved, nil
		}},
	}
	m := newTuiModel(src, "INBOX", bk)
	m.cfg = src.Cfg
	m.width, m.height = 100, 40
	m.mode = modeIndex
	m.emails = []email.Email{{ID: "m1", Subject: "one"}, {ID: "m2", Subject: "two"}}
	m.rawEmails = m.emails
	m.selectedID = "m1"
	m.taggedIDs = map[email.ID]bool{"m1": true, "m2": true}
	return m, calls, home
}

func slicesContains(ids []email.ID, id email.ID) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

// ;a in an mbox account offers the first server account's inbox; Enter moves
// the tagged messages there through a transfer, under a busy notice that
// keys cannot get past, and the moved rows leave the list.
func TestAccountMoveFromMbox(t *testing.T) {
	m, calls, _ := accountMoveFixture(t)
	press(m, ";")
	press(m, "a")
	if !m.inMovePrompt || m.moveQuery != "Home:INBOX" {
		t.Fatalf("prompt open %v with %q; want Home:INBOX offered", m.inMovePrompt, m.moveQuery)
	}
	cmd := press(m, "enter")
	if m.showLoad != loadMovingAccounts || !m.busyBlocks("d") || !m.loadIsMutation() {
		t.Fatalf("busy %q; want a modal that Esc cannot hide", m.showLoad)
	}
	if cmd == nil {
		t.Fatal("no move was started")
	}
	_, _ = m.Update(cmd())
	if len(*calls) != 1 {
		t.Fatalf("MoveAcross calls %+v; want one", *calls)
	}
	c := (*calls)[0]
	if c.src != "Local" || c.dst != "Home" || c.source != "INBOX" || c.dest != "INBOX" || len(c.ids) != 2 {
		t.Errorf("MoveAcross(%+v); want both from Local INBOX to Home INBOX", c)
	}
	if m.showLoad != "" || len(m.emails) != 0 || m.showError {
		t.Errorf("after the move: busy %q, %d rows, error %v", m.showLoad, len(m.emails), m.err)
	}
	if !strings.Contains(m.saveStatus, "Moved 2") {
		t.Errorf("status %q; want the count", m.saveStatus)
	}
}

// A message the transfer leaves in the source stays in the list, and the
// error says so.
func TestAccountMoveKeepsWhatDidNotMove(t *testing.T) {
	m, _, _ := accountMoveFixture(t, "m2")
	press(m, ";")
	press(m, "a")
	cmd := press(m, "enter")
	_, _ = m.Update(cmd())
	if len(m.emails) != 1 || m.emails[0].ID != "m2" {
		t.Errorf("rows %v; want m2 kept", m.emails)
	}
	if !m.showError || !strings.Contains(m.err.Error(), "moved 1") {
		t.Errorf("error %v; want the partial move reported", m.err)
	}
}

// A folder with no account prefix is a move within the account, by the
// server's own move and never a transfer.
func TestAccountMoveWithinTheAccount(t *testing.T) {
	m, calls, _ := accountMoveFixture(t)
	src := m.client.(*mailclient.MockMailClient)
	src.Labels = []string{"INBOX", "kept"}
	press(m, ";")
	press(m, "a")
	m.moveQuery = "kep"
	cmd := press(m, "enter")
	if len(m.emails) != 2 {
		t.Fatal("rows left before the move was done")
	}
	_, _ = m.Update(cmd())
	if len(*calls) != 0 || len(src.MovedEmails) != 1 || src.MovedEmails[0].DestLabel != "kept" {
		t.Errorf("transfers %+v, server moves %+v; want one server move to kept", *calls, src.MovedEmails)
	}
	if len(m.emails) != 0 {
		t.Errorf("%d rows left after the move", len(m.emails))
	}
	if !strings.Contains(m.saveStatus, "to kept") {
		t.Errorf("status %q; want the folder the partial name resolved to", m.saveStatus)
	}
}

// The destination must be writable, and must exist: a typo is refused rather
// than filed into a new folder.
func TestAccountMoveRefusals(t *testing.T) {
	m, _, _ := accountMoveFixture(t)
	m.cfg.Accounts[1].ReadOnly = true
	press(m, ";")
	press(m, "a")
	if m.moveQuery != "" {
		t.Errorf("offered %q; a read-only account is no default", m.moveQuery)
	}
	m.moveQuery = "Home:INBOX"
	press(m, "enter")
	if !m.showError || !strings.Contains(m.err.Error(), "read-only") || m.showLoad != "" {
		t.Errorf("into a read-only account: error %v, busy %q", m.err, m.showLoad)
	}

	m, _, _ = accountMoveFixture(t)
	m.cfg.Accounts[1].ReadOnlyFolders = []string{"receipts"}
	press(m, ";")
	press(m, "a")
	m.moveQuery = "Home:receipts"
	press(m, "enter")
	if !m.showError || !strings.Contains(m.err.Error(), "locked") || m.showLoad != "" {
		t.Errorf("into a locked folder: error %v, busy %q", m.err, m.showLoad)
	}

	m, calls, _ := accountMoveFixture(t)
	press(m, ";")
	press(m, "a")
	m.moveQuery = "Home:recipts-typo"
	cmd := press(m, "enter")
	_, _ = m.Update(cmd())
	if len(*calls) != 0 || !m.showError || len(m.emails) != 2 {
		t.Errorf("a missing folder: calls %+v, error %v, %d rows", *calls, m.err, len(m.emails))
	}
}

// In the folder tree ;s opens, : turns it into the prompt, since another
// account's folders are not in the tree.
func TestAccountMoveFromTheMoveTree(t *testing.T) {
	m, _, _ := accountMoveFixture(t)
	press(m, ";")
	press(m, "s")
	if !m.bulkMovingToFolder || !m.treeOpen {
		t.Fatal(";s must open the move tree")
	}
	press(m, ":")
	if m.bulkMovingToFolder || m.treeOpen || !m.inMovePrompt {
		t.Errorf("after :, tree open %v, moving %v, prompt %v; want only the prompt", m.treeOpen, m.bulkMovingToFolder, m.inMovePrompt)
	}
	if got := m.drawnScreen(); got != scrMovePrompt {
		t.Errorf("screen %s; want %s", got, scrMovePrompt)
	}
}

// Moving into the folder the messages are already in changes nothing: on
// Gmail a server move onto the same label would take the label away.
func TestAccountMoveOntoItselfIsANoOp(t *testing.T) {
	m, calls, _ := accountMoveFixture(t)
	src := m.client.(*mailclient.MockMailClient)
	src.Labels = []string{"INBOX"}
	press(m, ";")
	press(m, "a")
	m.moveQuery = "Local:INBOX"
	cmd := press(m, "enter")
	_, _ = m.Update(cmd())
	if len(*calls) != 0 || len(src.MovedEmails) != 0 || len(m.emails) != 2 {
		t.Errorf("transfers %+v, moves %+v, %d rows; want nothing done", *calls, src.MovedEmails, len(m.emails))
	}
	if !strings.Contains(m.saveStatus, "already in") {
		t.Errorf("status %q; want it to say the messages are already there", m.saveStatus)
	}
}

// A background mail check that lands while a transfer is under way leaves
// the busy notice up: keys must not reach rows that are being moved.
func TestAccountMoveBusyOutlivesABackgroundCheck(t *testing.T) {
	m, _, _ := accountMoveFixture(t)
	press(m, ";")
	press(m, "a")
	_ = press(m, "enter")
	_, _ = m.Update(bgCheckDoneMsg{folder: "INBOX", foreground: true})
	_, _ = m.Update(emailsLoadedMsg{client: m.client, folder: "INBOX", emails: m.emails})
	if m.showLoad != loadMovingAccounts || !m.busyBlocks("d") {
		t.Errorf("busy %q after a background check; want the move's notice kept", m.showLoad)
	}
}
