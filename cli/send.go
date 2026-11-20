package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/sarielhp/feral/app"
	"github.com/sarielhp/feral/contacts"
	"github.com/sarielhp/feral/when"

	"github.com/sarielhp/clihelp"
)

// SendCmd returns the clihelp.Command for sending outbound emails.
func SendCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "send",
		NoAbbrev:    true,
		Group:       "Messages",
		Description: "Send an email directly or from stdin.",
		LongDescription: "Send an email via configured account or standard input pipe, with optional attachments and Cc/Bcc recipients. " +
			"Without -s, and with an LLM endpoint configured, the subject line is written by the model from the body and the " +
			"attachment names and printed before the send; set llm_auto_subject to false to keep sending without one. " +
			"The account's signature (`account signature`) is appended under a \"-- \" line unless the body already has " +
			"such a line or --no-signature is given; a --raw message is sent as it is.",
		Notes: []clihelp.Note{
			{Heading: "Sending later (--at)", Text: "With --at the message is built now, queued in the outbox with a due time, " +
				"and sent by the first outbox flush at or after it: feral watch flushes after every clean poll, or run " +
				"msg outbox flush from cron. The TUI never flushes, so with neither running the message waits; " +
				"msg outbox ls shows when each one is due. Times are local: \"2026-11-20 08:00\", \"+2h\", \"+30m\", " +
				"\"+1d\", \"today 17:00\", \"tomorrow 8am\" -- as in feral msg send --at \"tomorrow 8am\" -s Status boss " +
				"< status.txt. A time already past is refused. Like --queue, --at is refused under --read-only."},
		},
		UsageLine: "feral msg send [flags] <recipient...>",
		Parameters: []clihelp.Param{
			{Name: "<recipient...>", Description: "One or more recipient email addresses or contact aliases.", Variadic: true, Complete: completeRecipients(session)},
		},
		Options: []clihelp.Option{
			clihelp.String(&flagSendSubject, "-s, --subject <text>", "", "Email subject line."),
			withCompleter(clihelp.String(&flagSendCc, "-c, --cc <address>", "", "Carbon copy (Cc) recipient address (comma-separated)."), completeRecipients(session)),
			withCompleter(clihelp.String(&flagSendBcc, "-b, --bcc <address>", "", "Bcc recipient (comma-separated)."), completeRecipients(session)),
			withCompleter(clihelp.StringSlice(&flagSendAttach, "-a, --attach <path>", nil, "Attach files; repeat, comma-separate, or quote a glob."), completeAttachments),
			clihelp.String(&flagSendInclude, "-i, --include <path>", "", "Path to file containing email body."),
			clihelp.String(&flagSendBody, "--body <text>", "", "Inline email body text."),
			clihelp.Bool(&flagSendRaw, "-t, --raw", false, "Read a full RFC822 message from stdin."),
			clihelp.Bool(&flagSendQueue, "-q, --queue", false, "Queue message in outbox without sending."),
			clihelp.String(&flagSendAt, "--at <time>", "", "Queue it to send at a time: +2h, tomorrow 8am."),
			clihelp.Bool(&flagSendNoSig, "--no-signature", false, "Send without the account's signature."),
			clihelp.Bool(&flagSendSign, "--sign", false, "PGP-sign the message body."),
			clihelp.Bool(&flagSendEncrypt, "--encrypt", false, "PGP-encrypt; fails unless all recipients have keys."),
			clihelp.String(&flagSendPgpKey, "--pgp-key <id>", "", "Key to sign with (default: account key, else gpg's)."),
		},
		OptionsValidator: clihelp.ValidateOptions(
			clihelp.MutuallyExclusive("body", "include", "raw"),
			refuseBlankAt,
		),
		Run: func(ctx *clihelp.Context) error {
			return runSend(session, ctx.Args)
		},
		Examples: []clihelp.Example{
			{Line: `feral msg send -s "Meeting notes" user@example.com < notes.txt`, Description: "Send email with body from stdin pipe"},
			{Line: `feral msg send -s "Quick update" --body "Everything on track" boss@company.com`, Description: "Send quick email with inline body"},
			{Line: `feral msg send -s "Report" -a report.pdf -c team@company.com boss < body.txt`, Description: "Send with attachment, CC, and alias"},
			{Line: `feral msg send -s "Dry run test" --dry-run user@example.com < body.txt`, Description: "Preview RFC822 message without sending"},
			{Line: `feral msg send -a report.pdf boss`, Description: "Send a file with \"See attachment.\" as the body and an LLM-written subject"},
			{Line: `feral msg send --raw < patch.eml`, Description: "Send pre-formatted RFC822 message directly (git send-email mode)"},
		},
	}
}

