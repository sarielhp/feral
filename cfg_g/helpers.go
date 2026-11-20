package cfg_g

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/fileformat"

	"github.com/sarielhp/feral/cfg_acc"
)

// ResolveConfigPath returns the config directory and the path to config.json
// within it. It errors when the config directory was never initialized.
func ResolveConfigPath(config *Config) (string, string, error) {
	if config.ConfigDir == "" {
		return "", "", fmt.Errorf("config directory not initialized")
	}
	configPath := filepath.Join(config.ConfigDir, "config.json")
	return config.ConfigDir, configPath, nil
}

// LoadConfigFile reads and parses config.json, then folds each account's
// separately stored whitelist, blacklist and rules back in from disk. A missing
// or malformed file is an error; a config listing no accounts yields an empty
// account list.
func LoadConfigFile(configPath string) (*FileConfig, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("error reading config file %s: %w", configPath, err)
	}
	var fc FileConfig
	if err := json.Unmarshal(data, &fc); err != nil {
		return nil, fmt.Errorf("error parsing config file %s: %w", configPath, err)
	}

	if fc.Accounts == nil {
		empty := []cfg_acc.AccountConfig{}
		fc.Accounts = &empty
	}

	if err := fileformat.Check(configPath, fc.Format); err != nil {
		return nil, err
	}
	configDir := filepath.Dir(configPath)
	reads := diskReads{configPath: data}
	for i := range *fc.Accounts {
		if err := loadAccountFromDisk(configDir, &(*fc.Accounts)[i], reads); err != nil {
			return nil, err
		}
	}
	markDefault(*fc.Accounts, fc.DefaultAccount)
	fc.loaded = newLoadState(configPath, &fc, reads)
	return &fc, nil
}

// loadAccountFromDisk folds an account's own files -- account.json and its
// white list, black list and rules -- into acc.
//
// A file that exists but cannot be read, does not parse, or is in a format a
// newer build wrote is an error. They used to be read as empty -- or, for
// account.json, silently replaced by what config.json holds -- and every
// config save writes each account's files back, so one damaged file and an
// unrelated command such as `config add-email` erased a black list or an
// account's settings for good.
func loadAccountFromDisk(configDir string, acc *cfg_acc.AccountConfig, reads diskReads) error {
	MigrateDirSanitization(filepath.Join(configDir, "accounts"), acc.Name)
	sanitizedName := SanitizeLabelForCache(acc.Name)
	accountsDir := AccountDir(configDir, acc.Name)
	if err := loadAccountJSON(filepath.Join(accountsDir, "account.json"), acc, reads); err != nil {
		return err
	}

	if len(acc.Whitelist) == 0 {
		list, err := loadListWithLegacyFallback(
			filepath.Join(accountsDir, "whitelist.json"),
			filepath.Join(configDir, fmt.Sprintf("%s_whitelist.json", sanitizedName)),
			reads,
		)
		if err != nil {
			return err
		}
		acc.Whitelist = list
	}
	if len(acc.Blacklist) == 0 {
		list, err := loadListWithLegacyFallback(
			filepath.Join(accountsDir, "blacklist.json"),
			filepath.Join(configDir, fmt.Sprintf("%s_blacklist.json", sanitizedName)),
			reads,
		)
		if err != nil {
			return err
		}
		acc.Blacklist = list
	}
	if len(acc.Rules) == 0 {
		rules, err := loadRulesWithLegacyFallback(
			filepath.Join(accountsDir, "rules.json"),
			filepath.Join(configDir, fmt.Sprintf("%s_rules.json", sanitizedName)),
			reads,
		)
		if err != nil {
			return err
		}
		if rules != nil {
			acc.Rules = rules
		} else if acc.Rules == nil {
			acc.Rules = []cfg_acc.Rule{}
		}
	}
	return nil
}

