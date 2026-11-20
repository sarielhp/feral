package tui

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/sarielhp/feral/contacts"
	"github.com/sarielhp/feral/contactsui"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/filebrowser"
	"github.com/sarielhp/feral/pgp"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *tuiModel) handleComposeDiscardPrompt(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := k.String()
	switch s {
	case "y", "Y":
		// Only an explicit y: this removes the local file and the server copy
		// in one motion.
		session := m.composeSession()
		if err := cleanupLocalDraft(m.compose.DraftPath); err != nil {
			m.saveStatus = "Draft discarded, but the local copy could not be removed: " + err.Error()
		} else {
			m.saveStatus = "Draft discarded"
		}
		req := syncRequest{kind: syncRetire, client: m.client, encrypted: m.compose.Encrypt}
		m.compose = nil
		m.showList()
		if m.standaloneCompose {
			return m, m.requestQuitAfterSync(session, req)
		}
		return m, m.requestSync(session, req)
	case "n", "N", "esc", "q", "enter":
		m.compose.DiscardConfirm = false
		return m, nil
	}
	return m, nil
}

func (m *tuiModel) handleComposeWarnEmptyPrompt(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := k.String()
	switch s {
	case "y", "Y":
		m.compose.WarnEmptySend = false
		return m.executeSendComposeEmail()
	case "n", "N", "esc", "q", "enter":
		m.compose.WarnEmptySend = false
		return m, nil
	}
	return m, nil
}

// handleComposeSendConfirmPrompt answers the question a clicked Send asks.
func (m *tuiModel) handleComposeSendConfirmPrompt(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "y", "Y":
		m.compose.SendConfirm = false
		return m.sendComposeEmailCmd()
	case "n", "N", "esc", "q":
		m.compose.SendConfirm = false
	}
	return m, nil
}

