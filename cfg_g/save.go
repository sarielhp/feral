package cfg_g

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/sarielhp/feral/cache/dirlock"
	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/fileformat"
)

// A save writes what this run holds without losing a change another feral
// process made underneath it. A FileConfig remembers, for each shared file,
// the bytes it read at load (fileState.disk) and the file as its loaded
// values render it (fileState.base). At save, under a lock on config.json,
// each file is read again: unchanged since the load, it is written as this
// run holds it; changed, it is merged (mergeSharedFile) -- base the loaded
// values, ours the values now, theirs the disk -- so a setting this run did
// not touch keeps the other process's value.
//
// config.json is decided first. When the merge drops an account another run
// removed or renamed, that account's files are not written: a stale run
// must not recreate the directory of an account that is gone. If this run
// changed that account, the save is refused, so a command does not report
// as saved a change that went nowhere.
//
// After a save the FileConfig's memory still holds only this run's values.
// A file that was merged is marked stale, so the next save of the same
// FileConfig merges again and keeps the other run's edits; one that was not
// records what was written.

// fileState is one shared file as a FileConfig last saw it.
type fileState struct {
	// read is true when disk holds the file as read at load or written at
	// the last save. A file the load never read -- an account whose lists
	// config.json holds inline -- gives a save nothing to tell a change by,
	// so it is written as held, as before.
	read bool
	// disk is the file's bytes, nil when it did not exist.
	disk []byte
	// stale is true after a merge: memory does not hold what is on disk.
	stale bool
	// base is the file as this run's values rendered it then.
	base []byte
}

// writtenAsHeld reports whether a save writes the file as this run holds it
// rather than merging: when this run never read it, or when current, the
// file now (nil if absent), is what this run last saw.
func (s fileState) writtenAsHeld(current []byte) bool {
	if !s.read {
		return true
	}
	if s.stale || (s.disk == nil) != (current == nil) {
		return false
	}
	return bytes.Equal(s.disk, current)
}

// loadState is what a FileConfig saw of each shared file, by path.
type loadState map[string]fileState

// diskReads records, by path, the bytes a load read of each shared file; a
// nil value is a file that was not there. A nil diskReads records nothing.
type diskReads map[string][]byte

func (r diskReads) record(path string, data []byte) {
	if r != nil {
		r[path] = data
	}
}

// newLoadState is the state of a FileConfig just loaded from configPath,
// whose shared files were read as reads records.
func newLoadState(configPath string, fc *FileConfig, reads diskReads) loadState {
	files, err := renderSharedFiles(configPath, fc)
	if err != nil {
		return nil
	}
	st := loadState{}
	for _, f := range files {
		data, read := reads[f.path]
		st[f.path] = fileState{read: read, disk: data, base: f.data}
	}
	return st
}

// sharedFile is one shared file as this run's values render it.
type sharedFile struct {
	path string
	// keep is the type whose unknown keys a plain save carries over from the
	// file on disk (keepUnknownKeys); nil for a list.
	keep reflect.Type
	data []byte
}

// renderSharedFiles is every shared file a save of fc writes: each
// account's files in account order, config.json last.
func renderSharedFiles(configPath string, fc *FileConfig) ([]sharedFile, error) {
	var files []sharedFile
	if fc.Accounts != nil {
		for i := range *fc.Accounts {
			accFiles, err := renderAccountFiles(filepath.Dir(configPath), &(*fc.Accounts)[i])
			if err != nil {
				return nil, err
			}
			files = append(files, accFiles...)
		}
	}
	main, err := renderMainConfig(configPath, fc)
	if err != nil {
		return nil, err
	}
	return append(files, main), nil
}

// renderAccountFiles is acc's account.json -- without the lists, which have
// files of their own -- and its white list, black list and rules.
func renderAccountFiles(configDir string, acc *cfg_acc.AccountConfig) ([]sharedFile, error) {
	dir := AccountDir(configDir, acc.Name)
	parts := []struct {
		name string
		keep reflect.Type
		v    any
	}{
		{"account.json", reflect.TypeFor[cfg_acc.AccountConfig](), accountJSONValue(acc)},
		{"whitelist.json", nil, nonNil(acc.Whitelist)},
		{"blacklist.json", nil, nonNil(acc.Blacklist)},
		{"rules.json", nil, nonNil(acc.Rules)},
	}
	files := make([]sharedFile, 0, len(parts))
	for _, p := range parts {
		data, err := json.Marshal(p.v)
		if err != nil {
			return nil, fmt.Errorf("marshalling %s for account %q: %w", p.name, acc.Name, err)
		}
		files = append(files, sharedFile{path: filepath.Join(dir, p.name), keep: p.keep, data: data})
	}
	return files, nil
}

// accountJSONValue is what account.json holds of acc: everything but the
// lists, which have files of their own.
func accountJSONValue(acc *cfg_acc.AccountConfig) cfg_acc.AccountConfig {
	accDisk := *acc
	accDisk.Whitelist = nil
	accDisk.Blacklist = nil
	accDisk.Rules = nil
	accDisk.Format = fileformat.Current
	return accDisk
}

