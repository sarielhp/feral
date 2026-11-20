// Package inboxage checks relay accounts: accounts whose inbox something else
// empties on a schedule, such as a fetcher that moves their mail into another
// account every few minutes. Mail that sits in such an inbox longer than the
// account's inbox_max_age means the fetcher has stopped -- typically because
// it needs to sign in again -- and nothing else would say so.
//
// A check is one question to the server: is anything older than the limit
// still in the inbox. It reads the server's arrival time, never the sender's
// Date header, and downloads no body. The watcher, the TUI and
// `account stale` all ask it the same way.
package inboxage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/mailclient"
)

// MinMaxAge is the shortest limit accepted. Anything shorter would flag mail
// the fetcher has simply not reached yet.
const MinMaxAge = time.Minute

// CheckTimeout bounds one account's check, so a hung server cannot stall the
// loop that runs it.
const CheckTimeout = 30 * time.Second

// ErrBadMaxAge reports an inbox_max_age that is not a usable duration.
var ErrBadMaxAge = errors.New("inbox_max_age must be a duration of at least 1m, such as 15m")

// ParseMaxAge reads an inbox_max_age value. Empty means the check is off and
// returns 0 with no error.
func ParseMaxAge(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < MinMaxAge {
		return 0, fmt.Errorf("%w: got %q", ErrBadMaxAge, s)
	}
	return d, nil
}

// Relays returns the accounts that have inbox_max_age set, in config order.
// An mbox account is never one: nothing empties a local file on a schedule.
// An unparsable value still counts: Check reports it, rather than the
// account silently going unwatched.
func Relays(accounts []cfg_acc.AccountConfig) []cfg_acc.AccountConfig {
	var out []cfg_acc.AccountConfig
	for _, acc := range accounts {
		if acc.InboxMaxAge != "" && !acc.IsMbox() {
			out = append(out, acc)
		}
	}
	return out
}

// Status is one account's answer.
type Status struct {
	Account string
	MaxAge  time.Duration
	// Stale is true when mail older than MaxAge is in the inbox; Entry is
	// one such message.
	Stale bool
	Entry mailclient.InboxEntry
	// Err is set when the check could not be made at all. It is not "fine":
	// a relay that cannot be checked is as unknown as one that is stuck.
	Err error
}

// OK reports whether the account was checked and found drained.
func (s Status) OK() bool { return s.Err == nil && !s.Stale }

// Check asks the account's server whether its inbox holds mail older than
// the account's inbox_max_age.
func Check(ctx context.Context, client mailclient.Reader, acc cfg_acc.AccountConfig, now time.Time) Status {
	st := Status{Account: acc.Name}
	maxAge, err := ParseMaxAge(acc.InboxMaxAge)
	if err != nil {
		st.Err = err
		return st
	}
	st.MaxAge = maxAge
	if maxAge == 0 {
		return st
	}
	ctx, cancel := context.WithTimeout(ctx, CheckTimeout)
	defer cancel()
	entry, found, err := client.InboxMessageBefore(ctx, now.Add(-maxAge))
	if err != nil {
		st.Err = err
		return st
	}
	entry.Subject = displaySubject(entry.Subject)
	st.Stale, st.Entry = found, entry
	return st
}

// Summary is the one-line description of a status: what is wrong and since
// when, or that the inbox is drained.
func (s Status) Summary(now time.Time) string {
	switch {
	case s.Err != nil:
		return fmt.Sprintf("%s: cannot check the inbox: %v", s.Account, s.Err)
	case s.Stale:
		line := fmt.Sprintf("%s: inbox has mail at least %s old (received %s) -- has the fetcher stopped?",
			s.Account, age(now.Sub(s.Entry.Received)), s.Entry.Received.Local().Format("Jan 2 15:04"))
		if s.Entry.Subject != "" {
			line += fmt.Sprintf(" [%s]", s.Entry.Subject)
		}
		return line
	default:
		return fmt.Sprintf("%s: inbox drained (nothing older than %s)", s.Account, age(s.MaxAge))
	}
}

// age renders a duration in whole minutes, or hours and minutes.
func age(d time.Duration) string {
	d = d.Truncate(time.Minute)
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
}

// subjectLineBreaks folds a subject onto the one line every caller prints.
var subjectLineBreaks = strings.NewReplacer("\r", " ", "\n", " ", "\t", " ")

// displaySubject makes a sender-written subject safe for a terminal line:
// RFC 2047 words decoded (Gmail returns the raw header; decoding a decoded
// subject changes nothing), escape sequences removed, one line, and
// right-to-left text in visual order.
func displaySubject(s string) string {
	return email.BidiDisplay(subjectLineBreaks.Replace(email.DecodeHeader(nil, s)))
}