func (m *tuiModel) handleComposeKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.compose == nil {
		m.showList()
		return m, nil
	}

	// While a send is in flight nothing may send again or save the message
	// as a draft: a second send key used to send twice.
	if m.compose.sending {
		if k.String() == "esc" {
			m.compose.sendEscNoted = true
		}
		return m, nil
	}

	// A subject being written for send is waited for, or dropped with Esc.
	if m.compose.suggestingSubject && m.compose.subjectForSend {
		if k.String() == "esc" {
			m.cancelSubjectSuggestion()
		}
		return m, nil
	}

	if m.compose.DiscardConfirm {
		return m.handleComposeDiscardPrompt(k)
	}

	if m.compose.WarnEmptySend {
		return m.handleComposeWarnEmptyPrompt(k)
	}

	if m.compose.SendConfirm {
		return m.handleComposeSendConfirmPrompt(k)
	}

	if m.compose.FieldEditing != fieldNone {
		return m.kComposeFieldEdit(k)
	}

	s := k.String()
	switch s {
	case "t":
		m.compose.FieldEditing = fieldTo
		m.compose.FieldInput = m.compose.To
		return m, nil

	case "c":
		m.compose.FieldEditing = fieldCc
		m.compose.FieldInput = m.compose.Cc
		return m, nil

	case "b":
		m.compose.FieldEditing = fieldBcc
		m.compose.FieldInput = m.compose.Bcc
		return m, nil

	case "s":
		m.compose.FieldEditing = fieldSubject
		m.compose.FieldInput = m.compose.Subject
		return m, nil

	case "e":
		return m, m.reEditBodyCmd()

	case "a":
		m.fileBrowser = filebrowser.NewFileSelector(".", m.width, m.height)
		m.fileBrowserOpen = true
		return m, nil

	case "d", "D":
		return m.deleteCurrentAttachment()

	case "p":
		m.compose.Sign = !m.compose.Sign
		m.setComposePgpStatus()
		return m, nil

	case "P":
		return m.toggleComposeEncrypt()

	case "v":
		if len(m.compose.Attachments) > 0 && m.compose.AttCursor >= 0 && m.compose.AttCursor < len(m.compose.Attachments) {
			att := m.compose.Attachments[m.compose.AttCursor]
			return m.openAttachmentViewer(att)
		}
		return m, nil

	case "f4", "F4":
		if m.compose.suggestingSubject {
			return m, nil
		}
		if !m.canSuggestSubject() {
			m.saveStatus = "Cannot generate subject: message has no body or attachments"
			m.saveStatusTime = time.Now()
			return m, nil
		}
		// The old subject stays until a replacement arrives, so a failed or
		// cancelled generation loses nothing.
		m.compose.replaceSubject = true
		return m.startSubjectSuggestion(false)

	case "f5", "F5":
		m.openContactPicker()
		return m, nil

	case "f6", "F6":
		return m.openAliasBrowser(false, "", "")

	case "f3", "F3":
		// The same picker the index view opens on F3: the sending account is a
		// choice to confirm in a dialog, not a state a stray key press cycles.
		m.openAccountOverlay()
		return m, nil

	// Only y sends: Enter is too easy to press by accident.
	case "y":
		return m.sendComposeEmailCmd()

	case "q", "esc":
		return m.saveDraftAndExit()

	case "x":
		m.compose.DiscardConfirm = true
		return m, nil

	case "?", "h", "f1":
		m.openHelp()
		return m, nil

	case "up", "k":
		if len(m.compose.Attachments) > 0 && m.compose.AttCursor > 0 {
			m.compose.AttCursor--
			return m, nil
		}
		m.compose.Vp.ScrollUp(1)
		return m, nil

	case "down", "j":
		if len(m.compose.Attachments) > 0 && m.compose.AttCursor < len(m.compose.Attachments)-1 {
			m.compose.AttCursor++
			return m, nil
		}
		m.compose.Vp.ScrollDown(1)
		return m, nil

	case "pageup", "pgup":
		m.compose.Vp.HalfPageUp()
		return m, nil

	case "pagedown", "pgdown", "pgdn":
		m.compose.Vp.HalfPageDown()
		return m, nil

	case "tab":
		if len(m.compose.Attachments) > 0 {
			m.compose.AttCursor = (m.compose.AttCursor + 1) % len(m.compose.Attachments)
		}
		return m, nil

	case "shift+tab":
		numAtts := len(m.compose.Attachments)
		if numAtts > 0 {
			m.compose.AttCursor = (m.compose.AttCursor - 1 + numAtts) % numAtts
		}
		return m, nil
	}

	return m, nil
}

func (m *tuiModel) kCompose(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m.handleComposeKey(k)
}

// toggleComposeEncrypt turns encryption on or off and rewrites the draft, whose
// own form depends on it: on, the draft is encrypted to the sender; off, it is
// written back in the clear. Signing needs no such rewrite -- a signed-only
// session's draft is not signed, so p can change without touching the disk.
//
// Turning it on is refused when the sender has no key in the keyring, because
// the alternative is a draft that cannot be written and a send that fails later
// with a gpg error, neither of which says what the person actually needs to
// know.
func (m *tuiModel) toggleComposeEncrypt() (tea.Model, tea.Cmd) {
	if m.compose.Encrypt {
		m.compose.Encrypt = false
		m.setComposePgpStatus()
		return m, m.saveAndSyncDraftCmd()
	}

	sender := m.composeSenderAddress()
	if !pgp.HasPublicKey(context.Background(), sender) {
		m.saveStatus = "No PGP key for " + sender + "; cannot encrypt"
		m.saveStatusTime = time.Now()
		return m, nil
	}

	m.compose.Encrypt = true
	m.setComposePgpStatus()
	return m, m.saveAndSyncDraftCmd()
}

