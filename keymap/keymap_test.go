package keymap

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/sarielhp/feral/fileformat"
)

func testTable() *Table {
	return &Table{
		Groups: []Group{
			{Name: "list", Bindings: []Binding{
				{Action: "open", Keys: []string{"e"}},
				{Action: "archive", Keys: []string{"E"}},
				{Action: "delete", Keys: []string{"d"}},
				{Action: "browser", Keys: []string{"w", "W"}},
				{Action: "split", Keys: []string{" "}},
			}},
			{Name: "message", Bindings: []Binding{
				{Action: "archive", Keys: []string{"E"}},
				{Action: "search", Keys: []string{"/"}},
			}},
		},
		Reserved:         map[string]string{"q": "quit", "enter": "open", "f1": "help"},
		ReservedPrefixes: map[string]string{"alt+": "the menu"},
	}
}

func TestDefaultLooksUpBothWays(t *testing.T) {
	tb := testTable()
	if err := tb.Check(); err != nil {
		t.Fatal(err)
	}
	km := Default(tb)
	if a := km.Action("list", "W"); a != "browser" {
		t.Errorf("W runs %q", a)
	}
	if a := km.Action("message", "d"); a != "" {
		t.Errorf("d runs %q in a message", a)
	}
	if !slices.Equal(km.Keys("list", "browser"), []string{"w", "W"}) || km.Changed("list", "browser") {
		t.Errorf("browser keys %v changed %v", km.Keys("list", "browser"), km.Changed("list", "browser"))
	}
}

func TestParseOverridesOneGroupOnly(t *testing.T) {
	km, err := Parse(testTable(), "keys.json", []byte(`{"format": 1, "list": {"archive": "A", "browser": ["o", "Ctrl+B"], "split": "space"}}`))
	if err != nil {
		t.Fatal(err)
	}
	checks := []struct{ group, key, want string }{
		{"list", "A", "archive"}, {"list", "E", ""}, {"message", "E", "archive"},
		{"list", "o", "browser"}, {"list", "ctrl+b", "browser"}, {"list", "w", ""}, {"list", " ", "split"},
	}
	for _, c := range checks {
		if got := km.Action(c.group, c.key); got != c.want {
			t.Errorf("%s %q runs %q, want %q", c.group, c.key, got, c.want)
		}
	}
	if !km.Changed("list", "archive") || km.Changed("message", "archive") || km.Changed("list", "delete") {
		t.Error("Changed does not follow the file")
	}
}

// Each refusal names the file, the line and the entry.
func TestParseRefusesWithTheLine(t *testing.T) {
	cases := []struct{ name, file, want string }{
		{"unknown group", "{\n\"lists\": {}\n}", `keys.json: line 2: lists: no such group`},
		{"unknown action", "{\"list\": {\n  \"archiv\": \"A\"}}", `line 2: list.archiv: no such action`},
		{"not a key", "{\"list\": {\n\n  \"archive\": \"Hyper+A\"}}", `line 3: list.archive: "Hyper+A" is not a key`},
		{"reserved", `{"list": {"archive": "q"}}`, `line 1: list.archive: "q" is not configurable: it is quit`},
		{"reserved prefix", `{"list": {"archive": "alt+x"}}`, `"alt+x" is not configurable: it is the menu`},
		{"reserved after normalizing", `{"list": {"archive": "F1"}}`, `"f1" is not configurable`},
		{"shared with a default", "{\"list\": {\n  \"archive\": \"e\"}}", `line 2: list.archive: "e" is also the key of open; rebind open too`},
		{"shared, blames the later", "{\"list\": {\n \"delete\": \"x\",\n \"archive\": \"x\"}}", `line 3: list.archive: "x" is also the key of delete`},
		{"twice in one list", `{"list": {"archive": ["A", "A"]}}`, `"A" is listed twice`},
		{"empty", `{"list": {"archive": []}}`, `no key: every action needs one`},
		{"empty string", `{"list": {"archive": ""}}`, `"" is not a key`},
		{"member twice", "{\"list\": {\"archive\": \"A\",\n \"archive\": \"B\"}}", `line 2: list.archive: named twice`},
		{"group twice", "{\"list\": {},\n \"list\": {}}", `line 2: list: named twice`},
		{"wrong value", `{"list": {"archive": 5}}`, `must be a key or a list of keys`},
		{"group not an object", `{"list": ["A"]}`, `list: must be an object of actions`},
		{"not an object", `["list"]`, `the file must be a JSON object`},
		{"format not a number", `{"format": "1"}`, `format: must be a number`},
		{"broken JSON", `{"list": {"archive": "A"`, `not valid JSON`},
		{"trailing text", `{} {}`, `unexpected text after the object`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse(testTable(), "keys.json", []byte(c.file))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %v, want it to contain %q", err, c.want)
			}
			if !strings.HasPrefix(err.Error(), "keys.json") {
				t.Errorf("error %q does not name the file", err)
			}
		})
	}
}

