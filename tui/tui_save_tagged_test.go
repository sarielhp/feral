package tui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/cache/virtual"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/label"
	"github.com/sarielhp/feral/mailclient"
	"github.com/sarielhp/feral/tui/render"
)

// mboxCall records one AppendMessagesToMbox call, so a test can prove where the
// writer was pointed and which messages it was handed.
type mboxCall struct {
	path    string
	sources []MboxSource
}

// taggedSetFixture is a session in a real folder with two tagged messages and a
// wired mbox writer, which is what ;v and ;x act on.
func taggedSetFixture(t *testing.T) (*tuiModel, *[]mboxCall) {
	t.Helper()
	cacheDir := t.TempDir()
	mock := &mailclient.MockMailClient{
		Cfg: &cfg_g.Config{SelectedAccount: "Work", AccountCache: cachedir.At(cacheDir)},
		RawMessages: map[email.ID][]byte{
			"m1": []byte("From: Alice <alice@example.com>\r\nSubject: one\r\n\r\nbody\r\n"),
			"m2": []byte("From: Bob <bob@example.com>\r\nSubject: two\r\n\r\nbody\r\n"),
		},
	}
	calls := new([]mboxCall)
	bk := &Backend{Mbox: stubMbox{AppendFn: func(path string, sources []MboxSource) (int, error) {
		n := 0
		for _, s := range sources {
			n += len(s.IDs)
		}
		*calls = append(*calls, mboxCall{path: path, sources: sources})
		return n, nil
	}}}

	m := newTuiModel(mock, "INBOX", bk)
	m.width, m.height = 100, 40
	m.mode = modeIndex
	m.folders = []label.Item{{Name: "INBOX", FullName: "INBOX"}}
	m.emails = []email.Email{{ID: "m1", Subject: "one"}, {ID: "m2", Subject: "two"}}
	m.eIdx = 0
	m.selectedID = "m1"
	m.taggedIDs = map[email.ID]bool{"m1": true, "m2": true}
	return m, calls
}

// typeName sends a name into whichever prompt is open.
func typeName(m *tuiModel, name string) {
	for _, r := range name {
		press(m, string(r))
	}
	press(m, "enter")
}

func TestTagPrefixVSavesATaggedSet(t *testing.T) {
	m, _ := taggedSetFixture(t)
	press(m, ";")
	if !m.inTagPrefix {
		t.Fatal("; must arm the tag prefix")
	}
	press(m, "v")
	if !m.inSaveNamePrompt || m.saveNameKind != saveNameVirtualFolder {
		t.Fatal(";v must open the save-name prompt")
	}
	typeName(m, "quarterly")

	vm, err := virtual.Load(m.client.Config().AccountCache, "quarterly")
	if err != nil {
		t.Fatalf("the tagged set was not saved: %v", err)
	}
	if len(vm.MessageIDs) != 2 {
		t.Errorf("saved %d message(s), want 2", len(vm.MessageIDs))
	}
	if vm.FolderMap["m1"] != "INBOX" || vm.FolderMap["m2"] != "INBOX" {
		t.Errorf("FolderMap = %v, want INBOX for both", vm.FolderMap)
	}
	if !strings.Contains(m.View(), "quarterly") {
		t.Error("the confirmation must be on screen")
	}
}

// The tags survive a save: nothing left the view, so the same set can be
// exported to mbox afterwards.
func TestTagPrefixVKeepsTheTags(t *testing.T) {
	m, _ := taggedSetFixture(t)
	press(m, ";")
	press(m, "v")
	typeName(m, "kept")

	if m.countTagged() != 2 {
		t.Errorf("saved with %d tagged, want the set kept", m.countTagged())
	}
}

func TestTagPrefixVNeedsSomethingTagged(t *testing.T) {
	m, _ := taggedSetFixture(t)
	m.taggedIDs = nil

	press(m, ";")
	press(m, "v")
	if m.inSaveNamePrompt {
		t.Error(";v with nothing tagged must not open the prompt")
	}
	if !strings.Contains(m.notice, "Tag messages") {
		t.Errorf("notice = %q, want a hint to tag first", m.notice)
	}
}

func TestSaveNamePromptEscapeSavesNothing(t *testing.T) {
	m, _ := taggedSetFixture(t)
	press(m, ";")
	press(m, "v")
	press(m, "esc")

	if m.inSaveNamePrompt || m.saveNameKind != saveNameNone {
		t.Error("Esc must close the name prompt")
	}
	if boxes, _ := virtual.List(m.client.Config().AccountCache); len(boxes) != 0 {
		t.Errorf("Esc saved %d set(s), want none", len(boxes))
	}
}

// A saved search is one account's, so a set spanning accounts cannot be
// frozen: the refusal names the mbox export, which has no such limit.
func TestTagPrefixVRefusesASetSpanningAccounts(t *testing.T) {
	m, _, _ := unifiedFixture()
	m.width, m.height = 100, 40
	m.theme = render.NewThemeManager()
	m.taggedIDs = map[email.ID]bool{"p1": true, "w1": true}

	press(m, ";")
	press(m, "v")
	typeName(m, "mixed")

	if m.err == nil || !strings.Contains(m.err.Error(), "spans accounts") {
		t.Fatalf("err = %v, want the cross-account refusal", m.err)
	}
	if !strings.Contains(m.err.Error(), ";x") {
		t.Errorf("the refusal must name the mbox export: %v", m.err)
	}
}

