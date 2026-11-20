package cfg_g

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/fileformat"
)

func writeConfigJSON(t *testing.T, dir, body string) string {
	t.Helper()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadLinks(t *testing.T) {
	for _, tt := range []struct {
		name    string
		body    string // "" means no config.json
		want    map[string]Link
		present bool
		wantErr bool
	}{
		{name: "no config"},
		{name: "no key", body: `{"accounts": []}`},
		{name: "null key", body: `{"links": null}`},
		{name: "empty", body: `{"links": {}}`, want: map[string]Link{}, present: true},
		{name: "entries", body: `{"links": {"mt": {"account": "fastmail", "args": ["tui"]}}}`,
			want: map[string]Link{"mt": {Account: "fastmail", Args: []string{"tui"}}}, present: true},
		{name: "damaged file", body: `{"links": {`, wantErr: true},
		{name: "damaged links", body: `{"links": ["mt"]}`, present: true, wantErr: true},
		{name: "newer format", body: `{"format": 99, "links": {}}`, wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.body != "" {
				writeConfigJSON(t, dir, tt.body)
			}
			got, present, err := ReadLinks(dir)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, want error %v", err, tt.wantErr)
			}
			if present != tt.present {
				t.Errorf("present = %v, want %v", present, tt.present)
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("links = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestReadLinksRefusesNewerFormat(t *testing.T) {
	dir := t.TempDir()
	writeConfigJSON(t, dir, `{"format": 99, "links": {}}`)
	var tooNew *fileformat.TooNewError
	if _, _, err := ReadLinks(dir); !errors.As(err, &tooNew) {
		t.Fatalf("err = %v, want a TooNewError", err)
	}
}

// The pointer is what tells "not migrated" from "no links": nil is left out of
// config.json, and an empty map is written as {}.
func TestLinksKeyPresence(t *testing.T) {
	data, err := json.Marshal(FileConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"links"`) {
		t.Errorf("a nil Links was written: %s", data)
	}
	empty := map[string]Link{}
	data, err = json.Marshal(FileConfig{Links: &empty})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"links":{}`) {
		t.Errorf("an empty Links was not written as {}: %s", data)
	}
}

func TestLinksSurviveLoadAndSave(t *testing.T) {
	ForbidConfigWrites(false)
	dir := t.TempDir()
	path := writeConfigJSON(t, dir, `{"accounts": [{"name": "work", "type": "jmap"}],
		"links": {"mt": {"account": "work", "args": ["tui"], "description": "mail"}}}`)
	fc, err := LoadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = SaveConfigFile(path, fc); err != nil {
		t.Fatal(err)
	}
	links, present, err := ReadLinks(dir)
	want := map[string]Link{"mt": {Account: "work", Args: []string{"tui"}, Description: "mail"}}
	if err != nil || !present || !reflect.DeepEqual(links, want) {
		t.Fatalf("after a save: links=%#v present=%v err=%v", links, present, err)
	}
}

// A build that predates the field keeps links on its saves (keepUnknownKeys):
// what such a build marshals is a type without Links.
func TestLinksSurviveASaveByABuildWithoutThem(t *testing.T) {
	dir := t.TempDir()
	path := writeConfigJSON(t, dir, `{"username": "u", "links": {"mt": {"args": ["tui"]}}}`)
	type olderFileConfig struct {
		Username string `json:"username"`
	}
	data, err := json.MarshalIndent(olderFileConfig{Username: "u"}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, keepUnknownKeys(path, data, reflect.TypeFor[olderFileConfig]()), 0o600); err != nil {
		t.Fatal(err)
	}
	links, present, err := ReadLinks(dir)
	if err != nil || !present || len(links["mt"].Args) != 1 {
		t.Fatalf("links after an older save: %#v present=%v err=%v", links, present, err)
	}
}

func TestSaveConfigFileRefusesUnderDryRun(t *testing.T) {
	dir := t.TempDir()
	path := writeConfigJSON(t, dir, `{"accounts": []}`)
	ForbidConfigWrites(true)
	t.Cleanup(func() { ForbidConfigWrites(false) })
	links := map[string]Link{"mt": {Args: []string{"tui"}}}
	if err := SaveConfigFile(path, &FileConfig{Links: &links}); !errors.Is(err, ErrConfigWriteForbidden) {
		t.Fatalf("err = %v, want ErrConfigWriteForbidden", err)
	}
	if _, present, _ := ReadLinks(dir); present {
		t.Fatal("a --dry-run save wrote links")
	}
}

func TestValidateLinkName(t *testing.T) {
	for name, ok := range map[string]bool{
		"mt": true, "gmail": true, "mail_cli": true, "a.b+c-d_e": true, "MT2": true,
		"": false, "feral": false, "-x": false, ".": false, "..": false, "a/b": false, "a b": false, "é": false,
	} {
		if err := ValidateLinkName(name); (err == nil) != ok {
			t.Errorf("ValidateLinkName(%q) = %v, want ok=%v", name, err, ok)
		}
	}
}

func TestMigrateAliasesToLinks(t *testing.T) {
	prev := ProgramNamesBesideBinary
	t.Cleanup(func() { ProgramNamesBesideBinary = prev })
	ProgramNamesBesideBinary = func() []string { return []string{"feral", "GMail", "mail_cli"} }

	accounts := []cfg_acc.AccountConfig{
		{Name: "gmail", Aliases: []string{"gmail", "feral", "Mail_CLI", "bad name"}},
		{Name: "fastmail", Aliases: []string{"fastmail", "fm"}},
		{Name: "plain"},
	}
	fc := &FileConfig{Accounts: &accounts}
	if !MigrateAliasesToLinks(fc) {
		t.Fatal("the migration reported no change")
	}
	want := map[string]Link{
		"GMail":    {Account: "gmail", Description: MigratedLinkDescription},
		"fastmail": {Account: "fastmail", Description: MigratedLinkDescription},
		"fm":       {Account: "fastmail", Description: MigratedLinkDescription},
	}
	if !reflect.DeepEqual(*fc.Links, want) {
		t.Errorf("links = %#v, want %#v", *fc.Links, want)
	}
	if MigrateAliasesToLinks(fc) {
		t.Error("a second migration changed a config that has links")
	}
}

func TestMigrateAliasesToLinksWithNoAliases(t *testing.T) {
	accounts := []cfg_acc.AccountConfig{{Name: "work"}}
	fc := &FileConfig{Accounts: &accounts}
	if !MigrateAliasesToLinks(fc) || fc.Links == nil || len(*fc.Links) != 0 {
		t.Fatalf("no aliases must still write an empty links: %#v", fc.Links)
	}
	existing := map[string]Link{"mt": {Args: []string{"tui"}}}
	accounts[0].Aliases = []string{"w"}
	fc = &FileConfig{Accounts: &accounts, Links: &existing}
	if MigrateAliasesToLinks(fc) || len(*fc.Links) != 1 {
		t.Fatalf("the migration touched a config that already has links: %#v", *fc.Links)
	}
}

func TestLinksNamingAndNamesAccount(t *testing.T) {
	links := map[string]Link{
		"b": {Account: "Work"}, "a": {Account: "work"}, "c": {Account: "home"}, "d": {Args: []string{"tui"}},
	}
	if got := LinksNaming(links, "work"); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("LinksNaming = %v", got)
	}
	accounts := []cfg_acc.AccountConfig{{Name: "work", DisplayName: "Office", Aliases: []string{"job"}}}
	for name, want := range map[string]bool{"WORK": true, "office": true, "Job": true, "mt": false} {
		if got := NamesAccount(accounts, name); got != want {
			t.Errorf("NamesAccount(%q) = %v, want %v", name, got, want)
		}
	}
}
