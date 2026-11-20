package cfg_g

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/feral/cfg_acc"
)

func TestValidateAccountName(t *testing.T) {
	tests := []struct {
		name string
		ok   bool
	}{
		{"work", true},
		{"my-gmail_2", true},
		{"Fastmail", true},
		{"default", false},
		{"Default", false},
		{"ALL", false},
		{"", false},
		{"2work", false},
		{"-work", false},
		{"my gmail", false},
		{"work:home", false},
		{"bücher", false},
		{strings.Repeat("a", maxAccountName+1), false},
	}
	for _, tt := range tests {
		err := ValidateAccountName(tt.name)
		if (err == nil) != tt.ok {
			t.Errorf("ValidateAccountName(%q) = %v, want ok=%v", tt.name, err, tt.ok)
		}
	}
}

func TestCheckNewAccountNameCollisions(t *testing.T) {
	accounts := []cfg_acc.AccountConfig{
		{Name: "personal", DisplayName: "GMail"},
		{Name: "work", Aliases: []string{"office"}},
	}
	for _, taken := range []string{"Personal", "gmail", "WORK", "office"} {
		if err := CheckNewAccountName(accounts, taken, -1); err == nil {
			t.Errorf("CheckNewAccountName(%q) accepted a name another account answers to", taken)
		}
	}
	if err := CheckNewAccountName(accounts, "gmail", 0); err != nil {
		t.Errorf("an account may take its own display name as its name: %v", err)
	}
	if err := CheckNewAccountName(accounts, "fastmail", -1); err != nil {
		t.Errorf("a free name was refused: %v", err)
	}
}

// The default account is what an empty identifier and an empty selection
// mean; it used to be hard-wired to the first account.
func TestDefaultAccountSelection(t *testing.T) {
	accounts := []cfg_acc.AccountConfig{{Name: "first"}, {Name: "second"}, {Name: "third"}}
	if got := DefaultOf(accounts); got.Name != "first" {
		t.Fatalf("with none marked, the default is the first account, got %q", got.Name)
	}
	markDefault(accounts, "Third")
	acc, err := Lookup(accounts, "")
	if err != nil || acc.Name != "third" {
		t.Fatalf("Lookup(\"\") = %v, %v; want third", acc, err)
	}
	cfg := &Config{Accounts: accounts}
	if !cfg.IsSelected(accounts[2]) || cfg.IsSelected(accounts[0]) {
		t.Error("with nothing selected, IsSelected must pick the default account and only it")
	}
	cfg.SelectedAccount = "second"
	if !cfg.IsSelected(accounts[1]) || cfg.IsSelected(accounts[2]) {
		t.Error("an explicit selection must win over the default")
	}
	markDefault(accounts, "gone")
	if got := DefaultOf(accounts); got.Name != "first" {
		t.Errorf("a dangling default_account falls back to the first account, got %q", got.Name)
	}
	if !DanglingDefault(accounts, "gone") || DanglingDefault(accounts, "second") || DanglingDefault(accounts, "") {
		t.Error("DanglingDefault misreports")
	}
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadConfigFileMarksDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	writeJSON(t, path, map[string]any{
		"accounts":        []map[string]string{{"name": "a", "type": "jmap"}, {"name": "b", "type": "jmap"}},
		"default_account": "b",
	})
	fc, err := LoadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if (*fc.Accounts)[0].IsDefault || !(*fc.Accounts)[1].IsDefault {
		t.Errorf("default_account %q did not mark its account: %+v", fc.DefaultAccount, *fc.Accounts)
	}
	if err = SaveConfigFile(path, fc); err != nil {
		t.Fatal(err)
	}
	again, err := LoadConfigFile(path)
	if err != nil || again.DefaultAccount != "b" {
		t.Errorf("default_account did not survive a save: %q, %v", again.DefaultAccount, err)
	}
}

func TestCommitAccountRename(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	writeJSON(t, path, map[string]any{
		"accounts":             []map[string]string{{"name": "default", "type": "gmail"}, {"name": "work", "type": "outlook"}},
		"default_account":      "default",
		"default_send_account": "Default",
		"editor":               "vi",
	})
	for range 2 { // the second run must find nothing to do
		if err := CommitAccountRename(path, "default", "personal"); err != nil {
			t.Fatal(err)
		}
	}
	var top map[string]any
	data, _ := os.ReadFile(path)
	if err := json.Unmarshal(data, &top); err != nil {
		t.Fatal(err)
	}
	accounts := top["accounts"].([]any)
	if got := accounts[0].(map[string]any)["name"]; got != "personal" {
		t.Errorf("account not renamed: %v", got)
	}
	if got := accounts[1].(map[string]any)["name"]; got != "work" {
		t.Errorf("another account was renamed: %v", got)
	}
	if top["default_account"] != "personal" || top["default_send_account"] != "personal" {
		t.Errorf("references not renamed: %v / %v", top["default_account"], top["default_send_account"])
	}
	if top["editor"] != "vi" {
		t.Error("an unrelated key was lost")
	}
}

// While a rename is unfinished, every load but the rename's own refuses.
func TestLoadRefusesDuringRename(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configDir := filepath.Join(home, ".config", AppName)
	writeJSON(t, filepath.Join(configDir, "config.json"), map[string]any{
		"accounts": []map[string]string{{"name": "old", "type": "jmap"}},
	})
	if err := BeginRename(configDir, "old", "new"); err != nil {
		t.Fatal(err)
	}
	_, err := LoadConfigNoWrite()
	var inProgress *RenameInProgressError
	if !errors.As(err, &inProgress) || inProgress.From != "old" || inProgress.To != "new" {
		t.Fatalf("load during a rename = %v, want RenameInProgressError", err)
	}
	if !strings.Contains(err.Error(), "account rename old new") {
		t.Errorf("the refusal must say how to finish: %v", err)
	}
	if _, err := LoadConfigForRename(); err != nil {
		t.Fatalf("the rename's own load refused: %v", err)
	}
	if err := EndRename(configDir); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfigNoWrite(); err != nil {
		t.Fatalf("load after the rename: %v", err)
	}
}
