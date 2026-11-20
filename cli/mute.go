package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/sarielhp/clihelp"

	"github.com/sarielhp/feral/app"
	"github.com/sarielhp/feral/cache"
	"github.com/sarielhp/feral/cache/meta"
	"github.com/sarielhp/feral/cache/msg"
	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/mute"
)

const muteNotes = "A muted thread's new messages skip the inbox: the watcher (feral watch, " +
	"or the one feral tui runs) archives them as they arrive and does not announce them. " +
	"Mail already in the inbox stays where it is. A thread is recognized by the Message-ID at " +
	"its root, so a reply that names any message of the thread in References or In-Reply-To " +
	"belongs to it. The list is kept per account in muted_threads.json in the account's " +
	"settings directory. Muting is refused under --read-only, where the archiving it asks " +
	"for could not happen; --dry-run reports what would change and writes nothing."

// msgMuteCmd mutes the threads of messages named by ID.
func msgMuteCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "mute",
		Title:           "msg mute <message-id...>",
		Description:     "Archive a thread's new messages on arrival.",
		LongDescription: muteNotes,
		UsageLine:       "feral msg mute <message-id...>",
		Parameters: []clihelp.Param{
			{Name: "<message-id...>", Description: "A message of each thread to mute (ID or short ID).", Variadic: true},
		},
		Args: clihelp.MinimumNArgs(1),
		Run: func(ctx *clihelp.Context) error {
			return muteByIDs(session, ctx.Args)
		},
		Examples: []clihelp.Example{
			{Line: "feral msg mute abc123de", Description: "Mute the thread of one message"},
		},
	}
}

// msgUnmuteCmd unmutes threads named by a message or by their key.
func msgUnmuteCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "unmute",
		Title:       "msg unmute <message-id|key...>",
		Description: "Let a muted thread reach the inbox again.",
		LongDescription: "Name the thread by any cached message of it, or by the <key> msg muted prints " +
			"(the brackets may be left off). New mail in the thread reaches the inbox and is announced " +
			"again; what the watcher already archived stays archived. Unmuting is allowed under " +
			"--read-only, since it records nothing the server did not do; --dry-run reports.",
		UsageLine: "feral msg unmute <message-id|key...>",
		Parameters: []clihelp.Param{
			{Name: "<message-id|key...>", Description: "A message of the thread, or a <key> from msg muted.", Variadic: true},
		},
		Args: clihelp.MinimumNArgs(1),
		Run: func(ctx *clihelp.Context) error {
			return unmuteRefs(session, ctx.Args)
		},
		Examples: []clihelp.Example{
			{Line: "feral msg unmute abc123de", Description: "Unmute the thread of one message"},
			{Line: "feral msg unmute '<root@example.com>'", Description: "Unmute a thread by its listed key"},
		},
	}
}

// msgMutedCmd lists the account's muted threads.
func msgMutedCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "muted",
		Description: "List the account's muted threads.",
		UsageLine:   "feral msg muted",
		Args:        clihelp.NoArgs,
		Run: func(*clihelp.Context) error {
			return listMuted(session)
		},
		Examples: []clihelp.Example{
			{Line: "feral -A work msg muted", Description: "List the work account's muted threads"},
		},
	}
}

// muteByIDs mutes the thread of each message, in the account that holds it.
func muteByIDs(session *Session, ids []string) error {
	if session.Config.ReadOnly && !session.Config.DryRun {
		return fmt.Errorf("msg mute refused under --read-only: a muted thread is archived on arrival, which --read-only cannot do")
	}
	for _, ref := range ids {
		acc, cached, err := findCachedForMute(session, ref)
		if err != nil {
			return err
		}
		thread, err := mute.ThreadOf(cached, time.Now())
		if err != nil {
			return fmt.Errorf("message %s: %w", ref, err)
		}
		changed, err := editMuted(session, acc, func(l *mute.List) bool { return l.Add(thread) })
		if err != nil {
			return err
		}
		reportMuteChange(session, acc, thread.Key, changed, "muted")
	}
	return nil
}

// unmuteRefs unmutes each thread, named by a listed <key> or by one of its
// messages.
func unmuteRefs(session *Session, refs []string) error {
	for _, ref := range refs {
		acc, key, err := unmuteTarget(session, ref)
		if err != nil {
			return err
		}
		changed, err := editMuted(session, acc, func(l *mute.List) bool { return l.Remove(key) })
		if err != nil {
			return err
		}
		reportMuteChange(session, acc, key, changed, "unmuted")
	}
	return nil
}

