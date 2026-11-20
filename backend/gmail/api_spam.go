package gmail

import (
	"encoding/base64"
	"fmt"
	"github.com/sarielhp/feral/app"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/report"
	"net/url"
	"strings"

	gmailapi "google.golang.org/api/gmail/v1"
)

// reportSpamToGmailREST reports the emails as spam by adding the SPAM label and removing the source folder label.
func reportSpamToGmailREST(rep report.Reporter, messageIDs []email.ID, config *Config, sourceLabelName string) error {
	if len(messageIDs) == 0 {
		return nil
	}

	srv, err := GetGmailService(rep, config)
	if err != nil {
		return err
	}

	sourceLabelID, err := resolveLabelIDByName(srv, config, sourceLabelName)
	if err != nil {
		return err
	}

	removeLabels := []string{}
	if sourceLabelID != "" && !strings.EqualFold(sourceLabelID, "SENT") {
		removeLabels = append(removeLabels, sourceLabelID)
	}

	err = srv.Users.Messages.BatchModify("me", &gmailapi.BatchModifyMessagesRequest{
		Ids:            email.IDStrings(messageIDs),
		AddLabelIds:    []string{"SPAM"},
		RemoveLabelIds: removeLabels,
	}).Do()
	if err != nil {
		return fmt.Errorf("failed to batch report spam: %w", err)
	}

	return nil
}

// moveToInboxGmailREST moves the specified messages back to the INBOX by adding the INBOX label and removing the source label.
func moveToInboxGmailREST(rep report.Reporter, messageIDs []email.ID, config *Config, sourceLabelName string) error {
	if len(messageIDs) == 0 {
		return nil
	}

	if strings.EqualFold(sourceLabelName, "inbox") {
		if config.Verbose {
			rep.Printf("    [Debug] MoveToInbox: message already in Inbox. Skipping.\n")
		}
		return nil
	}

	srv, err := GetGmailService(rep, config)
	if err != nil {
		return err
	}

	sourceLabelID, err := resolveLabelIDByName(srv, config, sourceLabelName)
	if err != nil {
		return err
	}
	removeLabels := []string{}
	if sourceLabelID != "" && sourceLabelID != "INBOX" && !strings.EqualFold(sourceLabelID, "SENT") {
		removeLabels = append(removeLabels, sourceLabelID)
	}
	removeLabels = append(removeLabels, "SPAM")

	addLabels := []string{"INBOX"}
	if config.UnspamLearn != "" {
		labelsRes, listErr := srv.Users.Labels.List("me").Do()
		if listErr == nil {
			labelNameToID := make(map[string]string)
			labelIDToName := make(map[string]string)
			for _, l := range labelsRes.Labels {
				labelNameToID[strings.ToLower(l.Name)] = l.Id
				labelIDToName[l.Id] = l.Name
			}
			unspamLabelID, errEnsure := ensureLabelHierarchyREST(rep, srv, config.UnspamLearn, labelNameToID, labelIDToName)
			if errEnsure == nil {
				addLabels = append(addLabels, unspamLabelID)
			} else {
				rep.Warnf("    Warning: failed to ensure Gmail label %q: %v\n", config.UnspamLearn, errEnsure)
			}
		}
	}

	err = srv.Users.Messages.BatchModify("me", &gmailapi.BatchModifyMessagesRequest{
		Ids:            email.IDStrings(messageIDs),
		AddLabelIds:    addLabels,
		RemoveLabelIds: removeLabels,
	}).Do()
	if err != nil {
		return fmt.Errorf("failed to batch move to inbox: %w", err)
	}

	return nil
}

