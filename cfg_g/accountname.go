package cfg_g

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/sarielhp/feral/cfg_acc"
)

// An account's name is its identity on disk as well as on the command line:
// it names the account's directory under accounts/, its cache directory, its
// OAuth token file and its watcher state. So a new name is held to rules an
// old one never was.
//
//   - Letters, digits, '-' and '_', starting with a letter. Those are the
//     characters the file-name sanitizers keep as they are, so two names
//     never share a directory, and `%name:label` always parses.
//   - Not a reserved word. "default" is what the program used to call an
//     account it made up, and what an account cache with no name falls back
//     to (cachedir.ForAccount), so an account named "default" shares that
//     directory; it also reads as "whichever account is the default", which
//     `account default` now names. "all" is how -A asks for every account.
//
// An existing account that breaks the rules still loads -- refusing it would
// lock its owner out of the command that renames it -- and `account list` and
// `config check` say what to do about it.

// ReservedAccountNames are the names no account may take.
var ReservedAccountNames = []string{"default", "all"}

var accountNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)

// maxAccountName bounds a name, which becomes a file name several times over.
const maxAccountName = 64

// ValidateAccountName reports why name cannot be an account's name, or nil.
func ValidateAccountName(name string) error {
	switch {
	case name == "":
		return fmt.Errorf("an account name cannot be empty")
	case IsReservedAccountName(name):
		return fmt.Errorf("%q is reserved and cannot name an account; choose a name for the account itself, such as its provider or what it is for", name)
	case len(name) > maxAccountName:
		return fmt.Errorf("account name %q is longer than %d characters", name, maxAccountName)
	case !accountNamePattern.MatchString(name):
		return fmt.Errorf("account name %q may hold only letters, digits, '-' and '_', and must start with a letter", name)
	}
	return nil
}

// IsReservedAccountName reports whether name is one of ReservedAccountNames.
func IsReservedAccountName(name string) bool {
	for _, r := range ReservedAccountNames {
		if strings.EqualFold(name, r) {
			return true
		}
	}
	return false
}

// CheckNewAccountName validates name and checks that it identifies no other
// account: not its name, display name or alias, since Lookup accepts all
// three. self is the index of the account being renamed, or -1 for a new one.
func CheckNewAccountName(accounts []cfg_acc.AccountConfig, name string, self int) error {
	if err := ValidateAccountName(name); err != nil {
		return err
	}
	for i, acc := range accounts {
		if i == self {
			continue
		}
		if strings.EqualFold(acc.Name, name) || strings.EqualFold(acc.DisplayName, name) || matchesAlias(acc, name) {
			return fmt.Errorf("account %q already exists (it names account %q)", name, acc.Name)
		}
	}
	return nil
}

// AccountDir is the directory holding an account's own files -- account.json,
// its white and black lists, its rules and its signature -- under the
// configuration directory.
func AccountDir(configDir, name string) string {
	return filepath.Join(configDir, "accounts", SanitizeLabelForCache(name))
}

// AccountFiles are the paths under configDir named after an account: its
// directory, and the list and rules files that predate it. Renaming an account
// moves each of them.
func AccountFiles(configDir, name string) []string {
	safe := SanitizeLabelForCache(name)
	return []string{
		AccountDir(configDir, name),
		filepath.Join(configDir, safe+"_whitelist.json"),
		filepath.Join(configDir, safe+"_blacklist.json"),
		filepath.Join(configDir, safe+"_rules.json"),
	}
}

// markDefault sets IsDefault on the account name names, and clears it on the
// rest. A name that matches no account marks none, and DefaultOf falls back to
// the first: `account list` and `config check` report the dangling name.
func markDefault(accounts []cfg_acc.AccountConfig, name string) {
	for i := range accounts {
		accounts[i].IsDefault = name != "" && strings.EqualFold(accounts[i].Name, name)
	}
}

// DefaultOf is the account a command that names none acts on: the one marked
// default, or else the first. Nil when there are no accounts.
func DefaultOf(accounts []cfg_acc.AccountConfig) *cfg_acc.AccountConfig {
	if len(accounts) == 0 {
		return nil
	}
	for i := range accounts {
		if accounts[i].IsDefault {
			return &accounts[i]
		}
	}
	return &accounts[0]
}

// DanglingDefault reports a default_account that names no account.
func DanglingDefault(accounts []cfg_acc.AccountConfig, name string) bool {
	if name == "" {
		return false
	}
	for _, acc := range accounts {
		if strings.EqualFold(acc.Name, name) {
			return false
		}
	}
	return true
}