// loadAccountJSON lays account.json over acc, keeping the name config.json
// gives it and the lists, which have files of their own. A missing file is
// not an error: the account is then described by config.json alone.
func loadAccountJSON(path string, acc *cfg_acc.AccountConfig, reads diskReads) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		reads.record(path, nil)
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	reads.record(path, data)
	var diskAcc cfg_acc.AccountConfig
	if err := json.Unmarshal(data, &diskAcc); err != nil {
		return damagedFile(path, err)
	}
	if err := fileformat.Check(path, diskAcc.Format); err != nil {
		return err
	}
	name := acc.Name
	whitelist, blacklist, rules := acc.Whitelist, acc.Blacklist, acc.Rules
	*acc = diskAcc
	acc.Name = name
	acc.Whitelist = whitelist
	acc.Blacklist = blacklist
	acc.Rules = rules
	return nil
}

// damagedFile is the refusal for a configuration file that does not parse.
func damagedFile(path string, err error) error {
	return fmt.Errorf("%s is damaged (%v): fix it or move it aside; feral will not overwrite it", path, err)
}

// readListFile reads the newer of primaryPath and the legacy legacyPath, as
// bytes, refusing one a newer build wrapped. A missing file yields nil.
// reads records what primaryPath held, the file a save writes: nothing, when
// the legacy file was read instead.
func readListFile(primaryPath, legacyPath string, reads diskReads) (string, []byte, error) {
	target := primaryPath
	if _, err := os.Stat(primaryPath); os.IsNotExist(err) {
		target = legacyPath
		reads.record(primaryPath, nil)
	}
	data, err := os.ReadFile(target)
	if errors.Is(err, os.ErrNotExist) {
		return target, nil, nil
	}
	if err != nil {
		return target, nil, fmt.Errorf("reading %s: %w", target, err)
	}
	if target == primaryPath {
		reads.record(primaryPath, data)
	}
	return target, data, fileformat.RefuseWrapped(target, data)
}

func loadListWithLegacyFallback(primaryPath, legacyPath string, reads diskReads) ([]string, error) {
	path, data, err := readListFile(primaryPath, legacyPath, reads)
	if err != nil || data == nil {
		return []string{}, err
	}
	var list []string
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, damagedFile(path, err)
	}
	return list, nil
}

func loadRulesWithLegacyFallback(primaryPath, legacyPath string, reads diskReads) ([]cfg_acc.Rule, error) {
	path, data, err := readListFile(primaryPath, legacyPath, reads)
	if err != nil || data == nil {
		return nil, err
	}
	var rules []cfg_acc.Rule
	if err := json.Unmarshal(data, &rules); err != nil {
		return nil, damagedFile(path, err)
	}
	return rules, nil
}

// atomicWriteFile writes data to path via a temporary file in the same
// directory followed by a rename, so a crash or a full disk can never leave a
// truncated file behind.
//
// This matters most for config.json and accounts/*/account.json: os.WriteFile
// truncates before writing, and a kill between the two left invalid JSON that
// LoadConfigFile could not parse -- with no backup, the user could not even list
// their accounts to recover. The temp file must live in the destination
// directory, because os.Rename is only atomic within one filesystem.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("creating temp file for %s: %w", path, err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() // no-op once the rename succeeds

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("syncing %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp file for %s: %w", path, err)
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return fmt.Errorf("setting mode on %s: %w", path, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("renaming into place %s: %w", path, err)
	}
	return nil
}

// ErrConfigWriteForbidden is SaveConfigFile's refusal while writes are
// forbidden: a --dry-run run promises to change nothing, config.json included.
var ErrConfigWriteForbidden = errors.New("config.json not written: --dry-run changes nothing")

// configWritesForbidden is the process's --dry-run promise as it bears on
// config.json. It is a switch rather than an argument because some thirty
// commands save the configuration, and a guard each had to remember is how
// rule, account and config edits came to be written under --dry-run.
var configWritesForbidden atomic.Bool

// ForbidConfigWrites sets whether SaveConfigFile refuses. The CLI sets it on
// every run, from --dry-run.
func ForbidConfigWrites(forbid bool) { configWritesForbidden.Store(forbid) }

