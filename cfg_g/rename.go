package cfg_g

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sarielhp/feral/cache/dirlock"
	"github.com/sarielhp/feral/fileformat"
)

// Renaming an account moves several things that are named after it -- its
// directory under accounts/, its cache, its token, its watcher state -- and
// then, last, its name in config.json. No single write does all of that, so
// the rename leaves a marker (RenameMarkerFile) in the configuration directory
// for as long as it runs, and every command but `account rename` refuses to
// load the configuration while it is there.
//
// The refusal is the point. Half-renamed, config.json still names the old
// account while its files are under the new name; loading it then reads the
// account without its account.json and rules, and the next command that saves
// the configuration writes that loss back. Rerunning the same rename finishes
// it: each step is "move if only the old one is there".

// RenameMarkerFile is the marker an account rename keeps while it runs.
const RenameMarkerFile = "account_rename.json"

// RenameMarker is an account rename in progress.
type RenameMarker struct {
	Format int    `json:"format,omitempty"`
	From   string `json:"from"`
	To     string `json:"to"`
}

// RenameMove is one path an account rename moves.
type RenameMove struct{ From, To string }

// AccountRename is what renaming an account did, or under --dry-run would do.
type AccountRename struct {
	From, To string
	// Resumed is true when this run finished an interrupted rename.
	Resumed bool
	// Moves are the paths named after the account that exist, each with the
	// path it moves to.
	Moves []RenameMove
	// Requeued counts the queued outgoing messages relabelled.
	Requeued int
}

// RenameInProgressError reports a rename that was interrupted.
type RenameInProgressError struct {
	From, To string
}

func (e *RenameInProgressError) Error() string {
	return fmt.Sprintf("renaming account %q to %q was interrupted; finish it with: feral account rename %s %s",
		e.From, e.To, e.From, e.To)
}

// PendingRename returns the rename in progress in configDir, or nil.
func PendingRename(configDir string) (*RenameMarker, error) {
	path := filepath.Join(configDir, RenameMarkerFile)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var m RenameMarker
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, damagedFile(path, err)
	}
	if err := fileformat.Check(path, m.Format); err != nil {
		return nil, err
	}
	if m.From == "" || m.To == "" {
		return nil, damagedFile(path, errors.New("it names no rename"))
	}
	return &m, nil
}

// checkNoPendingRename is the gate the loader applies.
func checkNoPendingRename(configDir string) error {
	m, err := PendingRename(configDir)
	if err != nil {
		return err
	}
	if m != nil {
		return &RenameInProgressError{From: m.From, To: m.To}
	}
	return nil
}

// BeginRename records the rename from -> to as in progress.
func BeginRename(configDir, from, to string) error {
	data, err := json.MarshalIndent(RenameMarker{Format: fileformat.Current, From: from, To: to}, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(filepath.Join(configDir, RenameMarkerFile), data, 0o600)
}

// EndRename removes the marker.
func EndRename(configDir string) error {
	err := os.Remove(filepath.Join(configDir, RenameMarkerFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// CommitAccountRename renames account from to to in config.json itself, and
// in default_account, default_send_account and each link that names it. It edits the
// file's JSON in place rather than loading and saving the configuration: a
// save rewrites every account's own files from what was loaded, and mid-rename
// the account's files are not where its old name says.
//
// It is idempotent: a config.json that already names to is left as it is.
func CommitAccountRename(configPath, from, to string) error {
	if configWritesForbidden.Load() {
		return ErrConfigWriteForbidden
	}
	// The same lock SaveConfigFile holds, so a save in another process
	// neither reads half of this edit nor writes over it.
	release, err := dirlock.GuardFile(configPath)
	defer release()
	if err != nil {
		return fmt.Errorf("locking %s: %w", configPath, err)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}
	var top map[string]json.RawMessage
	if err = json.Unmarshal(data, &top); err != nil {
		return damagedFile(configPath, err)
	}
	changed, err := renameInAccounts(top, from, to)
	if err != nil {
		return fmt.Errorf("%s: %w", configPath, err)
	}
	linksChanged, err := renameInLinks(top, from, to)
	if err != nil {
		return fmt.Errorf("%s: %w", configPath, err)
	}
	changed = changed || linksChanged
	for _, key := range []string{"default_account", "default_send_account"} {
		var v string
		if raw, ok := top[key]; ok && json.Unmarshal(raw, &v) == nil && strings.EqualFold(v, from) {
			top[key], _ = json.Marshal(to)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	out, err := json.MarshalIndent(top, "", "  ")
	if err != nil {
		return err
	}
	_, err = writeShared(configPath, out)
	return err
}

// renameInAccounts renames the accounts[] entry named from, and reports
// whether it changed anything.
func renameInAccounts(top map[string]json.RawMessage, from, to string) (bool, error) {
	var accounts []map[string]json.RawMessage
	if raw, ok := top["accounts"]; ok {
		if err := json.Unmarshal(raw, &accounts); err != nil {
			return false, fmt.Errorf("accounts: %w", err)
		}
	}
	changed := false
	for _, acc := range accounts {
		var name string
		if json.Unmarshal(acc["name"], &name) != nil || !strings.EqualFold(name, from) || name == to {
			continue
		}
		acc["name"], _ = json.Marshal(to)
		changed = true
	}
	if !changed {
		return false, nil
	}
	raw, err := json.Marshal(accounts)
	if err != nil {
		return false, err
	}
	top["accounts"] = raw
	return true, nil
}

// renameInLinks points every link whose account is from at to, and reports
// whether it changed anything. A link's name is left as it is, even one equal
// to the old account name: the symlink on disk is the user's command. Each
// link is edited as raw JSON, so a field this build does not know survives.
func renameInLinks(top map[string]json.RawMessage, from, to string) (bool, error) {
	raw, ok := top["links"]
	if !ok || string(raw) == "null" {
		return false, nil
	}
	var links map[string]map[string]json.RawMessage
	if err := json.Unmarshal(raw, &links); err != nil {
		return false, fmt.Errorf("links: %w", err)
	}
	changed := false
	for _, l := range links {
		var account string
		if json.Unmarshal(l["account"], &account) != nil || !strings.EqualFold(account, from) || account == to {
			continue
		}
		l["account"], _ = json.Marshal(to)
		changed = true
	}
	if !changed {
		return false, nil
	}
	out, err := json.Marshal(links)
	if err != nil {
		return false, err
	}
	top["links"] = out
	return true, nil
}
