package cli

import (
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/sarielhp/clihelp"

	"github.com/sarielhp/feral/app"
	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
)

// A link is another name for feral: `mt` runs `feral -A fastmail tui`. The
// list is config.json's "links" (cfg_g.Link); clihelp expands a link when
// the binary is started under its name, completes it, and registers it with
// the shell. This file is the `link` command group that manages the list and
// the symlinks; links_fs.go places and inspects the symlinks themselves.

// LinkCmd returns the `link` command group.
func LinkCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "link",
		Description: "Manage other names that run feral.",
		LongDescription: "A link is another name for feral that runs it with arguments put first: with " +
			"`feral link add mt -- -A fastmail tui`, typing `mt` runs `feral -A fastmail tui`, and `mt -2` runs " +
			"it on account 2, because the last account named on a line wins. A link is a symlink to the feral " +
			"binary plus an entry under \"links\" in config.json; Tab completes it and Alt-H explains it like feral " +
			"itself, once `feral completion install` has set up the shell.",
		UsageLine: "feral link <subcommand> [args...]",
		Notes: []clihelp.Note{
			{Heading: "Aliases", Text: "An account's aliases[] used to select it when feral was started under " +
				"that name. The first run that may write turns each such alias into a link of the same name; " +
				"after that an alias is only a name -A accepts. A symlink named after an account that is not a " +
				"link is refused rather than run against the default account."},
		},
		Subcommands: []clihelp.Command{
			linkAddCmd(session),
			linkListCmd(session),
			linkRmCmd(session),
		},
		Examples: []clihelp.Example{
			{Line: "feral link add mt -- -A fastmail tui", Description: "`mt` opens the TUI on fastmail"},
			{Line: "feral link list"},
			{Line: "feral link rm mt"},
		},
	}
}

func linkAddCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "add",
		Title:       "link add <name> [-- <args>...]",
		Description: "Add a name that runs feral with arguments.",
		LongDescription: "Record a link in config.json and put a symlink of that name beside the feral binary " +
			"(or in --dir). The arguments come after `--`, so that their flags are the link's and not this " +
			"command's; with no flags among them the `--` may be left out. They are stored as they will run: " +
			"the account named by -A, a display name or -N is stored by its name, and a command must be " +
			"spelled in full.",
		UsageLine: "feral link add <name> [--description T] [--force] [--dir D] -- <args...>",
		Parameters: []clihelp.Param{
			{Name: "<name>", Description: "The name to type: letters, digits, - _ . or +."},
			{Name: "[args...]", Description: "What it runs, as typed after feral."},
		},
		Options: []clihelp.Option{
			clihelp.String(&flagLinkDescription, "--description <text>", "", "What the link is for, for link list."),
			clihelp.Bool(&flagLinkForce, "--force", false, "Replace a link or file of the same name."),
			clihelp.String(&flagLinkDir, "--dir <dir>", "", "Put the symlink here, not beside feral."),
		},
		Notes: []clihelp.Note{
			{Heading: "Symlink", Text: "The symlink points at the running binary. An existing symlink to it is " +
				"kept; any other file of that name -- a wrapper script, a symlink elsewhere -- is replaced only " +
				"with --force, and a directory never. A directory not on $PATH gets the link and a warning."},
			{Heading: "Dry run", Text: "--dry-run says what it would do and writes nothing."},
		},
		Args: clihelp.MinimumNArgs(1),
		Run: func(ctx *clihelp.Context) error {
			// Read the flags now: canonicalLink walks the preset, and the
			// walk binds options -- these among them, when the preset names
			// a link command -- which resets their variables.
			opts := linkAddOptions{description: flagLinkDescription, force: flagLinkForce, dir: flagLinkDir}
			return runLinkAdd(session, ctx.App, ctx.Args[0], ctx.Args[1:], opts)
		},
		Examples: []clihelp.Example{
			{Line: "feral link add mt -- -A fastmail tui"},
			{Line: "feral link add work -- -A work"},
			{Line: "feral link add mail_cli", Description: "Make the old name complete"},
		},
	}
}

func linkListCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "list",
		Aliases:     []string{"ls"},
		Description: "List link names and what each one runs.",
		LongDescription: "Each link with what it runs and its state: ok; no symlink; symlink elsewhere (a file " +
			"of that name that is not this feral); not on PATH (the symlink is there, but typing the name does " +
			"not reach it); account missing (its account was removed -- running it fails until an account of " +
			"that name exists again).",
		UsageLine: "feral link list",
		Args:      clihelp.NoArgs,
		Run: func(_ *clihelp.Context) error {
			return runLinkList(session)
		},
		Examples: []clihelp.Example{
			{Line: "feral link list"},
			{Line: "feral --json link list"},
		},
	}
}

func linkRmCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "rm",
		NoAbbrev:    true,
		Title:       "link rm <name>",
		Description: "Remove a link and its symlink.",
		LongDescription: "Remove the link from config.json, and its symlink beside the feral binary (or in " +
			"--dir) if that symlink is this feral; any other file of that name is left alone and reported.",
		UsageLine: "feral link rm <name> [--dir D]",
		Parameters: []clihelp.Param{
			{Name: "<name>", Description: "The link to remove.", Complete: completeLinkNames},
		},
		Options: []clihelp.Option{
			clihelp.String(&flagLinkDir, "--dir <dir>", "", "Look for the symlink here, not beside feral."),
		},
		Args: clihelp.ExactArgs(1),
		Run: func(ctx *clihelp.Context) error {
			return runLinkRm(session, ctx.App, ctx.Args[0], flagLinkDir)
		},
		Examples: []clihelp.Example{
			{Line: "feral link rm mt"},
		},
	}
}

// completeLinkNames completes the declared link names.
func completeLinkNames(toComplete string) []string {
	configDir, err := cfg_g.UserConfigDir()
	if err != nil {
		return nil
	}
	links, _, err := cfg_g.ReadLinks(configDir)
	if err != nil {
		return nil
	}
	var out []string
	for _, name := range cfg_g.LinkNames(links) {
		if strings.HasPrefix(name, toComplete) {
			out = append(out, name+"\t"+linkCommandLine(links[name]))
		}
	}
	return out
}

// linkCommandLine is what a link runs, as one line: "feral -A fastmail tui".
func linkCommandLine(l cfg_g.Link) string {
	words := []string{app.AppName}
	for _, w := range l.Preset() {
		words = append(words, shellWord(w))
	}
	return strings.Join(words, " ")
}

// shellWord quotes w for a shell when it needs it.
func shellWord(w string) string {
	if w != "" && !strings.ContainsAny(w, " \t\n'\"\\$`*?[]{}()<>|&;#~!") {
		return w
	}
	return "'" + strings.ReplaceAll(w, "'", `'\''`) + "'"
}

// canonicalLink is the link a typed preset stores: the account taken out into
// its own field and named by its canonical name, the preset otherwise as
// typed, and the whole checked the way clihelp will run it.
func canonicalLink(a *clihelp.App, accounts []cfg_acc.AccountConfig, name string, preset []string) (cfg_g.Link, error) {
	p, err := splitPresetAccount(a, preset)
	if err != nil {
		return cfg_g.Link{}, err
	}
	account, err := presetAccountName(accounts, p.choice)
	if err != nil {
		return cfg_g.Link{}, err
	}
	link := cfg_g.Link{Account: account}
	if len(p.rest) > 0 {
		link.Args = p.rest
	}
	if !slices.Equal(p.full, p.rest) {
		full := cfg_g.Link{Account: account, Args: p.full}
		return cfg_g.Link{}, fmt.Errorf("%w: a link must spell its command in full, or a later command could "+
			"change what it runs: feral link add %s -- %s", clihelp.ErrUsage, name,
			strings.TrimPrefix(linkCommandLine(full), app.AppName+" "))
	}
	warnings, err := a.ValidateInvocation(clihelp.Invocation{Name: name, Args: link.Preset()})
	if err != nil {
		return cfg_g.Link{}, fmt.Errorf("%w: %w", clihelp.ErrUsage, err)
	}
	if len(warnings) > 0 {
		return cfg_g.Link{}, fmt.Errorf("%w: %s", clihelp.ErrUsage, warnings[0])
	}
	return link, nil
}