// Resolved is the config file, the account a command is acting on, and where
// that file lives.
//
// It replaced a five-value return whose third element, the config directory,
// was discarded at all 23 call sites.
type Resolved struct {
	File    *FileConfig
	Account *cfg_acc.AccountConfig

	// Dir is the directory holding Path. Tokens and logs live beside it.
	Dir string

	// Path is the config file itself, for saving back.
	Path string
}

// Resolve loads the config file and picks out the account to act on.
func Resolve(config *Config) (Resolved, error) {
	fc, acc, dir, path, err := resolveAccount(config)
	if err != nil {
		return Resolved{Dir: dir, Path: path}, err
	}
	return Resolved{File: fc, Account: acc, Dir: dir, Path: path}, nil
}

func resolveAccount(config *Config) (*FileConfig, *cfg_acc.AccountConfig, string, string, error) {
	configDir, configPath, err := ResolveConfigPath(config)
	if err != nil {
		return nil, nil, "", "", err
	}
	fc, err := LoadConfigFile(configPath)
	if err != nil {
		return nil, nil, "", "", err
	}
	if fc.Accounts == nil || len(*fc.Accounts) == 0 {
		return nil, nil, configDir, configPath, fmt.Errorf("no accounts configured")
	}
	// An identifier that names no account used to fall back to the first one,
	// silently. Nine packages reach an account through here and then write the
	// config back, so a typo in -A edited a different account's rules.
	targetAcc, err := Lookup(*fc.Accounts, string(config.SelectedAccount))
	if err != nil {
		return nil, nil, configDir, configPath, err
	}
	return fc, targetAcc, configDir, configPath, nil
}

// ValidateAccountParams rejects an account whose required fields are missing or
// still hold a config template's placeholder value, checking only the fields the
// account's type actually needs. Outlook is exempt from the password check
// because it authenticates with an OAuth token.
func ValidateAccountParams(acc cfg_acc.AccountConfig) error {
	if acc.Name == "" {
		return fmt.Errorf("account name is missing")
	}
	if acc.Type == "" {
		return fmt.Errorf("account type is missing")
	}
	// A local mbox file has no server, so no credentials and no folders to
	// configure: the file is all there is.
	if acc.IsMbox() {
		_, err := acc.MboxPath()
		return err
	}
	if acc.Username == "" || acc.Username == "your-email@gmail.com" || acc.Username == "your-email@example.com" {
		return fmt.Errorf("account username is missing or contains placeholder value")
	}
	if !strings.EqualFold(acc.Type, "outlook") {
		if acc.Password == "" || acc.Password == "your-app-password" || acc.Password == "your-jmap-api-token" {
			return fmt.Errorf("account password is missing or contains placeholder value")
		}
	}
	switch {
	case strings.EqualFold(acc.Type, "gmail"):
		if acc.IMAPHost == "" {
			return fmt.Errorf("imap_host is missing")
		}
	case strings.EqualFold(acc.Type, "jmap"):
		if acc.SessionURL == "" {
			return fmt.Errorf("jmap_session_url is missing")
		}
	case strings.EqualFold(acc.Type, "outlook"):
		// Outlook only needs Username (email) and OAuth token (stored on disk).
	default:
		return fmt.Errorf("unsupported account type %q", acc.Type)
	}
	if acc.SpamFolder == "" {
		return fmt.Errorf("spam_folder is missing")
	}
	if acc.ReceivedFolder == "" {
		return fmt.Errorf("received_folder is missing")
	}
	return nil
}

// NeedsOnboarding reports whether config holds no usable account -- either none
// at all, or only accounts still carrying the placeholder credentials a freshly
// created config.json is seeded with.
//
// A brand-new install is never literally account-less at run time: loadConfig
// seeds a "gmail" account in memory (never on disk) so the older
// single-account code paths still resolve one. That seeding is why the TUI's own "no accounts" check never fired and a
// first launch fell through to a connection attempt against
// "your-email@gmail.com". This is the check that tells a first run apart from a
// configured one, and it mirrors ValidateAccountParams's placeholder rules.
func NeedsOnboarding(config *Config) bool {
	if config == nil {
		return true
	}
	return OnlyPlaceholders(config.Accounts)
}