// deleteAllSpamInGmailREST permanently deletes all messages in the Gmail Spam folder forever.
func deleteAllSpamInGmailREST(rep report.Reporter, config *Config) error {
	srv, err := GetGmailService(rep, config)
	if err != nil {
		return err
	}

	rep.Infof("Fetching all spam email IDs to delete permanently...\n")

	totalDeleted := 0
	batchSize := 1000
	for {
		call := srv.Users.Messages.List("me").Q("label:SPAM").IncludeSpamTrash(true).MaxResults(int64(batchSize))
		res, err := call.Do()
		if err != nil {
			return fmt.Errorf("failed to list spam for deletion: %w", err)
		}
		if len(res.Messages) == 0 {
			break
		}

		var ids []email.ID
		for _, m := range res.Messages {
			ids = append(ids, email.ID(m.Id))
		}
		for i := 0; i < len(ids); i += batchSize {
			end := i + batchSize
			if end > len(ids) {
				end = len(ids)
			}
			err = srv.Users.Messages.BatchDelete("me", &gmailapi.BatchDeleteMessagesRequest{
				Ids: email.IDStrings(ids[i:end]),
			}).Do()
			if err != nil {
				return fmt.Errorf("failed to batch delete spam: %w", err)
			}
		}
		totalDeleted += len(ids)
	}

	if totalDeleted == 0 {
		rep.Successf("Gmail Spam folder is already empty!\n")
		return nil
	}

	rep.Successf("Successfully deleted and expunged ")
	rep.Printf("%s", app.ColorBoldGreen.Sprintf("%d", totalDeleted))
	rep.Printf(" emails from Gmail Spam folder forever!\n")
	return nil
}

// executeMailtoUnsubscribeREST sends the mailto unsubscribe email using the Gmail REST API (Send).
func executeMailtoUnsubscribeREST(srv *gmailapi.Service, _ *Config, mailtoLink string) error {
	u, err := url.Parse(mailtoLink)
	if err != nil {
		return err
	}

	toEmail := u.Path
	if toEmail == "" {
		toEmail = u.Opaque
	}

	queryParams := u.Query()
	subject := queryParams.Get("subject")
	if subject == "" {
		subject = "Unsubscribe"
	}
	body := queryParams.Get("body")
	if body == "" {
		body = "Unsubscribe me from this mailing list."
	}

	rawMessage := []byte("To: " + toEmail + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"\r\n" +
		body + "\r\n")

	gMsg := &gmailapi.Message{
		Raw: base64.URLEncoding.EncodeToString(rawMessage),
	}

	_, err = srv.Users.Messages.Send("me", gMsg).Do()
	return err
}

// moveLabelChange is the label edit that moves a message from source to dest.
// "Archive" is special on either side: an account with no such label archives
// by removing the source label alone, so as a destination it adds nothing and
// as a source -- moving archived mail back, as undo does -- it removes nothing.
// A message is never taken out of SENT.
func moveLabelChange(resolve func(string) (string, error), source, dest string) (add, remove []string, err error) {
	sourceID, err := resolveMoveSide(resolve, source)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to resolve source label %q: %w", source, err)
	}
	destID, err := resolveMoveSide(resolve, dest)
	if err != nil {
		return nil, nil, err
	}
	if sourceID != "" && sourceID != destID && !strings.EqualFold(sourceID, "SENT") && !strings.EqualFold(source, "SENT") {
		remove = []string{sourceID}
	}
	if destID != "" {
		add = []string{destID}
	}
	return add, remove, nil
}

// resolveMoveSide resolves one side of a move, taking a missing "Archive"
// label as no label at all.
func resolveMoveSide(resolve func(string) (string, error), name string) (string, error) {
	id, err := resolve(name)
	if err != nil && strings.EqualFold(name, "archive") {
		return "", nil
	}
	return id, err
}

// moveEmailGmailREST moves emails from a source label to a destination label.
func moveEmailGmailREST(rep report.Reporter, messageIDs []email.ID, config *Config, sourceLabelName string, destLabelName string) error {
	if len(messageIDs) == 0 {
		return nil
	}
	if strings.EqualFold(sourceLabelName, destLabelName) {
		if config.Verbose {
			rep.Printf("    [Debug] MoveEmail: source and destination are the same (%q). Skipping.\n", destLabelName)
		}
		return nil
	}

	srv, err := GetGmailService(rep, config)
	if err != nil {
		return err
	}

	add, remove, err := moveLabelChange(func(name string) (string, error) {
		return resolveLabelIDByName(srv, config, name)
	}, sourceLabelName, destLabelName)
	if err != nil {
		return err
	}
	if len(add) == 0 && len(remove) == 0 {
		return nil
	}
	req := &gmailapi.BatchModifyMessagesRequest{
		Ids:            email.IDStrings(messageIDs),
		AddLabelIds:    add,
		RemoveLabelIds: remove,
	}

	err = srv.Users.Messages.BatchModify("me", req).Do()
	if err != nil {
		return fmt.Errorf("failed to batch move emails: %w", err)
	}

	return nil
}

