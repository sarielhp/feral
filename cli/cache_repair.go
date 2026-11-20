package cli

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/sarielhp/feral/app"
	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/cache/meta"
	"github.com/sarielhp/feral/cache/msg"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/train"

	"github.com/sarielhp/clihelp"
)

// runCacheRepair checks the selected account's index against its message
// files and the training ledgers, and rewrites the index unless --dry-run or
// --read-only asked for a report. It touches only local files.
func runCacheRepair(session *Session) error {
	account := resolveAccountCacheDir(session)
	if account.Path() == "" {
		return fmt.Errorf("%w: no account selected", app.ErrUsage)
	}
	fix := !session.Config.DryRun && !session.Config.ReadOnly
	rep, err := msg.Repair(account, fix)
	if err != nil {
		return fmt.Errorf("repairing %s: %w", account, err)
	}

	w := session.out()
	fmt.Fprintf(w, "Index of %s\n", account)
	printRepairReport(w, rep)
	printLedgerHealth(w, []string{
		train.SpamUIDPath(account),
		train.HamPath(account),
		train.MessageIDPath(session.Config.ConfigDir),
	})

	repairManifest(w, session, account, fix)

	switch {
	case !rep.NeedsRewrite():
		fmt.Fprintln(w, "Nothing to repair.")
	case fix:
		fmt.Fprintf(w, "Rewrote the index: %d record(s).\n", rep.Records-rep.Orphans+rep.Unindexed)
	default:
		fmt.Fprintln(w, "Report only (--dry-run or --read-only); run without either to rewrite the index.")
	}
	return nil
}

func printRepairReport(w io.Writer, rep msg.RepairReport) {
	fmt.Fprintf(w, "  %d message(s), %d carrying your spam/not-spam decision\n", rep.Records, rep.Decided)
	fmt.Fprintf(w, "  %d record(s) in the log: %d superseded, %d in an older format\n",
		rep.Records+rep.Superseded, rep.Superseded, rep.OldFormat)
	if rep.Damaged {
		fmt.Fprintln(w, "  the index ends in a damaged record")
	}
	fmt.Fprintf(w, "  %d record(s) with no message file", rep.Orphans+rep.KeptOrphans)
	if rep.KeptOrphans > 0 {
		fmt.Fprintf(w, " (%d kept: they hold your decision)", rep.KeptOrphans)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %d message file(s) with no record, %d filed under the wrong date\n", rep.Unindexed, rep.Misdated)
	if rep.Duplicates > 0 {
		fmt.Fprintf(w, "  %d duplicate message file(s) under another date, left in place\n", rep.Duplicates)
	}
}

// printLedgerHealth reports a damaged training ledger without touching it:
// guessing its contents would retrain bogofilter on what it already learned.
func printLedgerHealth(w io.Writer, paths []string) {
	for _, p := range paths {
		if _, err := train.Open(p); err != nil {
			fmt.Fprintf(w, "  %s is damaged and was left alone: %v\n", filepath.Base(p), err)
		}
	}
}

func newCacheRepairCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "repair",
		Description: "Check and rewrite this account's index.",
		LongDescription: "Check the account's message index against the cached message files and rewrite it: " +
			"one current-format record per message, a record for every file, and none for a missing file " +
			"unless it holds your spam/not-spam decision, which a re-download then gets back. " +
			"Damaged training ledgers are reported but never rewritten. Only local files are touched.",
		UsageLine: "feral utils cache repair",
		Args:      clihelp.NoArgs,
		Notes: []clihelp.Note{
			{Heading: "Reporting only", Text: "With --dry-run or --read-only it reports what it would change and changes nothing."},
		},
		Run: func(_ *clihelp.Context) error {
			return runCacheRepair(session)
		},
		Examples: []clihelp.Example{
			{Line: "feral --dry-run utils cache repair", Description: "Report what a repair would change"},
			{Line: "feral utils cache repair"},
		},
	}
}

// repairManifest rebuilds a damaged cache manifest from what the cache holds:
// the guard that refuses a damaged one sends the user here. A manifest that
// parses is left alone -- a newer build's is not this one's to rewrite.
func repairManifest(w io.Writer, session *Session, account cachedir.Account, fix bool) {
	_, _, err := meta.Read(account)
	if !errors.Is(err, meta.ErrDamaged) {
		return
	}
	acc, lookupErr := cfg_g.Lookup(session.Config.Accounts, string(session.Config.SelectedAccount))
	if lookupErr != nil {
		fmt.Fprintf(w, "  the cache manifest is damaged, and the account's type is unknown: %v\n", lookupErr)
		return
	}
	if !fix {
		fmt.Fprintln(w, "  the cache manifest is damaged; run without --dry-run or --read-only to rebuild it")
		return
	}
	var sample []string
	msg.ForEachID(account, func(id email.ID) bool {
		sample = append(sample, string(id))
		return len(sample) >= 50
	})
	m, err := meta.Rebuild(account, acc.Type, app.Version, sample)
	if err != nil {
		fmt.Fprintf(w, "  could not rebuild the cache manifest: %v\n", err)
		return
	}
	fmt.Fprintf(w, "  rebuilt the cache manifest: %s cache, %s message IDs\n", m.Backend, m.IDScheme)
}