// OnlyPlaceholders reports whether accounts holds nothing but unconfigured
// placeholders -- such as the "gmail" account loadConfig seeds in memory --
// or is empty. What a command that adds an account replaces is narrower:
// DropPlaceholders.
func OnlyPlaceholders(accounts []cfg_acc.AccountConfig) bool {
	for _, acc := range accounts {
		if !isPlaceholderAccount(acc) {
			return false
		}
	}
	return true
}

// isSeededPlaceholder is the account an older first run wrote: a gmail
// account with no username, or the template's. No real account looks like
// it -- ValidateAccountParams refuses both usernames.
func isSeededPlaceholder(acc cfg_acc.AccountConfig) bool {
	return strings.EqualFold(acc.Type, "gmail") && isPlaceholderUsername(acc.Username)
}

func isPlaceholderUsername(username string) bool {
	return username == "" || username == "your-email@gmail.com" || username == "your-email@example.com"
}

func isPlaceholderAccount(acc cfg_acc.AccountConfig) bool {
	if acc.IsMbox() {
		return strings.TrimSpace(acc.Path) == ""
	}
	if isPlaceholderUsername(acc.Username) {
		return true
	}
	// Outlook authenticates through OAuth, so its password is intentionally
	// blank; a real username is enough to call it configured.
	if strings.EqualFold(acc.Type, "outlook") {
		return false
	}
	return acc.Password == "" || acc.Password == "your-app-password" || acc.Password == "your-jmap-api-token"
}

// DropPlaceholders empties fc's account list when it holds nothing but the
// account older first runs seeded (isSeededPlaceholder), and clears the
// default_account and default_send_account that could only have named it. A
// command that adds an account calls it first, before checking the new name:
// appending kept the placeholder first in the list, so later runs selected it,
// and its name counted as taken. It is narrower than OnlyPlaceholders on
// purpose: `account add jmap` offers the placeholder token as its default, and
// an account saved that way is the user's, not one to drop.
func DropPlaceholders(fc *FileConfig) {
	if fc.Accounts == nil || len(*fc.Accounts) == 0 {
		return
	}
	for _, acc := range *fc.Accounts {
		if !isSeededPlaceholder(acc) {
			return
		}
	}
	*fc.Accounts = (*fc.Accounts)[:0]
	fc.DefaultSendAccount = ""
	fc.DefaultAccount = ""
}

// SanitizeLabelForCache maps a label or account name to a filesystem-safe
// name segment; see cachedir.SanitizeName, which it is.
func SanitizeLabelForCache(label string) string { return cachedir.SanitizeName(label) }

// AutoBlacklistInternal adds an internal blacklist rule for the sender if not already present.
func AutoBlacklistInternal(config *Config, senderEmail string) error {
	fc, targetAcc, _, configPath, err := resolveAccount(config)
	if err != nil {
		return err
	}

	// 1. Check if the sender is already whitelisted
	if IsWhitelisted(senderEmail, targetAcc.Whitelist) {
		return nil
	}

	// 2. Determine target label for Spam Learn
	targetLabel := targetAcc.SpamLearn
	if targetLabel == "" {
		targetLabel = "LearnSpam"
	}

	// 3. Check if a rule for this sender already exists
	for _, r := range targetAcc.Rules {
		if strings.EqualFold(r.Sender, senderEmail) {
			return nil
		}
	}

	// 4. Create internal rule
	targetAcc.Rules = append(targetAcc.Rules, cfg_acc.Rule{
		Sender:   senderEmail,
		Label:    targetLabel,
		Internal: true,
	})

	fmt.Printf("[+] Auto-blacklisted sender '%s' locally (internal rule pointing to %s).\n",
		senderEmail, targetLabel)

	// 5. Save back to config
	return SaveConfigFile(configPath, fc)
}

