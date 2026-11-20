package cli

import (
	"reflect"
	"testing"

	"github.com/sarielhp/clihelp"

	"github.com/sarielhp/feral/cfg_g"
)

func TestSplitPresetAccount(t *testing.T) {
	for _, tt := range []struct {
		name    string
		preset  []string
		choice  accountChoice
		rest    []string
		full    []string
		wantErr bool
	}{
		{name: "short", preset: []string{"-A", "fastmail", "tui"},
			choice: accountChoice{value: "fastmail"}, rest: []string{"tui"}, full: []string{"tui"}},
		{name: "long attached", preset: []string{"tui", "--account=work"},
			choice: accountChoice{value: "work"}, rest: []string{"tui"}, full: []string{"tui"}},
		{name: "short attached", preset: []string{"-Awork"}, choice: accountChoice{value: "work"}},
		{name: "index", preset: []string{"-2", "tui"},
			choice: accountChoice{value: "2", byIndex: true}, rest: []string{"tui"}, full: []string{"tui"}},
		{name: "long index", preset: []string{"--account-index", "3"}, choice: accountChoice{value: "3", byIndex: true}},
		{name: "last wins", preset: []string{"-A", "a", "-2", "-A", "b"}, choice: accountChoice{value: "b"}},
		{name: "none", preset: []string{"tui"}, rest: []string{"tui"}, full: []string{"tui"}},
		{name: "abbreviation", preset: []string{"-A", "a", "tu"},
			choice: accountChoice{value: "a"}, rest: []string{"tu"}, full: []string{"tui"}},
		{name: "other flags kept", preset: []string{"--read-only", "tui"},
			rest: []string{"--read-only", "tui"}, full: []string{"--read-only", "tui"}},
		{name: "value of another flag", preset: []string{"scan", "--pattern", "-2"},
			rest: []string{"scan", "--pattern", "-2"}, full: []string{"scan", "--pattern", "-2"}},
		{name: "after the terminator", preset: []string{"scan", "--", "-A", "x"},
			rest: []string{"scan", "--", "-A", "x"}, full: []string{"scan", "--", "-A", "x"}},
		{name: "cluster", preset: []string{"-vA", "x"}, wantErr: true},
		{name: "no value", preset: []string{"tui", "-A"}, wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a := InitCLI(&Session{Config: &cfg_g.Config{}})
			got, err := splitPresetAccount(a, tt.preset)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, want error %v", err, tt.wantErr)
			}
			if tt.wantErr {
				if !clihelp.IsUsageError(err) {
					t.Errorf("err = %v, want a usage error", err)
				}
				return
			}
			if got.choice != tt.choice || !reflect.DeepEqual(got.rest, nonNil(tt.rest)) || !reflect.DeepEqual(got.full, tt.full) {
				t.Errorf("got choice=%+v rest=%q full=%q, want %+v %q %q", got.choice, got.rest, got.full, tt.choice, tt.rest, tt.full)
			}
		})
	}
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// The two account spellings share one value, and building the options
// resets it, as clihelp.String resets its target.
func TestAccountOptionsShareOneChoice(t *testing.T) {
	var choice accountChoice
	opts := accountOptions(&choice)
	if len(opts) != 2 {
		t.Fatalf("options = %d", len(opts))
	}
	choice = accountChoice{value: "x"}
	accountOptions(&choice)
	if choice.set() {
		t.Fatal("building the options did not reset the choice")
	}
	byName := accountChoiceValue{choice: &choice}
	byIndex := accountChoiceValue{choice: &choice, byIndex: true}
	_ = byName.Set("work")
	_ = byIndex.Set("2")
	if choice != (accountChoice{value: "2", byIndex: true}) || byName.String() != "" || byIndex.String() != "2" {
		t.Fatalf("choice = %+v name=%q index=%q", choice, byName.String(), byIndex.String())
	}
}
