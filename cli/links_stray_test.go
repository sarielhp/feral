package cli

import (
	"strings"
	"testing"

	"github.com/sarielhp/clihelp"

	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
)

// D6: once config.json has links, a name that is not a link but names an
// account is refused, because clihelp would run it unexpanded -- against the
// default account. Every other undeclared name runs as feral.
func TestRefuseStrayAccountName(t *testing.T) {
	accounts := []cfg_acc.AccountConfig{
		{Name: "gmail"},
		{Name: "fastmail", DisplayName: "Fast"},
		{Name: "work", Aliases: []string{"job"}},
	}
	declared := map[string]cfg_g.Link{"mt": {Account: "fastmail"}, "gmail": {Account: "gmail"}}
	for _, tt := range []struct {
		name     string
		invoked  string
		links    map[string]cfg_g.Link
		declared bool
		refused  bool
	}{
		{"account name", "fastmail", declared, true, true},
		{"display name, other case", "fast", declared, true, true},
		{"alias", "JOB", declared, true, true},
		{"declared link", "gmail", declared, true, false},
		{"another link", "mt", declared, true, false},
		{"feral", "feral", declared, true, false},
		{"mail_cli", "mail_cli", declared, true, false},
		{"development binary", "feral2", declared, true, false},
		{"test binary", "cli.test", declared, true, false},
		{"before the migration", "fastmail", nil, false, false},
		{"empty links", "work", map[string]cfg_g.Link{}, true, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			config := &cfg_g.Config{Accounts: accounts, Links: tt.links, LinksDeclared: tt.declared}
			err := refuseStrayAccountName(config, tt.invoked)
			if (err != nil) != tt.refused {
				t.Fatalf("err = %v, want refused %v", err, tt.refused)
			}
			if err != nil && !clihelp.IsUsageError(err) {
				t.Errorf("the refusal is not a usage error: %v", err)
			}
		})
	}
}

func TestStrayAccountNameSaysHowToFixIt(t *testing.T) {
	config := &cfg_g.Config{
		Accounts:      []cfg_acc.AccountConfig{{Name: "work", Aliases: []string{"job"}}},
		Links:         map[string]cfg_g.Link{},
		LinksDeclared: true,
	}
	err := refuseStrayAccountName(config, "job")
	if err == nil {
		t.Fatal("not refused")
	}
	if want := `started as "job", which is not a link; run "feral link add job -- -A work" or remove the symlink`; !strings.Contains(err.Error(), want) {
		t.Errorf("err = %v, want it to say %q", err, want)
	}
}
