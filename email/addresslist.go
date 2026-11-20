package email

import (
	"mime"
	"net/mail"
	"strings"
)

// addressSpecials are the characters that make a display name unusable as a
// bare phrase in an address (RFC 5322 §3.2.3), so the name must be quoted.
// The period is left out: "Harvest H." is common and every parser in use
// accepts it unquoted.
const addressSpecials = `()<>[]:;@\,"`

// FormatAddress is a recipient as a person types it: `Name <addr>`, the name
// quoted when it holds a special such as the comma in "Doe, Alice", or the
// bare address when there is no name. Non-ASCII names stay as written; the
// backend encodes headers when the message is sent.
func FormatAddress(name, addr string) string {
	name = strings.TrimSpace(name)
	if addr == "" {
		return ""
	}
	if name == "" {
		return addr
	}
	if strings.ContainsAny(name, addressSpecials) {
		name = `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(name) + `"`
	}
	return name + " <" + addr + ">"
}

// SplitAddressList splits a comma-separated recipient field into its trimmed,
// non-empty entries. A comma inside a quoted name or inside angle brackets is
// part of its entry, so `"Doe, Alice" <a@x>` stays one recipient.
func SplitAddressList(list string) []string {
	var out []string
	start := 0
	for _, sep := range addressSeparators(list) {
		out = appendEntry(out, list[start:sep])
		start = sep + 1
	}
	return appendEntry(out, list[start:])
}

// LastAddressStart is the byte offset where the last entry of a recipient
// field begins: just past the last separating comma, or 0.
func LastAddressStart(list string) int {
	seps := addressSeparators(list)
	if len(seps) == 0 {
		return 0
	}
	return seps[len(seps)-1] + 1
}

// addressSeparators are the offsets of the commas that separate entries:
// those outside a quoted name and outside angle brackets.
func addressSeparators(list string) []int {
	var seps []int
	quoted, angle := false, false
	for i := 0; i < len(list); i++ {
		switch c := list[i]; {
		case quoted && c == '\\':
			i++ // an escaped character inside quotes
		case c == '"':
			quoted = !quoted
		case quoted:
		case c == '<':
			angle = true
		case c == '>':
			angle = false
		case c == ',' && !angle:
			seps = append(seps, i)
		}
	}
	return seps
}

func appendEntry(out []string, entry string) []string {
	if entry = strings.TrimSpace(entry); entry != "" {
		out = append(out, entry)
	}
	return out
}

// ParsedRecipient represents a parsed email address with name and address parts.
type ParsedRecipient struct {
	Name    string
	Address string
}

// ParseRecipientList parses a header containing one or more email addresses,
// decoding MIME-encoded words and handling malformed entries gracefully.
func ParseRecipientList(dec *mime.WordDecoder, hdr string) []ParsedRecipient {
	hdr = strings.TrimSpace(hdr)
	if hdr == "" {
		return nil
	}
	decoded := DecodeHeader(dec, hdr)
	addrs, err := mail.ParseAddressList(decoded)
	if err == nil {
		out := make([]ParsedRecipient, 0, len(addrs))
		for _, a := range addrs {
			out = append(out, ParsedRecipient{
				Name:    strings.TrimSpace(a.Name),
				Address: strings.TrimSpace(a.Address),
			})
		}
		return out
	}
	return parseRecipientListFallback(decoded)
}

func parseRecipientListFallback(decoded string) []ParsedRecipient {
	var out []ParsedRecipient
	for _, entry := range SplitAddressList(decoded) {
		if a, err := mail.ParseAddress(entry); err == nil && a != nil {
			out = append(out, ParsedRecipient{
				Name:    strings.TrimSpace(a.Name),
				Address: strings.TrimSpace(a.Address),
			})
			continue
		}
		name, addr := extractFallbackNameAndAddr(entry)
		out = append(out, ParsedRecipient{Name: name, Address: addr})
	}
	return out
}

func extractFallbackNameAndAddr(entry string) (string, string) {
	entry = strings.TrimSpace(entry)
	start := strings.Index(entry, "<")
	end := strings.Index(entry, ">")
	if start != -1 && end > start {
		name := strings.Trim(strings.TrimSpace(entry[:start]), `"`)
		addr := strings.TrimSpace(entry[start+1 : end])
		return name, addr
	}
	return "", entry
}

// FormatAddressListNames formats recipient list hdr (e.g. CC/BCC) by showing
// only the display name of each person when available, falling back to their
// email address only when no name is present.
func FormatAddressListNames(dec *mime.WordDecoder, hdr string) string {
	recipients := ParseRecipientList(dec, hdr)
	if len(recipients) == 0 {
		return ""
	}
	names := make([]string, 0, len(recipients))
	for _, r := range recipients {
		if r.Name != "" {
			names = append(names, r.Name)
		} else if r.Address != "" {
			names = append(names, r.Address)
		}
	}
	return strings.Join(names, ", ")
}

// FormatAddressListVerbose formats recipient list hdr (e.g. To) showing both
// the display name and email address for each person (Name <email>, or just email).
func FormatAddressListVerbose(dec *mime.WordDecoder, hdr string) string {
	recipients := ParseRecipientList(dec, hdr)
	if len(recipients) == 0 {
		return ""
	}
	items := make([]string, 0, len(recipients))
	for _, r := range recipients {
		switch {
		case r.Name != "" && r.Address != "":
			items = append(items, r.Name+" <"+r.Address+">")
		case r.Name != "":
			items = append(items, r.Name)
		case r.Address != "":
			items = append(items, r.Address)
		}
	}
	return strings.Join(items, ", ")
}
