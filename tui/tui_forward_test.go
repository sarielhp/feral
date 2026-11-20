package tui

import (
	"os"
	"strings"
	"testing"

	"github.com/sarielhp/feral/email"
)

// forwardRawMessage is a message with a text part and one attachment, as
// GetRawMessage would return it.
func forwardRawMessage() []byte {
	return []byte("From: Alice <alice@example.com>\r\n" +
		"To: user@personal.com\r\n" +
		"Cc: Carol <carol@example.com>\r\n" +
		"Subject: Lunch on Friday\r\n" +
		"Date: Sun, 11 Oct 2026 12:00:00 +0000\r\n" +
		"Message-ID: <orig-123@example.com>\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/mixed; boundary=\"BOUND\"\r\n" +
		"\r\n" +
		"--BOUND\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"\r\n" +
		"Are we still on for Friday?\r\n" +
		"--BOUND\r\n" +
		"Content-Type: application/pdf\r\n" +
		"Content-Disposition: attachment; filename=\"menu.pdf\"\r\n" +
		"Content-Transfer-Encoding: base64\r\n" +
		"\r\n" +
		"aGVsbG8=\r\n" +
		"--BOUND--\r\n")
}

func TestTUI_PrepareForward(t *testing.T) {
	m, _, _ := setupTestComposeModel(t, false)

	prepared, err := m.prepareForward(forwardRawMessage())
	if err != nil {
		t.Fatal(err)
	}

	state := prepared.state
	if !state.IsForward {
		t.Error("a forward session must be marked as one")
	}
	if state.Subject != "Fwd: Lunch on Friday" {
		t.Errorf("Subject = %q, want %q", state.Subject, "Fwd: Lunch on Friday")
	}
	// A forward is a new message, not a reply: no threading, and the
	// original is not marked answered.
	if state.OriginalID != "" || state.InReplyTo != "" || state.References != "" {
		t.Errorf("a forward carries no threading headers: %+v", state)
	}
	if !strings.Contains(state.From, "user@personal.com") {
		t.Errorf("From = %q, want the active account's address", state.From)
	}
	if len(state.Attachments) != 1 || state.Attachments[0].Filename != "menu.pdf" {
		t.Fatalf("attachments = %+v, want the original's menu.pdf", state.Attachments)
	}
	if string(state.Attachments[0].Data) != "hello" {
		t.Errorf("attachment data = %q, want the decoded base64 body", state.Attachments[0].Data)
	}
	if !strings.Contains(prepared.body, "Are we still on for Friday?") {
		t.Errorf("body = %q, want the original's text", prepared.body)
	}
}

func TestTUI_ForwardTemplate(t *testing.T) {
	m, _, _ := setupTestComposeModel(t, false)
	prepared, err := m.prepareForward(forwardRawMessage())
	if err != nil {
		t.Fatal(err)
	}

	tempPath, _, err := m.createForwardBodyTemplate(prepared.state.AccountName, prepared.orig, prepared.body)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tempPath)

	data, err := os.ReadFile(tempPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	// The template is the text the editor edits: the original's routing
	// noise has no place in it, only the header block the reader needs.
	for _, h := range []string{"MIME-Version:", "Content-Type:", "Message-ID:", "In-Reply-To:", "References:"} {
		if strings.Contains(content, h) {
			t.Errorf("forbidden raw header %q found in forward editor buffer:\n%s", h, content)
		}
	}
	for _, want := range []string{
		"---------- Forwarded email ----------",
		"From: Alice <alice@example.com>",
		"To: user@personal.com",
		"Cc: Carol <carol@example.com>",
		"Subject: Lunch on Friday",
		"Are we still on for Friday?",
		"---------- End of forwarded email ----------",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("forward editor buffer lost %q:\n%s", want, content)
		}
	}
}

// A forward's template is the message itself: an editor quit without a
// change still leaves a finished forward, where an untouched reply is a
// cancelled one.
func TestForwardUntouchedTemplateKeepsCompose(t *testing.T) {
	m, _, _ := setupTestComposeModel(t, false)
	prepared, err := m.prepareForward(forwardRawMessage())
	if err != nil {
		t.Fatal(err)
	}
	tempPath, templateContent, err := m.createForwardBodyTemplate(prepared.state.AccountName, prepared.orig, prepared.body)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tempPath)
	prepared.state.InitialTemplate = templateContent
	if err := os.WriteFile(tempPath, []byte(templateContent), 0600); err != nil {
		t.Fatal(err)
	}

	m2, _ := m.handleComposeEditorFinished(composeEditorFinishedMsg{path: tempPath, initialState: prepared.state})
	m = m2.(*tuiModel)

	if m.compose == nil || m.mode != modeCompose {
		t.Fatalf("an untouched forward was cancelled: compose=%v mode=%d", m.compose != nil, m.mode)
	}
	if !m.compose.IsForward {
		t.Error("the session lost its forward mark")
	}
	if len(m.compose.Attachments) != 1 || m.compose.Attachments[0].Filename != "menu.pdf" {
		t.Errorf("the forward's attachments were lost: %+v", m.compose.Attachments)
	}
}

// b opens a forward of the current message, from the list and while reading.
func TestForwardKeyBinding(t *testing.T) {
	m, mock, _ := setupTestComposeModel(t, false)
	const id = email.ID("fwd-1")
	mock.RawMessages = map[email.ID][]byte{id: forwardRawMessage()}
	m.emails = []email.Email{{ID: id, Subject: "Lunch on Friday"}}
	m.eIdx = 0
	m.mode = modeIndex

	if cmd := press(m, "b"); cmd == nil {
		t.Fatal("b in the list did not start a forward")
	}
	// z is undo on night/undo and H snoozes, so the unbound contrast key is Z.
	if cmd := press(m, "Z"); cmd != nil || m.keyScreen() != scrList {
		t.Fatal("Z is bound to something; the contrast key for this test is not")
	}

	m.mode = modeDetail
	m.selectedID = id
	if cmd := press(m, "b"); cmd == nil {
		t.Fatal("b while reading did not start a forward")
	}
}
