package mbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/mail"
	"os"
	"strings"
	"time"

	"github.com/sarielhp/feral/cache"
	"github.com/sarielhp/feral/cache/folderindex"
	"github.com/sarielhp/feral/cache/msg"
	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/label"
	"github.com/sarielhp/feral/mailclient"
)

type client struct {
	acc      cfg_acc.AccountConfig
	localCfg *cfg_g.Config
	mboxPath string
	dispName string
}

// NewClient returns a MailClient backed by the mbox file at mboxPath, using acc
// for account metadata and localCfg for cache and display settings.
func NewClient(acc cfg_acc.AccountConfig, localCfg *cfg_g.Config, mboxPath string) mailclient.MailClient {
	disp := acc.DisplayName
	if disp == "" {
		disp = acc.Name
	}
	return &client{
		acc:      acc,
		localCfg: localCfg,
		mboxPath: mboxPath,
		dispName: disp,
	}
}

func (c *client) Validate() error {
	fi, err := os.Stat(c.mboxPath)
	if err != nil {
		return fmt.Errorf("cannot access mbox %q: %w", c.mboxPath, err)
	}
	if fi.IsDir() {
		return fmt.Errorf("%q is a directory, not an mbox file", c.mboxPath)
	}
	return nil
}

func (c *client) BackendType() string {
	return "mbox"
}

func (c *client) Config() *cfg_g.Config {
	return c.localCfg
}

func (c *client) inboxName() string {
	return "INBOX"
}

func (c *client) GetMatchingLabels(_ string) ([]string, error) {
	return []string{"INBOX"}, nil
}

// GetLabelItems is the one INBOX, counted: every message the file holds, and
// as unread those the local read state -- an mbox file keeps none of its own --
// does not mark read. It reads the file through the cache first, as listing a
// server's folders asks the server: the TUI lists folders as it opens, before
// the inbox is loaded, so counting only what was already cached showed INBOX
// (0) beside a header counting 30 unread. An unchanged file costs a stat. A
// file it cannot read leaves the counts out rather than failing the listing
// they decorate; opening the folder reports why.
func (c *client) GetLabelItems() ([]cfg_acc.Item, error) {
	// IsLabel: a folder that holds messages, so the tree keeps its own total.
	inbox := cfg_acc.Item{Name: "INBOX", FullName: "INBOX", IsLabel: true}
	ids, err := IngestMbox(c.mboxPath, c.localCfg.AccountCache, c.dispName)
	if err != nil {
		return []cfg_acc.Item{inbox}, nil
	}
	read := cache.LoadReadState(c.localCfg.AccountCache)
	inbox.MessagesTotal = int64(len(ids))
	for _, id := range ids {
		if !read[id] {
			inbox.MessagesUnread++
		}
	}
	return []cfg_acc.Item{inbox}, nil
}

func (c *client) FetchAndDownloadEmails(_ label.Folder, _ string) ([]email.ID, error) {
	return IngestMbox(c.mboxPath, c.localCfg.AccountCache, c.dispName)
}

func (c *client) FetchLatestAccountEmails(limit int) ([]cfg_acc.MessageFolderRef, error) {
	ids, err := folderindex.IDs(c.localCfg.AccountCache, "INBOX")
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(ids) > limit {
		ids = ids[:limit]
	}
	refs := make([]cfg_acc.MessageFolderRef, len(ids))
	for i, id := range ids {
		refs[i] = cfg_acc.MessageFolderRef{MessageID: id, Folder: "INBOX"}
	}
	return refs, nil
}

// LatestInboxIDs is the INBOX index, which is all an mbox view has.
func (c *client) LatestInboxIDs(ctx context.Context, limit int) ([]email.ID, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ids, err := folderindex.IDs(c.localCfg.AccountCache, "INBOX")
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(ids) > limit {
		ids = ids[:limit]
	}
	return ids, nil
}

