package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/sarielhp/clihelp"

	"github.com/sarielhp/feral/app"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
)

func newAccountSignatureCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "signature",
		Title:       "account signature <account>",
		Description: "Show or set an account's signature.",
		LongDescription: "An account's signature goes under what it sends -- `write`, `msg reply`, `msg send` and " +
			"the TUI's compose, reply and forward -- below a line holding \"-- \", and above the quoted text of a " +
			"reply. It is part of the body, so a PGP-signed or encrypted message covers it. Without an option " +
			"the signature is printed. It is kept as signature.txt in the account's directory beside " +
			"config.json, so `account rename` takes it along.",
		UsageLine: "feral account signature <account> [--file <path> | --edit | --clear]",
		Parameters: []clihelp.Param{
			{Name: "<account>", Description: "The account.", Complete: CompleteAccountNames},
		},
		Options: []clihelp.Option{
			clihelp.String(&flagAccountSigFile, "--file <path>", "", "Set the signature from a file (- reads stdin)."),
			clihelp.Bool(&flagAccountSigEdit, "--edit", false, "Edit the signature in the configured editor."),
			clihelp.Bool(&flagAccountSigClear, "--clear", false, "Sign nothing from this account."),
		},
		OptionsValidator: clihelp.ValidateOptions(
			clihelp.MutuallyExclusive("file", "edit", "clear"),
		),
		Notes: []clihelp.Note{
			{Heading: "Not signed", Text: "`msg send --raw` sends the message as it is piped in. A body that " +
				"already has a \"-- \" line -- a resumed draft, or text written with a signature of its own -- " +
				"is not signed a second time."},
			{Heading: "~/.signature", Text: "An account with no signature of its own is signed in the TUI with " +
				"~/.signature, if there is one, as before signatures were per account. --clear stops that too."},
			{Heading: "Modes", Text: "--dry-run refuses to set one; --read-only does not, since a signature " +
				"changes nothing on the server."},
		},
		Args: clihelp.ExactArgs(1),
		Run: func(ctx *clihelp.Context) error {
			return runAccountSignature(session, ctx.Args[0])
		},
		Examples: []clihelp.Example{
			{Line: "feral account signature work", Description: "Print the signature"},
			{Line: "feral account signature work --file ~/sig-work.txt"},
			{Line: "feral account signature work --edit"},
			{Line: "feral account signature work --clear"},
		},
	}
}

// runSignatureEditor runs the editor on the signature's temporary file, on the
// terminal. Tests replace it so that none starts a real editor.
var runSignatureEditor = func(name string, args []string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func runAccountSignature(session *Session, ident string) error {
	configDir, _, err := cfg_g.ResolveConfigPath(session.Config)
	if err != nil {
		return err
	}
	_, fc, err := loadFileConfig(session)
	if err != nil {
		return err
	}
	acc, err := cfg_g.Lookup(*fc.Accounts, ident)
	if err != nil {
		return err
	}
	current, _, err := cfg_g.ReadSignature(configDir, acc.Name)
	if err != nil {
		return err
	}
	current = email.NormalizeSignature(current)
	writing := flagAccountSigFile != "" || flagAccountSigEdit || flagAccountSigClear
	if !writing {
		return showSignature(session, acc.Name, current)
	}
	if err = refuseLocalWrite(session, "account signature", false); err != nil {
		return err
	}
	next, err := newSignatureText(session, current)
	if err != nil {
		return err
	}
	if flagAccountSigEdit && next == current {
		fmt.Fprintf(session.errOut(), "%s The signature of %q is unchanged.\n", app.PrefixInfo, acc.Name)
		return nil
	}
	if err := cfg_g.WriteSignature(configDir, acc.Name, storedSignature(next)); err != nil {
		return err
	}
	if next == "" {
		fmt.Fprintf(session.errOut(), "%s Account %q now signs nothing.\n", app.PrefixSuccess, acc.Name)
		return nil
	}
	fmt.Fprintf(session.errOut(), "%s Signature of %q set:\n", app.PrefixSuccess, acc.Name)
	fmt.Fprint(session.out(), email.SignatureBlock(next))
	return nil
}

// showSignature prints the account's signature, the product of the bare
// command, or says on the status channel that there is none.
func showSignature(session *Session, account, sig string) error {
	if sig == "" {
		fmt.Fprintf(session.errOut(), "%s Account %q has no signature; set one with --file or --edit.\n", app.PrefixInfo, account)
		return nil
	}
	fmt.Fprintln(session.out(), sig)
	return nil
}

// newSignatureText is the signature the options ask for, normalized.
func newSignatureText(session *Session, current string) (string, error) {
	switch {
	case flagAccountSigClear:
		return "", nil
	case flagAccountSigEdit:
		return editSignature(session, current)
	}
	var data []byte
	var err error
	if flagAccountSigFile == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(flagAccountSigFile)
	}
	if err != nil {
		return "", fmt.Errorf("reading the signature: %w", err)
	}
	return email.NormalizeSignature(string(data)), nil
}

// editSignature opens current in the configured editor and returns what was
// saved. An editor that fails changes nothing.
func editSignature(session *Session, current string) (string, error) {
	dir, err := app.GetTempDir()
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "signature.txt")
	if err = os.WriteFile(path, []byte(storedSignature(current)), 0o600); err != nil {
		return "", err
	}
	name, args := cfg_g.EditorCommand(session.Config, path)
	if err = runSignatureEditor(name, args); err != nil {
		return "", fmt.Errorf("editor %s failed; the signature is unchanged: %w", name, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return email.NormalizeSignature(string(data)), nil
}

// storedSignature is sig as signature.txt holds it: with a final newline, or
// empty for no signature.
func storedSignature(sig string) string {
	if sig == "" {
		return ""
	}
	return sig + "\n"
}
