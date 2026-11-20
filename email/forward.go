package email

import (
	"bytes"
	"fmt"
	"mime"
	"net/mail"
	"strings"
)

// A forwarded message is rendered into the body of the new one: a separator
// that says what follows, the original's identifying headers, then its full
// text. Its attachments are re-attached by the compose layer, not carried in
// this block.

const (
	forwardSeparatorTop = "---------- Forwarded email ----------"
	forwardSeparatorEnd = "---------- End of forwarded email ----------"
)

// FormatForwardSubject prefixes subj with "Fwd: " unless it already carries a
// forward marker, so forwarding a forward does not stack markers.
func FormatForwardSubject(subj string) string {
	trimmed := strings.TrimSpace(subj)
	if trimmed == "" {
		return "Fwd: "
	}
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "fwd:") || strings.HasPrefix(lower, "fw:") {
		return trimmed
	}
	return "Fwd: " + trimmed
}

// FormatForward renders the forwarded block for the body of a new message:
// the separator, the original's identifying headers, and its text.
func FormatForward(orig *mail.Message, body string) string {
	dec := new(mime.WordDecoder)
	var buf bytes.Buffer
	buf.WriteString(forwardSeparatorTop + "\n")
	writeForwardHeader(&buf, dec, "From", orig.Header.Get("From"))
	writeForwardHeader(&buf, dec, "Date", orig.Header.Get("Date"))
	writeForwardHeader(&buf, dec, "To", orig.Header.Get("To"))
	writeForwardHeader(&buf, dec, "Cc", orig.Header.Get("Cc"))
	writeForwardHeader(&buf, dec, "Subject", orig.Header.Get("Subject"))
	buf.WriteString("\n")
	writeForwardBody(&buf, body)
	buf.WriteString(forwardSeparatorEnd + "\n")
	return buf.String()
}

// writeForwardHeader writes one header of the original, skipping those it
// does not carry.
func writeForwardHeader(buf *bytes.Buffer, dec *mime.WordDecoder, name, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	fmt.Fprintf(buf, "%s: %s\n", name, DecDecode(dec, value))
}

// writeForwardBody writes the original's text with its line endings
// normalized, on a newline of its own so what follows starts on a fresh line.
func writeForwardBody(buf *bytes.Buffer, body string) {
	if strings.TrimSpace(body) == "" {
		return
	}
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")
	buf.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		buf.WriteString("\n")
	}
}