// presetAccountName is the canonical name of the account a preset chose: an
// index is stored as the account at that index now, since indexes shift when
// an account is removed, and a display name, alias or type as the name.
func presetAccountName(accounts []cfg_acc.AccountConfig, choice accountChoice) (string, error) {
	if !choice.set() {
		return "", nil
	}
	if choice.byIndex {
		idx, err := strconv.Atoi(choice.value)
		if err != nil || idx <= 0 || idx > len(accounts) {
			return "", fmt.Errorf("%w: account index %q is out of range; you have %d configured account(s)",
				clihelp.ErrUsage, choice.value, len(accounts))
		}
		return accounts[idx-1].Name, nil
	}
	acc, err := cfg_g.Lookup(accounts, choice.value)
	if err != nil {
		return "", err
	}
	return acc.Name, nil
}

// linkAddOptions are link add's flags, read before anything walks a preset.
type linkAddOptions struct {
	description string
	force       bool
	dir         string
}

func runLinkAdd(session *Session, a *clihelp.App, name string, preset []string, opts linkAddOptions) error {
	if err := cfg_g.ValidateLinkName(name); err != nil {
		return fmt.Errorf("%w: %w", clihelp.ErrUsage, err)
	}
	configPath, fc, err := loadFileConfig(session)
	if err != nil {
		return err
	}
	link, err := canonicalLink(a, *fc.Accounts, name, preset)
	if err != nil {
		return err
	}
	link.Description = opts.description
	// A config.json the aliases migration has not reached (one only ever
	// loaded under --read-only) is migrated first: the key appearing without
	// it would end program-name selection for every alias at once.
	cfg_g.MigrateAliasesToLinks(fc)
	if _, exists := (*fc.Links)[name]; exists && !opts.force {
		return fmt.Errorf("%w: link %q exists; --force replaces it", clihelp.ErrUsage, name)
	}
	place, err := linkPlacement(opts.dir, name)
	if err != nil {
		return err
	}
	if err := place.check(opts.force); err != nil {
		return err
	}
	if session.Config.DryRun {
		fmt.Fprintf(session.out(), "Would add link %q: runs %s (symlink %s)\n", name, linkCommandLine(link), place.path)
		return nil
	}
	if err := place.create(); err != nil {
		return err
	}
	(*fc.Links)[name] = link
	if err := cfg_g.SaveConfigFile(configPath, fc); err != nil {
		return err
	}
	session.Config.Links = *fc.Links
	fmt.Fprintf(session.errOut(), "%s %s runs: %s (symlink %s)\n", app.PrefixSuccess, name, linkCommandLine(link), place.path)
	if !dirOnPath(place.dir) {
		fmt.Fprintf(session.errOut(), "%s %s is not on $PATH, so %q is not a command yet.\n", app.PrefixWarn, place.dir, name)
	}
	refreshLinkIntegration(session, a, place.dir)
	return nil
}

// linkRow is one link as `link list` shows it.
type linkRow struct {
	Name        string `json:"name"`
	Runs        string `json:"runs"`
	State       string `json:"state"`
	Description string `json:"description,omitempty"`
}

