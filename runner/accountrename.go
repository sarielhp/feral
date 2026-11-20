package runner

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/mailops"
	"github.com/sarielhp/feral/outbox"
	"github.com/sarielhp/feral/snooze"
	"github.com/sarielhp/feral/watcher"
)

type (
	// AccountRename is cfg_g's, so the CLI can name it without this package.
	AccountRename = cfg_g.AccountRename
	// RenameMove is one path a rename moves.
	RenameMove = cfg_g.RenameMove
)

// RenameAccount renames an account: its name in config.json (and in
// default_account and default_send_account), and every file named after it --
// its directory of settings, its OAuth token, its cache, its watcher state and
// its queued messages. See cfg_g.RenameMarkerFile for how an interrupted
// rename is kept from being misread, and finished by running it again.
//
// ident is the account as the user names it; to must pass
// cfg_g.CheckNewAccountName. Under --dry-run it returns the plan and changes
// nothing.
func RenameAccount(config *cfg_g.Config, ident, to string) (AccountRename, error) {
	configDir, configPath, err := cfg_g.ResolveConfigPath(config)
	if err != nil {
		return AccountRename{}, err
	}
	pending, err := cfg_g.PendingRename(configDir)
	if err != nil {
		return AccountRename{}, err
	}
	fc, err := cfg_g.LoadConfigFile(configPath)
	if err != nil {
		return AccountRename{}, err
	}
	acc, err := renameSubject(*fc.Accounts, pending, ident, to)
	if err != nil {
		return AccountRename{}, err
	}
	plan := AccountRename{From: acc.Name, To: to, Resumed: pending != nil}
	if pending != nil {
		plan.From = pending.From
	}
	if plan.Moves, err = renameMoves(configDir, config.CacheRoot, acc.Type, plan.From, to); err != nil {
		return plan, err
	}
	if config.DryRun {
		return plan, nil
	}
	release, err := watcher.HoldWatchLock(config.CacheRoot)
	if err != nil {
		return plan, fmt.Errorf("cannot rename an account while a watcher runs (stop `feral watch` and close the TUI): %w", err)
	}
	defer release()
	if err = cfg_g.BeginRename(configDir, plan.From, to); err != nil {
		return plan, err
	}
	if plan.Requeued, err = applyRename(config.CacheRoot, plan); err != nil {
		return plan, err
	}
	if err := cfg_g.CommitAccountRename(configPath, plan.From, to); err != nil {
		return plan, err
	}
	return plan, cfg_g.EndRename(configDir)
}

// renameSubject finds the account a rename acts on. With a rename pending, it
// is that rename or nothing: the configuration is half moved, and a
// different rename would build on it.
func renameSubject(accounts []cfg_acc.AccountConfig, pending *cfg_g.RenameMarker, ident, to string) (*cfg_acc.AccountConfig, error) {
	if pending != nil {
		if !strings.EqualFold(ident, pending.From) || to != pending.To {
			return nil, &cfg_g.RenameInProgressError{From: pending.From, To: pending.To}
		}
		for _, name := range []string{pending.From, pending.To} {
			for i := range accounts {
				if accounts[i].Name == name {
					return &accounts[i], nil
				}
			}
		}
		return nil, fmt.Errorf("the interrupted rename of %q to %q names no configured account", pending.From, pending.To)
	}
	if ident == "" {
		return nil, fmt.Errorf("name the account to rename")
	}
	acc, err := cfg_g.Lookup(accounts, ident)
	if err != nil {
		return nil, err
	}
	if acc.Name == to {
		return nil, fmt.Errorf("account %q is already named %q", ident, to)
	}
	self := -1
	for i := range accounts {
		if accounts[i].Name == acc.Name {
			self = i
		}
	}
	if err := cfg_g.CheckNewAccountName(accounts, to, self); err != nil {
		return nil, err
	}
	return acc, nil
}

// renameMoves pairs every path named after from with the path named after to,
// keeping those that exist. A pair whose both ends exist is refused before
// anything moves: one of them belongs to something else, and the user must
// say which.
func renameMoves(configDir, cacheRoot, accType, from, to string) ([]RenameMove, error) {
	oldPaths := cfg_g.AccountFiles(configDir, from)
	newPaths := cfg_g.AccountFiles(configDir, to)
	oldPaths = append(oldPaths, mailops.TokenFiles(configDir, cfg_acc.AccountConfig{Name: from, Type: accType})...)
	newPaths = append(newPaths, mailops.TokenFiles(configDir, cfg_acc.AccountConfig{Name: to, Type: accType})...)
	if cacheRoot != "" {
		oldPaths = append(oldPaths, cachedir.ForAccount(cacheRoot, from).Path())
		newPaths = append(newPaths, cachedir.ForAccount(cacheRoot, to).Path())
	}
	var moves []RenameMove
	seen := map[string]bool{}
	for i, src := range oldPaths {
		dst := newPaths[i]
		if src == dst || seen[src] || !exists(src) {
			continue
		}
		seen[src] = true
		if exists(dst) {
			return nil, fmt.Errorf("cannot rename %q to %q: both %s and %s exist; move one of them aside first", from, to, src, dst)
		}
		moves = append(moves, RenameMove{From: src, To: dst})
	}
	return moves, nil
}

// applyRename moves the files, then renames the account in the watcher state
// and the outbox. Every step is idempotent, so a rerun after a crash redoes
// what is left.
func applyRename(cacheRoot string, plan AccountRename) (int, error) {
	for _, m := range plan.Moves {
		if err := moveIfPresent(m); err != nil {
			return 0, err
		}
	}
	if cacheRoot == "" {
		return 0, nil
	}
	if err := watcher.RenameAccount(cacheRoot, plan.From, plan.To); err != nil {
		return 0, fmt.Errorf("watcher state: %w", err)
	}
	if err := snooze.Relabel(cacheRoot, plan.From, plan.To); err != nil {
		return 0, fmt.Errorf("snooze record: %w", err)
	}
	n, err := outbox.Relabel(outbox.Dir(cacheRoot), plan.From, plan.To)
	if err != nil {
		return n, fmt.Errorf("outbox: %w", err)
	}
	return n, nil
}

// moveIfPresent renames m.From to m.To; a source already gone is a move a
// previous run made.
func moveIfPresent(m RenameMove) error {
	if !exists(m.From) {
		return nil
	}
	if exists(m.To) {
		return fmt.Errorf("both %s and %s exist; move one of them aside, then run the rename again", m.From, m.To)
	}
	if err := os.MkdirAll(filepath.Dir(m.To), 0o700); err != nil {
		return err
	}
	return os.Rename(m.From, m.To)
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil || !errors.Is(err, os.ErrNotExist)
}
