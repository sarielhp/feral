package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/sarielhp/feral/app"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/outlookids"

	"github.com/sarielhp/clihelp"
)

func newCacheOutlookIDsCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "outlook-ids",
		Description: "Key an Outlook cache by stable message IDs.",
		LongDescription: "Outlook gives a message a new ID whenever it moves to another folder, so archiving or " +
			"filing one orphaned what feral keeps by ID: the replied mark, your spam/not-spam decision, the " +
			"cached copy. Graph also offers IDs that survive a move. A new Outlook account uses them from the " +
			"start; this command moves an existing account's cache over. It asks the server to translate every " +
			"cached ID, builds a new cache beside the old one -- message copies with your decisions and LLM " +
			"summaries, read, replied and pending marks, training ledgers, saved result sets -- and swaps it in. " +
			"The old cache is kept, renamed with a .pre-immutable-<time> suffix, and can be deleted once the " +
			"account looks right. Folder indexes are rebuilt by the next listing. It changes only local files -- the " +
			"server is only asked to translate. Close every feral TUI and `feral watch` first: it refuses while a " +
			"watcher runs, but a TUI started with --no-watch holds no lock, and would go on writing the old IDs into the new cache.",
		UsageLine: "feral -A <account> utils cache outlook-ids",
		Notes: []clihelp.Note{
			{Heading: "Safety", Text: "Refused under --dry-run and --read-only. If the server translates none of the IDs, nothing is changed."},
			{Heading: "Interrupted runs", Text: "Run it again: a finished new cache is swapped in, an unfinished one is rebuilt."},
			{Heading: "Older builds", Text: "A feral from before this command does not know the new IDs and writes the old ones into a migrated cache. Install this build everywhere the account is used before migrating it."},
			{Heading: "Undoing it", Text: "Move the account's cache directory away and rename <account>.pre-immutable-<time> back to <account>."},
		},
		Examples: []clihelp.Example{
			{Line: "feral -A work utils cache outlook-ids", Description: "Migrate the account named work"},
		},
		Args: clihelp.NoArgs,
		Run: func(*clihelp.Context) error {
			return runCacheOutlookIDs(session)
		},
	}
}

func runCacheOutlookIDs(session *Session) error {
	if err := refuseLocalWrite(session, "utils cache outlook-ids", true); err != nil {
		return err
	}
	acc, err := cfg_g.Lookup(session.Config.Accounts, string(session.Config.SelectedAccount))
	if err != nil {
		return fmt.Errorf("%w: name the account with -A: %v", app.ErrUsage, err)
	}
	rep, err := session.accounts().MigrateOutlookIDs(context.Background(), session.Config, *acc)
	if err != nil {
		return fmt.Errorf("migrating %s: %w", acc.Name, err)
	}
	printOutlookIDsReport(session.out(), acc.Name, rep)
	return nil
}

func printOutlookIDsReport(w io.Writer, name string, rep outlookids.Report) {
	switch {
	case rep.AlreadyMigrated:
		fmt.Fprintf(w, "%s already uses stable message IDs.\n", name)
		return
	case rep.Empty:
		fmt.Fprintf(w, "%s had nothing cached; it now uses stable message IDs.\n", name)
		return
	}
	fmt.Fprintf(w, "%s now uses stable message IDs.\n", name)
	fmt.Fprintf(w, "  %d cached ID(s), %d translated by the server\n", rep.IDs, rep.Translated)
	fmt.Fprintf(w, "  %d message(s) carried, %d with your spam/not-spam decision; %d dropped (gone from the server)\n",
		rep.Messages.Carried, rep.Messages.Decided, rep.Messages.Dropped)
	fmt.Fprintf(w, "  %d read/replied/pending mark(s) carried, %d dropped\n", rep.FlagsCarried, rep.FlagsDropped)
	fmt.Fprintf(w, "  spam training ledger %d -> %d, ham ledger %d -> %d\n",
		rep.SpamLedger.Before, rep.SpamLedger.After, rep.HamLedger.Before, rep.HamLedger.After)
	fmt.Fprintf(w, "  %d saved result set(s) carried\n", rep.SavedSets)
	if rep.WatcherErr != nil {
		fmt.Fprintf(w, "  the watcher's seen list could not be updated (%v): its next poll may announce the inbox once\n", rep.WatcherErr)
	}
	fmt.Fprintf(w, "The old cache is at %s; delete it once %s looks right.\n", rep.Aside, name)
}
