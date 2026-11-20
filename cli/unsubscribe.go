package cli

import (
	"github.com/sarielhp/clihelp"
)

// msgUnsubCmd leaves the mailing list a message came from.
func msgUnsubCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "unsub",
		Title:       "msg unsub [flags] <message-id>",
		Description: "Leave the mailing list a message came from.",
		LongDescription: "Reads the message's List-Unsubscribe header and takes the first of these it offers: " +
			"a one-click unsubscribe (RFC 8058: List-Unsubscribe-Post with an https link) is a POST " +
			"through the guarded client that refuses loopback, private and metadata addresses; a mailto: " +
			"is a short message from the account the mail arrived in, shown in full first; a plain web " +
			"link is opened in your browser, never fetched by feral. It always shows what it will do and " +
			"asks first. --read-only and --dry-run show it and send nothing. Each attempt is recorded in " +
			"unsubscriptions.json beside config.json.",
		UsageLine: "feral msg unsub [flags] <message-id>",
		Parameters: []clihelp.Param{
			{Name: "<message-id>", Description: "The message's ID (short 8-character or full)."},
		},
		Options: []clihelp.Option{
			clihelp.Bool(&flagUnsubYes, "-y, --yes", false, "Proceed without the confirmation prompt."),
		},
		Args: clihelp.ExactArgs(1),
		Run: func(ctx *clihelp.Context) error {
			return runUnsubscribe(session, ctx.Args[0], flagUnsubYes)
		},
		Examples: []clihelp.Example{
			{Line: "feral msg unsub abc123de", Description: "Show how to leave the list, then ask"},
			{Line: "feral --dry-run msg unsub abc123de", Description: "Only show what it would do"},
		},
	}
}

// runUnsubscribe resolves the message in the selected account, or in any
// account when none is selected, and leaves its list once confirmed.
func runUnsubscribe(session *Session, ref string, assumeYes bool) error {
	account := ""
	if session.Config != nil {
		account = string(session.Config.SelectedAccount)
	}
	opts := UnsubscribeOptions{
		MessageID: ref,
		Account:   account,
		Confirm: func(action string) error {
			return session.Confirm(action, assumeYes)
		},
	}
	return declined(session, session.unsub().Unsubscribe(session.Config, opts))
}