func (c *client) GetRawMessageContext(ctx context.Context, msgID email.ID) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return msg.Read(c.localCfg.AccountCache, msgID)
}

func (c *client) GetRawMessage(msgID email.ID) ([]byte, error) {
	return c.GetRawMessageContext(context.Background(), msgID)
}

// batchFetch retrieves each ID with fetch, returning the results in input order
// and joining every per-ID error. A cancelled context returns what was fetched
// so far with the cancellation.
func batchFetch[T any](ctx context.Context, msgIDs []email.ID, fetch func(context.Context, email.ID) (T, error)) ([]T, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result := make([]T, len(msgIDs))
	var errs []error
	for i, id := range msgIDs {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		value, err := fetch(ctx, id)
		if err != nil {
			errs = append(errs, fmt.Errorf("id %s: %w", id, err))
			continue
		}
		result[i] = value
	}
	return result, errors.Join(errs...)
}

func (c *client) GetRawMessagesBatchContext(ctx context.Context, msgIDs []email.ID) ([][]byte, error) {
	return batchFetch(ctx, msgIDs, c.GetRawMessageContext)
}

func (c *client) GetRawMessagesBatch(msgIDs []email.ID) ([][]byte, error) {
	return c.GetRawMessagesBatchContext(context.Background(), msgIDs)
}

func (c *client) GetMessageSummaryContext(ctx context.Context, msgID email.ID) (*email.Email, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return msg.CachedEmail(c.localCfg.AccountCache, msgID)
}

func (c *client) GetMessageSummary(msgID email.ID) (*email.Email, error) {
	return c.GetMessageSummaryContext(context.Background(), msgID)
}

func (c *client) GetMessageSummariesBatchContext(ctx context.Context, msgIDs []email.ID) ([]*email.Email, error) {
	return batchFetch(ctx, msgIDs, c.GetMessageSummaryContext)
}

func (c *client) GetMessageSummariesBatch(msgIDs []email.ID) ([]*email.Email, error) {
	return c.GetMessageSummariesBatchContext(context.Background(), msgIDs)
}

func (c *client) PrefetchMessagesContext(ctx context.Context, _ []email.ID) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func (c *client) PrefetchMessages(_ []email.ID) error {
	return nil
}

// SearchEmails matches query, case-insensitively, against each message's
// subject, sender and decoded body text. The body goes through the same
// extractor the viewer uses, so a base64 or quoted-printable body matches on
// what it says rather than on its encoding.
func (c *client) SearchEmails(query string, limit int) ([]email.ID, error) {
	ids, err := folderindex.IDs(c.localCfg.AccountCache, "INBOX")
	if err != nil {
		return nil, err
	}
	if query == "" {
		if limit > 0 && len(ids) > limit {
			return ids[:limit], nil
		}
		return ids, nil
	}
	qLower := strings.ToLower(query)
	var matches []email.ID
	for _, id := range ids {
		if !c.messageMatches(id, qLower) {
			continue
		}
		matches = append(matches, id)
		if limit > 0 && len(matches) >= limit {
			break
		}
	}
	return matches, nil
}

// messageMatches reports whether the cached message id contains qLower in its
// subject, sender or body.
func (c *client) messageMatches(id email.ID, qLower string) bool {
	em, err := msg.CachedEmail(c.localCfg.AccountCache, id)
	if err != nil {
		return false
	}
	for _, field := range []string{em.Subject, em.FromEmail, em.FromRaw} {
		if strings.Contains(strings.ToLower(field), qLower) {
			return true
		}
	}
	raw, err := msg.Read(c.localCfg.AccountCache, id)
	if err != nil {
		return false
	}
	parsed, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return false
	}
	body, err := email.ExtractPlainBodyText(parsed)
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(body), qLower)
}

// LabelWriter stubs
func (c *client) RenameLabel(_ label.Folder, _ label.Folder) error {
	return mailclient.ErrReadOnlyOperationBlocked
}

