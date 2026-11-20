package cfg_g

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sharedFileBytes reads every shared file under configDir, by its path
// relative to configDir.
func sharedFileBytes(t *testing.T, configDir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(configDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".json") {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(configDir, path)
		out[rel] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	mkdirs(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// loadAndSave is one run that loads the configuration and saves it back
// unchanged.
func loadAndSave(t *testing.T, configPath string) {
	t.Helper()
	fc, err := LoadConfigFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveConfigFile(configPath, fc); err != nil {
		t.Fatal(err)
	}
}

func assertSameFiles(t *testing.T, what string, before, after map[string]string) {
	t.Helper()
	if len(before) != len(after) {
		t.Errorf("%s: files %v became %v", what, keysOf(before), keysOf(after))
	}
	for name, b := range before {
		if a := after[name]; a != b {
			t.Errorf("%s changed %s:\nbefore:\n%s\nafter:\n%s", what, name, b, a)
		}
	}
}

func keysOf(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

// A hand-written configuration -- keys in any order, sets unsorted, a key
// this build does not know in the middle of the alphabet -- is written in the
// canonical layout by the first save, and a load and save of that writes the
// same bytes again, in every shared file.
func TestCanonicalConfigIsAFixedPoint(t *testing.T) {
	ForbidConfigWrites(false)
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	writeTestFile(t, configPath, `{"username": "u", "my_emails": ["z@x.example", "a@x.example"],
		"allowed_languages": ["hebrew", "english"], "middle_unknown": {"b": 2, "a": 1.50},
		"accounts": [{"name": "work", "type": "jmap"}, {"name": "home", "type": "gmail"}],
		"links": {"zz": {"account": "work"}, "aa": {"args": ["tui", "--no-watch"]}}}`)
	writeTestFile(t, filepath.Join(AccountDir(dir, "work"), "account.json"),
		`{"type": "jmap", "name": "work", "zeta_future": true, "username": "w@x.example"}`)
	writeTestFile(t, filepath.Join(AccountDir(dir, "work"), "whitelist.json"), `["b@x.example", "a@x.example"]`)
	writeTestFile(t, filepath.Join(AccountDir(dir, "work"), "rules.json"),
		`[{"sender": "z@x.example", "label": "Z"}, {"sender": "a@x.example", "label": "A"}]`)

	loadAndSave(t, configPath)
	first := sharedFileBytes(t, dir)
	main := first["config.json"]
	for _, want := range []string{
		"\"my_emails\": [\n    \"a@x.example\",\n    \"z@x.example\"\n  ]",
		"\"allowed_languages\": [\n    \"english\",\n    \"hebrew\"\n  ]",
		"\"middle_unknown\": {\n    \"a\": 1.50,\n    \"b\": 2\n  },\n  \"my_emails\"",
		"\"aa\": {\n      \"args\": [\n        \"tui\",\n        \"--no-watch\"\n      ]",
		"\"accounts\": [\n    {\n      \"name\": \"work\"",
	} {
		if !strings.Contains(main, want) {
			t.Errorf("config.json lacks %q:\n%s", want, main)
		}
	}
	if !strings.HasSuffix(main, "}\n") {
		t.Errorf("config.json does not end in a newline")
	}
	white := first[filepath.Join("accounts", "work", "whitelist.json")]
	if white != "[\n  \"a@x.example\",\n  \"b@x.example\"\n]\n" {
		t.Errorf("whitelist.json is not sorted:\n%s", white)
	}
	rules := first[filepath.Join("accounts", "work", "rules.json")]
	if strings.Index(rules, "z@x.example") > strings.Index(rules, "a@x.example") {
		t.Errorf("rules.json was reordered; the order of rules decides which fires:\n%s", rules)
	}

	loadAndSave(t, configPath)
	assertSameFiles(t, "a load and save of a canonical configuration", first, sharedFileBytes(t, dir))
}

// An account rename edits config.json's raw JSON instead of saving a
// struct; it writes the same layout, so a load and save after it changes
// nothing.
func TestAccountRenameWritesTheCanonicalLayout(t *testing.T) {
	ForbidConfigWrites(false)
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	writeTestFile(t, configPath, `{"username": "u", "default_account": "old", "extra": [3, 1],
		"accounts": [{"name": "old", "type": "jmap"}], "links": {"m": {"account": "old", "args": ["tui"]}}}`)
	loadAndSave(t, configPath)
	if err := os.Rename(AccountDir(dir, "old"), AccountDir(dir, "new")); err != nil {
		t.Fatal(err)
	}
	if err := CommitAccountRename(configPath, "old", "new"); err != nil {
		t.Fatal(err)
	}
	renamed := sharedFileBytes(t, dir)
	if !strings.Contains(renamed["config.json"], `"default_account": "new"`) {
		t.Fatalf("the rename did not happen:\n%s", renamed["config.json"])
	}
	// Only config.json: account.json still carries the old name, which the
	// loader overrides from config.json and the next save rewrites -- a
	// change of content, not of layout.
	loadAndSave(t, configPath)
	after := sharedFileBytes(t, dir)
	if after["config.json"] != renamed["config.json"] {
		t.Errorf("a load and save after an account rename changed config.json:\nbefore:\n%s\nafter:\n%s",
			renamed["config.json"], after["config.json"])
	}
}

// The canonical layout keeps a number's text and refuses what is not one
// JSON document.
func TestCanonicalJSON(t *testing.T) {
	got, err := canonicalJSON("config.json", []byte(`{"b": 1e3, "a": [2, 1], "whitelist": ["y", "x"]}`))
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"a\": [\n    2,\n    1\n  ],\n  \"b\": 1e3,\n  \"whitelist\": [\n    \"x\",\n    \"y\"\n  ]\n}\n"
	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	if _, err := canonicalJSON("config.json", []byte(`{} {}`)); err == nil {
		t.Errorf("two documents were accepted")
	}
	// A set of anything but strings is left in its order.
	got, _ = canonicalJSON("whitelist.json", []byte(`[2, 1]`))
	if string(got) != "[\n  2,\n  1\n]\n" {
		t.Errorf("a non-string list was sorted: %s", got)
	}
}
