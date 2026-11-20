package tui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/mailclient"

	tea "github.com/charmbracelet/bubbletea"
)

// subjectLLM is a chat endpoint that answers every request with reply and
// records the user message it was sent; status 0 means 200.
func subjectLLM(t *testing.T, reply string, status int) (*httptest.Server, *string) {
	t.Helper()
	var userMsg string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct{ Role, Content string } `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		for _, msg := range req.Messages {
			if msg.Role == "user" {
				userMsg = msg.Content
			}
		}
		if status != 0 {
			http.Error(w, "model not loaded", status)
			return
		}
		quoted, _ := json.Marshal(reply)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":` + string(quoted) + `}}]}`))
	}))
	t.Cleanup(server.Close)
	return server, &userMsg
}

// composeWithoutSubject is a draft with a body and an attachment but no
// subject, on a model whose LLM endpoint is url.
func composeWithoutSubject(t *testing.T, url string) (*tuiModel, *mailclient.MockMailClient) {
	t.Helper()
	m, mockClient, _ := setupTestComposeModel(t, false)
	m.cfg.LLM.Endpoint = url
	m.mode = modeCompose
	m.compose = &composeState{
		AccountName: "Personal",
		From:        "user@personal.com",
		To:          "alice@example.com",
		Body:        "Hi Alice, the Q3 numbers are attached.",
		Attachments: []email.Attachment{{Filename: "q3_budget.xlsx", ContentType: "application/octet-stream", Data: []byte("x")}},
	}
	return m, mockClient
}

