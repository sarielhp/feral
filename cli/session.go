package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/sarielhp/feral/app"
	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/label"
	"github.com/sarielhp/feral/mailclient"
	"github.com/sarielhp/feral/report"
	"github.com/sarielhp/feral/ui"
)

// StartupOptions carries the parsed CLI choices that runtime initialization
// needs, without making the composition root read parser globals.
type StartupOptions struct {
	NoConfigWrite bool
	AppendLogs    bool
	// FinishRename loads a configuration whose account rename is
	// unfinished, which only `account rename` may do (cfg_g.LoadConfigForRename).
	FinishRename bool
}

// Session carries the runtime configuration, dependency operations, and I/O streams for a CLI command.
type Session struct {
	Config     *cfg_g.Config
	FileConfig *cfg_g.FileConfig
	Account    *cfg_acc.AccountConfig
	ConfigPath string
	MailClient mailclient.MailClient

	// Bootstrap initializes configuration and logging after parsing. Tests and
	// callers with an existing configuration leave it nil.
	Bootstrap func(context.Context, StartupOptions) (*cfg_g.Config, error)

	// Out is where a command's product goes -- the listing, the rendered
	// message, the thing that was asked for. clihelp supplies it per
	// invocation, so a command can be run against a buffer instead of the
	// process's stdout.
	Out io.Writer

	// In is where a confirmation is read from. Tests supply a reader; a real
	// run supplies the terminal, and a run with no terminal cannot be asked.
	In io.Reader

	// Err is where a command says what it is doing: progress, warnings,
	// confirmations. Keeping it apart from Out is what makes
	// `feral rule list > rules.txt` capture the rules and not the chatter.
	Err io.Writer

	// Deps is what this command layer needs from the layers beneath it.
	Deps Deps
}

// out is Out, defaulting to standard output.
func (s *Session) out() io.Writer {
	if s == nil || s.Out == nil {
		return os.Stdout
	}
	return s.Out
}

// status is a Reporter over errOut, for the layers below that report rather
// than write.
func (s *Session) status() report.Reporter {
	return ui.NewTerminalReporter(s.errOut())
}

// errOut is Err, defaulting to standard error.
func (s *Session) errOut() io.Writer {
	if s == nil || s.Err == nil {
		return os.Stderr
	}
	return s.Err
}

// ResolveClientAndLabel resolves a label reference -- `%[account:]label` -- to
// the client that owns it and the label itself.
//
// A label written without its sigil is an error: ParseLabelRef says what to
// type instead.
func (s *Session) ResolveClientAndLabel(rawSpec string) (mailclient.MailClient, string, error) {
	t, typed, err := s.labelTarget(rawSpec)
	if err != nil {
		return nil, "", err
	}
	return t.Client, typed, nil
}

// FolderTarget is a label reference resolved against the account it names.
type FolderTarget struct {
	// Config is the session config scoped to that account, for the services
	// that build their own client from one.
	Config *cfg_g.Config
	Client mailclient.MailClient
	Folder label.Folder
}

// ResolveFolder resolves a label reference to a folder that must exist, the
// way every command that reads from or acts on one folder should. It is
// ResolveClientAndLabel followed by mailclient.ResolveOne: case-insensitive,
// a unique prefix or substring accepted, an ambiguous or unknown name an error.
//
// Commands used to hand their argument to the backend unparsed, so
// `spam learn --ham %archive` asked the server for a folder named
// "%archive" while `feral %archive` found Archive. The services such a
// command calls now take a label.Folder, which only resolution produces.
func (s *Session) ResolveFolder(raw string) (FolderTarget, error) {
	t, typed, err := s.labelTarget(raw)
	if err != nil {
		return FolderTarget{}, err
	}
	name, err := mailclient.ResolveOne(t.Client, typed)
	if err != nil {
		return FolderTarget{}, err
	}
	t.Folder = name
	return t, nil
}

// ResolveDestFolder is ResolveFolder for a destination, which may not exist
// yet: a name that matches nothing comes back as written, for the backend to
// create, provided label.ForCreate accepts it. An ambiguous one is still an
// error -- filing into a guess is worse than asking.
func (s *Session) ResolveDestFolder(raw string) (FolderTarget, error) {
	t, typed, err := s.labelTarget(raw)
	if err != nil {
		return FolderTarget{}, err
	}
	if t.Folder, err = resolveDest(t.Client, typed); err != nil {
		return FolderTarget{}, err
	}
	return t, nil
}

// resolveDest resolves typed as a destination on client: an existing folder
// it names, or, when it names none, the folder to create.
func resolveDest(client mailclient.MailClient, typed string) (label.Folder, error) {
	name, err := mailclient.ResolveOne(client, typed)
	var notFound *mailclient.NotFoundError
	if errors.As(err, &notFound) {
		return label.ForCreate(typed)
	}
	return name, err
}

// labelTarget parses a reference and builds the client for its account. The
// label comes back separately, as typed: it is not a Folder until resolved.
func (s *Session) labelTarget(raw string) (FolderTarget, string, error) {
	ref, err := cfg_g.ParseLabelRef(raw)
	if err != nil {
		return FolderTarget{}, "", err
	}
	config := s.Config
	if ref.Account != "" {
		if config, err = cfg_g.SelectAccount(config, ref.Account); err != nil {
			return FolderTarget{}, "", err
		}
	}
	client, err := s.clients().ForSelected(config)
	if err != nil {
		return FolderTarget{}, "", err
	}
	return FolderTarget{Config: config, Client: client}, ref.Label, nil
}

// FormatAccountLabel formats a label string with its associated account display name indicator (e.g. "GMail:INBOX").
func FormatAccountLabel(session *Session, client mailclient.Reader, label string) string {
	if session == nil || session.Config == nil {
		return label
	}
	accName := ""
	if client != nil && client.Config() != nil {
		accName = string(client.Config().SelectedAccount)
	}
	if accName == "" {
		accName = string(session.Config.SelectedAccount)
	}
	for _, a := range session.Config.Accounts {
		if a.MatchesName(accName) {
			displayName := a.GetDisplayName()
			if displayName != "" {
				return fmt.Sprintf("%s:%s", displayName, label)
			}
		}
	}
	if accName == "" || strings.EqualFold(accName, "default") {
		if resolved, err := cfg_g.Resolve(session.Config); err == nil && resolved.Account != nil {
			displayName := resolved.Account.GetDisplayName()
			if displayName != "" {
				return fmt.Sprintf("%s:%s", displayName, label)
			}
		} else if def := cfg_g.DefaultOf(session.Config.Accounts); def != nil {
			displayName := def.GetDisplayName()
			if displayName != "" {
				return fmt.Sprintf("%s:%s", displayName, label)
			}
		}
	}
	if accName != "" {
		return fmt.Sprintf("%s:%s", accName, label)
	}
	return label
}

// FormatAccountFolder formats an account and folder with the folder colored in blue.
func FormatAccountFolder(session *Session, client mailclient.Reader, folder string) string {
	coloredFolder := app.ColorBlue.Sprint(folder)
	return FormatAccountLabel(session, client, coloredFolder)
}