// unmuteTarget resolves an unmute argument: a <key>, as `msg muted` prints
// it, names a thread in the selected account; anything else is first looked
// up as a message, whose thread's root is the key, and only failing that is
// taken as a key written without its brackets -- an mbox message ID can
// contain '@', so the '@' alone cannot decide it.
func unmuteTarget(session *Session, ref string) (cfg_acc.AccountConfig, email.HeaderID, error) {
	ref = strings.TrimSpace(ref)
	if !strings.HasPrefix(ref, "<") {
		acc, cached, err := findCachedForMute(session, ref)
		if err == nil {
			thread, threadErr := mute.ThreadOf(cached, time.Now())
			if threadErr != nil {
				return cfg_acc.AccountConfig{}, "", fmt.Errorf("message %s: %w", ref, threadErr)
			}
			return acc, thread.Key, nil
		}
		if !strings.Contains(ref, "@") {
			return cfg_acc.AccountConfig{}, "", err
		}
		ref = "<" + ref + ">"
	}
	acc, err := cfg_g.Lookup(session.Config.Accounts, string(session.Config.SelectedAccount))
	if err != nil {
		return cfg_acc.AccountConfig{}, "", err
	}
	return *acc, email.HeaderID(ref), nil
}

// editMuted loads the account's list, applies change and saves it when change
// reports one -- except under --dry-run, which writes nothing.
func editMuted(session *Session, acc cfg_acc.AccountConfig, change func(*mute.List) bool) (bool, error) {
	path, err := mute.Path(session.Config.ConfigDir, acc.Name)
	if err != nil {
		return false, err
	}
	list, err := mute.Load(path)
	if err != nil {
		return false, err
	}
	if !change(list) {
		return false, nil
	}
	if session.Config.DryRun {
		return true, nil
	}
	return true, list.Save(path)
}

// reportMuteChange says what a mute or unmute did, or under --dry-run what
// it would have done.
func reportMuteChange(session *Session, acc cfg_acc.AccountConfig, key email.HeaderID, changed bool, verb string) {
	switch {
	case !changed:
		fmt.Fprintf(session.out(), "%s %s: %s was already %s\n", app.PrefixInfo, acc.Name, key, verb)
	case session.Config.DryRun:
		fmt.Fprintf(session.out(), "%s dry-run: %s: would have %s %s\n", app.PrefixInfo, acc.Name, verb, key)
	default:
		fmt.Fprintf(session.out(), "%s %s: %s %s\n", app.PrefixSuccess, acc.Name, verb, key)
	}
}

// listMuted prints the selected account's muted threads.
func listMuted(session *Session) error {
	acc, err := cfg_g.Lookup(session.Config.Accounts, string(session.Config.SelectedAccount))
	if err != nil {
		return err
	}
	path, err := mute.Path(session.Config.ConfigDir, acc.Name)
	if err != nil {
		return err
	}
	list, err := mute.Load(path)
	if err != nil {
		return err
	}
	if len(list.Threads) == 0 {
		fmt.Fprintf(session.out(), "%s %s: no muted threads\n", app.PrefixInfo, acc.Name)
		return nil
	}
	for _, t := range list.Threads {
		fmt.Fprintf(session.out(), "%s  %s  %s\n", t.MutedAt.Local().Format("2006-01-02"), t.Key, email.StripEscapeSequences(t.Subject))
	}
	return nil
}

// findCachedForMute finds a message in the caches, the selected account's
// first, and returns its summary and the account that holds it: a thread is
// muted in the account its mail arrives in.
func findCachedForMute(session *Session, ref string) (cfg_acc.AccountConfig, *email.Email, error) {
	accounts := session.Config.Accounts
	if sel, err := cfg_g.Lookup(accounts, string(session.Config.SelectedAccount)); err == nil {
		accounts = append([]cfg_acc.AccountConfig{*sel}, accounts...)
	}
	for _, acc := range accounts {
		dir := session.Config.AccountCacheFor(acc)
		if dir.IsZero() || meta.Check(dir, acc.Type) != nil {
			continue
		}
		id, _, err := cache.FindCachedEmailByID(dir, ref)
		if err != nil {
			continue
		}
		cached, err := msg.CachedEmail(dir, id)
		if err != nil {
			return cfg_acc.AccountConfig{}, nil, fmt.Errorf("message %s not readable: %w", ref, err)
		}
		return acc, cached, nil
	}
	return cfg_acc.AccountConfig{}, nil, fmt.Errorf("no cached message with ID %q", ref)
}
