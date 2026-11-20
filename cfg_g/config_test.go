package cfg_g

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadOrInitFileConfig_FirstRunSucceeds is H3's regression. The
// !configExists branch used to set Accounts to a pointer to config.Accounts,
// which was still nil at that point, so the file was written with
// "accounts": null; the write-then-reload verification then saw an empty
// account list, synthesized the "default" account itself, and reported the
// config still dirty -- so a brand-new user's first run always exited 1,
// even though the file it wrote was fine to use.
func TestLoadOrInitFileConfig_FirstRunSucceeds(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	config := newDefaultConfig(tempDir)

	fc, err := loadOrInitFileConfig(configPath, config, false)
	if err != nil {
		t.Fatalf("first-run config creation failed: %v", err)
	}
	// No credentials, nothing to migrate: the accounts list is written empty,
	// so the first account `account add` makes is the only one.
	if fc.Accounts == nil || len(*fc.Accounts) != 0 {
		t.Fatalf("expected an empty accounts list, got %+v", fc.Accounts)
	}
	if _, statErr := os.Stat(configPath); statErr != nil {
		t.Fatalf("expected config file to be written, stat failed: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(tempDir, "accounts", "gmail")); !os.IsNotExist(statErr) {
		t.Errorf("a first run left a placeholder account directory: %v", statErr)
	}
}

// A config from before accounts[] -- gmail credentials at the top level --
// still becomes an account named "gmail" on the first load, with or without a
// password in the file (GMAIL_PASS may supply it).
func TestLoadOrInitFileConfig_MigratesTopLevelCredentials(t *testing.T) {
	for name, file := range map[string]string{
		"with a password": `{"username": "me@gmail.example", "password": "app-password"}`,
		"username only":   `{"username": "me@gmail.example"}`,
	} {
		t.Run(name, func(t *testing.T) { checkTopLevelMigration(t, file) })
	}
}

func checkTopLevelMigration(t *testing.T, file string) {
	t.Helper()
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	if err := os.WriteFile(configPath, []byte(file), 0o600); err != nil {
		t.Fatal(err)
	}
	fc, err := loadOrInitFileConfig(configPath, newDefaultConfig(tempDir), false)
	if err != nil {
		t.Fatalf("loading a single-account config failed: %v", err)
	}
	if fc.Accounts == nil || len(*fc.Accounts) != 1 {
		t.Fatalf("expected one migrated account, got %+v", fc.Accounts)
	}
	// Never "default": that name is reserved (ValidateAccountName).
	acc := (*fc.Accounts)[0]
	if acc.Name != "gmail" || ValidateAccountName(acc.Name) != nil || acc.Username != "me@gmail.example" {
		t.Errorf("migrated account = %q (%q), want gmail with the top-level username", acc.Name, acc.Username)
	}
}

// TestLoadOrInitFileConfig_NoWriteSkipsFirstRunWrite is L3's regression for a
// brand-new config: a --read-only/--dry-run invocation must return a usable
// in-memory config without ever creating config.json.
func TestLoadOrInitFileConfig_NoWriteSkipsFirstRunWrite(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	config := newDefaultConfig(tempDir)

	fc, err := loadOrInitFileConfig(configPath, config, true)
	if err != nil {
		t.Fatalf("noWrite first-run load failed: %v", err)
	}
	if fc == nil || fc.Accounts == nil {
		t.Fatalf("expected a usable in-memory config, got %+v", fc)
	}
	if _, statErr := os.Stat(configPath); !os.IsNotExist(statErr) {
		t.Errorf("expected config file NOT to be written under noWrite, stat err = %v", statErr)
	}
}

// TestLoadOrInitFileConfig_NoWriteSkipsAutoFixOfExistingConfig is L3's
// regression for an existing config missing fields: loading it under
// --read-only/--dry-run must not rewrite config.json to fill them in.
func TestLoadOrInitFileConfig_NoWriteSkipsAutoFixOfExistingConfig(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"username":"test@example.com"}`), 0600); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	before, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	config := newDefaultConfig(tempDir)
	if _, loadErr := loadOrInitFileConfig(configPath, config, true); loadErr != nil {
		t.Fatalf("noWrite load of existing config failed: %v", loadErr)
	}

	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("expected config file to be untouched under --read-only/--dry-run, was %q now %q", before, after)
	}
}
