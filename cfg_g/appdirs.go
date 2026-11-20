package cfg_g

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// LegacyAppName is the program's name before it was feral. Its configuration
// and cache directories are still found, and the configuration directory is
// moved to feral's the first time a run may write.
const LegacyAppName = "mail_cli"

// The configuration directory holds the accounts, their OAuth tokens and
// their rules, so it is found in one place. Each caller used to build
// ~/.config/<name> for itself, and a rename that missed one would have read an
// empty directory -- a first run, as far as that caller could tell -- and
// written a placeholder config into it.

// ConfigDirIn is the configuration directory beneath home: feral's, or
// mail_cli's while only that one exists -- a host not yet migrated, or a run
// that may not write and so may not migrate it.
func ConfigDirIn(home string) string {
	return existingOr(filepath.Join(home, ".config", AppName), filepath.Join(home, ".config", LegacyAppName))
}

// UserConfigDir is ConfigDirIn for the user's home.
func UserConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("detect home directory: %w", err)
	}
	return ConfigDirIn(home), nil
}

// CacheDirIn is the default cache directory beneath base (os.UserCacheDir):
// feral's, or mail_cli's while only that one exists. A cache is never moved:
// download_dir in config.json names it, and drafts and the outbox live there.
func CacheDirIn(base string) string {
	return existingOr(filepath.Join(base, AppName), filepath.Join(base, LegacyAppName))
}

// UserCacheDir is CacheDirIn for the user's cache base, ~/.cache by default.
func UserCacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return "", fmt.Errorf("detect cache directory: %w", err)
		}
		base = filepath.Join(home, ".cache")
	}
	return CacheDirIn(base), nil
}

func existingOr(current, legacy string) string {
	if _, err := os.Stat(current); err == nil {
		return current
	}
	if _, err := os.Stat(legacy); err == nil {
		return legacy
	}
	return current
}

// BothConfigDirsError reports a home holding feral's configuration directory
// and mail_cli's as two real directories. Neither is chosen: one of them holds
// the accounts and the other is stale, and only the user knows which.
type BothConfigDirsError struct{ Current, Legacy string }

func (e *BothConfigDirsError) Error() string {
	return fmt.Sprintf("both %s and %s exist; keep the one holding your accounts, move the other aside, and run again", e.Current, e.Legacy)
}

// MigrateConfigDir moves mail_cli's configuration directory to feral's and
// leaves mail_cli's name behind as a symlink to it, so a build from before the
// rename -- still installed on another host, or in a script -- finds the same
// accounts. It returns the directory to use. It moves nothing when feral's
// already exists, or when mail_cli's is absent or is itself a link.
func MigrateConfigDir(home string) (string, error) {
	current := filepath.Join(home, ".config", AppName)
	legacy := filepath.Join(home, ".config", LegacyAppName)
	legacyInfo, legacyErr := os.Lstat(legacy)
	legacyIsDir := legacyErr == nil && legacyInfo.IsDir()
	_, currentErr := os.Lstat(current)
	switch {
	case currentErr == nil && legacyIsDir:
		return "", &BothConfigDirsError{Current: current, Legacy: legacy}
	case currentErr == nil || !legacyIsDir:
		return ConfigDirIn(home), nil
	case !errors.Is(currentErr, os.ErrNotExist):
		return "", currentErr
	}
	if err := os.Rename(legacy, current); err != nil {
		// Another run may have moved it a moment ago.
		if _, statErr := os.Stat(current); statErr == nil {
			return current, nil
		}
		return "", fmt.Errorf("move %s to %s: %w", legacy, current, err)
	}
	if err := os.Symlink(AppName, legacy); err != nil {
		slog.Warn("configuration moved, but the link for older builds was not made",
			slog.String("link", legacy), slog.Any("error", err))
	}
	return current, nil
}
