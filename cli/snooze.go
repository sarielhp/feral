package cli

import (
	"fmt"
	"time"

	"github.com/sarielhp/feral/app"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/snooze"
	"github.com/sarielhp/feral/when"

	"github.com/sarielhp/clihelp"
)

// msgSnoozeCmd hides messages until a time; see package snooze.
func msgSnoozeCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "snooze",
		Title:       "msg snooze [flags] [<message-id>...]",
		Description: "Hide messages until a time, then return them.",
		LongDescription: "msg snooze --until <time> moves messages into a Snoozed folder on their account, created " +
			"the first time, and records when each is due in snooze.json in the download directory (H, for hide until, in the TUI, " +
			"on the list or an open message, does the same for that message). When the time " +
			"comes a watcher puts it back in the inbox, unread: feral watch, or the one the TUI runs while it is open. " +
			"With neither running the message waits; msg snooze --wake brings back " +
			"what is due, or the messages named, at once. Without flags, msg snooze lists what is snoozed.",
		Notes: []clihelp.Note{
			{Heading: "Times", Text: "Local time, as msg send --at reads it: " + when.Forms + ". A time already past is refused."},
			{Heading: "When a message cannot come back", Text: "A message deleted meanwhile, or one whose account was " +
				"removed, keeps its record and is reported by the watcher; msg snooze --forget drops the record and moves " +
				"nothing. An Outlook account whose cache still holds default message IDs is refused, because they change " +
				"on the move: run feral utils cache outlook-ids first."},
			{Heading: "Read-only and dry runs", Text: "Snoozing and waking are refused under --read-only, since the record " +
				"would describe a move that did not happen; --dry-run says what would move and changes nothing."},
		},
		UsageLine: "feral msg snooze [flags] [<message-id>...]",
		Parameters: []clihelp.Param{
			{Name: "[<message-id>...]", Description: "Message IDs (short 8-character or full).", Variadic: true},
		},
		Options: []clihelp.Option{
			clihelp.String(&flagSnoozeUntil, "-u, --until <time>", "", "Snooze until a time: +2h, tomorrow 8am."),
			clihelp.Bool(&flagSnoozeList, "-l, --list", false, "List snoozed messages (the default)."),
			clihelp.Bool(&flagSnoozeWake, "--wake", false, "Return messages now; with no IDs, those due."),
			clihelp.Bool(&flagSnoozeForget, "--forget", false, "Drop the records; move nothing."),
		},
		OptionsValidator: clihelp.ValidateOptions(
			clihelp.MutuallyExclusive("until", "list", "wake", "forget"),
			refuseBlankUntil,
		),
		Run: func(ctx *clihelp.Context) error {
			return runSnooze(session, ctx.Args)
		},
		Examples: []clihelp.Example{
			{Line: `feral msg snooze --until "tomorrow 8am" abc123de`, Description: "Hide a message until the morning"},
			{Line: "feral msg snooze -u +2h abc123de 9f8e7d6c", Description: "Hide two messages for two hours"},
			{Line: "feral msg snooze", Description: "List what is snoozed and when it returns"},
			{Line: "feral msg snooze --wake abc123de", Description: "Bring one back now"},
		},
	}
}

// refuseBlankUntil refuses an --until given with no time, as send refuses a
// blank --at: a script's --until "$WHEN" with WHEN unset is a mistake, not a
// request to list.
func refuseBlankUntil(opts clihelp.Flags) error { return refuseBlankTime(opts, "until") }

func runSnooze(session *Session, refs []string) error {
	switch {
	case flagSnoozeWake:
		return runSnoozeWake(session, refs)
	case flagSnoozeForget:
		return runSnoozeForget(session, refs)
	case flagSnoozeUntil != "":
		return runSnoozeUntil(session, refs)
	case flagSnoozeList && len(refs) > 0:
		return fmt.Errorf("%w: --list takes no message IDs", app.ErrUsage)
	case len(refs) > 0:
		return fmt.Errorf("%w: say until when: --until <time> (%s)", app.ErrUsage, when.Forms)
	default:
		return runSnoozeList(session)
	}
}

