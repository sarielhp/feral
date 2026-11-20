package tui

import (
	"bytes"
	"context"
	"fmt"
	"mime"
	"net/mail"

	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/pgp"

	tea "github.com/charmbracelet/bubbletea"
)

// forwardCmd opens the compose editor on a forward of the message: its text
// under a separator that carries its identifying headers, and its
// attachments re-attached. A bounce used to send the original on as-is,
// which no receiving server trusts any more; the forward re-wraps it, so
// what goes out is a new message from the forwarder carrying the original.
func (m *tuiModel) forwardCmd(targetID email.ID) tea.Cmd {
	cfg := m.Config()
	if cfg == nil {
		cfg = m.cfg
	}
	if cfg == nil {
		return func() tea.Msg {
			return composeEditorFinishedMsg{err: fmt.Errorf("configuration unavailable")}
		}
	}

	// Through the accessor, as with reply: forwarding a message that was
	// never downloaded fetches it first.
	data, err := m.client.GetRawMessage(targetID)
	if err != nil {
		return func() tea.Msg {
			return composeEditorFinishedMsg{err: fmt.Errorf("could not retrieve the message being forwarded: %w", err)}
		}
	}

	prepared, err := m.prepareForward(data)
	if err != nil {
		return func() tea.Msg {
			return composeEditorFinishedMsg{err: err}
		}
	}

	tempPath, templateContent, err := m.createForwardBodyTemplate(prepared.state.AccountName, prepared.orig, prepared.body)
	if err != nil {
		return func() tea.Msg {
			return composeEditorFinishedMsg{err: fmt.Errorf("failed to create forward body template: %w", err)}
		}
	}
	prepared.state.InitialTemplate = templateContent

	return m.openEditorCmd(tempPath, prepared.state, false)
}

// forwardPrepared is a message ready to be forwarded: the compose session to
// open, and the original message and its text, from which the body template
// is built.
type forwardPrepared struct {
	state *composeState
	orig  *mail.Message
	body  string
}

// prepareForward turns a message's raw bytes into a forward. The PGP layer
// runs first, as in the detail view, so an encrypted message forwards the
// plaintext it decrypts to and a signed one the part its signature covers:
// the forward carries what the reader saw, not the envelope around it. The
// original's attachments are re-attached and its subject marked as forwarded.
func (m *tuiModel) prepareForward(data []byte) (*forwardPrepared, error) {
	processed := pgp.Process(context.Background(), data, pgp.Options{Verify: true, Decrypt: true})
	orig, err := mail.ReadMessage(bytes.NewReader(processed.Raw))
	if err != nil {
		return nil, fmt.Errorf("failed to parse email: %w", err)
	}

	body := processed.Body
	if body == "" {
		body, _ = email.ExtractPlainBodyText(orig)
	}

	activeAcc, activeIdx := m.resolveActiveAccount()
	cfg := m.Config()
	if cfg == nil {
		cfg = m.cfg
	}
	atts, _ := email.ExtractAttachments(processed.Raw)
	if atts == nil {
		atts = []email.Attachment{}
	}

	dec := new(mime.WordDecoder)
	state := &composeState{
		AccountIndex: activeIdx,
		AccountName:  activeAcc.Name,
		From:         formatFromHeader(activeAcc, cfg),
		Subject:      email.FormatForwardSubject(email.DecDecode(dec, orig.Header.Get("Subject"))),
		Attachments:  atts,
		IsForward:    true,
	}
	return &forwardPrepared{state: state, orig: orig, body: body}, nil
}
