package main

import (
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/cli"
	"github.com/sarielhp/feral/runner"
	"github.com/sarielhp/feral/ui"
	"os"
)

// buildSession binds the CLI's declared dependencies to their implementations.
//
// This is the composition root, and it lives in main because main is the only
// package that may name both sides. cli declares the interfaces it needs;
// mailops and the domain packages provide types that satisfy them structurally;
// neither imports the other. A missing binding is a compile error here rather
// than a nil check in every handler.
func buildSession(config *cfg_g.Config) *cli.Session {
	// Progress goes to stderr, a command's product to stdout, so `feral
	// label list > labels.txt` captures the labels and not the chatter about
	// fetching them. The TUI builds its own reporter when it starts, and there
	// the same calls become bubbletea messages rather than bytes.
	rep := ui.NewTerminalReporter(os.Stderr)

	return &cli.Session{
		Config: config,
		Deps: cli.Deps{
			Clients:  runner.ClientService{Rep: rep},
			Mail:     runner.MailService{Rep: rep, Out: os.Stdout},
			Spam:     runner.SpamService{Rep: rep},
			Rules:    runner.RuleService{Rep: rep},
			Config:   runner.ConfigService{Out: os.Stdout},
			Organize: runner.OrganizeService{Rep: rep},
			Calendar: runner.CalendarService{Rep: rep},
			Accounts: runner.AccountService{},
			TUI:      runner.TUIService{},
			Contacts: runner.ContactService{Rep: rep, Out: os.Stdout},
			Send:     runner.SendService{Rep: rep, Out: os.Stdout, TUI: runner.TUIService{}},
			Unsub:    runner.UnsubscribeService{Rep: rep, Out: os.Stdout},
			Outbox:   runner.OutboxService{Rep: rep, Out: os.Stdout},
			Watch:    runner.WatchService{Rep: rep},
			Snooze:   runner.SnoozeService{Rep: rep},
		},
	}
}