func runSnoozeUntil(session *Session, refs []string) error {
	if len(refs) == 0 {
		return fmt.Errorf("%w: name the messages to snooze", app.ErrUsage)
	}
	until, err := when.Future(flagSnoozeUntil, time.Now())
	if err != nil {
		return fmt.Errorf("%w: --until: %w", app.ErrUsage, err)
	}
	dryRun := session.Config.DryRun
	if !dryRun {
		if refused := refuseLocalWrite(session, "msg snooze", true); refused != nil {
			return refused
		}
	}
	client, err := GetValidatedClient(session)
	if err != nil {
		return err
	}
	items, err := session.snoozer().Items(client, refs)
	if err != nil {
		return err
	}
	out := session.out()
	if dryRun {
		for _, item := range items {
			fmt.Fprintf(out, "[DRY-RUN] Would snooze %s from %s to %s until %s: %s\n",
				email.ComputeShortID(item.ID), item.Folder, snooze.FolderName, localTime(until), item.Subject)
		}
		return nil
	}
	done, err := session.snoozer().Snooze(session.Config, client, items, until)
	for _, e := range done {
		fmt.Fprintf(out, "%s Snoozed %s until %s: %s\n", app.PrefixSuccess, email.ComputeShortID(e.ID), localTime(e.Until), e.Subject)
	}
	return err
}

func runSnoozeWake(session *Session, refs []string) error {
	out := session.out()
	if session.Config.DryRun {
		entries, err := session.snoozer().Select(session.Config, refs)
		if err != nil {
			return err
		}
		for _, e := range entries {
			fmt.Fprintf(out, "[DRY-RUN] Would return %s to the inbox of %s, unread: %s\n",
				email.ComputeShortID(e.ID), e.Account, e.Subject)
		}
		reportNothingDue(session, refs, len(entries))
		return nil
	}
	if err := refuseLocalWrite(session, "msg snooze --wake", true); err != nil {
		return err
	}
	woken, err := session.snoozer().Wake(session.Config, refs)
	for _, e := range woken {
		fmt.Fprintf(out, "%s Back in the inbox of %s: %s (%s)\n", app.PrefixSuccess, e.Account, e.Subject, email.ComputeShortID(e.ID))
	}
	if err == nil {
		reportNothingDue(session, refs, len(woken))
	}
	return err
}

// reportNothingDue says so when a wake of what is due found nothing due.
func reportNothingDue(session *Session, refs []string, n int) {
	if len(refs) == 0 && n == 0 {
		fmt.Fprintln(session.out(), "Nothing snoozed is due.")
	}
}

func runSnoozeForget(session *Session, refs []string) error {
	if len(refs) == 0 {
		return fmt.Errorf("%w: name the snoozed messages to forget", app.ErrUsage)
	}
	out := session.out()
	if session.Config.DryRun {
		entries, err := session.snoozer().Select(session.Config, refs)
		for _, e := range entries {
			fmt.Fprintf(out, "[DRY-RUN] Would forget %s; it would stay in %s of %s\n", email.ComputeShortID(e.ID), e.Folder, e.Account)
		}
		return err
	}
	forgotten, err := session.snoozer().Forget(session.Config, refs)
	for _, e := range forgotten {
		fmt.Fprintf(out, "Forgot %s; it stays in %s of %s\n", email.ComputeShortID(e.ID), e.Folder, e.Account)
	}
	return err
}

func runSnoozeList(session *Session) error {
	entries, err := session.snoozer().List(session.Config)
	if err != nil {
		return err
	}
	if wantJSON() {
		return writeJSONList(session, entries)
	}
	out := session.out()
	if len(entries) == 0 {
		fmt.Fprintln(out, "Nothing is snoozed.")
		return nil
	}
	now := time.Now()
	for _, e := range entries {
		until := localTime(e.Until)
		if e.Due(now) {
			until = "due " + until
		}
		fmt.Fprintf(out, "%s  %-12s  %-20s  %-14s  %s\n", email.ComputeShortID(e.ID), truncateField(e.Account, 12),
			until, truncateField(e.FromFolder, 14), e.Subject)
	}
	return nil
}

// localTime is how a snooze time is shown: local, to the minute.
func localTime(t time.Time) string { return t.Local().Format("2006-01-02 15:04") }
