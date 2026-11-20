package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/sarielhp/feral/app"
	"github.com/sarielhp/feral/cache/meta"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/scan"

	"github.com/sarielhp/clihelp"
)

var exitFunc = os.Exit

func cliExamples() []clihelp.Example {
	// The root's own examples, which -E leads with and the front page shows.
	// Six is about the ceiling before a list stops being a cheat sheet, so these
	// are the things worth typing daily rather than a tour of the tree.
	return []clihelp.Example{
		{Line: "feral %", Description: "Scan the inbox -- a label is written %name, and % alone is the inbox"},
		{Line: "feral %work:receipts", Description: "Scan one folder on one account"},
		{Line: "feral tui", Description: "Browse mail interactively"},
		{Line: "feral label sort %receipts", Description: "File a folder into the dated keep/ hierarchy"},
		{Line: "feral spam sweep", Description: "Move spam out of the inbox and train the filter"},
		{Line: "feral -E <command>", Description: "Show the examples for one command"},
	}
}

func cliPersistentOptions() []clihelp.Option {
	return append(accountOptions(&flagAccount),
		clihelp.Group("Execution Modes", clihelp.Bool(&app.FlagReadOnly, "--read-only", false, "Block server writes (cache still updates)")),
		clihelp.Group("Execution Modes", clihelp.Bool(&app.FlagDryRun, "-D, --dry-run", false, "Report changes only; implies --read-only")),
		clihelp.Group("Logging & Output", clihelp.Bool(&app.FlagVerbose, "-v, --verbose", false, "Enable verbose output")),
		clihelp.Group("Logging & Output", clihelp.Bool(&app.FlagJSON, "--json", false, "Emit result as JSON on stdout")),
		clihelp.Group("Logging & Output", clihelp.Bool(&app.FlagLogAppend, "--logappend", false, "Append to existing logs instead of truncating")),
	)
}

func cliBeforeRun(session *Session) func(ctx *clihelp.Context) error {
	return func(ctx *clihelp.Context) error {
		if app.FlagVersion {
			var out io.Writer = os.Stdout
			if ctx != nil && ctx.Stdout != nil {
				out = ctx.Stdout
			}
			fmt.Fprintf(out, "feral version %s\n", app.Version)
			exitFunc(0)
			return nil
		}
		if session.Bootstrap != nil {
			config, err := session.Bootstrap(ctx.Context, StartupOptions{
				NoConfigWrite: app.FlagReadOnly || app.FlagDryRun,
				AppendLogs:    app.FlagLogAppend,
				FinishRename:  ctx != nil && ctx.Command != nil && ctx.Command.UsageLine == accountRenameUsage,
			})
			if err != nil {
				return fmt.Errorf("initialize CLI session: %w", err)
			}
			session.Config = config
		}
		restoreSetAsideTokens(session)
		app.CmdExecutionStarted = true
		if ctx != nil && ctx.Stdout != nil {
			session.Out = ctx.Stdout
		}
		if ctx != nil && ctx.App != nil && ctx.App.Stderr != nil {
			session.Err = ctx.App.Stderr
		}
		session.Config.Verbose = app.FlagVerbose
		// --dry-run implies --read-only: both forbid server mutations, which
		// the read-only client decorator simulates. Caching still happens under
		// either, because without downloaded bodies a message cannot be shown;
		// --dry-run additionally makes commands report instead of act where
		// they check Config.DryRun.
		if app.FlagReadOnly || app.FlagDryRun {
			session.Config.ReadOnly = true
		}
		session.Config.DryRun = app.FlagDryRun
		cfg_g.ForbidConfigWrites(app.FlagDryRun)
		if err := openDirManifests(session.Config); err != nil {
			return err
		}
		invoked := ""
		if ctx != nil && ctx.App != nil {
			invoked = ctx.App.InvokedName()
		}
		if err := refuseStrayAccountName(session.Config, invoked); err != nil {
			return err
		}
		return selectCLIAccount(session.Config, flagAccount)
	}
}

