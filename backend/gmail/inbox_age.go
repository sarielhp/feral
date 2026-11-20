package gmail

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/mailclient"
)

// InboxMessageBefore asks Gmail for one inbox message received before cutoff,
// then reads its arrival time and subject: two requests, no body.
//
// The cutoff goes in as epoch seconds. That is the only form of Gmail's
// before: that resolves below a day; the date form would match nothing until
// midnight. Gmail lists newest first, so the message found is the newest of
// those too old, not the oldest.
func (c *client) InboxMessageBefore(ctx context.Context, cutoff time.Time) (mailclient.InboxEntry, bool, error) {
	srv, err := GetGmailServiceContext(ctx, c.rep, c.config)
	if err != nil {
		return mailclient.InboxEntry{}, false, err
	}
	res, err := srv.Users.Messages.List("me").LabelIds("INBOX").
		Q(fmt.Sprintf("before:%d", cutoff.Unix())).MaxResults(1).
		Fields("messages/id").Context(ctx).Do()
	if err != nil {
		return mailclient.InboxEntry{}, false, fmt.Errorf("search inbox: %w", err)
	}
	if len(res.Messages) == 0 {
		return mailclient.InboxEntry{}, false, nil
	}
	msg, err := srv.Users.Messages.Get("me", res.Messages[0].Id).Format("metadata").
		MetadataHeaders("Subject").Fields("id,internalDate,payload/headers").Context(ctx).Do()
	if err != nil {
		return mailclient.InboxEntry{}, false, fmt.Errorf("read inbox message: %w", err)
	}
	entry := mailclient.InboxEntry{ID: email.ID(msg.Id), Received: time.UnixMilli(msg.InternalDate)}
	if msg.Payload != nil {
		for _, h := range msg.Payload.Headers {
			if strings.EqualFold(h.Name, "Subject") {
				entry.Subject = h.Value
			}
		}
	}
	// The search is floored to the second, so a match is always older than the
	// cutoff; one that is not means the server ignored the filter, and taking
	// it as "nothing old" would hide a stale inbox.
	if !entry.Received.Before(cutoff) {
		return mailclient.InboxEntry{}, false, fmt.Errorf("inbox search for mail before %s returned mail received %s",
			cutoff.Format(time.RFC3339), entry.Received.Format(time.RFC3339))
	}
	return entry, true, nil
}
