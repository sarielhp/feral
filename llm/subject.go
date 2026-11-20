package llm

import (
	"context"
	"fmt"
	"strings"

	"github.com/sarielhp/feral/cfg_g"
)

// The subject prompt asks for one line and nothing else. A small model still
// tends to quote it, prefix "Subject:" or end with a period, and
// cleanSubjectLine takes those off rather than asking harder.
const (
	subjectSystemPrompt = "You write email subject lines.\n" +
		"Given the draft below, reply with one concise subject line of at most ten words " +
		"that says what the message is about. Name the attached files only when they are the point of the message.\n" +
		"Reply with the subject line only: no quotes, no \"Subject:\" prefix, no explanation."

	// subjectMaxTokens is the reply budget: a subject line is a dozen tokens,
	// and a model that runs on past that is not writing one.
	subjectMaxTokens = 60

	// subjectMaxInputChars is smaller than the summary bound because a subject
	// is decided by the opening of a message: the first pages say what it is
	// about, and the rest is detail.
	subjectMaxInputChars = 12000

	// MaxSubjectLen caps a generated subject so a runaway reply cannot become
	// a paragraph-long header.
	MaxSubjectLen = 120
)

// SubjectDraft is what the subject is written from: who the message goes to,
// its body, and the names of the attached files.
type SubjectDraft struct {
	To          string
	Body        string
	Attachments []string
}

// SuggestSubject asks the model for a subject line for a draft and returns it
// cleaned to a single header-safe line. A draft with neither body nor
// attachments is refused: there is nothing to write a subject about.
func SuggestSubject(ctx context.Context, cfg cfg_g.LLMConfig, d SubjectDraft) (string, error) {
	if strings.TrimSpace(d.Body) == "" && len(d.Attachments) == 0 {
		return "", fmt.Errorf("nothing to write a subject about: the message has no body and no attachments")
	}
	raw, err := complete(ctx, cfg, chatPrompt{
		System:    subjectSystemPrompt,
		User:      formatSubjectDraft(d),
		MaxTokens: subjectMaxTokens,
	})
	if err != nil {
		return "", err
	}
	subject := cleanSubjectLine(raw)
	if subject == "" {
		return "", fmt.Errorf("LLM returned an empty subject line")
	}
	return subject, nil
}

// formatSubjectDraft lays the draft out the way a person would read it, so
// the model sees recipients and attachments as context rather than as text
// to summarise.
func formatSubjectDraft(d SubjectDraft) string {
	var sb strings.Builder
	if to := strings.TrimSpace(d.To); to != "" {
		fmt.Fprintf(&sb, "To: %s\n", to)
	}
	if len(d.Attachments) > 0 {
		fmt.Fprintf(&sb, "Attachments: %s\n", strings.Join(d.Attachments, ", "))
	}
	sb.WriteString("\n")
	sb.WriteString(TruncateForModel(d.Body, subjectMaxInputChars))
	return sb.String()
}

// cleanSubjectLine turns a model reply into a subject: the first non-empty
// line, with any "Subject:" prefix, surrounding quotes, markdown emphasis and
// trailing period removed, whitespace collapsed, and the length capped.
func cleanSubjectLine(raw string) string {
	line := firstNonEmptyLine(raw)
	line = strings.TrimSpace(line)
	line = trimWrapping(line)
	if strings.HasPrefix(strings.ToLower(line), "subject:") {
		line = trimWrapping(line[len("subject:"):])
	}
	line = strings.TrimRight(strings.TrimSpace(line), ".")
	line = strings.Join(strings.Fields(line), " ")
	if len(line) > MaxSubjectLen {
		line = strings.TrimSpace(truncateFront(line, MaxSubjectLen))
	}
	return line
}

// trimWrapping removes the quotes and markdown emphasis a model wraps a line in.
func trimWrapping(line string) string {
	return strings.TrimSpace(strings.Trim(strings.TrimSpace(line), "*_`\"'“”‘’"))
}

// firstNonEmptyLine skips the blank lines and a thinking trace's leftovers at
// the head of a reply; a `</think>` block is dropped whole when present.
func firstNonEmptyLine(raw string) string {
	if i := strings.LastIndex(raw, "</think>"); i >= 0 {
		raw = raw[i+len("</think>"):]
	}
	for _, line := range strings.Split(raw, "\n") {
		if strings.TrimSpace(line) != "" {
			return line
		}
	}
	return ""
}