// Swapping two actions' keys is not a clash: the check runs once every
// entry is applied.
func TestParseAllowsASwap(t *testing.T) {
	km, err := Parse(testTable(), "keys.json", []byte(`{"list": {"archive": "d", "delete": "E"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if km.Action("list", "d") != "archive" || km.Action("list", "E") != "delete" {
		t.Error("the swap did not apply")
	}
}

// A newer build's file is refused as newer, wherever its format stands and
// whatever groups it names that this build lacks.
func TestParseRefusesANewerFormat(t *testing.T) {
	for _, file := range []string{
		`{"format": 99}`,
		`{"format": 2, "tree": {"open": "o"}}`,
		`{"tree": {"open": "o"}, "format": 2}`,
		`{"list": {"undo_all": "Z"}, "format": 2}`,
	} {
		_, err := Parse(testTable(), "keys.json", []byte(file))
		var tooNew *fileformat.TooNewError
		if !errors.As(err, &tooNew) {
			t.Errorf("%s: error %v, want a TooNewError", file, err)
		}
	}
}

func writeKeys(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	km, err := Load(testTable(), dir)
	if err != nil || km.Action("list", "E") != "archive" {
		t.Fatalf("a missing file must be the defaults: %v", err)
	}
	writeKeys(t, dir, `{"list": {"archive": "A"}}`)
	if km, err = Load(testTable(), dir); err != nil || km.Action("list", "A") != "archive" {
		t.Fatalf("Load: %v", err)
	}
	writeKeys(t, dir, `{"list": {"archive": "q"}}`)
	if _, err = Load(testTable(), dir); err == nil || !strings.Contains(err.Error(), filepath.Join(dir, FileName)) {
		t.Fatalf("a broken file must be refused by its path: %v", err)
	}
}

// The printed defaults are a keys file that loads back to the defaults.
func TestJSONRoundTrips(t *testing.T) {
	tb := testTable()
	out := tb.JSON()
	km, err := Parse(tb, "keys.json", out)
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, g := range tb.Groups {
		for _, b := range g.Bindings {
			if km.Changed(g.Name, b.Action) {
				t.Errorf("%s.%s changed in the round trip", g.Name, b.Action)
			}
		}
	}
	for _, want := range []string{`"format": 1`, `"browser": ["w", "W"]`, `"split": "space"`} {
		if !strings.Contains(string(out), want) {
			t.Errorf("output lacks %s:\n%s", want, out)
		}
	}
}

func TestNormalizeAndDisplay(t *testing.T) {
	for in, want := range map[string]string{
		"G": "G", "g": "g", "Ctrl+F": "ctrl+f", "F5": "f5", "space": " ", "Space": " ", "PgDn": "pgdown",
		"ש": "ש", "alt+X": "alt+X", "shift+left": "shift+left", "Enter": "enter",
	} {
		if got, err := Normalize(in); err != nil || got != want {
			t.Errorf("Normalize(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "ctrl", "hyper+a", "\x01", "alt+"} {
		if _, err := Normalize(bad); err == nil {
			t.Errorf("Normalize(%q) accepted it", bad)
		}
	}
	for in, want := range map[string]string{
		"ctrl+a": "Ctrl+a", "f5": "F5", " ": "Space", "E": "E", "f": "f", "up": "↑", "alt+x": "Alt+x", "pgdown": "PgDn",
	} {
		if got := Display(in); got != want {
			t.Errorf("Display(%q) = %q, want %q", in, got, want)
		}
	}
	if got := DisplayKeys([]string{"U", "u"}); got != "U / u" {
		t.Errorf("DisplayKeys = %q", got)
	}
}

func TestCheckRefusesABrokenTable(t *testing.T) {
	for name, tb := range map[string]*Table{
		"shared key":    {Groups: []Group{{Name: "g", Bindings: []Binding{{"a", []string{"x"}}, {"b", []string{"x"}}}}}},
		"twice":         {Groups: []Group{{Name: "g", Bindings: []Binding{{"a", []string{"x"}}, {"a", []string{"y"}}}}}},
		"misspelled":    {Groups: []Group{{Name: "g", Bindings: []Binding{{"a", []string{"Ctrl+X"}}}}}},
		"reserved":      {Groups: []Group{{Name: "g", Bindings: []Binding{{"a", []string{"q"}}}}}, Reserved: map[string]string{"q": "quit"}},
		"without a key": {Groups: []Group{{Name: "g", Bindings: []Binding{{"a", nil}}}}},
	} {
		if err := tb.Check(); err == nil {
			t.Errorf("%s: Check accepted it", name)
		}
	}
}
