package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/sarielhp/feral/cache/cachedir"
)

func TestRecordRecentFoldersOrdersDeduplicatesAndCaps(t *testing.T) {
	account := cachedir.At(t.TempDir())
	if got, err := LoadRecentFolders(account); err != nil || len(got) != 0 {
		t.Fatalf("a missing file must read as an empty list, got %q, %v", got, err)
	}
	if _, err := RecordRecentFolders(account, "INBOX", "", "Work"); err != nil {
		t.Fatal(err)
	}
	got, err := RecordRecentFolders(account, "Lists", "INBOX")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"INBOX", "Lists", "Work"}; !slices.Equal(got, want) {
		t.Errorf("list = %q, want %q (newest first, no duplicates, no empty names)", got, want)
	}
	for i := range RecentFoldersLimit + 3 {
		if _, recordErr := RecordRecentFolders(account, fmt.Sprintf("F%d", i)); recordErr != nil {
			t.Fatal(recordErr)
		}
	}
	got, err = LoadRecentFolders(account)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != RecentFoldersLimit || got[0] != fmt.Sprintf("F%d", RecentFoldersLimit+2) {
		t.Errorf("list = %q, want the newest %d folders", got, RecentFoldersLimit)
	}
}

func TestRecordRecentFoldersPreservesADamagedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, recentFoldersFile)
	if err := os.WriteFile(path, []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRecentFolders(cachedir.At(dir)); err == nil {
		t.Error("a damaged file must be an error, not an empty list")
	}
	if _, err := RecordRecentFolders(cachedir.At(dir), "INBOX"); err == nil {
		t.Error("recording over a damaged file must fail")
	}
	if data, _ := os.ReadFile(path); string(data) != "{not json" {
		t.Errorf("the damaged file was overwritten: %q", data)
	}
}

func TestRecentFoldersWithoutAnAccountCacheDoNothing(t *testing.T) {
	var none cachedir.Account
	if got, err := RecordRecentFolders(none, "INBOX"); err != nil || got != nil {
		t.Errorf("RecordRecentFolders on no cache = %q, %v", got, err)
	}
	if got, err := LoadRecentFolders(none); err != nil || got != nil {
		t.Errorf("LoadRecentFolders on no cache = %q, %v", got, err)
	}
}