// cliRootRun handles `feral`, `feral %label`, and anything else that
// reached the root.
//
// A label reference is unambiguous -- only a label can start with the sigil --
// so naming one is taken as a request to scan it: `feral %` is the inbox,
// `feral %work:receipts` is that folder on that account. Nothing else at the
// root does anything, and a zero-argument invocation still prints help, because
// it once ran a scan across *every* configured account.
func cliRootRun(session *Session) func(ctx *clihelp.Context) error {
	return func(ctx *clihelp.Context) error {
		var out io.Writer = os.Stdout
		if ctx.App != nil && ctx.App.Stdout != nil {
			out = ctx.App.Stdout
		}

		if len(ctx.Args) == 0 {
			// A first run has no usable account, so the help page is a dead
			// end -- offer the wizard instead. Only at a terminal, because the
			// wizard prompts, and only for the bare invocation; an explicit
			// command keeps its own behavior.
			if cfg_g.NeedsOnboarding(session.Config) && isTerminal(session.stdin()) {
				return runSetup(session)
			}
			ctx.App.RenderGlobal(clihelp.Options{Writer: out})
			fmt.Fprintf(out, "\nNote: running with no arguments does not scan. Use '%s%s' for the inbox.\n",
				"feral ", cfg_g.LabelSigil)
			return nil
		}

		arg := ctx.Args[0]
		if !strings.HasPrefix(arg, cfg_g.LabelSigil) {
			return fmt.Errorf("%w: unknown command %q for %q. Run %q for a list of commands (a label is written %sname)",
				clihelp.ErrUsage, arg, app.AppName, app.AppName+" -h", cfg_g.LabelSigil)
		}
		return scanLabelRef(session, arg)
	}
}

// scanLabelRef runs a scan over the folder a reference names.
func scanLabelRef(session *Session, ref string) error {
	if err := session.clients().PreCheck(session.Config); err != nil {
		return err
	}

	parsed, err := cfg_g.ParseLabelRef(ref)
	if err != nil {
		return err
	}

	opts := scan.Options{
		Pattern:       flagScanPattern,
		ExplicitInbox: strings.EqualFold(parsed.Label, cfg_g.DefaultLabel),
		MoveSpam:      flagMoveSpam,
		MoveFrom:      flagMoveSpamFrom,
		MoveInbox:     flagMoveInboxStr,
		JSON:          wantJSON(),
	}
	moved, err := session.mail().Scan(opts, session.Config, ref)
	if err != nil {
		return err
	}
	if moved > 0 && !wantJSON() {
		fmt.Fprintf(session.out(), "\nTotal emails moved during scan: %s\n",
			app.ColorBoldYellow.Sprint(moved))
	}
	return nil
}

func versionCmd() clihelp.Command {
	return clihelp.Command{
		Name:        "version",
		Description: "Print version number and exit",
		Hidden:      true,
		Args:        clihelp.NoArgs,
		Run: func(ctx *clihelp.Context) error {
			var out io.Writer = os.Stdout
			if ctx != nil && ctx.Stdout != nil {
				out = ctx.Stdout
			}
			fmt.Fprintf(out, "feral version %s\n", app.Version)
			return nil
		},
	}
}

func cliCommands(session *Session) []clihelp.Command {
	// Keep daily entry points direct; group other actions by the thing they manage.
	// Two headings keep every command visible within concise help's 24 lines.
	commands := []clihelp.Command{
		ScanCmd(session),
		SearchCmd(session),
		ComposeCmd(session),
		TuiCmd(session),
		WatchCmd(session),
		MsgCmd(session),
		LabelsCmd(session),
		MboxCmd(session),
		SpamCmd(session),
		RuleCmd(session),
		CalendarCmd(session),
		ContactsCmd(session),
		buildAccountCommands(session),
		LinkCmd(session),
		buildConfigCommands(session),
		buildUtilsCommands(session),
	}
	for i := range commands {
		commands[i].Group = "Daily"
		if i >= 4 {
			commands[i].Group = "Manage"
		}
		// A moved root command's former heading has no meaning inside its parent.
		for j := range commands[i].Subcommands {
			commands[i].Subcommands[j].Group = ""
		}
	}
	return append(commands, legacyCommands(session)...)
}