func TestTUI_Compose_EmptySubjectIsWrittenByLLMThenConfirmed(t *testing.T) {
	server, userMsg := subjectLLM(t, `"Q3 budget numbers attached."`, 0)
	m, mockClient := composeWithoutSubject(t, server.URL)

	m2, cmd := sendComposeKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd == nil {
		t.Fatal("y with an empty subject and an LLM configured: want the suggestion command")
	}
	if m2.compose.WarnEmptySend || !m2.compose.suggestingSubject {
		t.Fatalf("warn=%v suggesting=%v; want the model asked, not the warning", m2.compose.WarnEmptySend, m2.compose.suggestingSubject)
	}

	msg, ok := cmd().(subjectSuggestedMsg)
	if !ok {
		t.Fatalf("command produced %T; want subjectSuggestedMsg", msg)
	}
	if msg.err != nil {
		t.Fatalf("suggestion failed: %v", msg.err)
	}
	for _, want := range []string{"q3_budget.xlsx", "Q3 numbers are attached", "alice@example.com"} {
		if !strings.Contains(*userMsg, want) {
			t.Errorf("the model was not shown %q:\n%s", want, *userMsg)
		}
	}

	m3, _ := m2.Update(msg)
	m2 = m3.(*tuiModel)
	if m2.compose.Subject != "Q3 budget numbers attached" {
		t.Errorf("subject = %q; want the cleaned model line", m2.compose.Subject)
	}
	if !m2.compose.SendConfirm || m2.compose.suggestingSubject || m2.showLoad != "" {
		t.Fatalf("confirm=%v suggesting=%v load=%q; want the send confirmation showing the new subject",
			m2.compose.SendConfirm, m2.compose.suggestingSubject, m2.showLoad)
	}
	if view := m2.renderComposeScreen(); !strings.Contains(view, "Subject (LLM): Q3 budget numbers attached") {
		t.Errorf("confirmation does not show the generated subject:\n%s", view)
	}
	if len(mockClient.SentRawEmails) != 0 {
		t.Fatal("sent before the generated subject was confirmed")
	}

	m4, sendCmd := sendComposeKey(m2, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if sendCmd == nil {
		t.Fatal("confirming: want the send command")
	}
	runCmd(sendCmd)
	if len(mockClient.SentRawEmails) != 1 {
		t.Fatalf("sent %d messages; want 1", len(mockClient.SentRawEmails))
	}
	if raw := string(mockClient.SentRawEmails[0]); !strings.Contains(raw, "Subject: Q3 budget numbers attached") {
		t.Errorf("sent message lacks the generated subject:\n%s", raw)
	}
	_ = m4
}

func TestTUI_Compose_SubjectLLMFailureFallsBackToWarning(t *testing.T) {
	server, _ := subjectLLM(t, "", http.StatusInternalServerError)
	m, mockClient := composeWithoutSubject(t, server.URL)

	m2, cmd := sendComposeKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd == nil {
		t.Fatal("want the suggestion command")
	}
	m3, _ := m2.Update(cmd())
	m2 = m3.(*tuiModel)
	if !m2.compose.WarnEmptySend || !strings.Contains(m2.compose.WarnEmptyReason, "Subject is empty") {
		t.Fatalf("warn=%v reason=%q; want the empty-subject warning", m2.compose.WarnEmptySend, m2.compose.WarnEmptyReason)
	}
	if !strings.Contains(m2.compose.WarnEmptyReason, "model not loaded") {
		t.Errorf("reason does not say why: %q", m2.compose.WarnEmptyReason)
	}
	if m2.compose.Subject != "" || m2.compose.SendConfirm || len(mockClient.SentRawEmails) != 0 {
		t.Errorf("subject=%q confirm=%v sent=%d; a failure must change nothing", m2.compose.Subject, m2.compose.SendConfirm, len(mockClient.SentRawEmails))
	}
}

func TestTUI_Compose_SubjectSuggestionEscCancelsAndLateReplyIsDropped(t *testing.T) {
	server, _ := subjectLLM(t, "Late subject", 0)
	m, _ := composeWithoutSubject(t, server.URL)

	m2, cmd := sendComposeKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	reply := cmd()

	// Any other key is swallowed while the model is writing.
	m3, _ := sendComposeKey(m2, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if m3.compose.FieldEditing != fieldNone || !m3.compose.suggestingSubject {
		t.Fatal("a key reached the review screen while the subject was being written")
	}

	m4, _ := sendComposeKey(m3, tea.KeyMsg{Type: tea.KeyEsc})
	if m4.compose.suggestingSubject || m4.showLoad != "" {
		t.Fatalf("Esc did not cancel: suggesting=%v load=%q", m4.compose.suggestingSubject, m4.showLoad)
	}

	m5, _ := m4.Update(reply)
	if got := m5.(*tuiModel).compose; got.Subject != "" || got.SendConfirm {
		t.Errorf("a reply after Esc was applied: subject=%q confirm=%v", got.Subject, got.SendConfirm)
	}
}

func TestTUI_Compose_SubjectReplyForAnotherSessionIsIgnored(t *testing.T) {
	server, _ := subjectLLM(t, "Other draft", 0)
	m, _ := composeWithoutSubject(t, server.URL)

	m2, _ := sendComposeKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m3, _ := m2.Update(subjectSuggestedMsg{session: "<someone-else@example.com>", subject: "Other draft"})
	got := m3.(*tuiModel).compose
	if got.Subject != "" || !got.suggestingSubject {
		t.Errorf("another session's reply was applied: subject=%q suggesting=%v", got.Subject, got.suggestingSubject)
	}
}

func TestTUI_Compose_AutoSubjectOffKeepsTheWarning(t *testing.T) {
	server, userMsg := subjectLLM(t, "Should not be asked", 0)
	m, _ := composeWithoutSubject(t, server.URL)
	off := false
	m.cfg.LLM.AutoSubject = &off

	m2, cmd := sendComposeKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd != nil || !m2.compose.WarnEmptySend {
		t.Fatalf("cmd=%v warn=%v; with auto_subject off the warning is what an empty subject gets", cmd != nil, m2.compose.WarnEmptySend)
	}
	if *userMsg != "" {
		t.Error("the model was called with auto_subject off")
	}
}

func TestTUI_Compose_NothingToWriteSubjectFromWarnsInstead(t *testing.T) {
	server, userMsg := subjectLLM(t, "Should not be asked", 0)
	m, _ := composeWithoutSubject(t, server.URL)
	m.compose.Body = ""
	m.compose.Attachments = nil

	m2, cmd := sendComposeKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd != nil || !strings.Contains(m2.compose.WarnEmptyReason, "Both Subject and Message Body") {
		t.Fatalf("cmd=%v reason=%q; an empty draft has nothing for the model", cmd != nil, m2.compose.WarnEmptyReason)
	}
	if *userMsg != "" {
		t.Error("the model was called for an empty draft")
	}

	// Attachments alone are something to write about.
	m.compose.Attachments = []email.Attachment{{Filename: "agenda.pdf", Data: []byte("x")}}
	if _, cmd = sendComposeKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}}); cmd == nil {
		t.Fatal("an attachment-only draft should ask the model")
	}
}