func runSend(session *Session, recipients []string) error {
	if flagSendRaw {
		if flagSendSign || flagSendEncrypt {
			return fmt.Errorf("%w: --sign and --encrypt apply to a message this command builds; a --raw message must be signed or encrypted before it is piped in", app.ErrUsage)
		}
		return runSendRaw(session, recipients)
	}
	if len(recipients) == 0 {
		return fmt.Errorf("%w: send requires at least one recipient", app.ErrUsage)
	}
	sendAfter, err := parseSendAt(time.Now())
	if err != nil {
		return err
	}

	// send has no review screen: an unquoted -a *.pdf would mail the other
	// matches' names as addresses the moment it ran.
	attachments, err := expandAttachments(flagSendAttach)
	if err != nil {
		return err
	}
	body, err := resolveSendBody()
	// A file sent on its own needs no message: `send -a report.pdf bob`.
	if len(attachments) > 0 && (errors.Is(err, errNoSendBody) || err == nil && strings.TrimSpace(body) == "") {
		body, err = attachmentNote(len(attachments)), nil
	}
	if err != nil {
		return err
	}
	book, _ := contacts.LoadBook(contacts.DefaultPath(sessionCfg(session)))
	if err := checkRecipientsAreNotFiles(book, recipients); err != nil {
		return err
	}

	selectedAcc := ""
	dryRun := false
	if session != nil && session.Config != nil {
		selectedAcc = string(session.Config.SelectedAccount)
		dryRun = session.Config.DryRun
	}

	opts := SendOptions{
		To:          recipients,
		Cc:          splitCommaList(flagSendCc),
		Bcc:         splitCommaList(flagSendBcc),
		Attachments: attachments,
		Subject:     flagSendSubject,
		Body:        body,
		Account:     selectedAcc,
		DryRun:      dryRun,
		Queue:       flagSendQueue,
		SendAfter:   sendAfter,
		NoSignature: flagSendNoSig,
		Sign:        flagSendSign,
		Encrypt:     flagSendEncrypt,
		PgpKeyID:    flagSendPgpKey,
	}
	fillSubjectFromLLM(session, &opts)

	return session.send().Send(session.Config, opts)
}

func runSendRaw(session *Session, recipients []string) error {
	sendAfter, err := parseSendAt(time.Now())
	if err != nil {
		return err
	}
	rawBytes, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("reading raw message from stdin: %w", err)
	}
	if len(strings.TrimSpace(string(rawBytes))) == 0 {
		return fmt.Errorf("%w: --raw requires RFC822 message on stdin", app.ErrUsage)
	}

	selectedAcc := ""
	dryRun := false
	if session != nil && session.Config != nil {
		selectedAcc = string(session.Config.SelectedAccount)
		dryRun = session.Config.DryRun
	}

	opts := SendOptions{
		To:        recipients,
		Account:   selectedAcc,
		DryRun:    dryRun,
		Raw:       true,
		RawBytes:  rawBytes,
		Queue:     flagSendQueue,
		SendAfter: sendAfter,
	}

	return session.send().Send(session.Config, opts)
}

// refuseBlankAt refuses an --at given with no time. Read as "not given", it
// would send at once: a script's --at "$WHEN" with WHEN unset would mail now.
func refuseBlankAt(opts clihelp.Flags) error { return refuseBlankTime(opts, "at") }

// refuseBlankTime refuses a time flag (--at, --until) that was given with
// no time: a script's --flag "$WHEN" with WHEN unset is a mistake, never the
// flag's absence.
func refuseBlankTime(opts clihelp.Flags, name string) error {
	given, err := opts.Changed(name)
	if err != nil || !given {
		return err
	}
	if value, _ := opts.Value(name); strings.TrimSpace(value) == "" {
		return fmt.Errorf("%w: --%s needs a time: %s", app.ErrUsage, name, when.Forms)
	}
	return nil
}

// parseSendAt reads --at against now; a zero time means send at once. It runs
// before anything that costs a request, such as the LLM subject.
func parseSendAt(now time.Time) (time.Time, error) {
	if strings.TrimSpace(flagSendAt) == "" {
		return time.Time{}, nil
	}
	t, err := when.Future(flagSendAt, now)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: --at: %w", app.ErrUsage, err)
	}
	return t, nil
}

func resolveSendBody() (string, error) {
	if flagSendBody != "" {
		return flagSendBody, nil
	}
	if flagSendInclude != "" {
		data, err := os.ReadFile(flagSendInclude)
		if err != nil {
			return "", fmt.Errorf("reading body file %q: %w", flagSendInclude, err)
		}
		return string(data), nil
	}

	if body, ok, err := readRedirectedStdin(); ok || err != nil {
		return body, err
	}

	return "", errNoSendBody
}

// errNoSendBody is a send with no body and nothing attached.
var errNoSendBody = fmt.Errorf("%w: no message body provided; pipe body via stdin (< file.txt), use -i/--include <path>, pass --body <text>, or attach a file with -a", app.ErrUsage)

// attachmentNote is the body of a message that only carries files.
func attachmentNote(files int) string {
	if files == 1 {
		return "See attachment.\n"
	}
	return "See attachments.\n"
}

func splitCommaList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	res := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			res = append(res, trimmed)
		}
	}
	return res
}
