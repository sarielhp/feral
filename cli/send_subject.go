package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/sarielhp/feral/app"
	"github.com/sarielhp/feral/llm"
)

// sendSubjectTimeout bounds the wait for a generated subject so a send does
// not hang on an endpoint that is configured but not answering.
const sendSubjectTimeout = 45 * time.Second

// fillSubjectFromLLM writes the subject an omitted -s left empty, from the
// body and the attachment names, when the config allows it. The line is
// printed so the user sees what went out. A failure is a warning and the
// message is sent without a subject, as it was before.
func fillSubjectFromLLM(session *Session, opts *SendOptions) {
	if strings.TrimSpace(opts.Subject) != "" || session == nil || session.Config == nil {
		return
	}
	if !session.Config.LLM.IsAutoSubjectEnabled() {
		return
	}
	// Nothing is sent under --read-only or --dry-run, so the body need not go
	// to the LLM either: it used to, before the send was refused.
	if session.Config.ReadOnly {
		fmt.Fprintf(session.errOut(), "%s No subject given; not asking the LLM for one under --read-only/--dry-run\n", app.PrefixInfo)
		return
	}
	if strings.TrimSpace(opts.Body) == "" && len(opts.Attachments) == 0 {
		return
	}

	names := make([]string, 0, len(opts.Attachments))
	for _, path := range opts.Attachments {
		names = append(names, filepath.Base(path))
	}
	ctx, cancel := context.WithTimeout(context.Background(), sendSubjectTimeout)
	defer cancel()
	subject, err := llm.SuggestSubject(ctx, session.Config.LLM, llm.SubjectDraft{
		To:          strings.Join(opts.To, ", "),
		Body:        opts.Body,
		Attachments: names,
	})
	if err != nil {
		fmt.Fprintf(session.errOut(), "%s No subject given, and the LLM could not write one: %v\n", app.PrefixWarn, err)
		return
	}
	opts.Subject = subject
	fmt.Fprintf(session.out(), "Subject (written by the LLM): %s\n", subject)
}
