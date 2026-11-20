package cli

import (
	"fmt"
	"strconv"

	"github.com/sarielhp/clihelp"

	"github.com/sarielhp/feral/app"
	"github.com/sarielhp/feral/cfg_g"
)

// accountChoice is the one account a command line names. -A/--account and
// --account-index (and -N, which main rewrites to it) all write it, so the
// last of them on the line wins: a link's preset comes first, and whatever
// the user types after it beats it. The two used to be separate values, and
// naming both was an error -- which a link that names an account would have
// made of every -2 typed after it.
type accountChoice struct {
	value   string
	byIndex bool
}

// flagAccount is the parsed account choice of this run.
var flagAccount accountChoice

func (c *accountChoice) set() bool { return c.value != "" }

// accountChoiceValue is one spelling's view of the shared choice: by name or
// by index. Both are clihelp.Var targets.
type accountChoiceValue struct {
	choice  *accountChoice
	byIndex bool
}

// String is the choice when this spelling made it, so that the declared
// default -- what pflag reads at bind time, before anything is parsed -- is
// empty and the help pages do not change.
func (v accountChoiceValue) String() string {
	if v.choice == nil || v.choice.byIndex != v.byIndex {
		return ""
	}
	return v.choice.value
}

func (v accountChoiceValue) Set(s string) error {
	v.choice.value = s
	v.choice.byIndex = v.byIndex
	return nil
}

func (v accountChoiceValue) Type() string { return "string" }

// accountOptions are -A/--account and --account-index, bound to one choice.
// They reset it, as clihelp.String resets its target: the options are built
// once per App, and a test process runs several.
func accountOptions(choice *accountChoice) []clihelp.Option {
	*choice = accountChoice{}
	byName := clihelp.Var(accountChoiceValue{choice: choice}, "-A, --account <name>", "Target account name")
	byName.Complete = CompleteAccountNames
	byIndex := clihelp.Var(accountChoiceValue{choice: choice, byIndex: true}, "--account-index N",
		"Target account by config index (also -N)")
	return []clihelp.Option{
		clihelp.Group("Target & Account", byName),
		clihelp.Group("Target & Account", byIndex),
	}
}

// refuseStrayAccountName refuses a run under a name that is not a link but is
// an account's name, display name or alias. clihelp runs an undeclared name
// unexpanded, so a `gmail` symlink left after its link was removed -- or one
// made by hand once aliases stopped selecting by program name -- would run
// `gmail delete` against the default account. Any other undeclared name
// (mail_cli, a development binary, a test) runs as feral. Before config.json
// has links, aliases still select by program name, and nothing is refused.
// The list checked is the one the loader read, not clihelp's, which on the
// run that migrated was read before the key existed.
func refuseStrayAccountName(config *cfg_g.Config, invoked string) error {
	if !config.LinksDeclared || invoked == "" || invoked == app.AppName || invoked == cfg_g.LegacyAppName {
		return nil
	}
	if _, declared := config.Links[invoked]; declared || !cfg_g.NamesAccount(config.Accounts, invoked) {
		return nil
	}
	account := invoked
	if acc, err := cfg_g.Lookup(config.Accounts, invoked); err == nil {
		account = acc.Name
	}
	return fmt.Errorf("%w: started as %q, which is not a link; run \"%s link add %s -- -A %s\" or remove the symlink",
		clihelp.ErrUsage, invoked, app.AppName, invoked, account)
}

// selectCLIAccount applies the account choice to the loaded configuration.
func selectCLIAccount(config *cfg_g.Config, choice accountChoice) error {
	if !choice.set() {
		return nil
	}
	if choice.byIndex {
		idx, err := strconv.Atoi(choice.value)
		if err != nil || idx <= 0 || idx > len(config.Accounts) {
			return fmt.Errorf("%w: account index %q is out of range; you have %d configured account(s)",
				clihelp.ErrUsage, choice.value, len(config.Accounts))
		}
		config.SelectedAccount = cfg_g.NameOf(config.Accounts[idx-1])
		return nil
	}
	acc, err := cfg_g.Lookup(config.Accounts, choice.value)
	if err != nil {
		return err
	}
	config.SelectedAccount = cfg_g.NameOf(*acc)
	return nil
}