// ExpandSavePath expands ~, %YYYY, %YY, %MM, %DD, %hh, %mm, %ss in the given directory path pattern.
func ExpandSavePath(pattern string, t time.Time) (string, error) {
	if strings.TrimSpace(pattern) == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get user home directory: %w", err)
		}
		return filepath.Join(homeDir, "Downloads"), nil
	}

	res := strings.TrimSpace(pattern)
	if strings.HasPrefix(res, "~/") || res == "~" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get user home directory: %w", err)
		}
		if res == "~" {
			res = homeDir
		} else {
			res = filepath.Join(homeDir, res[2:])
		}
	}

	if t.IsZero() {
		t = time.Now()
	}

	yyyy := fmt.Sprintf("%04d", t.Year())
	yy := fmt.Sprintf("%02d", t.Year()%100)
	mm := fmt.Sprintf("%02d", int(t.Month()))
	dd := fmt.Sprintf("%02d", t.Day())
	hh := fmt.Sprintf("%02d", t.Hour())
	minStr := fmt.Sprintf("%02d", t.Minute())
	ss := fmt.Sprintf("%02d", t.Second())

	res = strings.ReplaceAll(res, "%YYYY", yyyy)
	res = strings.ReplaceAll(res, "%YY", yy)
	res = strings.ReplaceAll(res, "%Y", yyyy)
	res = strings.ReplaceAll(res, "%MM", mm)
	res = strings.ReplaceAll(res, "%M", mm)
	res = strings.ReplaceAll(res, "%DD", dd)
	res = strings.ReplaceAll(res, "%D", dd)
	res = strings.ReplaceAll(res, "%hh", hh)
	res = strings.ReplaceAll(res, "%H", hh)
	res = strings.ReplaceAll(res, "%mm", minStr)
	res = strings.ReplaceAll(res, "%ss", ss)

	return filepath.Clean(res), nil
}

// AccountLabelSpec holds a parsed label specification with an optional account indicator.
type AccountLabelSpec struct {
	AccountIndex int    // 1-based account index (1, 2, ...), or 0 if omitted.
	AccountName  string // Account name, set when prefix is not a number (e.g. "GMail:folder").
	Label        string // Clean label path without "account:" prefix.
}

// ParseAccountLabelSpec parses a string like "GMail:inbox" or "inbox".
// If raw starts with an account name followed by a colon, the two are separated.
// Only prefixes starting with a letter are treated as account names.
// Numeric prefixes like "1:label" are rejected since account indices are fragile.
// Returns an error if a numeric prefix is used.
func ParseAccountLabelSpec(raw string) (AccountLabelSpec, error) {
	if idx := strings.Index(raw, ":"); idx > 0 {
		prefix := raw[:idx]
		if num, err := strconv.Atoi(prefix); err == nil && num > 0 {
			return AccountLabelSpec{}, fmt.Errorf("numeric account index (%d:) is no longer supported — use account name instead (e.g. %q)", num, raw[idx+1:])
		}
		// Non-numeric prefix — treat as account name if it starts with a letter
		if len(prefix) > 0 && ((prefix[0] >= 'a' && prefix[0] <= 'z') || (prefix[0] >= 'A' && prefix[0] <= 'Z')) {
			return AccountLabelSpec{
				AccountName: prefix,
				Label:       raw[idx+1:],
			}, nil
		}
	}
	return AccountLabelSpec{
		Label: raw,
	}, nil
}

// DefaultSaveLocations are the save dialog's fixed jumps when the config
// names none: Downloads and home.
var DefaultSaveLocations = []string{"~/Downloads", "~"}

// SaveLocationsOrDefault is the configured save_locations, or the default
// pair when the key is absent. An explicitly empty list stays empty.
func (c *Config) SaveLocationsOrDefault() []string {
	if c.SaveLocations == nil {
		return DefaultSaveLocations
	}
	return c.SaveLocations
}

// ParseSaveLocations splits a comma-separated list of directories, as
// `config set save_locations` takes it, dropping blanks.
func ParseSaveLocations(value string) []string {
	locs := []string{}
	for _, p := range strings.Split(value, ",") {
		if p = strings.TrimSpace(p); p != "" {
			locs = append(locs, p)
		}
	}
	return locs
}
