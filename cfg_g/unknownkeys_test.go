package cfg_g

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/sarielhp/feral/cfg_acc"
)

func readJSONObject(t *testing.T, path string) map[string]json.RawMessage {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("%s is not a JSON object after the save: %v\n%s", path, err, data)
	}
	return fields
}

// A save keeps what this build does not know -- a key a newer build wrote, or
// one added by hand -- in config.json and in account.json alike, and keeps
// nothing it does know: a cleared default_account stays cleared.
func TestSaveConfigFileKeepsUnknownKeys(t *testing.T) {
	ForbidConfigWrites(false)
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	accountPath := filepath.Join(AccountDir(dir, "work"), "account.json")
	if err := os.MkdirAll(filepath.Dir(accountPath), 0o700); err != nil {
		t.Fatal(err)
	}
	mainFile := `{"username": "u", "default_account": "work", "Download_Dir": "/x",
		"shortcuts": {"mt": {"account": "work", "args": ["tui"]}}, "zz_future": [1, 2]}`
	if err := os.WriteFile(configPath, []byte(mainFile), 0o600); err != nil {
		t.Fatal(err)
	}
	accountFile := `{"name": "work", "type": "jmap", "future_setting": {"on": true}}`
	if err := os.WriteFile(accountPath, []byte(accountFile), 0o600); err != nil {
		t.Fatal(err)
	}

	accounts := []cfg_acc.AccountConfig{{Name: "work", Type: "jmap"}}
	if err := SaveConfigFile(configPath, &FileConfig{Username: "u", Accounts: &accounts}); err != nil {
		t.Fatal(err)
	}

	main := readJSONObject(t, configPath)
	if got := string(main["shortcuts"]); got == "" {
		t.Errorf("shortcuts, a key this build does not know, was dropped from config.json")
	}
	if got := string(main["zz_future"]); got != "[\n    1,\n    2\n  ]" {
		t.Errorf("zz_future = %q, want the array kept in the file's layout", got)
	}
	if _, ok := main["default_account"]; ok {
		t.Errorf("default_account, a known key the save cleared, came back")
	}
	if _, ok := main["Download_Dir"]; ok {
		t.Errorf("Download_Dir matches download_dir regardless of case, so it is known and must not be kept")
	}
	if got := string(readJSONObject(t, accountPath)["future_setting"]); got == "" {
		t.Errorf("future_setting was dropped from account.json")
	}

	// A second save writes the same bytes: nothing is appended twice.
	before, _ := os.ReadFile(configPath)
	if err := SaveConfigFile(configPath, &FileConfig{Username: "u", Accounts: &accounts}); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(configPath)
	if string(before) != string(after) {
		t.Errorf("a second save changed config.json:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestKeepUnknownKeysWithoutAnOldFile(t *testing.T) {
	data := []byte("{\n  \"a\": 1\n}")
	if got := keepUnknownKeys(filepath.Join(t.TempDir(), "none.json"), data, nil); string(got) != string(data) {
		t.Errorf("no old file: got %s", got)
	}
}

func TestAppendJSONKeysToAnEmptyObject(t *testing.T) {
	got := appendJSONKeys([]byte("{}"), []string{"k"}, map[string]json.RawMessage{"k": json.RawMessage(`"v"`)})
	var fields map[string]string
	if err := json.Unmarshal(got, &fields); err != nil || fields["k"] != "v" {
		t.Errorf("got %s (%v)", got, err)
	}
}