func (c *client) FixLabels() error {
	return nil
}

func (c *client) DeleteLabel(_ label.Folder) error {
	return mailclient.ErrReadOnlyOperationBlocked
}

func (c *client) EnsureLabelExists(_ label.Folder) error {
	return nil
}

// EmailWriter stubs
func (c *client) MoveEmail(_ []email.ID, _ label.Folder, _ label.Folder) error {
	return mailclient.ErrReadOnlyOperationBlocked
}

func (c *client) CopyEmail(_ []email.ID, _ label.Folder, _ label.Folder) error {
	return mailclient.ErrReadOnlyOperationBlocked
}

func (c *client) MarkAsRead(_ []email.ID) error {
	return nil
}

// MarkAsUnread is a no-op: an mbox file keeps no server-side read state, so the
// local read cache is the only record.
func (c *client) MarkAsUnread(_ []email.ID) error {
	return nil
}

func (c *client) MoveToInbox(_ []email.ID, _ label.Folder) error {
	return mailclient.ErrReadOnlyOperationBlocked
}

func (c *client) UploadRawEmail(_ []byte, _ label.Folder) (email.ID, error) {
	return "", mailclient.ErrReadOnlyOperationBlocked
}

// DeleteMessages removes messages from the mbox file itself, in one rewrite
// under the locks mail delivery honors (see rewrite.go). An account is
// read-only unless its config says otherwise, and the read-only decorator
// stops this call before it gets here. A single .eml file is not rewritten.
func (c *client) DeleteMessages(ids []email.ID, folder label.Folder) error {
	if len(ids) == 0 {
		return nil
	}
	if isEmlPath(c.mboxPath) {
		return fmt.Errorf("%s is a single message, not an mbox: %w", c.mboxPath, errors.ErrUnsupported)
	}
	if !strings.EqualFold(folder.String(), c.inboxName()) {
		return fmt.Errorf("mbox %s has no folder %q, only %s", c.mboxPath, folder, c.inboxName())
	}
	return deleteFromMbox(c.mboxPath, c.localCfg.AccountCache, ids)
}

// SpamManager stubs
func (c *client) ReportSpam(_ []email.ID, _ label.Folder) error {
	return mailclient.ErrReadOnlyOperationBlocked
}

func (c *client) MoveAllSpam(_ label.Folder) error {
	return mailclient.ErrReadOnlyOperationBlocked
}

func (c *client) DeleteAllSpam() error {
	return mailclient.ErrReadOnlyOperationBlocked
}

func (c *client) ShowPoliticalSpam(_ bool) error {
	return nil
}

func (c *client) LearnSpam() error {
	return nil
}

// RuleManager stubs
func (c *client) CheckAndApplyRules(_ []email.ID, _ label.Folder, _ string) ([]email.ID, error) {
	return nil, nil
}

func (c *client) ExportRules() error {
	return nil
}

func (c *client) GetFilters() (mailclient.FilterListing, error) {
	return mailclient.FilterListing{Title: "Mbox (Local Archive)"}, nil
}

func (c *client) DeleteFilter(_ string) error {
	return fmt.Errorf("mbox archives do not support server-side filters")
}

func (c *client) RewriteRuleLabelCasing(label string) string {
	return label
}

// SendEmail stub
func (c *client) SendEmail(_ []byte) error {
	return mailclient.ErrReadOnlyOperationBlocked
}

// InboxFolder is the account's inbox, as the server names it.
func (c *client) InboxFolder() label.Folder {
	return label.FromServer(c.inboxName())
}

// InboxMessageBefore is unsupported: an mbox view has no arrival times and
// nothing empties its inbox.
func (c *client) InboxMessageBefore(context.Context, time.Time) (mailclient.InboxEntry, bool, error) {
	return mailclient.InboxEntry{}, false, fmt.Errorf("mbox: inbox age: %w", errors.ErrUnsupported)
}