// normalizeShared is data, a shared file as read from disk, as this build
// renders what it holds: decoded into the type a load decodes it into and
// rendered as a save would, with the keys that type does not know kept. The
// merge compares it with base and ours, rendered the same way, so a
// difference is a change of value -- never of key spelling, or of a default
// a hand-written file leaves out.
func normalizeShared(path string, data []byte) ([]byte, error) {
	var out []byte
	var err error
	var keep reflect.Type
	switch filepath.Base(path) {
	case "config.json":
		var fc FileConfig
		if err = json.Unmarshal(data, &fc); err == nil {
			var f sharedFile
			f, err = renderMainConfig(path, &fc)
			out, keep = f.data, f.keep
		}
	case "account.json":
		var acc cfg_acc.AccountConfig
		if err = json.Unmarshal(data, &acc); err == nil {
			out, err = json.Marshal(accountJSONValue(&acc))
			keep = reflect.TypeFor[cfg_acc.AccountConfig]()
		}
	case "rules.json":
		var rules []cfg_acc.Rule
		if err = json.Unmarshal(data, &rules); err == nil {
			out, err = json.Marshal(nonNil(rules))
		}
	default:
		var list []string
		if err = json.Unmarshal(data, &list); err == nil {
			out, err = json.Marshal(nonNil(list))
		}
	}
	if err != nil {
		return nil, damagedFile(path, err)
	}
	if keep != nil {
		out = keepUnknownKeysFrom(data, out, keep)
	}
	return out, nil
}

// nonNil is list, or an empty list for nil: a file holds [], never null.
func nonNil[T any](list []T) []T {
	if list == nil {
		return []T{}
	}
	return list
}

// renderMainConfig is config.json: fc with each account cut down to its name
// and type, and the global lists, which moved to the accounts, left out.
func renderMainConfig(configPath string, fc *FileConfig) (sharedFile, error) {
	var clone FileConfig
	cloneData, err := json.Marshal(fc)
	if err != nil {
		return sharedFile{}, fmt.Errorf("failed to clone config: %w", err)
	}
	if err = json.Unmarshal(cloneData, &clone); err != nil {
		return sharedFile{}, fmt.Errorf("failed to restore clone config: %w", err)
	}
	if clone.Accounts != nil {
		for i := range *clone.Accounts {
			acc := &(*clone.Accounts)[i]
			*acc = cfg_acc.AccountConfig{Name: acc.Name, Type: acc.Type}
		}
	}
	clone.Whitelist = nil
	clone.Blacklist = nil
	clone.Rules = nil
	clone.Format = fileformat.Current
	data, err := json.Marshal(clone)
	if err != nil {
		return sharedFile{}, fmt.Errorf("failed to marshal config: %w", err)
	}
	return sharedFile{path: configPath, keep: reflect.TypeFor[FileConfig](), data: data}, nil
}

// SaveConfigFile writes each account's own files, then config.json with the
// per-account detail stripped out, creating the directory if it does not
// exist. A file another process changed since fc was loaded is merged rather
// than overwritten (see fileState). Every file's content is decided before
// any is written, so a refusal -- a damaged file, one a newer build wrote, a
// change to an account another run took away -- writes nothing. Under
// --dry-run it writes nothing and returns ErrConfigWriteForbidden.
func SaveConfigFile(configPath string, fc *FileConfig) error {
	if configWritesForbidden.Load() {
		return ErrConfigWriteForbidden
	}
	configDir := filepath.Dir(configPath)
	_ = os.MkdirAll(configDir, 0700)
	release, err := dirlock.GuardFile(configPath)
	defer release()
	if err != nil {
		return fmt.Errorf("locking %s: %w", configPath, err)
	}

	files, err := renderSharedFiles(configPath, fc)
	if err != nil {
		return err
	}
	s := &sharedSave{seen: fc.loaded, next: maps.Clone(fc.loaded)}
	if s.next == nil {
		s.next = loadState{}
	}
	main := files[len(files)-1]
	mainData, mainMerged, err := s.content(main)
	if err != nil {
		return err
	}
	var accounts []cfg_acc.AccountConfig
	if fc.Accounts != nil {
		accounts = *fc.Accounts
	}
	plan, err := s.planAccounts(accounts, files[:len(files)-1], liveAccounts(mainData, mainMerged))
	if err != nil {
		return err
	}
	if err := s.writePlan(configDir, plan); err != nil {
		return err
	}
	if err := s.put(main, mainData, mainMerged); err != nil {
		return fmt.Errorf("failed to write config file %s: %w", configPath, err)
	}
	fc.loaded = s.next
	return nil
}

// liveAccounts is the accounts config.json will name, lower-cased, when a
// merge decided them; nil means every account this run holds.
func liveAccounts(mainData []byte, merged bool) map[string]bool {
	if !merged {
		return nil
	}
	return accountNames(mainData)
}

