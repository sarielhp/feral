package tui

import (
	"errors"
	"net/mail"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/mailclient"
	"github.com/sarielhp/feral/mute"
)

// listModel is cachedModel showing two messages from the dev list, one from
// the ops list and one from no list.
func listModel(t *testing.T) *tuiModel {
	t.Helper()
	m := cachedModel(t)
	m.rawEmails = []email.Email{
		{ID: "d1", Subject: "build broken", ListID: "dev.lists.example.org"},
		{ID: "o1", Subject: "dev box down", ListID: "ops.lists.example.org"},
		{ID: "d2", Subject: "release", ListID: "dev.lists.example.org"},
		{ID: "p1", Subject: "lunch?"},
	}
	m.rebuildVisibleEmails()
	return m
}

// visibleIDs is the rows on screen, sorted: these tests are about which rows
// a filter keeps, not the order the sort puts them in.
func visibleIDs(m *tuiModel) string {
	ids := make([]string, 0, len(m.emails))
	for _, em := range m.emails {
		ids = append(ids, string(em.ID))
	}
	slices.Sort(ids)
	return strings.Join(ids, ",")
}

func selectID(t *testing.T, m *tuiModel, id email.ID) {
	t.Helper()
	for i := range m.emails {
		if m.emails[i].ID == id {
			m.eIdx, m.selectedID = i, id
			return
		}
	}
	t.Fatalf("%s is not on screen: %s", id, visibleIDs(m))
}

// "=" narrows the list to the selected message's list through the ordinary
// list filter: pressing it again, or Esc, shows everything again.
func TestListFilterKey(t *testing.T) {
	m := listModel(t)
	selectID(t, m, "d2")

	m.Update(runeKey("="))
	if got := visibleIDs(m); got != "d1,d2" {
		t.Fatalf("after = the list shows %s, want the dev list's d1,d2", got)
	}
	if m.searchQuery != "list:dev.lists.example.org" || m.inSearch {
		t.Fatalf("filter = %q (typing %v), want a committed list: filter", m.searchQuery, m.inSearch)
	}
	if m.selectedID != "d2" {
		t.Errorf("selection moved to %s", m.selectedID)
	}

	m.Update(runeKey("="))
	if got := visibleIDs(m); got != "d1,d2,o1,p1" || m.searchQuery != "" {
		t.Fatalf("a second = left %s (filter %q), want everything", got, m.searchQuery)
	}

	m.Update(runeKey("="))
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if got := visibleIDs(m); got != "d1,d2,o1,p1" || m.searchQuery != "" {
		t.Fatalf("Esc left %s (filter %q), want everything", got, m.searchQuery)
	}
}

// "=" on a message from no list says so and filters nothing.
func TestListFilterKeyOnPlainMessage(t *testing.T) {
	m := listModel(t)
	selectID(t, m, "p1")
	m.Update(runeKey("="))
	if m.searchQuery != "" || visibleIDs(m) != "d1,d2,o1,p1" {
		t.Fatalf("= on a plain message filtered: %q -> %s", m.searchQuery, visibleIDs(m))
	}
	if !strings.Contains(m.saveStatus, "mailing list") {
		t.Errorf("status = %q, want it to say the message is from no list", m.saveStatus)
	}
}

// Typed after "/", list: matches the List-Id alone -- "dev box down" is on
// the ops list and its subject naming dev does not put it on the dev list.
func TestListFilterTyped(t *testing.T) {
	m := listModel(t)
	m.Update(runeKey("/"))
	typeKeys(m, "list:dev")
	if got := visibleIDs(m); got != "d1,d2" {
		t.Fatalf("/list:dev shows %s, want d1,d2", got)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	typeKeys(m, "/dev")
	if got := visibleIDs(m); got != "o1" {
		t.Fatalf("/dev, an ordinary filter, shows %s, want o1, whose subject names dev", got)
	}
}

const listReplyRaw = "From: Alice <alice@example.com>\r\nTo: dev@lists.example.org\r\n" +
	"Cc: carol@example.com\r\nReply-To: alice@home.example\r\nSubject: [dev] plan\r\n" +
	"Message-ID: <plan@example.com>\r\nList-Id: <dev.lists.example.org>\r\n" +
	"List-Post: <mailto:dev@lists.example.org>\r\n\r\nbody\r\n"

// The three reply keys choose three addressings: G goes to the list alone,
// never to the Reply-To or the Cc; r and g keep their old behaviour.
func TestReplyAddressingModes(t *testing.T) {
	orig, err := mail.ReadMessage(strings.NewReader(listReplyRaw))
	if err != nil {
		t.Fatal(err)
	}
	own := email.OwnSet("me@example.com")

	to, cc, err := replyAddressing(orig, false, replyModeForKey("G"), own)
	if err != nil || to != "dev@lists.example.org" || len(cc) != 0 {
		t.Fatalf("G: to %q cc %v err %v, want the list alone", to, cc, err)
	}
	to, cc, err = replyAddressing(orig, false, replyModeForKey("r"), own)
	if err != nil || to != "alice@home.example" || len(cc) != 0 {
		t.Fatalf("r: to %q cc %v err %v, want the Reply-To", to, cc, err)
	}
	to, cc, err = replyAddressing(orig, false, replyModeForKey("g"), own)
	if err != nil || to != "alice@home.example" || len(cc) != 2 {
		t.Fatalf("g: to %q cc %v err %v, want the Reply-To and both other recipients", to, cc, err)
	}
}

// A list reply to a message from no list, or from a closed one, is refused
// before any editor opens -- it is never sent to the sender instead.
func TestReplyToListRefused(t *testing.T) {
	cases := map[string]struct {
		raw  string
		want error
	}{
		"no List-Post": {replyFixtureRaw, email.ErrNoListPost},
		"closed list":  {strings.Replace(listReplyRaw, "<mailto:dev@lists.example.org>", "NO", 1), email.ErrListClosed},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			m, _, work := unifiedFixture()
			work.RawMessages = map[email.ID][]byte{"w1": []byte(tc.raw)}
			work.Cfg.AccountCache = cachedir.At(t.TempDir())
			got, ok := m.replyCmd("w1", replyToList)().(errMsg)
			if !ok {
				t.Fatalf("G on %s did not refuse", name)
			}
			if !errors.Is(got.err, tc.want) {
				t.Errorf("refusal = %v, want %v", got.err, tc.want)
			}
		})
	}
}