func copyEmailGmailREST(rep report.Reporter, config *Config, messageIDs []email.ID, sourceLabelName string, destLabelName string) error {
	if len(messageIDs) == 0 {
		return nil
	}
	srv, err := GetGmailService(rep, config)
	if err != nil {
		return err
	}
	destLabelID, err := resolveLabelIDByName(srv, config, destLabelName)
	if err != nil {
		return err
	}
	if destLabelID == "" {
		return nil
	}
	req := &gmailapi.BatchModifyMessagesRequest{
		Ids:         email.IDStrings(messageIDs),
		AddLabelIds: []string{destLabelID},
	}
	err = srv.Users.Messages.BatchModify("me", req).Do()
	if err != nil {
		return fmt.Errorf("failed to copy messages from %s to %s: %w", sourceLabelName, destLabelName, err)
	}
	return nil
}

func validateSpamDestinationLabel(spamFolder, destLabel string) error {
	if spamFolder == "" {
		spamFolder = "SPAM"
	}
	// Never escalate a move into a permanent purge. Reached from "spam learn",
	// "spam bye" and "spam pol unsub", none of which promise deletion.
	if strings.EqualFold(spamFolder, destLabel) || strings.EqualFold("SPAM", destLabel) || strings.EqualFold("[Gmail]/Spam", destLabel) {
		return fmt.Errorf("spam_learn is set to %q, which is the spam folder itself; refusing to silently purge it. Run 'feral spam del' to delete deliberately, or set spam_learn to a different folder", destLabel)
	}
	return nil
}

func resolveDestinationLabelID(rep report.Reporter, srv *gmailapi.Service, destLabel string) (string, error) {
	rep.Infof("Ensuring Gmail label %q exists...\n", destLabel)
	labelsRes, err := srv.Users.Labels.List("me").Do()
	if err != nil {
		return "", fmt.Errorf("failed to list Gmail labels: %w", err)
	}

	labelNameToID := make(map[string]string)
	labelIDToName := make(map[string]string)
	for _, l := range labelsRes.Labels {
		labelNameToID[strings.ToLower(l.Name)] = l.Id
		labelIDToName[l.Id] = l.Name
	}

	destLabelID, errLabel := ensureLabelHierarchyREST(rep, srv, destLabel, labelNameToID, labelIDToName)
	if errLabel != nil {
		return "", fmt.Errorf("failed to ensure label %q: %w", destLabel, errLabel)
	}
	return destLabelID, nil
}

func batchMoveSpamMessages(srv *gmailapi.Service, ids []email.ID, destLabelID, destLabel string) error {
	const batchSize = 1000
	for i := 0; i < len(ids); i += batchSize {
		end := i + batchSize
		if end > len(ids) {
			end = len(ids)
		}
		err := srv.Users.Messages.BatchModify("me", &gmailapi.BatchModifyMessagesRequest{
			Ids:            email.IDStrings(ids[i:end]),
			AddLabelIds:    []string{destLabelID},
			RemoveLabelIds: []string{"SPAM"},
		}).Do()
		if err != nil {
			return fmt.Errorf("failed to batch move spam to %q: %w", destLabel, err)
		}
	}
	return nil
}

// moveAllSpamInGmailREST moves all messages in the Gmail Spam folder to a destination label.
func moveAllSpamInGmailREST(rep report.Reporter, config *Config, destLabel string) error {
	if err := validateSpamDestinationLabel(config.SpamFolder, destLabel); err != nil {
		return err
	}

	srv, err := GetGmailService(rep, config)
	if err != nil {
		return err
	}

	destLabelID, err := resolveDestinationLabelID(rep, srv, destLabel)
	if err != nil {
		return err
	}

	rep.Infof("Fetching and moving spam emails to %q...\n", destLabel)

	totalMoved := 0
	for {
		call := srv.Users.Messages.List("me").Q("label:SPAM").IncludeSpamTrash(true).MaxResults(500)
		res, err := call.Do()
		if err != nil {
			return fmt.Errorf("failed to list spam for moving: %w", err)
		}
		if len(res.Messages) == 0 {
			break
		}

		var ids []email.ID
		for _, m := range res.Messages {
			ids = append(ids, email.ID(m.Id))
		}
		if err := batchMoveSpamMessages(srv, ids, destLabelID, destLabel); err != nil {
			return err
		}
		totalMoved += len(ids)
	}

	if totalMoved == 0 {
		rep.Successf("Gmail Spam folder is already empty!\n")
		return nil
	}

	rep.Successf("Successfully moved %d spam emails to %q on Gmail server.\n", totalMoved, destLabel)
	return nil
}
