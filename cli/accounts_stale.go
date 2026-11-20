package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sarielhp/clihelp"
	"github.com/sarielhp/feral/app"
	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/inboxage"
)

// errRelaysStale is the exit status of `account stale` when a relay needs
// attention: the report is on stdout, and the error makes the exit non-zero
// for cron and shell scripts.
var errRelaysStale = errors.New("a relay inbox is not being emptied")

func newAccountStaleCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "stale",
		Title:       "account stale [account_name]",
		Description: "Check relay inboxes for mail left too long.",
		LongDescription: "A relay account is one whose inbox something else empties on a schedule -- a fetcher " +
			"that moves its mail into another account every few minutes. Set its limit with " +
			"`account inbox-max-age`; mail older than that still in its inbox means the fetcher has " +
			"stopped. This asks each relay's server, reading arrival times only, and exits non-zero " +
			"when any inbox is stale or could not be checked, so it can run from cron. The TUI and " +
			"`watch` make the same check every couple of minutes and say so on screen and on the desktop.",
		UsageLine: "feral account stale [account_name]",
		Parameters: []clihelp.Param{
			{Name: "[account_name]", Description: "Relay to check (defaults to every account with a limit).", Complete: CompleteAccountNames},
		},
		Args: clihelp.MaximumNArgs(1),
		Run: func(ctx *clihelp.Context) error {
			return runAccountStale(session, ctx.Args)
		},
		Examples: []clihelp.Example{
			{Line: "feral account stale"},
			{Line: "feral account stale relay || notify-send 'mail relay stalled'"},
		},
	}
}

func newAccountInboxMaxAgeCmd(session *Session) clihelp.Command {
	return newAccountUserFieldCmd(session, accountUserFieldSpec{
		name:        "inbox-max-age",
		title:       "account inbox-max-age <account> <duration>",
		description: "Mark an account as a relay to watch.",
		longDesc: "Set how long mail may sit in the account's inbox before feral reports that whatever " +
			"empties it has stopped. Use it for an account whose mail a fetcher moves elsewhere every few " +
			"minutes. `off` removes the limit. See `account stale`.",
		valueName:   "<duration>",
		valueDesc:   "A duration of at least 1m, such as 15m, or off.",
		accountDesc: "The relay account.",
		example:     "feral account inbox-max-age relay 15m",
		set:         runAccountInboxMaxAge,
	})
}

func runAccountInboxMaxAge(session *Session, accountName, value string) error {
	value = strings.TrimSpace(value)
	if strings.EqualFold(value, "off") {
		value = ""
	}
	if _, err := inboxage.ParseMaxAge(value); err != nil {
		return fmt.Errorf("%w: %w", app.ErrUsage, err)
	}
	name, err := updateAccountConfig(session, accountName, func(acc *cfg_acc.AccountConfig) {
		acc.InboxMaxAge = value
	})
	if err != nil {
		return err
	}
	if value == "" {
		fmt.Fprintf(session.errOut(), "%s Account %q is no longer checked as a relay.\n", app.PrefixSuccess, name)
		return nil
	}
	fmt.Fprintf(session.errOut(), "%s Account %q: mail older than %s in its inbox will be reported.\n",
		app.PrefixSuccess, name, value)
	return nil
}

// runAccountStale checks the named relay, or every account with a limit, and
// prints one line per account.
func runAccountStale(session *Session, args []string) error {
	targets, err := staleTargets(session, args)
	if err != nil {
		return err
	}
	now := time.Now()
	bad := 0
	for _, t := range targets {
		st := checkRelay(session, t)
		mark := app.PrefixSuccess
		if !st.OK() {
			mark = app.PrefixError
			bad++
		}
		fmt.Fprintf(session.out(), "%s %s\n", mark, st.Summary(now))
	}
	if bad > 0 {
		return fmt.Errorf("%w: %d of %d relay(s)", errRelaysStale, bad, len(targets))
	}
	return nil
}

// relayTarget is an account to check and its 1-based position, which is how
// the client provider names accounts.
type relayTarget struct {
	acc   cfg_acc.AccountConfig
	index int
}

func staleTargets(session *Session, args []string) ([]relayTarget, error) {
	if len(args) > 0 {
		return namedRelay(session, args[0])
	}
	var targets []relayTarget
	for i, acc := range session.Config.Accounts {
		if acc.InboxMaxAge != "" {
			targets = append(targets, relayTarget{acc: acc, index: i + 1})
		}
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("%w: no account has an inbox_max_age; set one with `%s account inbox-max-age <account> 15m`",
			app.ErrUsage, app.AppName)
	}
	return targets, nil
}

// namedRelay is the one account the user named, which must have a limit.
func namedRelay(session *Session, name string) ([]relayTarget, error) {
	for i, acc := range session.Config.Accounts {
		if !strings.EqualFold(acc.Name, name) && !strings.EqualFold(acc.GetDisplayName(), name) {
			continue
		}
		if acc.InboxMaxAge == "" {
			return nil, fmt.Errorf("%w: account %q has no inbox_max_age; set one with `%s account inbox-max-age %s 15m`",
				app.ErrUsage, acc.Name, app.AppName, acc.Name)
		}
		return []relayTarget{{acc: acc, index: i + 1}}, nil
	}
	return nil, ErrAccountNotFoundAlt(name)
}

func checkRelay(session *Session, t relayTarget) inboxage.Status {
	client, err := session.clients().ForAccountIndex(session.Config, t.index)
	if err != nil {
		return inboxage.Status{Account: t.acc.Name, Err: err}
	}
	return inboxage.Check(context.Background(), client, t.acc, time.Now())
}