// muteModel is listModel with a configuration directory and a reply in a
// thread, selected.
func muteModel(t *testing.T) *tuiModel {
	t.Helper()
	m := listModel(t)
	client, ok := m.client.(*mailclient.MockMailClient)
	if !ok {
		t.Fatalf("listModel's client is %T", m.client)
	}
	m.cfg = &cfg_g.Config{
		Paths:        cfg_g.Paths{ConfigDir: t.TempDir()},
		AccountCache: client.Cfg.AccountCache,
		Accounts:     []cfg_acc.AccountConfig{{Name: "work", Type: "jmap"}},
	}
	client.Cfg = m.cfg
	m.rawEmails = append(m.rawEmails, email.Email{ID: "t2", Subject: "Re: noisy",
		MessageID: "<t2@example.com>", References: "<root@example.com> <t1@example.com>"})
	m.rebuildVisibleEmails()
	selectID(t, m, "t2")
	return m
}

func mutedRoots(t *testing.T, m *tuiModel) []email.HeaderID {
	t.Helper()
	path, err := mute.Path(m.cfg.ConfigDir, "work")
	if err != nil {
		t.Fatal(err)
	}
	list, err := mute.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	var roots []email.HeaderID
	for _, th := range list.Threads {
		roots = append(roots, th.Key)
	}
	return roots
}

// M mutes the selected thread in the active account's list, by its root, and
// M again unmutes it.
func TestMuteKeyToggles(t *testing.T) {
	m := muteModel(t)
	m.Update(runeKey("M"))
	if got := mutedRoots(t, m); len(got) != 1 || got[0] != "<root@example.com>" {
		t.Fatalf("after M muted %v, want the thread root", got)
	}
	if !strings.Contains(m.saveStatus, "muted") {
		t.Errorf("status = %q", m.saveStatus)
	}
	m.Update(runeKey("M"))
	if got := mutedRoots(t, m); len(got) != 0 {
		t.Fatalf("after a second M still muted: %v", got)
	}
}

// Under --read-only M refuses to mute, and under --dry-run it says what it
// would do; neither writes the list.
func TestMuteKeyReadOnlyAndDryRun(t *testing.T) {
	m := muteModel(t)
	m.cfg.ReadOnly = true
	m.Update(runeKey("M"))
	if !m.showError || m.err == nil || !strings.Contains(m.err.Error(), "Read-Only") {
		t.Errorf("M under --read-only: err %v", m.err)
	}
	m.showError, m.err = false, nil
	m.cfg.ReadOnly, m.cfg.DryRun = false, true
	m.Update(runeKey("M"))
	if !strings.Contains(m.saveStatus, "dry-run") {
		t.Errorf("M under --dry-run: status %q", m.saveStatus)
	}
	if got := mutedRoots(t, m); len(got) != 0 {
		t.Errorf("a refused or simulated mute wrote %v", got)
	}
}

// In the unified inbox a row's own account keeps the mute, not the active
// one: that is the account whose watcher sees the thread's mail.
func TestMuteKeyUsesTheRowsAccount(t *testing.T) {
	m := muteModel(t)
	for i := range m.emails {
		if m.emails[i].ID == "t2" {
			m.emails[i].Account = "other"
		}
	}
	m.Update(runeKey("M"))
	path, err := mute.Path(m.cfg.ConfigDir, "other")
	if err != nil {
		t.Fatal(err)
	}
	list, err := mute.Load(path)
	if err != nil || len(list.Threads) != 1 {
		t.Fatalf("the row's account holds %+v (%v), want the mute", list, err)
	}
	if got := mutedRoots(t, m); len(got) != 0 {
		t.Errorf("the active account was given the mute: %v", got)
	}
}
