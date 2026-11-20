package cli

import (
	"fmt"
	"strings"

	"github.com/sarielhp/clihelp"

	"github.com/sarielhp/feral/app"
	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
)

// accountRenameUsage is the rename command's usage line. cliBeforeRun matches
// it to let this one command load a configuration whose rename is unfinished.
const accountRenameUsage = "feral account rename <account> <new_name>"

func newAccountRenameCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "rename",
		Title:       "account rename <account> <new_name>",
		Description: "Rename an account and its files.",
		LongDescription: "Change an account's name -- what -A takes, what %name:label means, and what its files are " +
			"named after. The account's settings directory, its OAuth token, its cache, the watcher's record of " +
			"it and any queued outgoing message move to the new name, and default_account, " +
			"default_send_account and every link that runs it follow it, so nothing is downloaded again and no sign-in is needed. " +
			"A name is letters, digits, '-' and '_', starting with a letter; \"default\" and \"all\" are reserved. " +
			"Only the name shown in listings is changed by `account display-name`.",
		UsageLine: accountRenameUsage,
		Parameters: []clihelp.Param{
			{Name: "<account>", Description: "The account to rename.", Complete: CompleteAccountNames},
			{Name: "<new_name>", Description: "Its new name."},
		},
		Notes: []clihelp.Note{
			{Heading: "Interrupted", Text: "The rename writes account_rename.json beside config.json while it runs, and " +
				"every other command refuses to start until it is gone. Running the same rename again finishes it."},
			{Heading: "Other machines", Text: "Each machine keeps its own configuration: rename the account on each " +
				"one. --dry-run lists every file it would move."},
			{Heading: "Watchers", Text: "Refused while `feral watch` or a TUI's watcher runs; close them first."},
		},
		Args: clihelp.ExactArgs(2),
		Run: func(ctx *clihelp.Context) error {
			return runAccountRename(session, ctx.Args[0], ctx.Args[1])
		},
		Examples: []clihelp.Example{
			{Line: "feral --dry-run account rename default personal", Description: "List what would move"},
			{Line: "feral account rename default personal"},
		},
	}
}

func runAccountRename(session *Session, ident, newName string) error {
	plan, err := session.accounts().RenameAccount(session.Config, ident, newName)
	if err != nil {
		return err
	}
	if session.Config.DryRun {
		fmt.Fprintf(session.out(), "Would rename account %q to %q:\n", plan.From, plan.To)
		for _, m := range plan.Moves {
			fmt.Fprintf(session.out(), "  move %s\n    to %s\n", m.From, m.To)
		}
		fmt.Fprintf(session.out(), "  rename it in config.json (links too), the watcher state and the outbox\n")
		return nil
	}
	verb := "Renamed"
	if plan.Resumed {
		verb = "Finished renaming"
	}
	fmt.Fprintf(session.errOut(), "%s %s account %q to %q (%d file(s) moved, %d queued message(s) relabelled).\n",
		app.PrefixSuccess, verb, plan.From, plan.To, len(plan.Moves), plan.Requeued)
	return nil
}

func newAccountDisplayNameCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "display-name",
		Title:       "account display-name <account> <display_name>",
		Description: "Set the name an account is shown under.",
		LongDescription: "Set the label an account carries in listings, prompts and the TUI. It is also accepted " +
			"wherever an account is named, so it may not be another account's name or display name. It changes " +
			"no file; `account rename` changes the name itself.",
		UsageLine: "feral account display-name <account> <display_name>",
		Parameters: []clihelp.Param{
			{Name: "<account>", Description: "The account.", Complete: CompleteAccountNames},
			{Name: "<display_name>", Description: "The name to show it under."},
		},
		Args: clihelp.ExactArgs(2),
		Run: func(ctx *clihelp.Context) error {
			return runAccountDisplayName(session, ctx.Args[0], ctx.Args[1])
		},
		Examples: []clihelp.Example{
			{Line: "feral account display-name work \"Work Mail\""},
		},
	}
}

func runAccountDisplayName(session *Session, ident, display string) error {
	display = strings.TrimSpace(display)
	if display == "" {
		return fmt.Errorf("a display name cannot be empty")
	}
	configPath, fc, err := loadFileConfig(session)
	if err != nil {
		return err
	}
	acc, err := cfg_g.Lookup(*fc.Accounts, ident)
	if err != nil {
		return err
	}
	for _, other := range *fc.Accounts {
		if other.Name == acc.Name {
			continue
		}
		if strings.EqualFold(other.Name, display) || strings.EqualFold(other.DisplayName, display) {
			return ErrAccountExists(display)
		}
	}
	acc.DisplayName = display
	if err := cfg_g.SaveConfigFile(configPath, fc); err != nil {
		return err
	}
	fmt.Fprintf(session.errOut(), "%s Account %q is now shown as %q.\n", app.PrefixSuccess, acc.Name, display)
	return nil
}

