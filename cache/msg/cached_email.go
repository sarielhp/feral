package msg

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/mail"
	"os"
	"time"

	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/email"
)

// CachedEmail parses a cached message into a email.Email, filling in what the
// cache index knows on top of the headers: the classification flags, and a date
// for messages whose own Date header is missing or unparsable.
//
// This was open-coded three times -- in ui, in last, and inline in the TUI's
// load command -- with the copies having drifted apart. It lived in uicommon,
// which made the mbox backend import a UI package to read its own cache; there
// is nothing presentational in it. It returns an error
// when the message is not cached or does not parse, because the callers
// genuinely disagree about what that means: a folder listing shows an error
// row in place of the message, while the TUI leaves it out. That decision
// stays with them, as does whether to reverse Hebrew runs in the subject, which
// the two listing paths do and the TUI does not.
func CachedEmail(account cachedir.Account, id email.ID) (*email.Email, error) {
	return cachedEmail(account, id, false)
}

// CachedEmailRecipient is CachedEmail for a listing that shows the recipient
// rather than the sender -- a Sent or Drafts folder. A summary written before
// Email carried the To header has none, and such a row would show a blank
// recipient, so it is re-parsed once and rewritten instead of returned as is.
// Only authored folders pay for this; everything else accepts an old summary,
// which is why the check is not applied in CachedEmail.
func CachedEmailRecipient(account cachedir.Account, id email.ID) (*email.Email, error) {
	return cachedEmail(account, id, true)
}

func cachedEmail(account cachedir.Account, id email.ID, needRecipient bool) (*email.Email, error) {
	downloadDir := account.Path()
	if data, info, err := ReadParsedSummaryAndInfo(account, id); err == nil {
		var parsed email.Email
		if err := json.Unmarshal(data, &parsed); err == nil && (!needRecipient || summaryHasRecipient(data)) {
			if !summaryHasListID(data) {
				addListID(account, id, data, &parsed)
			}
			return applyInfo(info, &parsed), nil
		}
	}

	var parsed *email.Email
	readErr := WithReader(account, id, func(r io.Reader) error {
		parsed = email.ParseReader(r, id, "")
		return nil
	})
	if readErr != nil {
		return nil, fmt.Errorf("read cached message %s: %w", id, readErr)
	}
	if parsed == nil {
		return nil, fmt.Errorf("parse cached message %s: malformed MIME", id)
	}
	if p, err := lookupPath(downloadDir, id); err == nil {
		if st, err := os.Stat(p); err == nil {
			parsed.Size = st.Size()
			if parsed.DateReceived.IsZero() {
				parsed.DateReceived = st.ModTime()
			}
		}
	}

	if err := storeCachedSummary(downloadDir, id, parsed); err != nil {
		slog.Warn("failed to persist parsed summary cache", slog.String("id", string(id)), slog.Any("error", err))
	}
	return applyCachedInfo(downloadDir, id, parsed), nil
}

// summaryHasRecipient reports whether a stored parsed summary carries the To
// header. It is the cache's schema marker: Email marshals its fields under
// their Go names and no field is omitempty, so a summary written by a binary
// that predates ToRaw has no "ToRaw" key at all, while a current one always
// has it -- even for a message with no To, where the value is "".
func summaryHasRecipient(data []byte) bool {
	var probe struct {
		ToRaw *string `json:"ToRaw"`
	}
	return json.Unmarshal(data, &probe) == nil && probe.ToRaw != nil
}

// summaryHasListID reports whether a stored parsed summary carries ListID, the
// mailing list the message came through. A summary written before Email had
// the field has no "ListID" key; addListID fills it in.
func summaryHasListID(data []byte) bool {
	var probe struct {
		ListID *string `json:"ListID"`
	}
	return json.Unmarshal(data, &probe) == nil && probe.ListID != nil
}

// addListID gives a summary written before ListID existed its list, read
// from the cached message's header alone -- net/mail stops at the blank line,
// so the body is never read -- and stores the summary back, so each old
// summary pays this once. Only the header is consulted: the summary's other
// fields are left as they were, which is what the sender-showing listings
// accept, and the stored summary gains that one key and nothing else: writing
// the whole struct back would add an empty ToRaw, which the Sent-folder path
// would take for a parsed recipient. A message that cannot be read keeps its
// summary without a list.
func addListID(account cachedir.Account, id email.ID, data []byte, parsed *email.Email) {
	err := WithReader(account, id, func(r io.Reader) error {
		m, err := mail.ReadMessage(r)
		if err != nil {
			return err
		}
		parsed.ListID = email.ParseListID(m.Header.Get("List-Id"))
		return nil
	})
	if err != nil {
		slog.Debug("cannot read the list of a cached message", slog.String("id", string(id)), slog.Any("error", err))
		return
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil {
		return
	}
	listID, err := json.Marshal(parsed.ListID)
	if err != nil {
		return
	}
	fields["ListID"] = listID
	patched, err := json.Marshal(fields)
	if err != nil {
		return
	}
	if err := StoreParsedSummary(account, id, patched); err != nil {
		slog.Warn("failed to persist parsed summary cache", slog.String("id", string(id)), slog.Any("error", err))
	}
}

func storeCachedSummary(downloadDir string, id email.ID, parsed *email.Email) error {
	data, err := json.Marshal(parsed)
	if err != nil {
		return err
	}
	return StoreParsedSummary(cachedir.At(downloadDir), id, data)
}

func applyInfo(info *Info, parsed *email.Email) *email.Email {
	if parsed.EmailDate.IsZero() {
		switch {
		case info != nil && !info.Date.IsZero():
			parsed.EmailDate = info.Date
		default:
			parsed.EmailDate = time.Now()
		}
		parsed.FormattedDate = email.FormatEmailDate(parsed.EmailDate)
	}
	if parsed.DateReceived.IsZero() {
		parsed.DateReceived = parsed.EmailDate
	}
	if info != nil {
		parsed.IsSpam = info.IsSpam()
		parsed.IsPolitical = info.IsPolitical()
		parsed.IsBlacklisted = info.IsBlacklisted()
	}
	return parsed
}

func applyCachedInfo(downloadDir string, id email.ID, parsed *email.Email) *email.Email {
	info, infoErr := GetInfo(cachedir.At(downloadDir), id)
	if infoErr != nil {
		info = nil
	}
	return applyInfo(info, parsed)
}
