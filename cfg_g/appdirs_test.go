package cfg_g

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func mkdirs(t *testing.T, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if err := os.MkdirAll(p, 0o700); err != nil {
			t.Fatal(err)
		}
	}
}

// A home from before the rename: the directory moves once, and the old name
// stays as a link so a build from before the rename sees the same accounts.
func TestMigrateConfigDirMovesOnceAndLeavesALink(t *testing.T) {
	home := t.TempDir()
	legacy := filepath.Join(home, ".config", LegacyAppName)
	current := filepath.Join(home, ".config", AppName)
	mkdirs(t, legacy)
	if err := os.WriteFile(filepath.Join(legacy, "config.json"), []byte(`{"accounts":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	for range 2 { // the second run finds the move made
		got, err := MigrateConfigDir(home)
		if err != nil || got != current {
			t.Fatalf("MigrateConfigDir = %q, %v; want %q", got, err, current)
		}
	}
	if _, err := os.Stat(filepath.Join(current, "config.json")); err != nil {
		t.Errorf("config.json not moved: %v", err)
	}
	target, err := os.Readlink(legacy)
	if err != nil || target != AppName {
		t.Errorf("old name is not a link to the new directory: %q, %v", target, err)
	}
	if _, err := os.Stat(filepath.Join(legacy, "config.json")); err != nil {
		t.Errorf("an older build would not find config.json through the link: %v", err)
	}
}

func TestMigrateConfigDirFreshHomeAndBothPresent(t *testing.T) {
	home := t.TempDir()
	if got, err := MigrateConfigDir(home); err != nil || got != filepath.Join(home, ".config", AppName) {
		t.Errorf("fresh home: %q, %v", got, err)
	}
	if _, err := os.Lstat(filepath.Join(home, ".config", LegacyAppName)); !errors.Is(err, os.ErrNotExist) {
		t.Error("a fresh home was given a link to nothing")
	}
	mkdirs(t, filepath.Join(home, ".config", AppName), filepath.Join(home, ".config", LegacyAppName))
	_, err := MigrateConfigDir(home)
	var both *BothConfigDirsError
	if !errors.As(err, &both) {
		t.Errorf("two real directories = %v, want BothConfigDirsError", err)
	}
}

// A run that may not write reads the old directory where it is.
func TestConfigDirInReadsTheOldDirectoryInPlace(t *testing.T) {
	home := t.TempDir()
	legacy := filepath.Join(home, ".config", LegacyAppName)
	mkdirs(t, legacy)
	if got := ConfigDirIn(home); got != legacy {
		t.Errorf("ConfigDirIn = %q, want the old directory %q", got, legacy)
	}
	if _, err := os.Lstat(filepath.Join(home, ".config", AppName)); !errors.Is(err, os.ErrNotExist) {
		t.Error("ConfigDirIn created or moved something")
	}
	base := t.TempDir()
	mkdirs(t, filepath.Join(base, LegacyAppName))
	if got := CacheDirIn(base); got != filepath.Join(base, LegacyAppName) {
		t.Errorf("CacheDirIn = %q, want the existing old cache", got)
	}
}

// The loader migrates on a run that may write, and only then.
func TestLoadConfigMigratesOnlyWhenItMayWrite(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	legacy := filepath.Join(home, ".config", LegacyAppName)
	mkdirs(t, legacy)
	if err := os.WriteFile(filepath.Join(legacy, "config.json"),
		[]byte(`{"accounts":[{"name":"kept","type":"jmap","username":"u@example.com","password":"p"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfigNoWrite()
	if err != nil || cfg.ConfigDir != legacy || len(cfg.Accounts) != 1 {
		t.Fatalf("no-write load = dir %q, %d accounts, %v", cfg.ConfigDir, len(cfg.Accounts), err)
	}
	if info, _ := os.Lstat(legacy); info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("a no-write load moved the directory")
	}
	cfg, err = LoadConfig()
	if err != nil || cfg.ConfigDir != filepath.Join(home, ".config", AppName) || cfg.Accounts[0].Name != "kept" {
		t.Fatalf("writing load = dir %q, accounts %+v, %v", cfg.ConfigDir, cfg.Accounts, err)
	}
}