func runLinkList(session *Session) error {
	links := session.Config.Links
	exe, _ := runningBinary()
	rows := make([]linkRow, 0, len(links))
	for _, name := range cfg_g.LinkNames(links) {
		l := links[name]
		rows = append(rows, linkRow{
			Name:        name,
			Runs:        linkCommandLine(l),
			State:       linkState(name, l, session.Config.Accounts, exe),
			Description: l.Description,
		})
	}
	if wantJSON() {
		return writeJSONList(session, rows)
	}
	if len(rows) == 0 {
		fmt.Fprintf(session.errOut(), "%s No links. Add one: feral link add <name> -- <args>\n", app.PrefixInfo)
		return nil
	}
	width := 0
	for _, r := range rows {
		width = max(width, len(r.Name))
	}
	for _, r := range rows {
		fmt.Fprintf(session.out(), "%-*s  %-17s  %s\n", width, r.Name, r.State, r.Runs)
	}
	return nil
}

// linkState is a link's state for `link list`.
func linkState(name string, l cfg_g.Link, accounts []cfg_acc.AccountConfig, exe string) string {
	if l.Account != "" && !slices.ContainsFunc(accounts, func(acc cfg_acc.AccountConfig) bool {
		return strings.EqualFold(acc.Name, l.Account)
	}) {
		return "account missing"
	}
	return symlinkState(name, exe, filepath.Dir(exe))
}

func runLinkRm(session *Session, a *clihelp.App, name, dir string) error {
	configPath, fc, err := loadFileConfig(session)
	if err != nil {
		return err
	}
	cfg_g.MigrateAliasesToLinks(fc)
	if _, ok := (*fc.Links)[name]; !ok {
		return fmt.Errorf("%w: no link named %q; `feral link list` lists them", clihelp.ErrUsage, name)
	}
	place, placeErr := linkPlacement(dir, name)
	if session.Config.DryRun {
		fmt.Fprintf(session.out(), "Would remove link %q from config.json\n", name)
		return nil
	}
	delete(*fc.Links, name)
	if err := cfg_g.SaveConfigFile(configPath, fc); err != nil {
		return err
	}
	session.Config.Links = *fc.Links
	fmt.Fprintf(session.errOut(), "%s Removed link %q.\n", app.PrefixSuccess, name)
	switch {
	case placeErr != nil:
		fmt.Fprintf(session.errOut(), "%s Its symlink was not looked for: %v\n", app.PrefixWarn, placeErr)
	default:
		reportSymlinkRemoval(session, place)
	}
	refreshLinkIntegration(session, a, place.dir)
	return nil
}

// reportSymlinkRemoval removes a link's symlink if it is this feral, and says
// what it did.
func reportSymlinkRemoval(session *Session, place linkPlace) {
	removed, err := place.remove()
	switch {
	case err != nil:
		fmt.Fprintf(session.errOut(), "%s %s was left: %v\n", app.PrefixWarn, place.path, err)
	case removed:
		fmt.Fprintf(session.errOut(), "%s Removed the symlink %s.\n", app.PrefixSuccess, place.path)
	}
}

// refreshLinkIntegration rewrites the installed shell integration so that the
// new list completes in the next shell, and says why a name does not.
func refreshLinkIntegration(session *Session, a *clihelp.App, dir string) {
	// Without Invocations -- a test's App -- a refresh would register no
	// names at all, taking them from a file that may be the user's.
	if a == nil || a.Invocations == nil {
		return
	}
	unregistered, err := clihelp.RefreshIntegration(a)
	if err != nil {
		fmt.Fprintf(session.errOut(), "%s Shell integration not refreshed: %v\n", app.PrefixWarn, err)
		return
	}
	if !integrationInstalled(a) {
		fmt.Fprintf(session.errOut(), "%s No shell integration is installed, so links do not complete: "+
			"run `feral completion install`.\n", app.PrefixInfo)
		return
	}
	for _, name := range unregistered {
		reason := "typing it does not reach this feral"
		if !dirOnPath(dir) {
			reason = dir + " is not on $PATH"
		}
		fmt.Fprintf(session.errOut(), "%s %q does not complete yet: %s.\n", app.PrefixInfo, name, reason)
	}
}
