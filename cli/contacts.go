package cli

import (
	"github.com/sarielhp/feral/contacts"
	"github.com/sarielhp/feral/contactsui"

	"github.com/sarielhp/clihelp"
)

// ContactsCmd returns the clihelp.Command for managing contacts and aliases.
func ContactsCmd(session *Session) clihelp.Command {
	listRun := func(ctx *clihelp.Context) error {
		return runContactsList(session, ctx.Args)
	}

	return clihelp.Command{
		Name:            "contacts",
		Group:           "Contacts",
		Description:     "Manage contacts and aliases.",
		LongDescription: "View and manage email contacts and aliases from Google and Mutt.",
		UsageLine:       "feral contacts [subcommand]",
		Run:             listRun,
		Subcommands: []clihelp.Command{
			contactsListSubcmd(session),
			contactsTUISubcmd(session),
			contactsPickSubcmd(session),
			contactsSearchSubcmd(session),
			contactsSuggestSubcmd(session),
			contactsHarvestSubcmd(session),
			contactsImportMuttSubcmd(session),
			contactsSyncSubcmd(session),
			contactsPathSubcmd(session),
		},
		Examples: []clihelp.Example{
			{Line: "feral contacts", Description: "List all contacts and aliases"},
			{Line: "feral contacts tui", Description: "Manage contacts full screen"},
			{Line: "feral contacts search algo", Description: "Search contacts and groups"},
			{Line: "feral contacts suggest alice", Description: "Suggest ranked email recipients"},
			{Line: "feral contacts harvest", Description: "Harvest suggested recipients from cached mail"},
			{Line: "feral contacts sync", Description: "Sync contacts from Google Contacts"},
			{Line: "feral contacts import-mutt", Description: "Import Mutt aliases configured in config.json"},
			{Line: "feral contacts import-mutt ~/.mutt/aliases", Description: "Import Mutt aliases from a specific file"},
		},
	}
}

func contactsListSubcmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "list",
		Title:           "contacts list [filter]",
		Description:     "List stored contacts and aliases.",
		LongDescription: "List stored contacts and aliases with an optional substring filter.",
		UsageLine:       "feral contacts list [filter]",
		Parameters: []clihelp.Param{
			{Name: "[filter]", Description: "Optional substring to filter by name, email, or alias."},
		},
		Examples: []clihelp.Example{
			{Line: "feral contacts list"},
			{Line: "feral contacts list john"},
		},
		Args: clihelp.RangeArgs(0, 1),
		Run: func(ctx *clihelp.Context) error {
			return runContactsList(session, ctx.Args)
		},
	}
}

// runContactsList lists stored contacts, applying an optional substring filter.
// It backs both the bare `contacts` command and `contacts list`, which are the
// same listing.
func runContactsList(session *Session, args []string) error {
	filter := ""
	if len(args) > 0 {
		filter = args[0]
	}
	if wantJSON() {
		path := contacts.DefaultPath(session.Config)
		book, err := contacts.LoadBook(path)
		if err != nil {
			return err
		}
		return session.emitJSON(contacts.Filter(book, filter))
	}
	return session.contacts().List(session.Config, filter)
}

func contactsSearchSubcmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "search",
		Title:           "contacts search <query>",
		Description:     "Search contacts by name, email, or note.",
		LongDescription: "Search contacts across names, emails, nicknames, groups, and notes.",
		UsageLine:       "feral contacts search <query>",
		Parameters: []clihelp.Param{
			{Name: "<query>", Description: "Search query words."},
		},
		Examples: []clihelp.Example{
			{Line: "feral contacts search algo"},
			{Line: "feral contacts search \"dave mount\""},
		},
		Args: clihelp.ExactArgs(1),
		Run: func(ctx *clihelp.Context) error {
			if wantJSON() {
				path := contacts.DefaultPath(session.Config)
				book, err := contacts.LoadBook(path)
				if err != nil {
					return err
				}
				return session.emitJSON(contacts.Search(book, ctx.Args[0]))
			}
			return session.contacts().Search(session.Config, ctx.Args[0])
		},
	}
}

func contactsImportMuttSubcmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "import-mutt",
		Title:           "contacts import-mutt [file_path]",
		Description:     "Import contacts from a Mutt alias file.",
		LongDescription: "Import contacts and distribution lists from a Mutt alias file.",
		UsageLine:       "feral contacts import-mutt [file_path]",
		Parameters: []clihelp.Param{
			{Name: "[file_path]", Description: "Mutt alias file (default: from config)"},
		},
		Examples: []clihelp.Example{
			{Line: "feral contacts import-mutt"},
			{Line: "feral contacts import-mutt ~/.mutt/aliases"},
		},
		Args: clihelp.RangeArgs(0, 1),
		Run: func(ctx *clihelp.Context) error {
			path := ""
			if len(ctx.Args) > 0 {
				path = ctx.Args[0]
			}
			return session.contacts().ImportMutt(session.Config, path)
		},
	}
}

func contactsSyncSubcmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "sync",
		Title:           "contacts sync [account]",
		Description:     "Sync contacts from Google People API.",
		LongDescription: "Fetch and update contacts from Google People API into local contacts.json.",
		UsageLine:       "feral contacts sync [account]",
		Parameters: []clihelp.Param{
			{Name: "[account]", Description: "Name of the Google account to sync (defaults to selected account).", Complete: CompleteAccountNames},
		},
		Examples: []clihelp.Example{
			{Line: "feral contacts sync"},
			{Line: "feral contacts sync work"},
		},
		Args: clihelp.RangeArgs(0, 1),
		Run: func(ctx *clihelp.Context) error {
			account := ""
			if len(ctx.Args) > 0 {
				account = ctx.Args[0]
			}
			return session.contacts().SyncGoogle(session.Config, account)
		},
	}
}

func contactsPathSubcmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "path",
		Title:       "contacts path",
		Description: "Print the file path to local contacts store.",
		UsageLine:   "feral contacts path",
		Args:        clihelp.NoArgs,
		Run: func(_ *clihelp.Context) error {
			return session.contacts().Path(session.Config)
		},
	}
}

func contactsSuggestSubcmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "suggest",
		Title:           "contacts suggest [query]",
		Description:     "Suggest ranked recipients from history.",
		LongDescription: "Suggest ranked email recipients combining curated contacts and interaction history.",
		UsageLine:       "feral contacts suggest [query]",
		Parameters: []clihelp.Param{
			{Name: "[query]", Description: "Optional prefix or search string to match recipients."},
		},
		Examples: []clihelp.Example{
			{Line: "feral contacts suggest"},
			{Line: "feral contacts suggest alice"},
			{Line: "feral contacts suggest algo"},
		},
		Args: clihelp.RangeArgs(0, 1),
		Run: func(ctx *clihelp.Context) error {
			query := ""
			if len(ctx.Args) > 0 {
				query = ctx.Args[0]
			}
			if wantJSON() {
				path := contacts.DefaultPath(session.Config)
				book, _ := contacts.LoadBook(path)
				historyPath := contacts.DefaultHistoryPath(session.Config)
				history, _ := contacts.LoadHistory(historyPath)
				var blacklist []string
				if session.Config != nil {
					blacklist = session.Config.Policy.Blacklist
				}
				results := contacts.Suggest(book, history, blacklist, query, 15)
				return session.emitJSON(results)
			}
			return session.contacts().Suggest(session.Config, query)
		},
	}
}

func contactsHarvestSubcmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "harvest",
		Title:           "contacts harvest",
		Description:     "Build suggestion index from cached mail.",
		LongDescription: "Scan cached sent and archived messages across all accounts to build the recipient suggestion index.",
		UsageLine:       "feral contacts harvest",
		Args:            clihelp.NoArgs,
		Examples: []clihelp.Example{
			{Line: "feral contacts harvest"},
		},
		Run: func(_ *clihelp.Context) error {
			return session.contacts().Harvest(session.Config)
		},
	}
}

var (
	flagPickTitle     string
	flagPickQuery     string
	flagPickPreselect []string
	flagPickSingle    bool
	flagPickFormat    string
	flagPickExpand    bool
)

func contactsPickSubcmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "pick",
		Title:       "contacts pick",
		Description: "Choose contacts full screen; print them on stdout.",
		LongDescription: "Open a full-screen contacts picker on the terminal and print the chosen contacts on stdout, " +
			"for other programs to use (a calendar invite, a script). The list is ranked as Tab-completion ranks it: " +
			"stored contacts and aliases, then the addresses harvested from mail. Space marks, Enter chooses the " +
			"marked contacts (or the one under the cursor), Esc cancels. The picker draws on /dev/tty, so stdin and " +
			"stdout may be pipes. Exit status: 0 with the choice on stdout, 130 when cancelled or nothing was chosen " +
			"(no output), 2 for a bad command line, 1 for any other failure.",
		UsageLine: "feral contacts pick [flags]",
		Args:      clihelp.NoArgs,
		Options: []clihelp.Option{
			clihelp.String(&flagPickFormat, "-f, --format <fmt>", "rfc", "Output: rfc (Name <addr>), addr, or json."),
			clihelp.Bool(&flagPickSingle, "-1, --single", false, "Choose one contact; no marking."),
			clihelp.String(&flagPickQuery, "-q, --query <text>", "", "Start with this search."),
			clihelp.StringSlice(&flagPickPreselect, "-p, --preselect <addr>", nil, "Start with these addresses marked (repeatable)."),
			clihelp.Bool(&flagPickExpand, "-g, --expand-groups", false, "In json, list a group's members in its place."),
			clihelp.String(&flagPickTitle, "--title <text>", "", "Title shown above the list."),
		},
		Examples: []clihelp.Example{
			{Line: "feral contacts pick", Description: "Pick contacts; print Name <addr> per line"},
			{Line: "feral contacts pick --format=json", Description: "Print the choice as JSON for a program"},
			{Line: `feral contacts pick -p "a@x.org,b@y.org" --title "Invitees"`, Description: "Edit an existing list of invitees"},
			{Line: "feral contacts pick -1 -f addr", Description: "Choose one address"},
		},
		Run: func(_ *clihelp.Context) error {
			if err := contactsui.CheckFormat(flagPickFormat); err != nil {
				return err
			}
			return session.contacts().Pick(session.Config, PickOptions{
				Title:        flagPickTitle,
				Query:        flagPickQuery,
				Preselect:    flagPickPreselect,
				Single:       flagPickSingle,
				Format:       flagPickFormat,
				ExpandGroups: flagPickExpand,
			})
		},
	}
}

func contactsTUISubcmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "tui",
		Title:       "contacts tui",
		Description: "Manage contacts full screen.",
		LongDescription: "Open the full-screen contacts manager: search, view and edit contacts, add people and groups, " +
			"merge duplicates, hide imported contacts, and turn harvested addresses into contacts. Edits to a contact " +
			"imported from Google or mutt are kept beside what the source says and survive every re-sync; " +
			"`r` in the detail view reverts to the source. Press ? inside for every key. Under --read-only nothing " +
			"is saved; under --dry-run each change is checked and reported but not saved.",
		UsageLine: "feral contacts tui",
		Args:      clihelp.NoArgs,
		Examples: []clihelp.Example{
			{Line: "feral contacts tui"},
		},
		Run: func(_ *clihelp.Context) error {
			return session.contacts().Manage(session.Config)
		},
	}
}