// loadFileConfig reads config.json as it is on disk, as every command that
// edits it does.
func loadFileConfig(session *Session) (string, *cfg_g.FileConfig, error) {
	_, configPath, err := cfg_g.ResolveConfigPath(session.Config)
	if err != nil {
		return "", nil, err
	}
	fc, err := cfg_g.LoadConfigFile(configPath)
	if err != nil {
		return "", nil, err
	}
	return configPath, fc, nil
}

func newAccountDefaultCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "default",
		Title:       "account default [account]",
		Description: "Show or set the default account.",
		LongDescription: "The default account is the one a command acts on when no account is named -- by -A, " +
			"by a numeric shortcut such as -2, or by a link's preset (`feral link`). It is stored as " +
			"default_account in config.json; with none set, the first account is the default. A folder " +
			"reference that names no account -- `feral %`, `scan %inbox`, `msg show %inbox` -- is read " +
			"there; `%all:inbox` reads every account. `watch` still covers every account.",
		UsageLine: "feral account default [account]",
		Parameters: []clihelp.Param{
			{Name: "[account]", Description: "The account to make the default.", Complete: CompleteAccountNames},
		},
		Notes: []clihelp.Note{
			{Heading: "Clearing", Text: "`feral config unset default_account` returns to the first account."},
		},
		Args: clihelp.MaximumNArgs(1),
		Run: func(ctx *clihelp.Context) error {
			if len(ctx.Args) == 0 {
				return showDefaultAccount(session)
			}
			return setDefaultAccount(session, ctx.Args[0])
		},
		Examples: []clihelp.Example{
			{Line: "feral account default", Description: "Name the default account"},
			{Line: "feral account default fastmail"},
		},
	}
}

func showDefaultAccount(session *Session) error {
	_, fc, err := loadFileConfig(session)
	if err != nil {
		return err
	}
	def := cfg_g.DefaultOf(*fc.Accounts)
	if def == nil {
		return cfg_g.ErrNoAccounts
	}
	fmt.Fprintln(session.out(), def.Name)
	switch {
	case cfg_g.DanglingDefault(*fc.Accounts, fc.DefaultAccount):
		fmt.Fprintf(session.errOut(), "%s default_account names %q, which is not configured; using the first account.\n",
			app.PrefixWarn, fc.DefaultAccount)
	case fc.DefaultAccount == "":
		fmt.Fprintf(session.errOut(), "%s No default account is set; the first account is used.\n", app.PrefixInfo)
	}
	return nil
}

func setDefaultAccount(session *Session, ident string) error {
	configPath, fc, err := loadFileConfig(session)
	if err != nil {
		return err
	}
	acc, err := cfg_g.Lookup(*fc.Accounts, ident)
	if err != nil {
		return err
	}
	if session.Config.DryRun {
		fmt.Fprintf(session.errOut(), "%s Would make %q the default account (dry run).\n", app.PrefixInfo, acc.Name)
		return nil
	}
	fc.DefaultAccount = acc.Name
	if err := cfg_g.SaveConfigFile(configPath, fc); err != nil {
		return err
	}
	session.Config.DefaultAccount = acc.Name
	fmt.Fprintf(session.errOut(), "%s %q is now the default account.\n", app.PrefixSuccess, acc.Name)
	return nil
}

// warnAccountNames says what is wrong with the configured names: an account
// named with a reserved word or characters a new name may not hold, and a
// default_account that names nothing. They load as they are -- refusing them
// would lock the user out of `account rename` -- so this is where they hear.
func warnAccountNames(session *Session, accounts []cfg_acc.AccountConfig, defaultAccount string) {
	for _, acc := range accounts {
		if err := cfg_g.ValidateAccountName(acc.Name); err != nil {
			fmt.Fprintf(session.errOut(), "%s %v. Rename it: feral account rename %s <new_name>\n",
				app.PrefixWarn, err, acc.Name)
		}
	}
	if cfg_g.DanglingDefault(accounts, defaultAccount) {
		fmt.Fprintf(session.errOut(), "%s default_account names %q, which is not configured; the first account is used.\n",
			app.PrefixWarn, defaultAccount)
	}
}
