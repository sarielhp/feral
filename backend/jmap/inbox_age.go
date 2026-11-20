package jmap

import (
	"context"
	"fmt"
	"time"

	mailemail "github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/mailclient"

	"git.sr.ht/~rockorager/go-jmap/mail/email"
)

// InboxMessageBefore is an Email/query for one inbox message received before
// cutoff, then an Email/get for its arrival time and subject. No body.
//
// Every answer short of a clean "no match" is an error: a missing query
// response, a message the server will not describe, or one newer than the
// cutoff would otherwise read as an inbox with nothing old in it.
func (c *client) InboxMessageBefore(ctx context.Context, cutoff time.Time) (mailclient.InboxEntry, bool, error) {
	if err := c.initContext(ctx); err != nil {
		return mailclient.InboxEntry{}, false, err
	}
	inbox, err := c.getMailboxID(c.inboxName())
	if err != nil {
		return mailclient.InboxEntry{}, false, err
	}
	// JMAP dates are whole seconds, and Fastmail reads before as "at or
	// before" -- the spec says strictly before -- so ask for the last whole
	// second that is strictly before the cutoff either way.
	before := cutoff.Add(-time.Nanosecond).Truncate(time.Second).UTC()
	ids, err := c.queryLatestEmailIDs(ctx, &email.FilterCondition{InMailbox: inbox, Before: &before}, 1)
	if err != nil || len(ids) == 0 {
		return mailclient.InboxEntry{}, false, err
	}
	list, err := c.fetchEmailDetailsContext(ctx, ids[:1], []string{"id", "receivedAt", "subject"})
	if err != nil {
		return mailclient.InboxEntry{}, false, err
	}
	if len(list) == 0 || list[0].ReceivedAt == nil {
		return mailclient.InboxEntry{}, false, fmt.Errorf("inbox message %s has no arrival time", ids[0])
	}
	entry := mailclient.InboxEntry{
		ID:       mailemail.ID(list[0].ID),
		Received: *list[0].ReceivedAt,
		Subject:  list[0].Subject,
	}
	if !entry.Received.Before(cutoff) {
		return mailclient.InboxEntry{}, false, fmt.Errorf("inbox query for mail before %s returned mail received %s",
			cutoff.Format(time.RFC3339), entry.Received.Format(time.RFC3339))
	}
	return entry, true, nil
}
