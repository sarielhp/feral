// Package virtual stores named result sets: a list of message IDs plus the
// folder each came from, saved under the cache directory.
//
// It began as the backing store for the `last` command and lived in that
// package, but nothing about it is specific to recent mail. Any command that
// produces a set of messages the user will want to look through afterwards --
// `last`, `search` -- wants exactly this, and mirroring the set into the folder
// index means `show` and the TUI can browse it like any other folder.
package virtual

import (
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/fileformat"

	"github.com/sarielhp/feral/cache/dirlock"
	"github.com/sarielhp/feral/cache/folderindex"
	"github.com/sarielhp/feral/cfg_g"
)

// Mailbox represents a named virtual folder containing message IDs and their source folders.
type Mailbox struct {
	Format      int                 `json:"format,omitempty"` // file format (see fileformat); absent means 1
	Name        string              `json:"name"`
	Description string              `json:"description,omitempty"`
	MessageIDs  []email.ID          `json:"message_ids"`
	FolderMap   map[email.ID]string `json:"folder_map"` // message ID -> original folder name
}

// virtualDir returns the path to the virtual mailboxes directory in cache.
func virtualDir(downloadDir string) string {
	return filepath.Join(downloadDir, "virtual")
}

// virtualPath returns the path to a specific virtual mailbox JSON file.
func virtualPath(downloadDir, name string) string {
	return filepath.Join(virtualDir(downloadDir), cfg_g.SanitizeLabelForCache(name)+".json")
}

// Save writes a Mailbox to the cache. It also updates the cache/label index
// so that virtual mailboxes can be inspected using standard label cache APIs.
//
// Two writers of the same virtual mailbox -- two concurrent `last`/`search`
// runs, or the TUI racing a scheduled scan against a shared account cache --
// used to share one unlocked "<name>.json.tmp": their writes could interleave
// and a failing rename could replace the previous good file with a mangled
// one (M7). dirlock.AtomicWrite gives the JSON write a unique temp name and
// fsyncs it before rename; the directory guard also covers the label-index
// mirror, so a reader never observes the two out of step.
func Save(account cachedir.Account, vm *Mailbox) error {
	downloadDir := account.Path()
	release, err := dirlock.Guard(downloadDir)
	if err != nil {
		slog.Debug("cache lock unavailable, continuing with in-process lock only",
			slog.String("dir", downloadDir), slog.Any("error", err))
	}
	defer release()

	stamped := *vm
	stamped.Format = fileformat.Current
	data, err := json.Marshal(&stamped)
	if err != nil {
		return err
	}
	p := virtualPath(downloadDir, vm.Name)
	if err := dirlock.AtomicWrite(p, data, 0600); err != nil {
		return err
	}

	// Also mirror into label index under virtual/<name> or virtual_<name>
	virtualLabelName := "virtual/" + vm.Name
	_ = folderindex.ReplaceAll(account, virtualLabelName, vm.MessageIDs)
	return nil
}

// Load reads a Mailbox from the cache.
func Load(account cachedir.Account, name string) (*Mailbox, error) {
	downloadDir := account.Path()
	p := virtualPath(downloadDir, name)
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var vm Mailbox
	if err := json.Unmarshal(data, &vm); err != nil {
		return nil, err
	}
	if err := fileformat.Check(p, vm.Format); err != nil {
		return nil, err
	}
	return &vm, nil
}

// List returns every virtual mailbox in the account's cache, ordered by name.
// A cache with no virtual directory is not an error: nothing saved yet is an
// empty list, not a failure. A file that will not parse is skipped with a
// warning rather than failing the whole listing, so one damaged mailbox cannot
// hide the others.
func List(account cachedir.Account) ([]Mailbox, error) {
	downloadDir := account.Path()
	dir := virtualDir(downloadDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []Mailbox
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			slog.Warn("skipping unreadable virtual mailbox", slog.String("file", e.Name()), slog.Any("error", err))
			continue
		}
		var vm Mailbox
		if err := json.Unmarshal(data, &vm); err != nil {
			slog.Warn("skipping malformed virtual mailbox", slog.String("file", e.Name()), slog.Any("error", err))
			continue
		}
		if err := fileformat.Check(e.Name(), vm.Format); err != nil {
			slog.Warn("skipping a virtual mailbox a newer build wrote", slog.Any("error", err))
			continue
		}
		out = append(out, vm)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Delete removes a virtual mailbox: its file, and the label-index mirror Save
// wrote, so it stops appearing as a folder. A mailbox that is already gone is
// not an error.
func Delete(account cachedir.Account, name string) error {
	downloadDir := account.Path()
	release, err := dirlock.Guard(downloadDir)
	if err != nil {
		slog.Debug("cache lock unavailable, continuing with in-process lock only",
			slog.String("dir", downloadDir), slog.Any("error", err))
	}
	defer release()

	// Clear the mirror rather than removing the key: folderindex has no
	// delete-a-folder operation, and an empty index is what "no messages" is.
	_ = folderindex.ReplaceAll(account, "virtual/"+name, []email.ID{})
	if err := os.Remove(virtualPath(downloadDir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// NameForQuery is the mailbox name a search result set is saved under: the
// query itself. One scratch name for every search meant each run overwrote the
// last; naming by query lets results accumulate, and re-running a query
// replaces only its own set. An empty query falls back to "search".
func NameForQuery(query string) string {
	if q := strings.TrimSpace(query); q != "" {
		return q
	}
	return "search"
}

// FindMessageFolder looks up which source folder a message originated from across all virtual mailboxes or cached indexes.
//
// It used to reconstruct each mailbox's name from its filename
// (strings.TrimSuffix(e.Name(), ".json")) and hand that to Load, which
// re-sanitizes whatever name it is given. That round-trip was a no-op
// before H4, when SanitizeLabelForCache's output was always already a fixed
// point of itself (only [A-Za-z0-9_-], all already safe to pass through
// again). It stopped being one once the fix started escaping unsafe runes
// as "~<hex>~": '~' is not itself safe, so re-sanitizing an already-escaped
// filename escapes it a second time and looks under a path nothing was ever
// written to. Reading each file directly, instead of re-deriving and
// re-sanitizing its name, has no such round-trip to break.
func FindMessageFolder(account cachedir.Account, msgID email.ID) string {
	downloadDir := account.Path()
	// First check virtual mailboxes
	vDir := virtualDir(downloadDir)
	if entries, err := os.ReadDir(vDir); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(vDir, e.Name()))
			if err != nil {
				continue
			}
			var vm Mailbox
			if json.Unmarshal(data, &vm) == nil {
				if f, ok := vm.FolderMap[msgID]; ok && f != "" {
					return f
				}
			}
		}
	}
	return ""
}