// Without a configured cache root there is nowhere to write the set, and a
// relative path would land in the working directory.
func TestTagPrefixVRefusesWithoutACacheRoot(t *testing.T) {
	m, _ := taggedSetFixture(t)
	m.client.Config().AccountCache = cachedir.Account{}

	press(m, ";")
	press(m, "v")
	typeName(m, "nowhere")

	if m.err == nil || !strings.Contains(m.err.Error(), "cache directory") {
		t.Fatalf("err = %v, want the no-cache refusal", m.err)
	}
}

func TestTagPrefixVRefusesInAResultSetView(t *testing.T) {
	for _, folder := range []string{searchFolderPrefix + "q", virtualFolderPrefix + "last"} {
		m, _ := taggedSetFixture(t)
		m.currentF = folder

		press(m, ";")
		press(m, "v")
		typeName(m, "nested")

		if m.err == nil || !strings.Contains(m.err.Error(), "not one folder") {
			t.Errorf("%q: err = %v, want the result-set refusal", folder, m.err)
		}
	}
}

// A curated set is the user's work, and virtual.Save replaces the file it is
// in, so a name that is taken is refused rather than overwritten in silence.
func TestTagPrefixVRefusesToOverwriteASavedSearch(t *testing.T) {
	m, _ := taggedSetFixture(t)
	press(m, ";")
	press(m, "v")
	typeName(m, "quarterly")
	if m.err != nil {
		t.Fatalf("the first save failed: %v", m.err)
	}

	press(m, ";")
	press(m, "v")
	typeName(m, "quarterly")

	if m.err == nil || !strings.Contains(m.err.Error(), "already exists") {
		t.Fatalf("err = %v, want the overwrite refusal", m.err)
	}
	if !strings.Contains(m.err.Error(), "F7") {
		t.Errorf("the refusal must say how to replace it: %v", m.err)
	}
	vm, err := virtual.Load(m.client.Config().AccountCache, "quarterly")
	if err != nil {
		t.Fatalf("the original set is gone: %v", err)
	}
	if len(vm.MessageIDs) != 2 {
		t.Errorf("MessageIDs = %v, want the original two left alone", vm.MessageIDs)
	}
}

func TestTagPrefixXExportsTheTaggedSetToMbox(t *testing.T) {
	m, calls := taggedSetFixture(t)
	press(m, ";")
	press(m, "x")
	if !m.inSaveNamePrompt || m.saveNameKind != saveNameMbox {
		t.Fatal(";x must open the mbox name prompt")
	}
	typeName(m, "corpus")

	if !m.fileBrowserOpen {
		t.Fatal(";x must ask which directory to write into")
	}
	if m.fileBrowser.TargetFileName != "corpus.mbox" {
		t.Errorf("proposed file name = %q, want corpus.mbox", m.fileBrowser.TargetFileName)
	}

	dir := t.TempDir()
	m.fileBrowser.CurrentDir = dir
	cmd := press(m, "s")
	if cmd == nil {
		t.Fatal("confirming the directory must return the write command")
	}
	m.Update(cmd())

	if len(*calls) != 1 {
		t.Fatalf("the writer was called %d time(s), want 1", len(*calls))
	}
	if want := filepath.Join(dir, "corpus.mbox"); (*calls)[0].path != want {
		t.Errorf("wrote to %q, want %q", (*calls)[0].path, want)
	}
	if got := (*calls)[0].sources; len(got) != 1 || len(got[0].IDs) != 2 {
		t.Errorf("sources = %+v, want one account's two ids", got)
	}
	if !strings.Contains(m.notice, "Appended 2 message(s)") {
		t.Errorf("notice = %q, want the append confirmation", m.notice)
	}
}

// Both prompts must render: the top bar switches on which one is open.
func TestSavePromptsRender(t *testing.T) {
	m, _ := taggedSetFixture(t)
	press(m, ";")
	if view := m.View(); !strings.Contains(view, " ; (") {
		t.Errorf("the tag-prefix hint must be on screen, got:\n%s", view)
	}
	press(m, "v")
	if view := m.View(); !strings.Contains(view, "Save as saved search") {
		t.Errorf("the name prompt must be on screen, got:\n%s", view)
	}
	press(m, "esc")
	press(m, ";")
	press(m, "x")
	if view := m.View(); !strings.Contains(view, "Export to mbox") {
		t.Errorf("the mbox prompt must be on screen, got:\n%s", view)
	}
}

func TestTagPrefixXCancelWritesNothing(t *testing.T) {
	m, calls := taggedSetFixture(t)
	press(m, ";")
	press(m, "x")
	typeName(m, "corpus")
	press(m, "esc")

	if m.fileBrowserOpen {
		t.Error("Esc must close the save dialog")
	}
	if m.pendingMbox != nil {
		t.Error("a cancelled export must forget the pending set")
	}
	if len(*calls) != 0 {
		t.Errorf("a cancelled export called the writer %d time(s)", len(*calls))
	}
}

// The export works from a result-set view, where a saved search may not be
// made: it needs the messages, not their folders.
func TestTagPrefixXWorksInAResultSetView(t *testing.T) {
	m, calls := taggedSetFixture(t)
	m.currentF = virtualFolderPrefix + "last"

	press(m, ";")
	press(m, "x")
	typeName(m, "corpus")
	m.fileBrowser.CurrentDir = t.TempDir()
	if cmd := press(m, "s"); cmd != nil {
		m.Update(cmd())
	}

	if len(*calls) != 1 {
		t.Fatalf("the writer was called %d time(s), want 1", len(*calls))
	}
}