// setComposePgpStatus says what the session will do to the message, so a choice
// made with p or P is confirmed without waiting for the send dialog.
func (m *tuiModel) setComposePgpStatus() {
	switch {
	case m.compose.Sign && m.compose.Encrypt:
		m.saveStatus = "PGP: the message will be signed and encrypted"
	case m.compose.Encrypt:
		m.saveStatus = "PGP: the message will be encrypted"
	case m.compose.Sign:
		m.saveStatus = "PGP: the message will be signed"
	default:
		m.saveStatus = "PGP: off"
	}
	m.saveStatusTime = time.Now()
}

// composePgpBadge is what the session will do, shown in the title bar.
func (m *tuiModel) composePgpBadge() string {
	if m.compose == nil {
		return ""
	}
	var marks []string
	if m.compose.Sign {
		marks = append(marks, "sign")
	}
	if m.compose.Encrypt {
		marks = append(marks, "encrypt")
	}
	if len(marks) == 0 {
		return ""
	}
	return "PGP: " + strings.Join(marks, "+")
}

// commitComposeField stores the header field being edited.
func (m *tuiModel) commitComposeField() {
	if m.compose == nil || m.compose.FieldEditing == fieldNone {
		return
	}
	m.resetComposeSuggestions()
	val := strings.TrimSpace(m.compose.FieldInput)
	switch m.compose.FieldEditing {
	case fieldTo:
		m.compose.To = val
	case fieldCc:
		m.compose.Cc = val
	case fieldBcc:
		m.compose.Bcc = val
	case fieldSubject:
		m.compose.Subject = val
		m.compose.SubjectGenerated = false
	}
	m.compose.FieldEditing = fieldNone
	m.compose.FieldInput = ""
}

func (m *tuiModel) kComposeFieldEdit(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := k.String()
	switch {
	case k.Type == tea.KeyTab || s == "tab":
		if m.compose.FieldEditing == fieldTo || m.compose.FieldEditing == fieldCc || m.compose.FieldEditing == fieldBcc {
			return m.autocompleteRecipient()
		}
		return m, nil

	case k.Type == tea.KeyF5:
		m.openContactPicker()
		return m, nil

	case k.Type == tea.KeyF6:
		return m.openAliasBrowser(false, "", "")

	case k.Type == tea.KeyEsc:
		m.resetComposeSuggestions()
		m.compose.FieldEditing = fieldNone
		m.compose.FieldInput = ""
		return m, nil

	case k.Type == tea.KeyEnter:
		m.commitComposeField()
		return m, m.saveAndSyncDraftCmd()

	case k.Type == tea.KeyBackspace || s == "backspace" || s == "ctrl+h":
		m.resetComposeSuggestions()
		if len(m.compose.FieldInput) > 0 {
			m.compose.FieldInput = m.compose.FieldInput[:len(m.compose.FieldInput)-1]
		}
		return m, nil

	case k.Type == tea.KeySpace:
		m.resetComposeSuggestions()
		m.compose.FieldInput += " "
		return m, nil

	default:
		m.resetComposeSuggestions()
		if len(k.Runes) > 0 {
			m.compose.FieldInput += string(k.Runes)
		}
		return m, nil
	}
}

func (m *tuiModel) resetComposeSuggestions() {
	if m.compose != nil {
		m.compose.SuggestCandidates = nil
		m.compose.SuggestIndex = 0
		m.compose.SuggestQuery = ""
	}
}

