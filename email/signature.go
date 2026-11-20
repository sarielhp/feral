package email

import "strings"

// A signature is the text an account puts under everything it writes. It is
// set off by the conventional delimiter, a line holding "-- " (dash dash
// space), which mail readers use to grey it out and repliers to cut it from
// a quote. Where it goes is decided here, once, for every path that writes a
// message: under the writer's text, above the quoted original of a reply.

// SignatureDelimiter is the line that opens a signature.
const SignatureDelimiter = "-- "

// NormalizeSignature is sig as it is stored and appended: line endings made
// LF, trailing blank lines and trailing spaces removed, and a delimiter line
// the user typed at its head dropped, since SignatureBlock writes its own. A
// signature with no visible text is "".
func NormalizeSignature(sig string) string {
	sig = strings.ReplaceAll(sig, "\r\n", "\n")
	sig = strings.ReplaceAll(sig, "\r", "\n")
	sig = strings.TrimLeft(sig, "\n")
	if first, rest, ok := strings.Cut(sig, "\n"); ok && isDelimiterLine(first) {
		sig = rest
	} else if isDelimiterLine(sig) {
		sig = ""
	}
	sig = strings.TrimRight(sig, " \t\n")
	if strings.TrimSpace(sig) == "" {
		return ""
	}
	return sig
}

// SignatureBlock is the delimiter line followed by sig and a final newline,
// or "" when sig has no visible text.
func SignatureBlock(sig string) string {
	sig = NormalizeSignature(sig)
	if sig == "" {
		return ""
	}
	return SignatureDelimiter + "\n" + sig + "\n"
}

// HasSignatureDelimiter reports whether text holds a line that opens a
// signature: "-- ", or "--" as an editor that strips trailing spaces leaves
// it. A quoted delimiter ("> -- ") is not one.
func HasSignatureDelimiter(text string) bool {
	for line := range strings.SplitSeq(text, "\n") {
		if isDelimiterLine(line) {
			return true
		}
	}
	return false
}

func isDelimiterLine(line string) bool {
	line = strings.TrimRight(line, "\r")
	return line == SignatureDelimiter || line == "--"
}

// SignBody is the body of a message the user writes: text, then a blank line
// and sig's block, then below -- the quoted original of a reply, or "" --
// after another blank line. A text that already carries a delimiter is not
// signed again: it is a body that was signed before, a resumed draft or a
// file the user wrote with a signature of their own. A blank text keeps one
// empty line to write on, above the signature.
func SignBody(text, sig, below string) string {
	body := text
	if block := SignatureBlock(sig); block != "" && !HasSignatureDelimiter(text) {
		head := "\n"
		if strings.TrimSpace(text) != "" {
			head = strings.TrimRight(text, "\n") + "\n"
		}
		body = head + "\n" + block
	}
	if below == "" {
		return body
	}
	if strings.TrimSpace(body) == "" {
		return "\n" + below
	}
	return strings.TrimRight(body, "\n") + "\n\n" + below
}

// ReplaceSignature swaps the block of oldSig in body for newSig's, as when a
// message being written changes the account it is sent from. It acts only
// when the old block appears exactly once, unedited and on lines of its own;
// otherwise the user has changed it, or it cannot be told from quoted text,
// and body is returned as it is. A new account without a signature removes
// the old block; an old one without a signature adds nothing, since there is
// no telling where the user's text ends.
func ReplaceSignature(body, oldSig, newSig string) string {
	oldBlock, newBlock := SignatureBlock(oldSig), SignatureBlock(newSig)
	if oldBlock == "" || oldBlock == newBlock {
		return body
	}
	marked := "\n" + body
	needle := "\n" + oldBlock
	if strings.Count(marked, needle) != 1 {
		return body
	}
	at := strings.Index(marked, needle)
	if newBlock == "" {
		before := strings.TrimRight(marked[:at], "\n")
		after := strings.TrimLeft(marked[at+len(needle):], "\n")
		joined := before + "\n"
		if after != "" {
			joined += "\n" + after
		}
		return strings.TrimPrefix(joined, "\n")
	}
	return strings.TrimPrefix(marked[:at]+"\n"+newBlock+marked[at+len(needle):], "\n")
}