func TestTUI_Compose_F4GeneratesSubject(t *testing.T) {
	server, _ := subjectLLM(t, `"Review meeting agenda"`, 0)
	m, _ := composeWithoutSubject(t, server.URL)

	m2, cmd := sendComposeKey(m, tea.KeyMsg{Type: tea.KeyF4})
	if cmd == nil {
		t.Fatal("F4 with an empty subject and an LLM configured: want suggestion command")
	}
	if !m2.compose.suggestingSubject {
		t.Fatal("expected suggestingSubject to be true after F4")
	}
	if m2.compose.subjectForSend {
		t.Error("expected subjectForSend to be false for on-demand F4 generation")
	}

	msg, ok := cmd().(subjectSuggestedMsg)
	if !ok {
		t.Fatalf("command produced %T; want subjectSuggestedMsg", msg)
	}
	if msg.err != nil {
		t.Fatalf("suggestion failed: %v", msg.err)
	}

	m3, _ := m2.Update(msg)
	mFinal := m3.(*tuiModel)
	if mFinal.compose.Subject != "Review meeting agenda" {
		t.Errorf("subject = %q, want 'Review meeting agenda'", mFinal.compose.Subject)
	}
	if mFinal.compose.SendConfirm {
		t.Error("F4 generation should not automatically pop SendConfirm dialog")
	}
	if view := mFinal.renderComposeScreen(); !strings.Contains(view, "Review meeting agenda") {
		t.Errorf("rendered view missing generated subject: %s", view)
	}
}

