package cli

import (
	"bytes"
	"fmt"

	"github.com/sarielhp/clihelp"

	"github.com/sarielhp/feral/app"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/mailclient"
	"github.com/sarielhp/feral/transfer"
)

// mboxImportCmd copies every message of a local mbox file into an account, as
// mail it received.
func mboxImportCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "import",
		Description: "Import an mbox file as received mail.",
		LongDescription: "Copy every message of a local mbox (or .eml) file into the account's received folder -- " +
			"its `received_folder`, or the inbox when it has none -- or into the label given. A message the " +
			"folder already holds (same Message-ID; Subject and Date when a side has none) is skipped, so an " +
			"import can be run again, or resumed after a failure, without duplicating anything; that is the " +
			"difference from `upload`, which uploads every message it reads. The file is only read, never " +
			"changed. Under --read-only or --dry-run, or into a read-only account, it reports what it would " +
			"import and uploads nothing.",
		UsageLine: "feral mbox import <file> [%label]",
		Parameters: []clihelp.Param{
			{Name: "<file>", Description: "Local mbox or .eml file to read"},
			{Name: "[%label]", Description: "Destination (default: received folder)", Complete: CompleteLabelRefs},
		},
		Examples: []clihelp.Example{
			{Line: "feral mbox import mbox.fl", Description: "Into the selected account's received folder"},
			{Line: "feral mbox import mbox.fl %work:projects", Description: "Into a label of another account"},
		},
		Args: clihelp.RangeArgs(1, 2),
		Run: func(ctx *clihelp.Context) error {
			labelArg := ""
			if len(ctx.Args) == 2 {
				labelArg = ctx.Args[1]
			}
			return runMboxImport(session, ctx.Args[0], labelArg)
		},
	}
}

// importDestination is where an import files its messages: the label given,
// or the selected account's received folder, or its inbox when it has none.
func importDestination(session *Session, labelArg string) (FolderTarget, error) {
	if labelArg != "" {
		return session.ResolveDestFolder(labelArg)
	}
	t, _, err := session.labelTarget(cfg_g.LabelSigil)
	if err != nil {
		return FolderTarget{}, err
	}
	recv := ""
	if cfg := t.Client.Config(); cfg != nil {
		recv = cfg.ReceivedFolder
	}
	if recv == "" {
		t.Folder = t.Client.InboxFolder()
		return t, nil
	}
	if t.Folder, err = resolveDest(t.Client, recv); err != nil {
		return FolderTarget{}, err
	}
	return t, nil
}

func runMboxImport(session *Session, path, labelArg string) error {
	dest, err := importDestination(session, labelArg)
	if err != nil {
		return err
	}
	_, src, err := openMboxFile(session, path)
	if err != nil {
		return err
	}
	ids, err := src.FetchAndDownloadEmails(src.InboxFolder(), "")
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if len(ids) == 0 {
		fmt.Fprintf(session.errOut(), "%s No messages in %s.\n", app.PrefixInfo, path)
		return nil
	}
	where := fmt.Sprintf("%s:%s", accountLabelOf(dest.Client), dest.Folder)
	fmt.Fprintf(session.errOut(), "%s Importing %d message(s) from %s into %s...\n", app.PrefixInfo, len(ids), path, where)
	t, err := importMessages(session, src, dest, ids)
	if err != nil {
		return err
	}
	return t.report(session, where, mailclient.Refuses(dest.Client))
}

// importTally counts what an import did with each message.
type importTally struct {
	imported, present, failed int
}

// importMessages copies ids from src into dest, skipping what dest already
// holds. A destination that refuses changes is only checked against.
func importMessages(session *Session, src mailclient.MailClient, dest FolderTarget, ids []email.ID) (importTally, error) {
	mover, err := transfer.NewMover(src, dest.Client, src.InboxFolder(), transfer.Options{Reporter: session.status()})
	if err != nil {
		return importTally{}, err
	}
	defer func() { _ = mover.Close() }()

	checkOnly := mailclient.Refuses(dest.Client)
	seen := make(transfer.FolderCache)
	var t importTally
	for i, id := range ids {
		raw, err := src.GetRawMessage(id)
		if err != nil {
			t.failed++
			fmt.Fprintf(session.errOut(), "%s [%d/%d] %s: cannot read: %v\n", app.PrefixError, i+1, len(ids), id, err)
			continue
		}
		parsed := email.ParseReader(bytes.NewReader(raw), id, "")
		if checkOnly {
			if transfer.InDest(dest.Client, dest.Folder, id, parsed, seen) {
				t.present++
			} else {
				t.imported++
			}
			continue
		}
		res := mover.Copy(transfer.Item{ID: id, Parsed: parsed, Raw: raw, Dest: dest.Folder})
		switch {
		case res.Existed:
			t.present++
		case !res.OK():
			t.failed++
			fmt.Fprintf(session.errOut(), "%s [%d/%d] %q: %s failed: %v\n", app.PrefixError, i+1, len(ids), parsed.Subject, res.Failed, res.Err)
		default:
			t.imported++
		}
	}
	return t, nil
}

// report prints the tally, and fails the command when a message failed, so a
// script sees an incomplete import.
func (t importTally) report(session *Session, where string, checkOnly bool) error {
	if checkOnly {
		fmt.Fprintf(session.errOut(), "%s Would import %d message(s) into %s; %d already there. Nothing was uploaded (read-only or dry run).\n",
			app.PrefixInfo, t.imported, where, t.present)
	} else {
		fmt.Fprintf(session.errOut(), "%s Imported %d message(s) into %s; %d already there.\n",
			app.PrefixSuccess, t.imported, where, t.present)
	}
	if t.failed > 0 {
		return fmt.Errorf("%d message(s) were not imported; running the import again retries only those", t.failed)
	}
	return nil
}

func accountLabelOf(c mailclient.MailClient) string {
	if c == nil || c.Config() == nil {
		return "account"
	}
	return string(c.Config().SelectedAccount)
}
