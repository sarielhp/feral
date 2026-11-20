package email

import (
	"mime"
	"regexp"
	"strings"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/htmlindex"
	"golang.org/x/text/encoding/ianaindex"
)

// metaCharsetRE matches the charset an HTML document declares for itself,
// in either form: <meta charset="x"> or the http-equiv Content-Type one.
var metaCharsetRE = regexp.MustCompile(`(?i)(<meta\b[^>]*?charset\s*=\s*["']?)([a-z0-9_:.+-]+)`)

// TextToUTF8 converts a text part's payload, already stripped of its transfer
// encoding, from the charset its Content-Type declares to UTF-8. An HTML part
// with no charset in its header falls back to the one its <meta> declares, and
// any such declaration is rewritten to utf-8, so a browser or w3m reading the
// converted document does not decode it a second time as the old charset.
//
// Bodies used to be taken as UTF-8 whatever they declared, so a message in
// windows-1255 or iso-8859-8 -- most Hebrew mail sent from Outlook -- showed
// as replacement characters.
func TextToUTF8(data []byte, contentType string) []byte {
	mediaType, params, _ := mime.ParseMediaType(contentType)
	charset := params["charset"]
	if mediaType != "text/html" {
		return convertCharset(data, charset)
	}
	if charset == "" {
		if m := metaCharsetRE.FindSubmatch(data); m != nil {
			charset = string(m[2])
		}
	}
	enc := lookupCharset(charset)
	if enc == nil {
		return data
	}
	out, err := enc.NewDecoder().Bytes(data)
	if err != nil {
		return data
	}
	return metaCharsetRE.ReplaceAll(out, []byte("${1}utf-8"))
}

// convertCharset decodes data from charset; an unknown or UTF-8 charset, or
// one that fails to decode, leaves the bytes as they were.
func convertCharset(data []byte, charset string) []byte {
	enc := lookupCharset(charset)
	if enc == nil {
		return data
	}
	out, err := enc.NewDecoder().Bytes(data)
	if err != nil {
		return data
	}
	return out
}

// lookupCharset resolves a charset label, or returns nil when no conversion
// is needed or possible. The WHATWG labels come first because they are what
// mail clients actually emit (and read iso-8859-1 as windows-1252, as every
// browser does); the IANA registry covers the rest.
func lookupCharset(charset string) encoding.Encoding {
	label := strings.ToLower(strings.Trim(strings.TrimSpace(charset), `"'`))
	switch label {
	case "", "utf-8", "utf8", "us-ascii", "ascii":
		return nil
	}
	if enc, err := htmlindex.Get(label); err == nil && enc != nil {
		return enc
	}
	if enc, err := ianaindex.MIME.Encoding(label); err == nil && enc != nil {
		return enc
	}
	return nil
}
