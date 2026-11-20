package virtual

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/cache/folderindex"
	"github.com/sarielhp/feral/email"
)

func TestVirtualMailbox_SaveLoad(t *testing.T) {
	tempDir := t.TempDir()

	vm := &Mailbox{
		Name:        "test-virtual",
		Description: "A test virtual mailbox",
		MessageIDs:  []email.ID{"msg1", "msg2", "msg3"},
		FolderMap: map[email.ID]string{
			"msg1": "INBOX",
			"msg2": "Sent",
			"msg3": "Archive",
		},
	}

	if err := Save(cachedir.At(tempDir), vm); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	loaded, err := Load(cachedir.At(tempDir), "test-virtual")
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if loaded.Name != vm.Name {
		t.Errorf("expected name %q, got %q", vm.Name, loaded.Name)
	}
	if len(loaded.MessageIDs) != 3 {
		t.Errorf("expected 3 message IDs, got %d", len(loaded.MessageIDs))
	}
	if loaded.FolderMap["msg2"] != "Sent" {
		t.Errorf("expected FolderMap[msg2] = 'Sent', got %q", loaded.FolderMap["msg2"])
	}

	folder := FindMessageFolder(cachedir.At(tempDir), "msg3")
	if folder != "Archive" {
		t.Errorf("expected FindMessageFolder('msg3') = 'Archive', got %q", folder)
	}
}

// TestConcurrentSaveOfSameMailboxNeverProducesAMangledFile is M7's
// regression: concurrent Save calls for the same virtual mailbox name used
// to write through a shared, unlocked "<name>.json.tmp", so their writes
// could interleave and publish neither writer's JSON. Every concurrent
// writer's data is self-consistent (a single message ID mapped to a distinct
// folder), so whatever wins the race must still parse and its message ID
// must still resolve to its own folder -- never a mix. Run with -race.
func TestConcurrentSaveOfSameMailboxNeverProducesAMangledFile(t *testing.T) {
	tempDir := t.TempDir()
	const writers = 8

	var wg sync.WaitGroup
	for w := range writers {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			id := email.ID(fmt.Sprintf("msg-from-writer-%02d", w))
			vm := &Mailbox{
				Name:       "contested",
				MessageIDs: []email.ID{id},
				FolderMap:  map[email.ID]string{id: fmt.Sprintf("Folder%02d", w)},
			}
			if err := Save(cachedir.At(tempDir), vm); err != nil {
				t.Errorf("Save: %v", err)
			}
		}(w)
	}
	wg.Wait()

	loaded, err := Load(cachedir.At(tempDir), "contested")
	if err != nil {
		t.Fatalf("Load after concurrent Save: %v", err)
	}
	if len(loaded.MessageIDs) != 1 {
		t.Fatalf("expected exactly one writer's single-ID mailbox to win, got %v", loaded.MessageIDs)
	}
	id := loaded.MessageIDs[0]
	folder, ok := loaded.FolderMap[id]
	if !ok || folder == "" {
		t.Fatalf("winning message ID %q has no matching folder entry (mangled write): %+v", id, loaded)
	}
}

// TestFindMessageFolder_NameRequiringEscape is a regression for a bug H4
// introduced: FindMessageFolder used to re-derive a mailbox's name from its
// filename and hand that to Load, which sanitizes whatever name it is
// given. That was harmless while sanitization only ever produced already
// -safe characters, but a name containing one now sanitizes to something
// with '~' in it -- itself not a safe character -- so re-sanitizing the
// filename escapes it a second time and looks under the wrong path.
func TestFindMessageFolder_NameRequiringEscape(t *testing.T) {
	dir := t.TempDir()
	vm := &Mailbox{
		Name:       "search: quarterly.report",
		MessageIDs: []email.ID{"m1"},
		FolderMap:  map[email.ID]string{"m1": "Reports"},
	}
	if err := Save(cachedir.At(dir), vm); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if folder := FindMessageFolder(cachedir.At(dir), "m1"); folder != "Reports" {
		t.Errorf("FindMessageFolder(m1) = %q, want %q", folder, "Reports")
	}
}

func TestListReturnsEveryMailboxSortedByName(t *testing.T) {
	acc := cachedir.At(t.TempDir())
	for _, name := range []string{"zeta", "alpha", "search: from:bob"} {
		if err := Save(acc, &Mailbox{Name: name, MessageIDs: []email.ID{email.ID(name)}}); err != nil {
			t.Fatalf("Save(%q): %v", name, err)
		}
	}

	got, err := List(acc)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []string{"alpha", "search: from:bob", "zeta"}
	if len(got) != len(want) {
		t.Fatalf("List returned %d mailboxes, want %d: %+v", len(got), len(want), got)
	}
	for i, name := range want {
		if got[i].Name != name {
			t.Errorf("List()[%d].Name = %q, want %q", i, got[i].Name, name)
		}
	}
}

func TestListOfEmptyCacheIsEmptyNotAnError(t *testing.T) {
	got, err := List(cachedir.At(t.TempDir()))
	if err != nil {
		t.Fatalf("List on an empty cache: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("List on an empty cache returned %d mailboxes, want 0", len(got))
	}
}

func TestListSkipsMalformedMailbox(t *testing.T) {
	dir := t.TempDir()
	acc := cachedir.At(dir)
	if err := Save(acc, &Mailbox{Name: "good", MessageIDs: []email.ID{"m1"}}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := os.WriteFile(filepath.Join(virtualDir(dir), "broken.json"), []byte("{not json"), 0600); err != nil {
		t.Fatalf("write broken mailbox: %v", err)
	}

	got, err := List(acc)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].Name != "good" {
		t.Errorf("List must skip the malformed file and keep the good one, got %+v", got)
	}
}

func TestDeleteRemovesMailboxAndItsIndexMirror(t *testing.T) {
	acc := cachedir.At(t.TempDir())
	if err := Save(acc, &Mailbox{Name: "gone", MessageIDs: []email.ID{"m1", "m2"}}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := Delete(acc, "gone"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := Load(acc, "gone"); err == nil {
		t.Error("Load after Delete must fail")
	}
	ids, err := folderindex.IDs(acc, "virtual/gone")
	if err != nil {
		t.Fatalf("folderindex.IDs: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("the label-index mirror must be cleared, got %v", ids)
	}
	if err := Delete(acc, "gone"); err != nil {
		t.Errorf("Delete of an absent mailbox must be a no-op, got %v", err)
	}
}

func TestNameForQuery(t *testing.T) {
	cases := []struct{ query, want string }{
		{"from:bob@example.com", "from:bob@example.com"},
		{"  invoice  ", "invoice"},
		{"", "search"},
		{"   ", "search"},
	}
	for _, tc := range cases {
		if got := NameForQuery(tc.query); got != tc.want {
			t.Errorf("NameForQuery(%q) = %q, want %q", tc.query, got, tc.want)
		}
	}
}