func (m *tuiModel) autocompleteRecipient() (tea.Model, tea.Cmd) {
	if m.compose == nil {
		return m, nil
	}

	input := m.compose.FieldInput
	prefix := ""
	// The comma before the entry being typed; one inside a quoted name such
	// as "Doe, Alice" separates nothing.
	lastComma := email.LastAddressStart(input) - 1
	if lastComma >= 0 {
		prefix = strings.TrimSpace(input[lastComma+1:])
	} else {
		prefix = strings.TrimSpace(input)
	}

	if m.compose.SuggestQuery == prefix && len(m.compose.SuggestCandidates) > 0 {
		m.compose.SuggestIndex = (m.compose.SuggestIndex + 1) % len(m.compose.SuggestCandidates)
		m.applyCompletedRecipient(lastComma, m.compose.SuggestCandidates[m.compose.SuggestIndex])
		return m, nil
	}

	bookPath := contacts.DefaultPath(m.cfg)
	book, _ := contacts.LoadBook(bookPath)

	histPath := contacts.DefaultHistoryPath(m.cfg)
	history, _ := contacts.LoadHistory(histPath)

	var blacklist []string
	if m.cfg != nil {
		blacklist = m.cfg.Policy.Blacklist
	}

	suggestions := contacts.Suggest(book, history, blacklist, prefix, 10)
	if len(suggestions) == 0 {
		return m, nil
	}

	candidates := make([]string, 0, len(suggestions))
	for _, sc := range suggestions {
		c := formatCompletionString(sc)
		if c != "" {
			candidates = append(candidates, c)
		}
	}
	if len(candidates) == 0 {
		return m, nil
	}

	m.compose.SuggestCandidates = candidates
	m.compose.SuggestIndex = 0
	m.compose.SuggestQuery = prefix
	m.applyCompletedRecipient(lastComma, candidates[0])
	return m, nil
}

func (m *tuiModel) applyCompletedRecipient(lastComma int, completed string) {
	input := m.compose.FieldInput
	if lastComma >= 0 {
		m.compose.FieldInput = strings.TrimSpace(input[:lastComma+1]) + " " + completed
		return
	}
	m.compose.FieldInput = completed
}

// formatCompletionString is a contact as it lands in a recipient field: a
// group as all its members, a person as name and address, or the bare
// address when there is no name.
func formatCompletionString(sc contacts.ScoredContact) string {
	return contactsui.Recipient(sc)
}

func (m *tuiModel) deleteCurrentAttachment() (tea.Model, tea.Cmd) {
	if m.compose == nil || len(m.compose.Attachments) == 0 {
		return m, nil
	}
	idx := m.compose.AttCursor
	if idx >= 0 && idx < len(m.compose.Attachments) {
		m.compose.Attachments = append(m.compose.Attachments[:idx], m.compose.Attachments[idx+1:]...)
		if m.compose.AttCursor >= len(m.compose.Attachments) && m.compose.AttCursor > 0 {
			m.compose.AttCursor = len(m.compose.Attachments) - 1
		}
		return m, m.saveAndSyncDraftCmd()
	}
	return m, nil
}

// switchComposeAccountTo makes accounts[idx] the sending account of the open
// compose session and re-syncs the draft to it. The account overlay's
// selection lands here while composing.
func (m *tuiModel) switchComposeAccountTo(idx int) (tea.Model, tea.Cmd) {
	accounts := m.getAccounts()
	if m.compose == nil || idx < 0 || idx >= len(accounts) || idx == m.compose.AccountIndex {
		return m, nil
	}

	m.compose.AccountIndex = idx
	targetAcc := accounts[idx]
	// The signature belongs to the account it is sent from: an unedited copy
	// of the old account's goes, and the new account's takes its place.
	m.compose.Body = email.ReplaceSignature(m.compose.Body,
		m.composeSignature(m.compose.AccountName), m.composeSignature(targetAcc.Name))
	m.compose.Vp.SetContent(m.compose.Body)
	m.compose.AccountName = targetAcc.Name
	m.compose.From = formatFromHeader(targetAcc, m.cfg)

	// resolveClientForSwitch builds from the root config, as the index view's
	// account switch does. Deriving it from the current client's copy once
	// nested the new account's cache inside the old one's, when the root and
	// the account directory shared a field; CacheRoot is now never rewritten.
	if newClient, err := m.resolveClientForSwitch(targetAcc); err == nil && newClient != nil {
		m.client = newClient
	}

	return m, m.saveAndSyncDraftCmd()
}

