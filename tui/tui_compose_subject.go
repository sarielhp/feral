package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/llm"

	tea "github.com/charmbracelet/bubbletea"
)

// subjectSuggestTimeout bounds the wait for a subject line: a local model
// answers in seconds, and a send should not hang on one that does not.
const subjectSuggestTimeout = 45 * time.Second

// subjectSuggestedMsg carries the model's subject line for a compose session,
// or why it could not write one.
type subjectSuggestedMsg struct {
	session string
	subject string
	forSend bool
	err     error
}

// canSuggestSubject reports whether an empty subject can be written by the
// model: the config allows it, and there is a body or an attachment to write
// it from.
func (m *tuiModel) canSuggestSubject() bool {
	cfg := m.Config()
	if cfg == nil {
		cfg = m.cfg
	}
	if cfg == nil || !cfg.LLM.IsAutoSubjectEnabled() {
		return false
	}
	return strings.TrimSpace(m.compose.Body) != "" || len(m.compose.Attachments) > 0
}

// startSubjectSuggestion asks the model for a subject. When forSend is true,
// keys are swallowed meanwhile (except Esc) and confirmation is prompted on return.
func (m *tuiModel) startSubjectSuggestion(forSend bool) (tea.Model, tea.Cmd) {
	if m.compose.suggestingSubject {
		// One request per session: a send asked for meanwhile takes over the
		// pending one, whose reply then asks for confirmation.
		if forSend {
			m.compose.subjectForSend = true
			m.showLoad = "Writing a subject with the LLM... (Esc to cancel)"
		}
		return m, nil
	}
	cfg := m.Config()
	if cfg == nil {
		cfg = m.cfg
	}
	llmCfg := cfg.LLM
	session := m.composeSession()
	draft := llm.SubjectDraft{
		To:          m.compose.To,
		Body:        m.compose.Body,
		Attachments: attachmentNames(m.compose.Attachments),
	}

	m.compose.suggestingSubject = true
	m.compose.subjectForSend = forSend
	if forSend {
		m.showLoad = "Writing a subject with the LLM... (Esc to cancel)"
	} else {
		m.saveStatus = "Writing a subject with the LLM..."
		m.saveStatusTime = time.Now()
	}
	m.showError = false
	m.err = nil

	// The goroutine uses only what it captured: the session is how a reply
	// that lands after the draft was closed or switched is told apart.
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), subjectSuggestTimeout)
		defer cancel()
		subject, err := llm.SuggestSubject(ctx, llmCfg, draft)
		return subjectSuggestedMsg{session: session, subject: subject, forSend: forSend, err: err}
	}
}

// cancelSubjectSuggestion drops a pending request; its reply, if it comes,
// is ignored because the flag is down.
func (m *tuiModel) cancelSubjectSuggestion() {
	m.compose.suggestingSubject = false
	m.compose.subjectForSend = false
	m.compose.replaceSubject = false
	m.showLoad = ""
}

// handleSubjectSuggested fills the subject in. For a send request, it asks for
// the send to be confirmed; for a background generation, it leaves the subject
// populated on the review screen.
func (m *tuiModel) handleSubjectSuggested(msg subjectSuggestedMsg) (tea.Model, tea.Cmd) {
	if m.compose == nil || !m.compose.suggestingSubject || m.compose.MessageID != msg.session {
		return m, nil
	}
	forSend := msg.forSend || m.compose.subjectForSend
	replace := m.compose.replaceSubject
	m.cancelSubjectSuggestion()
	if msg.err != nil {
		if forSend {
			m.compose.WarnEmptySend = true
			m.compose.WarnEmptyReason = fmt.Sprintf("%s (no subject could be generated: %v)",
				emptyFieldsReason(true, strings.TrimSpace(m.compose.Body) == ""), msg.err)
			return m, nil
		}
		m.saveStatus = fmt.Sprintf("LLM subject generation failed: %v", msg.err)
		m.saveStatusTime = time.Now()
		return m, nil
	}
	if !forSend && !replace && strings.TrimSpace(m.compose.Subject) != "" && !m.compose.SubjectGenerated {
		return m, nil
	}
	m.compose.Subject = msg.subject
	m.compose.SubjectGenerated = true
	if forSend {
		m.compose.SendConfirm = true
		return m, m.saveAndSyncDraftCmd()
	}
	m.saveStatus = "Subject generated with LLM"
	m.saveStatusTime = time.Now()
	return m, m.saveAndSyncDraftCmd()
}

// emptyFieldsReason names what the send would go out without; empty when
// nothing is missing.
func emptyFieldsReason(emptySubject, emptyBody bool) string {
	switch {
	case emptySubject && emptyBody:
		return "Both Subject and Message Body are empty!"
	case emptySubject:
		return "Subject is empty!"
	case emptyBody:
		return "Message Body is empty!"
	}
	return ""
}

func attachmentNames(atts []email.Attachment) []string {
	names := make([]string, 0, len(atts))
	for _, a := range atts {
		if a.Filename != "" {
			names = append(names, a.Filename)
		}
	}
	return names
}
