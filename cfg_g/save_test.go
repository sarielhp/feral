package cfg_g

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/fileformat"
)

// twoRunsFixture is a configuration with two accounts, saved once, so both
// runs below load the same canonical files.
func twoRunsFixture(t *testing.T) string {
	t.Helper()
	ForbidConfigWrites(false)
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	limit := 100
	accounts := []cfg_acc.AccountConfig{
		{Name: "work", Type: "jmap", Whitelist: []string{"keep@x.example", "drop@x.example"}},
		{Name: "home", Type: "gmail"},
	}
	if err := SaveConfigFile(configPath, &FileConfig{Username: "u", Limit: &limit, Accounts: &accounts}); err != nil {
		t.Fatal(err)
	}
	return configPath
}

func mustLoad(t *testing.T, configPath string) *FileConfig {
	t.Helper()
	fc, err := LoadConfigFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	return fc
}

func mustSave(t *testing.T, configPath string, fc *FileConfig) {
	t.Helper()
	if err := SaveConfigFile(configPath, fc); err != nil {
		t.Fatal(err)
	}
}

func account(t *testing.T, fc *FileConfig, name string) *cfg_acc.AccountConfig {
	t.Helper()
	for i := range *fc.Accounts {
		if (*fc.Accounts)[i].Name == name {
			return &(*fc.Accounts)[i]
		}
	}
	t.Fatalf("no account %q", name)
	return nil
}

// readJSONAny is the file at path as the loader's dirty check reads it.
func readJSONAny(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	v, err := decodeJSONValue(data)
	raw, isObject := v.(map[string]any)
	if err != nil || !isObject {
		t.Fatalf("%s is not a JSON object (%v):\n%s", path, err, data)
	}
	return raw
}

func accountNamesOf(fc *FileConfig) []string {
	var names []string
	for _, a := range *fc.Accounts {
		names = append(names, a.Name)
	}
	return names
}

// Two runs load the same configuration, each changes different settings --
// in config.json and in an account's own files -- and both save. Every
// change is there afterwards: the second save merged instead of writing back
// what it loaded. A third save by the first run, which still holds its
// stale copy, keeps the second run's changes too.
func TestTwoRunsSaveWithoutLosingEachOthersChange(t *testing.T) {
	configPath := twoRunsFixture(t)
	a := mustLoad(t, configPath)
	b := mustLoad(t, configPath)

	limit := 250
	a.Limit = &limit
	work := account(t, a, "work")
	work.Whitelist = slices.DeleteFunc(work.Whitelist, func(s string) bool { return s == "drop@x.example" })
	work.DisplayName = "Work"

	b.MyEmails = []string{"me@x.example"}
	b.DefaultAccount = "home"
	account(t, b, "work").Whitelist = append(account(t, b, "work").Whitelist, "new@x.example")
	account(t, b, "home").Blacklist = []string{"spam@x.example"}

	mustSave(t, configPath, b)
	mustSave(t, configPath, a)

	check := func(when string) {
		t.Helper()
		got := mustLoad(t, configPath)
		if got.Limit == nil || *got.Limit != 250 {
			t.Errorf("%s: limit = %v, want 250 (run A)", when, got.Limit)
		}
		if !slices.Equal(got.MyEmails, []string{"me@x.example"}) || got.DefaultAccount != "home" {
			t.Errorf("%s: my_emails %v, default_account %q: run B's changes were lost", when, got.MyEmails, got.DefaultAccount)
		}
		gw := account(t, got, "work")
		if want := []string{"keep@x.example", "new@x.example"}; !slices.Equal(gw.Whitelist, want) {
			t.Errorf("%s: work whitelist = %v, want %v (A removed one, B added one)", when, gw.Whitelist, want)
		}
		if gw.DisplayName != "Work" {
			t.Errorf("%s: work display name = %q, want run A's", when, gw.DisplayName)
		}
		if bl := account(t, got, "home").Blacklist; !slices.Equal(bl, []string{"spam@x.example"}) {
			t.Errorf("%s: home blacklist = %v, want run B's", when, bl)
		}
	}
	check("after both saves")

	a.SpamFolder = "Junk"
	mustSave(t, configPath, a)
	check("after run A saved again")
	if got := mustLoad(t, configPath); got.SpamFolder != "Junk" {
		t.Errorf("run A's second change was not saved: spam_folder = %q", got.SpamFolder)
	}
}