// legacyCommands preserves scripts and muscle memory without crowding help or
// completion. Each spelling uses the same handler and flags as its new path.
func legacyCommands(session *Session) []clihelp.Command {
	compose := ComposeCmd(session)
	compose.Name = "compose"
	utils := buildUtilsCommands(session)
	utils.Name = "tools"
	commands := []clihelp.Command{
		legacyCommand(compose, "write"),
		legacyCommand(utils, "utils"),
		legacyCommand(LastCmd(session), "msg last"),
		legacyCommand(SendCmd(session), "msg send"),
		legacyCommand(ShowCmd(session), "msg show"),
		legacyCommand(DraftsCmd(session), "msg drafts"),
		legacyCommand(ReplyCmd(session), "msg reply"),
		legacyCommand(ExportCmd(session), "msg export"),
		legacyCommand(SetupCmd(session), "account setup"),
		legacyCommand(MigrateCmd(session), "account migrate"),
		legacyCommand(LLMCmd(session), "config llm"),
		legacyCommand(CacheCmd(session), "utils cache"),
		legacyCommand(ColorCmd(session), "utils color"),
		legacyCommand(DoctorCmd(session), "utils doctor"),
		clihelp.CompletionCommand(),
		clihelp.ManPageCommand(),
		ScanSpamCmd(session),
		SpamSweepShortcutCmd(session),
		WlistShortcutCmd(session),
		blistShortcutCmd(session),
		ArcCmd(session),
		versionCmd(),
	}
	for i := range commands {
		commands[i].Hidden = true
	}
	return commands
}

// InitCLI initializes and returns the *clihelp.App with all subcommands and global options.
func InitCLI(session *Session) *clihelp.App {
	configPath := filepath.Join("~", ".config", app.AppName, "config.json")

	githubURL := app.GitHubURL
	if session != nil && session.Config != nil && session.Config.GitHubURL != "" {
		githubURL = session.Config.GitHubURL
	}
	var globalNote string
	if githubURL != "" {
		globalNote = fmt.Sprintf("GitHub: [%s](%s)",
			strings.TrimPrefix(githubURL, "https://github.com/"), githubURL)
	}
	// -H is where the joke goes; -h stays a plain statement of what this is.
	globalNote = strings.TrimSpace("A wild dog to take care of your email. What could possibly go wrong?\n\n" +
		"Not much: --dry-run shows what any command would do, --read-only never touches the server, " +
		"spam is moved to a folder rather than deleted, and emptying that folder asks first.\n\n" + globalNote)

	return &clihelp.App{
		Name:        app.AppName,
		Description: nameMark() + " -- a terminal mail client for Linux: Gmail, Outlook, Fastmail.",
		Version:     app.Version,
		ConfigPath:  configPath,
		// The label, not the URL: on a terminal that understands OSC 8 this is a
		// clickable repo link, and where it is not, the reader sees a
		// name rather than a line of address. Writing the URL as its own label
		// linked correctly but showed the address twice over.
		GlobalNote:                globalNote,
		ExtendedHelpFlag:          true,
		AbbrevCommands:            true,
		OmitGlobalFlagsInCommands: true,
		AutoRefreshIntegration:    true,
		EnableExamplesFlag:        true,
		PositionalFilter: func(arg string) bool {
			return strings.HasPrefix(arg, cfg_g.LabelSigil)
		},
		Examples:          cliExamples(),
		PersistentOptions: cliPersistentOptions(),
		// The bare form `feral %inbox` runs scan, so it takes scan's flags.
		// App.Options binds them on the root alone and does not hand them to
		// every subcommand, which is what made them persistent before; the
		// shared scanOptions keeps this scope and `scan` from drifting.
		Options: scanOptions(),
		GlobalFlags: []clihelp.Option{
			clihelp.Bool(&app.FlagVersion, "--version", false, "Print the version number and exit"),
		},
		Args:      clihelp.MaximumNArgs(1),
		BeforeRun: cliBeforeRun(session),
		Run:       cliRootRun(session),
		Commands:  cliCommands(session),
	}
}

// openDirManifests is the forward guard for the download directory and the
// configuration directory (see cache/meta): a run refuses one a newer build
// reshaped. The download directory is a cache, so its manifest is recorded
// under --read-only too; the configuration directory is never written under
// --read-only or --dry-run, as config.json is not.
func openDirManifests(config *cfg_g.Config) error {
	if config == nil {
		return nil
	}
	if err := meta.OpenDir(config.CacheRoot, meta.DownloadDirFile, app.Version, !config.DryRun); err != nil {
		return err
	}
	return meta.OpenDir(config.ConfigDir, meta.ConfigDirFile, app.Version, !config.DryRun && !config.ReadOnly)
}

// nameMark is the program's name as its help heads a line with it. The dog is
// printed whatever the locale: the help pages under docs/ are generated from
// this text, and a mark that depended on LANG would make them differ from one
// machine to the next.
func nameMark() string { return app.AppName + " \U0001F415" }
