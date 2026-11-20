package cli

import (
	"fmt"
	"strings"

	"github.com/sarielhp/clihelp"
)

// ConfigCmd returns the clihelp.Command for the config command.
func ConfigCmd(session *Session) clihelp.Command {
	showRun := func(_ *clihelp.Context) error {
		return session.configOps().Show(session.Config)
	}

	return clihelp.Command{
		Name:        "config",
		Group:       "Setup",
		Description: "Show or manage configuration options.",
		UsageLine:   "feral config [subcommand]",
		Run:         showRun,
		Subcommands: []clihelp.Command{
			{
				Name:        "show",
				Title:       "config show",
				Description: "Display the active configuration values.",
				UsageLine:   "feral config show",
				Args:        clihelp.NoArgs,
				Run:         showRun,
				Examples: []clihelp.Example{
					{Line: "feral config show"},
					{Line: "feral config"},
				},
			},
			{
				Name:        "get",
				Title:       "config get <key>",
				Description: "Print a configuration parameter's value.",
				UsageLine:   "feral config get <key>",
				Parameters: []clihelp.Param{
					{Name: "<key>", Description: "The configuration parameter name.", Complete: CompleteConfigKeys},
				},
				Examples: []clihelp.Example{
					{Line: "feral config get mutt_alias_file"},
					{Line: "feral config get browser"},
					{Line: "feral config get download_dir"},
				},
				Args: clihelp.ExactArgs(1),
				Run: func(ctx *clihelp.Context) error {
					if err := session.configOps().Get(session.Config, ctx.Args[0]); err != nil {
						return err
					}
					// An empty list printed an empty line and nothing else. The
					// note goes to stderr: a script reading the value gets the
					// value, not "(none)".
					if ctx.Args[0] == "my_emails" && len(session.Config.MyEmails) == 0 {
						fmt.Fprintln(session.errOut(), "(none) -- `feral config emails` lists every address that counts as yours")
					}
					return nil
				},
			},
			{
				Name:            "set",
				Title:           "config set <key> <value>",
				Description:     "Set a configuration parameter value.",
				LongDescription: "Set a configuration parameter to a new value. Keys that belong to an account are written to the account -A selects.",
				UsageLine:       "feral config set <key> <value>",
				Parameters: []clihelp.Param{
					{Name: "<key>", Description: "The configuration parameter name.", Complete: CompleteConfigKeys},
					{Name: "<value>", Description: "The value to set for the parameter."},
				},
				Examples: []clihelp.Example{
					{Line: "feral config set spam_folder Junk"},
					{Line: "feral config set limit 50"},
					{Line: "feral -A work config set badge 💼", Description: "Tag the work account in All Inboxes"},
					{Line: "feral config set mutt_alias_file ~/.mutt/aliases"},
					{Line: "feral config set llm_endpoint http://localhost:4000/v1/chat/completions"},
					{Line: "feral config set llm_api_key sk-litellm-master-key"},
					{Line: "feral config set llm_auto_summary true"},
					{Line: "feral config set attachment_save_dir ~/info/misc/%YY/%MM/%DD/"},
					{Line: "feral config set save_locations ~/Downloads,~/info/misc,~", Description: "Jumps offered by the save dialog"},
				},
				// Two arguments, exactly. RangeArgs(2, 3) existed for a third
				// positional that selected an account override and was never
				// read, so a stray word vanished without a word. The account is
				// chosen with -A, like everywhere else.
				Args: clihelp.ExactArgs(2),
				Run: func(ctx *clihelp.Context) error {
					args := ctx.Args
					return session.configOps().Set(session.Config, args[0], args[1])
				},
			},
			{
				Name:        "add-email",
				Title:       "config add-email <address>...",
				Description: "Add addresses that are your own.",
				LongDescription: "Add to my_emails, the global list of your own addresses; each account's user_email counts already. " +
					"In the TUI index a message from one of them shows its recipient (→ To) instead of its sender, " +
					"and a reply to it goes to that recipient. `config emails` lists every address that counts, `config remove-email` takes one out, " +
					"and `config unset my_emails` clears the list.",
				UsageLine: "feral config add-email <address>...",
				Parameters: []clihelp.Param{
					{Name: "<address>", Description: "An address of yours."},
				},
				Examples: []clihelp.Example{
					{Line: "feral config add-email me@old-job.edu me@example.org"},
					{Line: "feral config emails"},
				},
				Args: clihelp.MinimumNArgs(1),
				Run: func(ctx *clihelp.Context) error {
					merged := append(append([]string{}, session.Config.MyEmails...), ctx.Args...)
					return session.configOps().Set(session.Config, "my_emails", strings.Join(merged, ","))
				},
			},
			newConfigRemoveEmailCmd(session),
			newConfigEmailsCmd(session),
			{
				Name:        "unset",
				Aliases:     []string{"reset"},
				Title:       "config unset <key>",
				Description: "Reset a configuration parameter to default.",
				UsageLine:   "feral config unset <key>",
				Parameters: []clihelp.Param{
					{Name: "<key>", Description: "The configuration parameter name to reset.", Complete: CompleteConfigKeys},
				},
				Examples: []clihelp.Example{
					{Line: "feral config unset score_threshold"},
					{Line: "feral config unset mutt_alias_file"},
				},
				Args: clihelp.ExactArgs(1),
				Run: func(ctx *clihelp.Context) error {
					key := ctx.Args[0]
					return session.configOps().Reset(session.Config, key)
				},
			},
			{
				Name:            "check",
				Aliases:         []string{"validate"},
				Title:           "config check",
				Description:     "Validate config, DNS, and Bogofilter.",
				LongDescription: "Validate configurations, account parameters, DNS reachability, and Bogofilter service.",
				UsageLine:       "feral config check [--fix]",
				Args:            clihelp.NoArgs,
				Options: []clihelp.Option{
					clihelp.Bool(&flagConfigFix, "--fix", false,
						"Repair a received_folder set to \"inbox\" instead of only reporting it."),
				},
				Run: func(_ *clihelp.Context) error {
					// The repair used to ride on a persistent --fix, so every
					// command carried it and any of them could rewrite the
					// config file from Validate(). Only the command whose job is
					// repair sets it now.
					session.Config.FixConfig = flagConfigFix
					return session.configOps().Validate(session.Config)
				},
				Examples: []clihelp.Example{
					{Line: "feral config check"},
					{Line: "feral config check --fix", Description: "Repair what the check reports"},
				},
			},
		},
		Examples: []clihelp.Example{
			{Line: "feral config show"},
			{Line: "feral config get mutt_alias_file"},
			{Line: "feral config set mutt_alias_file ~/.mutt/aliases"},
			{Line: "feral config set spam_folder Junk"},
			{Line: "feral config unset score_threshold"},
			{Line: "feral config check"},
		},
	}
}