// Two runs that each add an account keep both, and a key another build
// added underneath -- one this build does not know -- survives a merge.
func TestTwoRunsEachAddAnAccount(t *testing.T) {
	configPath := twoRunsFixture(t)
	a := mustLoad(t, configPath)
	b := mustLoad(t, configPath)
	*a.Accounts = append(*a.Accounts, cfg_acc.AccountConfig{Name: "alpha", Type: "jmap", Username: "a@x.example"})
	*b.Accounts = append(*b.Accounts, cfg_acc.AccountConfig{Name: "beta", Type: "gmail", Username: "b@x.example"})
	mustSave(t, configPath, a)

	var top map[string]any
	data, _ := os.ReadFile(configPath)
	if err := json.Unmarshal(data, &top); err != nil {
		t.Fatal(err)
	}
	top["zz_newer_build"] = map[string]any{"on": true}
	data, _ = json.Marshal(top)
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	mustSave(t, configPath, b)
	got := mustLoad(t, configPath)
	if want := []string{"work", "home", "alpha", "beta"}; !slices.Equal(accountNamesOf(got), want) {
		t.Errorf("accounts = %v, want %v", accountNamesOf(got), want)
	}
	if account(t, got, "alpha").Username != "a@x.example" || account(t, got, "beta").Username != "b@x.example" {
		t.Errorf("an added account lost its settings")
	}
	if fields := readJSONObject(t, configPath); fields["zz_newer_build"] == nil {
		t.Errorf("a key another build added underneath was dropped by the merge")
	}
}

// When both runs change the same setting, differently, the run that saves
// last wins -- as every save did before the merge -- and nothing else of
// the other run's is lost.
func TestBothRunsChangeTheSameSetting(t *testing.T) {
	configPath := twoRunsFixture(t)
	a := mustLoad(t, configPath)
	b := mustLoad(t, configPath)
	la, lb := 1, 2
	a.Limit, b.Limit = &la, &lb
	b.SpamFolder = "Junk"
	mustSave(t, configPath, b)
	mustSave(t, configPath, a)
	got := mustLoad(t, configPath)
	if *got.Limit != 1 || got.SpamFolder != "Junk" {
		t.Errorf("limit %d, spam_folder %q: want 1 (the last save's) and Junk (the other run's)", *got.Limit, got.SpamFolder)
	}
}

// A run that loaded the configuration before another renamed an account does
// not bring the old name back: config.json keeps the new name, and the old
// account's directory is not recreated from the stale copy.
func TestStaleRunDoesNotRecreateARenamedAccount(t *testing.T) {
	configPath := twoRunsFixture(t)
	dir := filepath.Dir(configPath)
	stale := mustLoad(t, configPath)
	if err := os.Rename(AccountDir(dir, "work"), AccountDir(dir, "office")); err != nil {
		t.Fatal(err)
	}
	if err := CommitAccountRename(configPath, "work", "office"); err != nil {
		t.Fatal(err)
	}

	stale.MyEmails = []string{"me@x.example"}
	mustSave(t, configPath, stale)

	got := mustLoad(t, configPath)
	if want := []string{"office", "home"}; !slices.Equal(accountNamesOf(got), want) {
		t.Errorf("accounts = %v, want %v", accountNamesOf(got), want)
	}
	if _, err := os.Stat(AccountDir(dir, "work")); !os.IsNotExist(err) {
		t.Errorf("the renamed account's old directory was recreated: %v", err)
	}
	if !slices.Equal(got.MyEmails, []string{"me@x.example"}) {
		t.Errorf("the stale run's change to config.json was lost: %v", got.MyEmails)
	}
	if wl := account(t, got, "office").Whitelist; len(wl) != 2 {
		t.Errorf("the renamed account's whitelist = %v, want what it had", wl)
	}
}

// A stale run that changed the account another run renamed is refused, and
// writes nothing: the command must not report as saved a change that went
// nowhere.
func TestStaleRunChangingARenamedAccountIsRefused(t *testing.T) {
	configPath := twoRunsFixture(t)
	dir := filepath.Dir(configPath)
	stale := mustLoad(t, configPath)
	if err := os.Rename(AccountDir(dir, "work"), AccountDir(dir, "office")); err != nil {
		t.Fatal(err)
	}
	if err := CommitAccountRename(configPath, "work", "office"); err != nil {
		t.Fatal(err)
	}
	before := sharedFileBytes(t, dir)

	account(t, stale, "work").Blacklist = []string{"late@x.example"}
	stale.MyEmails = []string{"me@x.example"}
	err := SaveConfigFile(configPath, stale)
	if err == nil || !strings.Contains(err.Error(), `"work"`) {
		t.Fatalf("the save was not refused: %v", err)
	}
	assertSameFiles(t, "a refused save", before, sharedFileBytes(t, dir))
}

