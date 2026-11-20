package tui

import (
	"fmt"
	"log/slog"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sarielhp/feral/app"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
)

// The editor opens on a template: the text the user starts from. Each kind of
// message places the sending account's signature where it belongs -- under
// the user's text, above the quoted original of a reply, and under the
// forwarded block of a forward -- through email.SignBody, so the placement is
// decided in one place for the TUI and the CLI alike. A resumed draft and a
// re-edit never pass through here: their body already carries whatever
// signature it has.

// composeSignature is the signature of the account named account: its own
// signature.txt (`account signature`), else ~/.signature, which the TUI read
// before signatures were per account. An account whose signature was set to
// nothing does not fall back.
func (m *tuiModel) composeSignature(account string) string {
	// The client's config is a copy of the session's (BuildAccountLocalConfig),
	// so either names the configuration directory; a client built without one
	// must not hide the session's.
	configDir := ""
	for _, cfg := range []*cfg_g.Config{m.Config(), m.cfg} {
		if cfg != nil && cfg.ConfigDir != "" {
			configDir = cfg.ConfigDir
			break
		}
	}
	sig, set, err := cfg_g.ReadSignature(configDir, account)
	if err != nil {
		slog.Warn("cannot read the account's signature", slog.String("account", account), slog.Any("error", err))
		return ""
	}
	if set {
		return sig
	}
	return homeSignature()
}

// homeSignature is ~/.signature, or "" without one.
func homeSignature() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(home, ".signature"))
	if err != nil {
		return ""
	}
	return string(data)
}

// writeBodyTemplate writes content to a new file in feral's temporary
// directory for the editor to open.
func writeBodyTemplate(kind, content string) (string, error) {
	tempDir, err := app.GetTempDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(tempDir, fmt.Sprintf("%s_body_%d.txt", kind, time.Now().UnixNano()))
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return "", err
	}
	return path, nil
}

func (m *tuiModel) createComposeBodyTemplate(account string) (string, string, error) {
	return m.createComposeBodyTemplateWithContent(account, "", "")
}

// createComposeBodyTemplateWithContent writes the template of a message the
// command line started: initialBody, the signature, then quote -- the quoted
// original when `msg reply` opens the editor. The baseline it returns is the
// template when the command line supplied no text, so leaving it untouched
// cancels; a supplied text is a message worth keeping, so there is none.
func (m *tuiModel) createComposeBodyTemplateWithContent(account, initialBody, quote string) (string, string, error) {
	if initialBody != "" && !strings.HasSuffix(initialBody, "\n") {
		initialBody += "\n"
	}
	content := email.SignBody(initialBody, m.composeSignature(account), quote)
	path, err := writeBodyTemplate("compose", content)
	if err != nil {
		return "", "", err
	}
	baseline := ""
	if initialBody == "" && quote == "" {
		baseline = content
	}
	return path, baseline, nil
}

// createReplyBodyTemplate writes a reply's template: a line to write on, the
// signature, then the quoted original.
func (m *tuiModel) createReplyBodyTemplate(account string, orig *mail.Message) (string, string, error) {
	content := email.SignBody("", m.composeSignature(account), email.FormatReplyQuote(orig))
	if content == "" {
		content = "\n"
	}
	path, err := writeBodyTemplate("reply", content)
	if err != nil {
		return "", "", err
	}
	return path, content, nil
}

// createForwardBodyTemplate writes a forward's template: a blank line to
// write in above the forwarded block, then the signature. The forwarded text
// is the original sender's, so it is never searched for a delimiter: its own
// signature must not stand in for the forwarder's.
func (m *tuiModel) createForwardBodyTemplate(account string, orig *mail.Message, body string) (string, string, error) {
	content := "\n" + email.FormatForward(orig, body)
	if block := email.SignatureBlock(m.composeSignature(account)); block != "" {
		content += "\n" + block
	}
	path, err := writeBodyTemplate("forward", content)
	if err != nil {
		return "", "", err
	}
	return path, content, nil
}
