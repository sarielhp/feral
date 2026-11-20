package cli

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/sarielhp/clihelp"
	"github.com/sarielhp/feral/cfg_g"
)

// The user's own addresses: `config emails` lists every address feral
// treats as the user's and where each is set, and `config remove-email` is
// the inverse of `config add-email`.

func newConfigEmailsCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "emails",
		Title:       "config emails",
		Description: "List the addresses that count as yours.",
		LongDescription: "List every address feral treats as your own -- the my_emails list, each account's user_email, " +
			"and each account's login when it is an address -- with where each one is set, followed by the commands that change them. " +
			"A message from one of these addresses is mail you wrote: the TUI index shows its recipient instead of its sender.",
		UsageLine: "feral config emails",
		Args:      clihelp.NoArgs,
		Run: func(_ *clihelp.Context) error {
			writeOwnAddresses(session.out(), session.Config.OwnAddressSources())
			return nil
		},
		Examples: []clihelp.Example{
			{Line: "feral config emails"},
		},
	}
}

func newConfigRemoveEmailCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "remove-email",
		Title:       "config remove-email <address>...",
		Description: "Remove addresses from my_emails.",
		LongDescription: "Remove addresses from my_emails, the global list `config add-email` adds to. " +
			"An address that comes from an account -- its user_email or its login -- is not in that list; " +
			"the command refuses it, says which account sets it, and changes nothing.",
		UsageLine: "feral config remove-email <address>...",
		Parameters: []clihelp.Param{
			{Name: "<address>", Description: "An address in my_emails."},
		},
		Examples: []clihelp.Example{
			{Line: "feral config remove-email me@old-job.edu"},
			{Line: "feral config emails", Description: "See what is left"},
		},
		Args: clihelp.MinimumNArgs(1),
		Run: func(ctx *clihelp.Context) error {
			return runRemoveEmails(session, ctx.Args)
		},
	}
}

// runRemoveEmails drops addrs from my_emails, refusing the whole command when
// any of them is not in the list, so a typo never half-applies.
func runRemoveEmails(session *Session, addrs []string) error {
	cfg := session.Config
	remaining := slices.Clone(cfg.MyEmails)
	var problems []string
	for _, addr := range addrs {
		i := slices.IndexFunc(remaining, func(e string) bool { return strings.EqualFold(e, strings.TrimSpace(addr)) })
		if i < 0 {
			problems = append(problems, notInMyEmails(cfg, addr))
			continue
		}
		remaining = slices.Delete(remaining, i, i+1)
	}
	if len(problems) > 0 {
		return fmt.Errorf("nothing removed:\n  %s", strings.Join(problems, "\n  "))
	}
	if len(remaining) == 0 {
		return session.configOps().Reset(cfg, "my_emails")
	}
	return session.configOps().Set(cfg, "my_emails", strings.Join(remaining, ","))
}

// notInMyEmails explains why addr cannot be removed from my_emails: it is set
// by an account, or it is not one of the user's addresses at all.
func notInMyEmails(cfg *cfg_g.Config, addr string) string {
	want := strings.ToLower(strings.TrimSpace(addr))
	for _, own := range cfg.OwnAddressSources() {
		if own.Address != want {
			continue
		}
		for _, src := range own.Sources {
			switch src.Field {
			case cfg_g.OwnFromUserEmail:
				return fmt.Sprintf("%s is account %s's user_email; change it with `feral account user-email %s <email>`",
					want, src.Account, src.Account)
			case cfg_g.OwnFromUsername:
				return fmt.Sprintf("%s is account %s's login; it counts as long as the account exists", want, src.Account)
			}
		}
	}
	return fmt.Sprintf("%s is not in my_emails (see `feral config emails`)", want)
}

// writeOwnAddresses is the `config emails` listing: one address a line with
// where it is set, then the commands that change the list.
func writeOwnAddresses(w io.Writer, own []cfg_g.OwnAddress) {
	fmt.Fprintln(w, "Addresses feral treats as yours:")
	fmt.Fprintln(w)
	if len(own) == 0 {
		fmt.Fprintln(w, "  (none)")
	}
	width := 0
	for _, a := range own {
		width = max(width, len(a.Address))
	}
	for _, a := range own {
		fmt.Fprintf(w, "  %-*s  %s\n", width, a.Address, ownSourcesText(a.Sources))
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "To change them:")
	fmt.Fprintln(w, "  feral config add-email <address>...         add to my_emails")
	fmt.Fprintln(w, "  feral config remove-email <address>...      remove from my_emails")
	fmt.Fprintln(w, "  feral account user-email <account> <email>  change an account's send address")
	fmt.Fprintln(w, "An account's login counts while the account exists.")
}

// ownSourcesText is where an address is set, e.g.
// "my_emails, account work (user_email)".
func ownSourcesText(sources []cfg_g.OwnSource) string {
	parts := make([]string, 0, len(sources))
	for _, src := range sources {
		if src.Field == cfg_g.OwnFromMyEmails {
			parts = append(parts, src.Field)
			continue
		}
		parts = append(parts, fmt.Sprintf("account %s (%s)", src.Account, src.Field))
	}
	return strings.Join(parts, ", ")
}