func (m *tuiModel) sendComposeEmailCmd() (tea.Model, tea.Cmd) {
	if m.compose == nil {
		return m, nil
	}

	emptySubj := strings.TrimSpace(m.compose.Subject) == ""
	emptyBody := strings.TrimSpace(m.compose.Body) == ""

	// An empty subject is written by the model when one is configured; the
	// send resumes, through the confirmation, once it has been seen.
	if emptySubj && m.canSuggestSubject() {
		return m.startSubjectSuggestion(true)
	}
	if reason := emptyFieldsReason(emptySubj, emptyBody); reason != "" {
		m.compose.WarnEmptySend = true
		m.compose.WarnEmptyReason = reason
		return m, nil
	}

	return m.executeSendComposeEmail()
}

func (m *tuiModel) executeSendComposeEmail() (tea.Model, tea.Cmd) {
	if m.compose == nil {
		return m, nil
	}

	if m.isAccountReadOnly() {
		m.err = fmt.Errorf("sending is disabled in read-only mode")
		m.showError = true
		return m, nil
	}

	rawBytes, err := m.buildComposeRFC822()
	if err != nil {
		m.err = fmt.Errorf("failed to compose email: %w", err)
		m.showError = true
		return m, nil
	}

	draftPath := m.compose.DraftPath
	targetID := m.compose.OriginalID
	// Where the replied flag goes: the original's account, or for a
	// session that does not know it -- a resumed draft -- the account
	// that sends, never whichever is active when the send completes.
	repliedCache := m.compose.OriginalCache
	if repliedCache.IsZero() {
		repliedCache = accountCacheOf(m.client)
	}
	session := m.composeSession()
	client := m.client
	cfg := m.cfg

	m.compose.sending = true
	m.compose.sendStart = time.Now()
	m.compose.sendEscNoted = false
	m.showLoad = "Sending email..."
	slog.Info("executeSendComposeEmail: dispatching SendEmail")

	// The goroutine uses only what it captured: an F3 switch during the send
	// swaps m.client on the UI goroutine.
	return m, tea.Batch(sendTickCmd(session), func() tea.Msg {
		if err := client.SendEmail(rawBytes); err != nil {
			return emailSentMsg{err: err, targetID: targetID, repliedCache: repliedCache, session: session}
		}
		contacts.RecordSentRecipients(cfg, rawBytes)
		_ = cleanupLocalDraft(draftPath)
		return emailSentMsg{targetID: targetID, repliedCache: repliedCache, session: session}
	})
}

func (m *tuiModel) saveDraftAndExit() (tea.Model, tea.Cmd) {
	return m.saveDraftAndLeave(m.standaloneCompose)
}

// saveDraftAndLeave writes the draft locally and queues its upload to the
// account's Drafts folder. With quit set the program exits once the
// session's server work is done, so the final edits reach the server;
// otherwise the upload finishes behind the index view.
func (m *tuiModel) saveDraftAndLeave(quit bool) (tea.Model, tea.Cmd) {
	if m.compose == nil {
		m.showList()
		if quit {
			return m, m.quitNow()
		}
		return m, nil
	}

	session := m.composeSession()
	path, raw, err := m.saveDraftCopy()
	req := syncRequest{kind: syncUpload, raw: raw, client: m.client, encrypted: m.compose.Encrypt}

	m.showList()
	m.compose = nil
	switch {
	case err == nil:
		m.saveStatus = fmt.Sprintf("Draft saved to %s", path)
	case errors.Is(err, errDraftEmpty):
		m.saveStatus = "Empty message; no draft saved."
		req.client = nil // nothing to upload either
	default:
		m.saveStatus = fmt.Sprintf("Draft NOT saved locally: %v", err)
		if !quit {
			m.err = fmt.Errorf("error saving draft: %w", err)
			m.showError = true
		}
	}
	if raw == nil {
		req.client = nil // nothing was built, so nothing to upload
	}
	m.saveStatusTime = time.Now()

	if quit {
		return m, m.requestQuitAfterSync(session, req)
	}
	return m, m.requestSync(session, req)
}
