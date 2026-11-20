package email

import (
	"bytes"
	"fmt"
	"maps"
	"mime"
	"net/mail"
	"strings"
)

// ReplyDetails contains pre-computed headers and quoted body for replying to an email.
type ReplyDetails struct {
	To         string
	Cc         []string
	Subject    string
	InReplyTo  string
	References string
	QuotedBody string
}

// BuildReplyDetails extracts reply headers and quotes the original message.
//
// authored says the original is a message the user wrote -- it was read from a
// Sent folder. A reply to one's own mail goes to the people it was sent to, not
// back to oneself, so authored selects the original To as the target; the
// original Cc becomes the reply's Cc on a group reply. own is the set of the
// user's own addresses, lower-cased (OwnSet); none of them is copied onto the
// Cc of a group reply.
func BuildReplyDetails(orig *mail.Message, own map[string]bool, groupReply, authored bool) ReplyDetails {
	if orig == nil {
		return ReplyDetails{}
	}

	toAddress := ""
	if authored {
		toAddress = strings.TrimSpace(orig.Header.Get("To"))
	}
	if toAddress == "" {
		toAddress = orig.Header.Get("Reply-To")
		if toAddress == "" {
			toAddress = orig.Header.Get("From")
		}
	}
	toAddress = strings.TrimSpace(toAddress)

	dec := new(mime.WordDecoder)
	subject := DecDecode(dec, orig.Header.Get("Subject"))
	subject = formatReplySubject(subject)

	msgID := strings.TrimSpace(orig.Header.Get("Message-ID"))
	origRefs := strings.TrimSpace(orig.Header.Get("References"))
	references := msgID
	if origRefs != "" {
		references = origRefs + " " + msgID
	}

	var cc []string
	switch {
	case !groupReply:
	case authored:
		cc = BuildAuthoredReplyCC(orig, own)
	default:
		cc = BuildGroupReplyCC(orig, toAddress, own)
	}

	return ReplyDetails{
		To:         toAddress,
		Cc:         cc,
		Subject:    subject,
		InReplyTo:  msgID,
		References: references,
		QuotedBody: FormatReplyQuote(orig),
	}
}

// formatReplySubject ensures the subject starts with "Re: ".
func formatReplySubject(subj string) string {
	trimmed := strings.TrimSpace(subj)
	if trimmed == "" {
		return "Re: "
	}
	if strings.HasPrefix(strings.ToLower(trimmed), "re:") {
		return trimmed
	}
	return "Re: " + trimmed
}

// FormatReplyQuote formats the plain-text body of the original message into a standard quote.
func FormatReplyQuote(orig *mail.Message) string {
	bodyText, err := ExtractPlainBodyText(orig)
	if err != nil || strings.TrimSpace(bodyText) == "" {
		return ""
	}

	bodyText = strings.ReplaceAll(bodyText, "\r\n", "\n")
	bodyText = strings.ReplaceAll(bodyText, "\r", "\n")

	dec := new(mime.WordDecoder)
	origFrom := DecDecode(dec, orig.Header.Get("From"))
	origDate := orig.Header.Get("Date")

	var buf bytes.Buffer
	switch {
	case origDate != "" && origFrom != "":
		fmt.Fprintf(&buf, "On %s, %s wrote:\n", origDate, origFrom)
	case origFrom != "":
		fmt.Fprintf(&buf, "%s wrote:\n", origFrom)
	default:
		buf.WriteString("Quoted message:\n")
	}

	lines := strings.Split(strings.TrimSpace(bodyText), "\n")
	for _, line := range lines {
		fmt.Fprintf(&buf, "> %s\n", line)
	}

	return buf.String()
}

// OwnSet returns the lower-cased set of the given addresses, skipping blanks:
// the form of the user's own addresses the reply builders take.
func OwnSet(addrs ...string) map[string]bool {
	own := make(map[string]bool, len(addrs))
	for _, a := range addrs {
		if a = strings.ToLower(strings.TrimSpace(a)); a != "" {
			own[a] = true
		}
	}
	return own
}

// BuildGroupReplyCC is the Cc list for a group reply to received mail: every
// address in the original To and Cc, except the user's own addresses (own,
// lower-cased) and toAddress, which is the reply's To.
func BuildGroupReplyCC(origMsg *mail.Message, toAddress string, own map[string]bool) []string {
	seen := maps.Clone(own)
	if seen == nil {
		seen = map[string]bool{}
	}
	if primaryTo, errTo := mail.ParseAddress(toAddress); errTo == nil && primaryTo != nil {
		seen[strings.ToLower(primaryTo.Address)] = true
	} else if toAddress != "" {
		seen[strings.ToLower(strings.TrimSpace(toAddress))] = true
	}
	var cc []string
	for _, hdr := range []string{origMsg.Header.Get("To"), origMsg.Header.Get("Cc")} {
		cc = append(cc, recipientsExcluding(hdr, seen)...)
	}
	return cc
}

// BuildAuthoredReplyCC is the Cc list for a group reply to a message the user
// wrote: the original Cc, minus the user's own addresses. The original To is
// not copied -- it is the reply's To -- and neither are addresses the user
// owns, which would mail the message back to its sender.
func BuildAuthoredReplyCC(origMsg *mail.Message, own map[string]bool) []string {
	return recipientsExcluding(origMsg.Header.Get("Cc"), maps.Clone(own))
}

// recipientsExcluding parses a header's address list and drops any address in
// seen (lower-cased), preserving the rest as written. Every address kept is
// added to seen, so a caller walking several headers gets each address once;
// seen may be nil.
func recipientsExcluding(header string, seen map[string]bool) []string {
	if header == "" {
		return nil
	}
	addrs, err := mail.ParseAddressList(header)
	if err != nil {
		return nil
	}
	if seen == nil {
		seen = map[string]bool{}
	}
	var out []string
	for _, addr := range addrs {
		key := strings.ToLower(addr.Address)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, addr.String())
	}
	return out
}
