package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/cache/dirlock"
)

// RecentFoldersLimit is how many folders an account's recent list keeps.
const RecentFoldersLimit = 10

// recentFoldersFile names the per-account list of the folders the TUI opened
// most recently, newest first. It lives in the account cache because each
// account has its own folders.
const recentFoldersFile = "recent_folders.json"

// LoadRecentFolders returns the account's most recently opened folders, newest
// first. A missing file is an empty list; a damaged one is an error, so a
// caller does not take it for a fresh start and write over it.
func LoadRecentFolders(account cachedir.Account) ([]string, error) {
	dir := account.Path()
	if dir == "" {
		return nil, nil
	}
	data, err := os.ReadFile(filepath.Join(dir, recentFoldersFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read recent folders: %w", err)
	}
	var folders []string
	if err := json.Unmarshal(data, &folders); err != nil {
		return nil, fmt.Errorf("decode recent folders: %w", err)
	}
	return folders, nil
}

// RecordRecentFolders moves each of folders, in order, to the front of the
// account's list, so the last one named is the newest, and keeps the newest
// RecentFoldersLimit. Empty names are skipped. The update holds the cache lock
// and re-reads the file, so two sessions do not drop each other's visits; a
// damaged file is left as it is and reported.
func RecordRecentFolders(account cachedir.Account, folders ...string) ([]string, error) {
	dir := account.Path()
	if dir == "" {
		return nil, nil
	}
	release := guard(dir)
	defer release()

	list, err := LoadRecentFolders(account)
	if err != nil {
		return nil, fmt.Errorf("preserve unreadable recent folders: %w", err)
	}
	for _, folder := range folders {
		if folder == "" {
			continue
		}
		rest := slices.DeleteFunc(list, func(f string) bool { return f == folder })
		list = append([]string{folder}, rest...)
	}
	if len(list) > RecentFoldersLimit {
		list = list[:RecentFoldersLimit]
	}
	data, err := json.Marshal(list)
	if err != nil {
		return nil, err
	}
	if err := dirlock.AtomicWrite(filepath.Join(dir, recentFoldersFile), data, 0600); err != nil {
		return nil, err
	}
	return list, nil
}
