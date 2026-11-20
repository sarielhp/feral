package outlook

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sarielhp/feral/cache"
	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/mailclient"
)

type graphAccountMessage struct {
	ID               string    `json:"id"`
	ParentFolderID   string    `json:"parentFolderId"`
	IsRead           bool      `json:"isRead"`
	ReceivedDateTime time.Time `json:"receivedDateTime"`
}

func (c *client) buildOutlookFolderMap() (map[string]string, error) {
	items, _, err := c.fetchFoldersRecursive("", "")
	if err != nil {
		return nil, err
	}
	folderMap := make(map[string]string, len(items))
	for _, it := range items {
		folderMap[it.Name] = it.FullName
	}
	return folderMap, nil
}

// LatestInboxIDs is one Graph request: the inbox, newest first, IDs only.
func (c *client) LatestInboxIDs(ctx context.Context, limit int) ([]email.ID, error) {
	if err := c.init(); err != nil {
		return nil, err
	}
	urlStr := fmt.Sprintf("https://graph.microsoft.com/v1.0/me/mailFolders/inbox/messages"+
		"?$top=%d&$orderby=receivedDateTime%%20desc&$select=id", limit)
	var res struct {
		Value []struct {
			ID string `json:"id"`
		} `json:"value"`
	}
	if err := c.graphGetJSON(ctx, urlStr, "listing the inbox", &res); err != nil {
		return nil, err
	}
	ids := make([]email.ID, 0, len(res.Value))
	for _, m := range res.Value {
		ids = append(ids, email.ID(m.ID))
	}
	return ids, nil
}

// InboxMessageBefore is one Graph request: an inbox message received before
// cutoff, with its arrival time and subject. It filters without ordering --
// Graph refuses an $orderby on a property the $filter does not lead with --
// so which of the old messages comes back is Graph's choice.
func (c *client) InboxMessageBefore(ctx context.Context, cutoff time.Time) (mailclient.InboxEntry, bool, error) {
	if err := c.init(); err != nil {
		return mailclient.InboxEntry{}, false, err
	}
	// The spaces are percent-encoded by hand: net/url leaves RawQuery as
	// given, and Graph answers a literal space with 400.
	urlStr := "https://graph.microsoft.com/v1.0/me/mailFolders/inbox/messages" +
		"?$top=1&$select=id,receivedDateTime,subject" +
		"&$filter=receivedDateTime%20lt%20" + cutoff.UTC().Format(time.RFC3339)
	var res struct {
		Value []struct {
			ID               string    `json:"id"`
			ReceivedDateTime time.Time `json:"receivedDateTime"`
			Subject          string    `json:"subject"`
		} `json:"value"`
	}
	if err := c.graphGetJSON(ctx, urlStr, "searching the inbox", &res); err != nil {
		return mailclient.InboxEntry{}, false, err
	}
	if len(res.Value) == 0 {
		return mailclient.InboxEntry{}, false, nil
	}
	m := res.Value[0]
	entry := mailclient.InboxEntry{ID: email.ID(m.ID), Received: m.ReceivedDateTime, Subject: m.Subject}
	if !entry.Received.Before(cutoff) {
		return mailclient.InboxEntry{}, false, fmt.Errorf("inbox filter for mail before %s returned mail received %s",
			cutoff.Format(time.RFC3339), entry.Received.Format(time.RFC3339))
	}
	return entry, true, nil
}

// graphGetJSON sends one GET and decodes its JSON answer into out. A refusal
// carries the start of Graph's explanation, which is what makes a 400
// actionable.
func (c *client) graphGetJSON(ctx context.Context, urlStr, what string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, http.NoBody)
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("%s failed: status %d: %s", what, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *client) fetchAccountMessages(limit int) ([]graphAccountMessage, error) {
	// The space in "receivedDateTime desc" must be percent-encoded. net/url
	// leaves RawQuery exactly as given, so a literal space reaches the wire and
	// Graph answers 400 for the whole request -- which is what it did, making
	// `last` and the account-wide query fail on every Outlook account.
	urlStr := fmt.Sprintf("https://graph.microsoft.com/v1.0/me/messages"+
		"?$top=%d&$orderby=receivedDateTime%%20desc"+
		"&$select=id,parentFolderId,isRead,receivedDateTime", limit)
	resp, err := c.client.Get(urlStr)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Graph explains a rejected query in the body; without it a 400 gives
		// nothing to act on.
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("fetching account messages failed: status %d: %s",
			resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var res struct {
		Value []graphAccountMessage `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return res.Value, nil
}

// FetchLatestAccountEmails queries Microsoft Graph account-wide for the latest N emails.
func (c *client) FetchLatestAccountEmails(limit int) ([]cfg_acc.MessageFolderRef, error) {
	if err := c.init(); err != nil {
		return nil, err
	}

	if limit <= 0 {
		limit = 100
	}

	folderMap, err := c.buildOutlookFolderMap()
	if err != nil {
		return nil, err
	}

	messages, err := c.fetchAccountMessages(limit)
	if err != nil {
		return nil, err
	}

	cacheDir := c.config.AccountCache
	_ = os.MkdirAll(filepath.Join(cacheDir.Path(), "messages"), 0700)

	_ = cache.UpdateReadState(cacheDir, func(state map[email.ID]bool) {
		for _, m := range messages {
			state[email.ID(m.ID)] = m.IsRead
		}
	})

	ids := make([]email.ID, len(messages))
	for i, m := range messages {
		ids[i] = email.ID(m.ID)
	}
	// A download failure is logged rather than fatal: the caller wants the list
	// of recent messages, and one that could not be cached is still worth
	// listing.
	if err := c.newMessageStore().Prefetch(ids); err != nil {
		slog.Warn("some recent messages could not be downloaded", slog.Any("error", err))
	}

	refs := make([]cfg_acc.MessageFolderRef, len(messages))
	for i, m := range messages {
		folderName := "Inbox"
		if name, ok := folderMap[m.ParentFolderID]; ok && name != "" {
			folderName = name
		}
		refs[i] = cfg_acc.MessageFolderRef{
			MessageID: email.ID(m.ID),
			Folder:    folderName,
		}
	}

	return refs, nil
}
