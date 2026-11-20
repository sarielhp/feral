package cfg_g

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/fileformat"
)

// A link is another name for feral: a symlink to the binary that runs it with
// arguments put in front of the ones typed. `mt` runs `feral -A fastmail tui`.
// clihelp does the running (App.Invocations); this file is where the list is
// kept -- config.json's "links" -- and how it is read cheaply enough for Tab.
//
// The account is a field of its own rather than words in the arguments, so
// `account rename`, `account rm` and `link list` read a field instead of
// parsing a preset, which needs the flag arity only the command tree has.

// Link is one entry of config.json's "links", keyed by the name.
type Link struct {
	// Account is the account's canonical name; the preset opens with -A and
	// it. Empty means none: the link runs against whatever is chosen then.
	Account string `json:"account,omitempty"`
	// Args are the preset's other words, stored as words: nothing is ever
	// split by shell rules or expanded, so "~" and "$HOME" stay as written.
	Args []string `json:"args,omitempty"`
	// Description is for `link list`.
	Description string `json:"description,omitempty"`
}

// Preset is the argument list the link puts in front of the typed words.
func (l Link) Preset() []string {
	var out []string
	if l.Account != "" {
		out = append(out, "-A", l.Account)
	}
	return append(out, l.Args...)
}

// MigratedLinkDescription marks a link made from an account's aliases[].
const MigratedLinkDescription = "from aliases[]"

// ReadLinks reads config.json's links from configDir without loading the
// configuration: it is called on every Tab, and loading would read every
// account's files and could write. present reports whether the key is there
// at all; a config.json without it has not been migrated (see
// MigrateAliasesToLinks). A missing config.json is no links and no error.
// A damaged file, a newer format, or a damaged links value is an error,
// never an empty list: an empty list would run `gmail delete` unexpanded,
// against the default account.
func ReadLinks(configDir string) (links map[string]Link, present bool, err error) {
	path := filepath.Join(configDir, "config.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var top struct {
		Format int             `json:"format,omitempty"`
		Links  json.RawMessage `json:"links"`
	}
	if err := json.Unmarshal(data, &top); err != nil {
		return nil, false, damagedFile(path, err)
	}
	if err := fileformat.Check(path, top.Format); err != nil {
		return nil, false, err
	}
	if len(top.Links) == 0 || string(top.Links) == "null" {
		return nil, false, nil
	}
	if err := json.Unmarshal(top.Links, &links); err != nil {
		return nil, true, damagedFile(path, fmt.Errorf("links: %w", err))
	}
	return links, true, nil
}

// LinkNames are the declared names, sorted.
func LinkNames(links map[string]Link) []string {
	names := make([]string, 0, len(links))
	for name := range links {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// ValidateLinkName reports why name cannot be a link: it must be a command
// name clihelp accepts (letters, digits, '-', '_', '.', '+'), and it may not
// be feral itself.
func ValidateLinkName(name string) error {
	switch {
	case name == "":
		return errors.New("a link name cannot be empty")
	case name == AppName:
		return fmt.Errorf("%q is the program's own name, not a link", name)
	case name == "." || name == ".." || strings.HasPrefix(name, "-"):
		return fmt.Errorf("link name %q cannot be a command name", name)
	}
	for _, r := range name {
		if !isLinkNameRune(r) {
			return fmt.Errorf("link name %q may hold only letters, digits, '-', '_', '.' and '+'", name)
		}
	}
	return nil
}

func isLinkNameRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	}
	return r == '-' || r == '_' || r == '.' || r == '+'
}

// LinksNaming lists, sorted, the links whose account is account (ignoring
// case, as account names are matched).
func LinksNaming(links map[string]Link, account string) []string {
	var names []string
	for name, l := range links {
		if l.Account != "" && strings.EqualFold(l.Account, account) {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}

// ProgramNamesBesideBinary lists the names in the running binary's directory
// that are this program -- symlinks to it, or it. The aliases migration uses
// them for the spelling of a link: an alias was matched ignoring case, and a
// link is matched exactly. It is a variable so that tests need no binary.
var ProgramNamesBesideBinary = programNamesBesideBinary

func programNamesBesideBinary() []string {
	exe, err := os.Executable()
	if err != nil {
		return nil
	}
	if resolved, evalErr := filepath.EvalSymlinks(exe); evalErr == nil {
		exe = resolved
	}
	self, err := os.Stat(exe)
	if err != nil {
		return nil
	}
	entries, err := os.ReadDir(filepath.Dir(exe))
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		info, statErr := os.Stat(filepath.Join(filepath.Dir(exe), e.Name()))
		if statErr == nil && os.SameFile(self, info) {
			names = append(names, e.Name())
		}
	}
	return names
}

// MigrateAliasesToLinks gives a config.json that has no "links" key one: an
// entry for each account alias that can be a link, running the account. The
// key's presence is the record that the migration ran, so with no aliases it
// is an empty object. It reports whether it changed fc.
//
// An alias used to select its account when feral was started under that
// name; after the migration the link does that, and the alias is only a name
// -A accepts. feral and mail_cli are never made links: one is the program and
// the other runs as it.
func MigrateAliasesToLinks(fc *FileConfig) bool {
	if fc.Links != nil {
		return false
	}
	links := map[string]Link{}
	if fc.Accounts != nil {
		var beside []string
		for _, acc := range *fc.Accounts {
			for _, alias := range acc.Aliases {
				if !migratableAlias(alias) {
					continue
				}
				if beside == nil {
					beside = append(ProgramNamesBesideBinary(), "")
				}
				name := spellingOf(alias, beside)
				if _, taken := links[name]; !taken {
					links[name] = Link{Account: acc.Name, Description: MigratedLinkDescription}
				}
			}
		}
	}
	fc.Links = &links
	return true
}

func migratableAlias(alias string) bool {
	return ValidateLinkName(alias) == nil && !strings.EqualFold(alias, AppName) && !strings.EqualFold(alias, LegacyAppName)
}

// spellingOf is the name among names that equals alias ignoring case, or
// alias as written.
func spellingOf(alias string, names []string) string {
	for _, n := range names {
		if n != "" && strings.EqualFold(n, alias) {
			return n
		}
	}
	return alias
}

// NamesAccount reports whether name equals, ignoring case, an account's
// name, display name or one of its aliases.
func NamesAccount(accounts []cfg_acc.AccountConfig, name string) bool {
	for _, acc := range accounts {
		if strings.EqualFold(acc.Name, name) || (acc.DisplayName != "" && strings.EqualFold(acc.DisplayName, name)) ||
			matchesAlias(acc, name) {
			return true
		}
	}
	return false
}