// An account whose lists config.json holds inline was never read from its
// own list files; its lists are written as held, not merged with files the
// load did not read. The auto-fix that moves the global lists to the
// accounts keeps every entry.
func TestInlineAccountListsSurviveTheAutoFix(t *testing.T) {
	ForbidConfigWrites(false)
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	writeTestFile(t, configPath, `{"username": "u", "whitelist": ["g@x.example"],
		"accounts": [{"name": "work", "type": "jmap", "whitelist": ["a@x.example", "b@x.example"]}]}`)
	fc := mustLoad(t, configPath)
	if !evaluateDirty(fc, readJSONAny(t, configPath), newDefaultConfig(dir)) {
		t.Fatal("the fixture needs no auto-fix; the test proves nothing")
	}
	mustSave(t, configPath, fc)
	want := []string{"a@x.example", "b@x.example", "g@x.example"}
	if got := account(t, mustLoad(t, configPath), "work").Whitelist; !slices.Equal(got, want) {
		t.Errorf("work whitelist = %v, want %v", got, want)
	}
}

// A shared file a newer build wrote underneath -- config.json in a newer
// format, a list wrapped as an object -- is refused at save, as at load,
// and left as it is.
func TestSaveRefusesANewerFileUnderneath(t *testing.T) {
	cases := map[string]func(dir string) (string, string){
		"config.json": func(dir string) (string, string) {
			return filepath.Join(dir, "config.json"), `{"format": 99, "username": "u", "accounts": []}`
		},
		"whitelist.json": func(dir string) (string, string) {
			return filepath.Join(AccountDir(dir, "work"), "whitelist.json"), `{"format": 2, "entries": ["a@x.example"]}`
		},
	}
	for name, newer := range cases {
		t.Run(name, func(t *testing.T) {
			configPath := twoRunsFixture(t)
			fc := mustLoad(t, configPath)
			path, content := newer(filepath.Dir(configPath))
			writeTestFile(t, path, content)
			fc.SpamFolder = "Junk"
			account(t, fc, "work").Whitelist = []string{"z@x.example"}
			var tooNew *fileformat.TooNewError
			if err := SaveConfigFile(configPath, fc); !errors.As(err, &tooNew) {
				t.Fatalf("the save did not refuse the newer file: %v", err)
			}
			if data, _ := os.ReadFile(path); string(data) != content {
				t.Errorf("the newer file was overwritten: %s", data)
			}
		})
	}
}

// The loader's auto-fix saves what it filled in and checks the result by
// reloading. A change made to config.json between the load and that save
// survives it, and the check still finds nothing left to fix.
func TestAutoFixSaveKeepsAChangeMadeUnderneath(t *testing.T) {
	ForbidConfigWrites(false)
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	writeTestFile(t, configPath, `{"username": "u", "accounts": [{"name": "work", "type": "jmap"}]}`)
	config := newDefaultConfig(dir)

	fc := mustLoad(t, configPath)
	raw := map[string]any{"username": "u", "accounts": nil}
	writeTestFile(t, configPath, `{"username": "u", "my_emails": ["me@x.example"], "accounts": [{"name": "work", "type": "jmap"}]}`)

	if !evaluateDirty(fc, raw, config) {
		t.Fatal("the fixture needs no auto-fix; the test proves nothing")
	}
	mustSave(t, configPath, fc)

	verify := mustLoad(t, configPath)
	if evaluateDirty(verify, readJSONAny(t, configPath), config) {
		data, _ := os.ReadFile(configPath)
		t.Errorf("the configuration is still dirty after the auto-fix save:\n%s", data)
	}
	if !slices.Equal(verify.MyEmails, []string{"me@x.example"}) {
		t.Errorf("the change made underneath was lost: my_emails = %v", verify.MyEmails)
	}
	if verify.Limit == nil || *verify.Limit != config.Limit {
		t.Errorf("the auto-fix's default was lost: limit = %v", verify.Limit)
	}
}

// A file that does not parse is refused at save, as at load, rather than
// overwritten or merged into.
func TestSaveRefusesADamagedFileUnderneath(t *testing.T) {
	configPath := twoRunsFixture(t)
	fc := mustLoad(t, configPath)
	writeTestFile(t, configPath, `{"username": `)
	fc.SpamFolder = "Junk"
	if err := SaveConfigFile(configPath, fc); err == nil {
		t.Fatal("a save merged into a damaged config.json")
	}
	if data, _ := os.ReadFile(configPath); string(data) != `{"username": ` {
		t.Errorf("the damaged file was overwritten: %s", data)
	}
}