// sharedSave is one SaveConfigFile.
type sharedSave struct {
	// seen is the FileConfig's state before this save; nil for one that was
	// not loaded, every file of which is written as it is held.
	seen loadState
	// next is the state after it.
	next loadState
}

// plannedWrite is one shared file a save will write, with what to write.
type plannedWrite struct {
	account string
	file    sharedFile
	data    []byte
	merged  bool
}

// content is what to write for f, and whether it was merged with a file
// changed on disk since this run saw it.
func (s *sharedSave) content(f sharedFile) ([]byte, bool, error) {
	current, err := os.ReadFile(f.path)
	if errors.Is(err, os.ErrNotExist) {
		current = nil
	} else if err != nil {
		return nil, false, fmt.Errorf("reading %s: %w", f.path, err)
	}
	st, known := s.seen[f.path]
	if !known || st.writtenAsHeld(current) {
		if f.keep == nil {
			return f.data, false, nil
		}
		return keepUnknownKeys(f.path, f.data, f.keep), false, nil
	}
	theirs := current
	if current != nil {
		if err = checkSharedFormat(f.path, current); err != nil {
			return nil, false, err
		}
		if theirs, err = normalizeShared(f.path, current); err != nil {
			return nil, false, err
		}
	}
	merged, err := mergeSharedFile(f.path, st.base, f.data, theirs)
	return merged, true, err
}

// planAccounts decides each account's four files -- files holds them in
// account order -- leaving out an account live does not name. Such an
// account was renamed or removed by another run; when this run changed it,
// the save is refused rather than reported as done.
func (s *sharedSave) planAccounts(accounts []cfg_acc.AccountConfig, files []sharedFile, live map[string]bool) ([]plannedWrite, error) {
	var plan []plannedWrite
	for i, acc := range accounts {
		accFiles := files[4*i : 4*i+4]
		if live != nil && !live[strings.ToLower(acc.Name)] {
			if s.changedSinceSeen(accFiles) {
				return nil, fmt.Errorf("account %q was renamed or removed by another feral after this one read the configuration; nothing was saved: run the command again", acc.Name)
			}
			slog.Warn("config: another run renamed or removed an account; its files were left alone",
				slog.String("account", acc.Name))
			continue
		}
		for _, f := range accFiles {
			data, merged, err := s.content(f)
			if err != nil {
				return nil, err
			}
			plan = append(plan, plannedWrite{account: acc.Name, file: f, data: data, merged: merged})
		}
	}
	return plan, nil
}

// changedSinceSeen reports whether this run's values for files differ from
// what it last saw of them.
func (s *sharedSave) changedSinceSeen(files []sharedFile) bool {
	for _, f := range files {
		st, ok := s.seen[f.path]
		if !ok || !bytes.Equal(st.base, f.data) {
			return true
		}
	}
	return false
}

// writePlan writes each planned file, then removes the per-account lists an
// older build kept beside config.json for each account written.
func (s *sharedSave) writePlan(configDir string, plan []plannedWrite) error {
	var written []string
	for _, w := range plan {
		_ = os.MkdirAll(filepath.Dir(w.file.path), 0700)
		if err := s.put(w.file, w.data, w.merged); err != nil {
			return err
		}
		if !slices.Contains(written, w.account) {
			written = append(written, w.account)
		}
	}
	for _, name := range written {
		legacy := SanitizeLabelForCache(name)
		for _, suffix := range []string{"whitelist", "blacklist", "rules"} {
			_ = os.Remove(filepath.Join(configDir, fmt.Sprintf("%s_%s.json", legacy, suffix)))
		}
	}
	return nil
}

// put writes data, f's content, and records what this run now knows of it.
func (s *sharedSave) put(f sharedFile, data []byte, merged bool) error {
	written, err := writeShared(f.path, data)
	if err != nil {
		return err
	}
	s.next[f.path] = fileState{read: true, disk: written, stale: merged, base: f.data}
	return nil
}

// checkSharedFormat refuses a shared file, changed on disk, that a newer
// build wrote: the loader's check, made again on what a merge would read.
func checkSharedFormat(path string, data []byte) error {
	if data == nil {
		return nil
	}
	if filepath.Base(path) != "config.json" && filepath.Base(path) != "account.json" {
		return fileformat.RefuseWrapped(path, data)
	}
	var probe struct {
		Format int `json:"format"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return damagedFile(path, err)
	}
	return fileformat.Check(path, probe.Format)
}

// saveAccountToDisk writes acc's own files as acc holds them.
func saveAccountToDisk(configDir string, acc *cfg_acc.AccountConfig) error {
	files, err := renderAccountFiles(configDir, acc)
	if err != nil {
		return err
	}
	s := &sharedSave{next: loadState{}}
	plan, err := s.planAccounts([]cfg_acc.AccountConfig{*acc}, files, nil)
	if err != nil {
		return err
	}
	return s.writePlan(configDir, plan)
}