func TestTUI_Compose_EditorFinishedAutoSuggestsSubject(t *testing.T) {
	server, _ := subjectLLM(t, `"Weekly team sync"`, 0)
	m, _ := composeWithoutSubject(t, server.URL)

	tmp, err := os.CreateTemp("", "compose_body_*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmp.Name())
	_, _ = tmp.WriteString("Let us do our weekly team sync tomorrow.")
	_ = tmp.Close()

	state := &composeState{
		AccountName: "Personal",
		From:        "user@personal.com",
		To:          "team@example.com",
		Subject:     "",
	}
	m2, cmd := m.handleComposeEditorFinished(composeEditorFinishedMsg{
		path:         tmp.Name(),
		initialState: state,
	})
	mRes := m2.(*tuiModel)
	if cmd == nil {
		t.Fatal("expected batch command with sync and subject suggestion")
	}
	if !mRes.compose.suggestingSubject {
		t.Fatal("expected suggestingSubject to be true upon returning from editor")
	}
	if mRes.compose.subjectForSend {
		t.Error("expected subjectForSend to be false upon returning from editor")
	}
}

func TestTUI_Compose_SubjectViewsGeneratingHint(t *testing.T) {
	m, _ := composeWithoutSubject(t, "http://localhost:9999")
	m.compose.suggestingSubject = true
	view := m.renderComposeScreen()
	if !strings.Contains(view, "(generating subject with LLM...)") {
		t.Errorf("view missing generating hint: %s", view)
	}

	m.compose.suggestingSubject = false
	view = m.renderComposeScreen()
	if !strings.Contains(view, "press 's' to edit, F4 for AI") {
		t.Errorf("view missing F4 hint: %s", view)
	}
	if !strings.Contains(view, "[F4]") || !strings.Contains(view, "AI Subj") {
		t.Errorf("footer missing F4 AI Subj: %s", view)
	}
}

func TestTUI_Compose_F4FailureKeepsTypedSubject(t *testing.T) {
	server, _ := subjectLLM(t, "", http.StatusInternalServerError)
	m, _ := composeWithoutSubject(t, server.URL)
	m.compose.Subject = "My own subject"

	m2, cmd := sendComposeKey(m, tea.KeyMsg{Type: tea.KeyF4})
	if cmd == nil {
		t.Fatal("F4: want the suggestion command")
	}
	m3, _ := m2.Update(cmd())
	if got := m3.(*tuiModel).compose.Subject; got != "My own subject" {
		t.Errorf("subject = %q after a failed F4; want the typed one kept", got)
	}
}

func TestTUI_Compose_SendDuringBackgroundSuggestionAsksConfirmation(t *testing.T) {
	server, _ := subjectLLM(t, `"Q3 budget numbers attached."`, 0)
	m, _ := composeWithoutSubject(t, server.URL)

	m2, cmd := m.startSubjectSuggestion(false)
	bg := m2.(*tuiModel)
	if _, again := bg.startSubjectSuggestion(true); again != nil {
		t.Fatal("a send during a pending suggestion must reuse it, not start a second request")
	}
	m3, _ := bg.Update(cmd())
	if !m3.(*tuiModel).compose.SendConfirm {
		t.Error("the reply to a pending suggestion was dropped; want the send confirmation")
	}
}

func TestTUI_Compose_TypedSubjectIsNoLongerGenerated(t *testing.T) {
	m, _ := composeWithoutSubject(t, "http://unused")
	m.compose.SubjectGenerated = true
	m.compose.FieldEditing = fieldSubject
	m.compose.FieldInput = "Retyped"
	m.commitComposeField()
	if m.compose.SubjectGenerated {
		t.Error("SubjectGenerated stays set after the user retyped the subject")
	}
}

// While a message is being sent the screen shows a dialog with the seconds
// elapsed; Esc cannot cancel the send and says so, and the tick keeps running.
func TestTUI_Compose_SendingDialogCountsAndEscSaysNoCancel(t *testing.T) {
	m, mockClient := composeWithoutSubject(t, "http://unused")
	m.compose.Subject = "Hello"
	m.compose.MessageID = "<s@x>"

	m2, cmd := m.executeSendComposeEmail()
	m = m2.(*tuiModel)
	if cmd == nil || !m.compose.sending {
		t.Fatal("want a send in flight")
	}
	m.compose.sendStart = time.Now().Add(-7 * time.Second)
	view := m.renderLoadingOverlay()
	if !strings.Contains(view, "Sending message... 7s") {
		t.Errorf("dialog lacks the elapsed time:\n%s", view)
	}
	if strings.Contains(view, "cannot be canceled") {
		t.Error("the cancel note should appear only after Esc")
	}

	m3, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = m3.(*tuiModel)
	if !m.compose.sending || !strings.Contains(m.renderLoadingOverlay(), "A send cannot be canceled") {
		t.Errorf("Esc must leave the send running and say it cannot be canceled:\n%s", m.renderLoadingOverlay())
	}

	_, tick := m.Update(sendTickMsg{session: m.compose.MessageID})
	if tick == nil {
		t.Error("the tick stopped while the send was in flight")
	}
	m.compose.sending = false
	if _, tick = m.Update(sendTickMsg{session: m.compose.MessageID}); tick != nil {
		t.Error("the tick kept running after the send ended")
	}
	_ = mockClient
}
