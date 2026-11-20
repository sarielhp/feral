package gmail

import (
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/label"
	"github.com/sarielhp/feral/report"

	gmailapi "google.golang.org/api/gmail/v1"
)

// setReadGmailREST adds or removes Gmail's UNREAD label. BatchModify accepts at
// most 1000 ids per call, so a whole folder marked read in one keystroke goes
// out as as many calls as it needs.
func setReadGmailREST(rep report.Reporter, config *Config, messageIDs []email.ID, read bool) error {
	if len(messageIDs) == 0 {
		return nil
	}
	srv, err := GetGmailService(rep, config)
	if err != nil {
		return err
	}
	const batchSize = 1000
	for i := 0; i < len(messageIDs); i += batchSize {
		req := &gmailapi.BatchModifyMessagesRequest{
			Ids: email.IDStrings(messageIDs[i:min(i+batchSize, len(messageIDs))]),
		}
		if read {
			req.RemoveLabelIds = []string{"UNREAD"}
		} else {
			req.AddLabelIds = []string{"UNREAD"}
		}
		if err := srv.Users.Messages.BatchModify("me", req).Do(); err != nil {
			return err
		}
	}
	return nil
}

// DeleteMessages permanently deletes messages; they do not pass through Trash.
func (c *client) DeleteMessages(messageIDs []email.ID, _ label.Folder) error {
	if len(messageIDs) == 0 {
		return nil
	}
	srv, err := GetGmailService(c.rep, c.config)
	if err != nil {
		return err
	}
	return srv.Users.Messages.BatchDelete("me", &gmailapi.BatchDeleteMessagesRequest{
		Ids: email.IDStrings(messageIDs),
	}).Do()
}
