package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sarielhp/feral/cfg_g"
)

// subjectServer answers every chat completion with reply and hands the
// decoded request to inspect.
func subjectServer(t *testing.T, reply string, inspect func(chatCompletionRequest)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if inspect != nil {
			inspect(req)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":` + strconvQuote(reply) + `}}]}`))
	}))
}

func strconvQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func TestSuggestSubject_SendsBodyRecipientsAndAttachmentNames(t *testing.T) {
	var got chatCompletionRequest
	server := subjectServer(t, `"Subject: Q3 budget figures for review."`, func(req chatCompletionRequest) { got = req })
	defer server.Close()

	subject, err := SuggestSubject(context.Background(), cfg_g.LLMConfig{Endpoint: server.URL}, SubjectDraft{
		To:          "alice@example.com",
		Body:        "Hi Alice, the Q3 numbers are attached. Please look before Friday.",
		Attachments: []string{"q3_budget.xlsx", "notes.pdf"},
	})
	if err != nil {
		t.Fatalf("SuggestSubject: %v", err)
	}
	if subject != "Q3 budget figures for review" {
		t.Errorf("subject = %q; want the quotes, prefix and period stripped", subject)
	}

	if len(got.Messages) != 2 || got.Messages[0].Role != "system" {
		t.Fatalf("messages = %+v; want a system and a user message", got.Messages)
	}
	if got.Messages[0].Content == defaultSystemPrompt {
		t.Errorf("subject request used the summary prompt")
	}
	user := got.Messages[1].Content
	for _, want := range []string{"alice@example.com", "q3_budget.xlsx", "notes.pdf", "Q3 numbers are attached"} {
		if !strings.Contains(user, want) {
			t.Errorf("user message lacks %q:\n%s", want, user)
		}
	}
	if got.MaxTokens != subjectMaxTokens {
		t.Errorf("max_tokens = %d; want the subject budget %d, not the summary one", got.MaxTokens, subjectMaxTokens)
	}
}

func TestSuggestSubject_IgnoresConfiguredSummaryPrompt(t *testing.T) {
	var got chatCompletionRequest
	server := subjectServer(t, "Lunch on Tuesday", func(req chatCompletionRequest) { got = req })
	defer server.Close()

	cfg := cfg_g.LLMConfig{Endpoint: server.URL, SystemPrompt: "Summarise as haiku."}
	if _, err := SuggestSubject(context.Background(), cfg, SubjectDraft{Body: "Lunch Tuesday?"}); err != nil {
		t.Fatalf("SuggestSubject: %v", err)
	}
	if got.Messages[0].Content != subjectSystemPrompt {
		t.Errorf("system prompt = %q; the summary override must not steer the subject", got.Messages[0].Content)
	}
}

func TestSuggestSubject_RefusesEmptyDraftWithoutCalling(t *testing.T) {
	called := false
	server := subjectServer(t, "x", func(chatCompletionRequest) { called = true })
	defer server.Close()

	_, err := SuggestSubject(context.Background(), cfg_g.LLMConfig{Endpoint: server.URL}, SubjectDraft{Body: "  \n"})
	if err == nil {
		t.Fatal("expected an error for a draft with no body and no attachments")
	}
	if called {
		t.Error("endpoint was called for an empty draft")
	}
}

func TestSuggestSubject_EmptyReplyIsAnError(t *testing.T) {
	server := subjectServer(t, "\n\n\"\"\n", nil)
	defer server.Close()
	if _, err := SuggestSubject(context.Background(), cfg_g.LLMConfig{Endpoint: server.URL}, SubjectDraft{Body: "hi"}); err == nil {
		t.Fatal("expected an error when the reply cleans to nothing")
	}
}

func TestSuggestSubject_NoEndpoint(t *testing.T) {
	if _, err := SuggestSubject(context.Background(), cfg_g.LLMConfig{}, SubjectDraft{Body: "hi"}); err == nil {
		t.Fatal("expected an error without an endpoint")
	}
}

func TestCleanSubjectLine(t *testing.T) {
	long := strings.Repeat("word ", 40)
	tests := []struct {
		name, in, want string
	}{
		{"plain", "Meeting moved to Thursday", "Meeting moved to Thursday"},
		{"quoted", `"Meeting moved to Thursday"`, "Meeting moved to Thursday"},
		{"curly quotes", "“Meeting moved”", "Meeting moved"},
		{"prefix", "Subject: Invoice attached", "Invoice attached"},
		{"prefix case", "SUBJECT:   Invoice attached", "Invoice attached"},
		{"trailing period", "Invoice attached.", "Invoice attached"},
		{"markdown bold", "**Invoice attached**", "Invoice attached"},
		{"first line only", "\n\nInvoice attached\nThis subject summarises...", "Invoice attached"},
		{"collapse whitespace", "Invoice \t  attached   now", "Invoice attached now"},
		{"thinking trace", "<think>\nhmm, invoice\n</think>\nInvoice attached", "Invoice attached"},
		{"empty", "  \n ", ""},
		{"only quotes", `""`, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := cleanSubjectLine(tc.in)
			if got != tc.want {
				t.Errorf("cleanSubjectLine(%q) = %q; want %q", tc.in, got, tc.want)
			}
			// Cleaning is idempotent: a line the model returned already clean
			// is left alone.
			if again := cleanSubjectLine(got); again != got {
				t.Errorf("cleanSubjectLine(%q) again = %q; want it unchanged", got, again)
			}
		})
	}
	if got := cleanSubjectLine(long); len(got) > MaxSubjectLen {
		t.Errorf("long reply not capped: %d chars", len(got))
	}
}
