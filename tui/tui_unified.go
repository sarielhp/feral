package tui

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/sarielhp/feral/cache"
	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/cache/folderindex"
	"github.com/sarielhp/feral/cache/meta"
	"github.com/sarielhp/feral/cache/msg"
	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/label"
	"github.com/sarielhp/feral/mailclient"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	// UnifiedInboxFolder is the visible display name for the aggregated inbox.
	UnifiedInboxFolder = "All Inboxes"
	// UnifiedInboxPrefix is the label sigil naming all inboxes.
	UnifiedInboxPrefix = "%all:inbox"
)

func isUnifiedInbox(folder string) bool {
	return strings.EqualFold(folder, UnifiedInboxFolder) ||
		strings.EqualFold(folder, UnifiedInboxPrefix) ||
		strings.EqualFold(folder, "%all")
}

// unifiedInboxItems is the folder entry for the unified inbox, offered when
// more than one account is configured. It counts what opening it shows: each
// account's cached INBOX index (loadUnifiedCachedEmails), unread by the
// local read marks. It used to carry no counts, so the tree always showed
// All Inboxes (0).
func (m *tuiModel) unifiedInboxItems() []label.Item {
	accounts := m.getAccounts()
	if len(accounts) < 2 {
		return nil
	}
	// IsLabel: it holds messages, so the tree keeps its total.
	item := label.Item{Name: UnifiedInboxFolder, FullName: UnifiedInboxFolder, IsSystem: true, IsLabel: true}
	cfg := m.Config()
	if cfg == nil {
		cfg = m.cfg
	}
	for _, acc := range accounts {
		total, unread := cachedInboxCounts(cfg.AccountCacheFor(acc), acc.Type)
		item.MessagesTotal += total
		item.MessagesUnread += unread
	}
	return []label.Item{item}
}

// refreshUnifiedCounts recounts All Inboxes where the folder trees show it.
// The folder list is fetched once, as an account opens, and another account's
// inbox may be cached only later; the count is local, so opening the tree
// recounts it rather than showing what the cache held at startup.
func (m *tuiModel) refreshUnifiedCounts() {
	fresh := m.unifiedInboxItems()
	if len(fresh) == 0 {
		return
	}
	for _, tree := range [][]*label.TreeNode{m.globalTree, m.tree} {
		for _, node := range tree {
			if node.FullName == UnifiedInboxFolder {
				node.MessagesTotal, node.MessagesUnread = fresh[0].MessagesTotal, fresh[0].MessagesUnread
			}
		}
	}
}

// cachedInboxCounts counts an account's cached INBOX index, leaving out a
// cache loadUnifiedCachedEmails would leave out.
func cachedInboxCounts(dir cachedir.Account, backend string) (total, unread int64) {
	if dir.IsZero() || !folderindex.HasStructure(dir) || meta.Check(dir, backend) != nil {
		return 0, 0
	}
	ids, err := folderindex.IDs(dir, "INBOX")
	if err != nil {
		return 0, 0
	}
	read := cache.LoadReadState(dir)
	for _, id := range ids {
		if !read[id] {
			unread++
		}
	}
	return int64(len(ids)), unread
}

// stampAccount marks a row with the account it came from: the name that
// routes actions to it, and the badge and colour that show it.
func stampAccount(em *email.Email, idx int, acc cfg_acc.AccountConfig) {
	em.Account = acc.Name
	em.AccountBadge = acc.Badge
	em.AccountIndex = idx
}

func (m *tuiModel) findEmailByID(id email.ID) *email.Email {
	for i := range m.emails {
		if m.emails[i].ID == id {
			return &m.emails[i]
		}
	}
	for i := range m.rawEmails {
		if m.rawEmails[i].ID == id {
			return &m.rawEmails[i]
		}
	}
	return nil
}

func (m *tuiModel) loadUnifiedCachedEmails() {
	var allEmails []email.Email
	accounts := m.getAccounts()
	cfg := m.Config()
	if cfg == nil {
		cfg = m.cfg
	}

	for idx, acc := range accounts {
		if cfg == nil {
			continue
		}
		accCache := cfg.AccountCacheFor(acc)
		if accCache.IsZero() || !folderindex.HasStructure(accCache) {
			continue
		}
		// One account a newer build has reshaped is left out of the view,
		// with a word in the status line, rather than failing all of it.
		if err := meta.Check(accCache, acc.Type); err != nil {
			slog.Warn("unified inbox: skipping an account cache", slog.String("account", acc.Name), slog.Any("error", err))
			m.saveStatus = fmt.Sprintf("%s left out: %v", acc.Name, err)
			continue
		}

		inboxName := "INBOX"
		ids, err := folderindex.IDs(accCache, inboxName)
		if err != nil {
			continue
		}
		allEmails = append(allEmails, unifiedAccountRows(accCache, idx, acc, ids)...)
	}

	sort.Slice(allEmails, func(i, j int) bool {
		return allEmails[i].EmailDate.Before(allEmails[j].EmailDate)
	})

	isRead, isReplied := rowFlags(allEmails)
	m.isRead = mergeFlags(m.isRead, isRead)
	m.isReplied = mergeFlags(m.isReplied, isReplied)
	m.rawEmails = allEmails
	m.rebuildVisibleEmails()
	if len(m.emails) > 0 {
		// The newest message is last: the cursor starts on it, as in mutt.
		m.eIdx = len(m.emails) - 1
		m.selectedID = m.emails[m.eIdx].ID
	} else {
		m.selectedID = ""
	}
	m.folderJustChanged = true
}

func (m *tuiModel) loadUnifiedEmailsCmd() tea.Cmd {
	client := m.client
	gen := m.flagGen
	folder := m.currentF
	accounts := m.getAccounts()
	cfg := m.Config()
	if cfg == nil {
		cfg = m.cfg
	}
	return func() tea.Msg {
		var allEmails []email.Email
		for idx, acc := range accounts {
			cl, err := m.resolveClientForSwitch(acc)
			if err != nil || cl == nil {
				continue
			}
			inboxFolder := cl.InboxFolder()
			ids, err := cl.FetchAndDownloadEmails(inboxFolder, "")
			if err != nil {
				continue
			}
			accCache := cfg.AccountCacheFor(acc)
			allEmails = append(allEmails, unifiedAccountRows(accCache, idx, acc, ids)...)
		}

		sort.Slice(allEmails, func(i, j int) bool {
			return allEmails[i].EmailDate.Before(allEmails[j].EmailDate)
		})
		isRead, isReplied := rowFlags(allEmails)
		return emailsLoadedMsg{client: client, folder: folder, emails: allEmails, isRead: isRead, isReplied: isReplied, mergeFlags: true, flagGen: gen}
	}
}

// actionPlan is what an action on messages needs to know about where they
// live, captured on the Update goroutine before their rows are removed. The
// command that performs the action runs later on its own goroutine, where the
// rows are gone and m.emails may be changing under it; resolving an id there
// found nothing, so unified-inbox actions fell back to the active account with
// "All Inboxes" as the source folder.
type actionPlan struct {
	// active is the client the session had when the action began; it takes
	// every id outside a unified inbox.
	active mailclient.MailClient
	// folder is the folder the action was taken in.
	folder string
	// accounts names the owning account of each id; set only in a unified
	// inbox, where each message belongs to a different account.
	accounts map[email.ID]string
	// folders names the source folder of each id; set only in a saved set,
	// whose rows come from several folders of the one account that owns it.
	folders map[email.ID]string
}

// planAction captures the session's client, folder and, in a unified inbox,
// each id's account, or in a saved set each id's folder. Call it on the Update
// goroutine, before removing rows.
func (m *tuiModel) planAction(ids []email.ID, folder string) actionPlan {
	p := actionPlan{active: m.client, folder: folder}
	switch {
	case isUnifiedInbox(folder):
		p.accounts = make(map[email.ID]string, len(ids))
		for _, id := range ids {
			if em := m.findEmailByID(id); em != nil {
				p.accounts[id] = em.Account
			}
		}
	case isVirtualView(folder):
		p.folders = m.savedSetFolders(folder, ids)
	}
	return p
}

func (p actionPlan) unified() bool { return isUnifiedInbox(p.folder) }

// clientForAccountName returns the client for the named account. It never
// substitutes another account's client: acting on the wrong server with the
// wrong folder is worse than a refusal.
func (m *tuiModel) clientForAccountName(active mailclient.MailClient, name string) (mailclient.MailClient, error) {
	if name == "" {
		return nil, errors.New("message has no known account")
	}
	if active != nil && active.Config() != nil && strings.EqualFold(string(active.Config().SelectedAccount), name) {
		return active, nil
	}
	for _, acc := range m.getAccounts() {
		if !strings.EqualFold(acc.Name, name) {
			continue
		}
		cl, err := m.resolveClientForSwitch(acc)
		if err != nil {
			return nil, fmt.Errorf("account %s: %w", name, err)
		}
		if cl == nil {
			return nil, fmt.Errorf("account %s: no client", name)
		}
		return cl, nil
	}
	return nil, fmt.Errorf("account %s is not configured", name)
}

// targetClient is the client that must act on id, without the source folder a
// move needs. Reading a message, marking it read or reporting it needs no
// folder at all, and a saved set's rows are not all placed in one.
func (m *tuiModel) targetClient(p actionPlan, id email.ID) (mailclient.MailClient, error) {
	if p.unified() {
		return m.clientForAccountName(p.active, p.accounts[id])
	}
	if p.active == nil {
		return nil, errors.New("no active account")
	}
	return p.active, nil
}

// target is the client and source folder an action on id must use: the active
// client and the message's own folder, which is the folder on screen, or in a
// unified inbox the message's own account and that account's inbox. An action
// that has to name a folder refuses an id no plan can place.
func (m *tuiModel) target(p actionPlan, id email.ID) (mailclient.MailClient, string, error) {
	cl, err := m.targetClient(p, id)
	if err != nil {
		return nil, "", err
	}
	switch {
	case p.unified():
		return cl, cl.InboxFolder().String(), nil
	case p.folders != nil:
		folder := p.folders[id]
		if folder == "" {
			return nil, "", fmt.Errorf("%s: %w", id, errNoSourceFolder)
		}
		return cl, folder, nil
	default:
		return cl, p.folder, nil
	}
}

// accountBatch is the ids of one account in a bulk action, with the client
// and source folder that account needs.
type accountBatch struct {
	client mailclient.MailClient
	folder string
	ids    []email.ID
}

// batches groups ids by the client and folder that must act on them: one batch
// per account in a unified inbox, and one per source folder in a saved set,
// whose rows come from several folders of the one account that owns it. An id
// whose target cannot be resolved is reported in the error and left out.
func (m *tuiModel) batches(p actionPlan, ids []email.ID) ([]accountBatch, error) {
	switch {
	case p.unified():
		return m.batchesByAccount(p, ids)
	case p.folders != nil:
		return m.batchesByFolder(p, ids)
	default:
		cl, folder, err := m.target(p, "")
		if err != nil {
			return nil, err
		}
		return []accountBatch{{client: cl, folder: folder, ids: ids}}, nil
	}
}

// clientBatches groups ids by the client that must read them, ignoring the
// source folder: an export needs the messages and not where they came from, so
// a saved set -- whose rows come from several folders -- is one batch like any
// other single-account view.
func (m *tuiModel) clientBatches(p actionPlan, ids []email.ID) ([]accountBatch, error) {
	if p.unified() {
		return m.batchesByAccount(p, ids)
	}
	if p.active == nil {
		return nil, errors.New("no active account")
	}
	return []accountBatch{{client: p.active, ids: ids}}, nil
}

// batchesByAccount groups a unified inbox's ids by the account each belongs to.
func (m *tuiModel) batchesByAccount(p actionPlan, ids []email.ID) ([]accountBatch, error) {
	byName := make(map[string][]email.ID)
	for _, id := range ids {
		byName[p.accounts[id]] = append(byName[p.accounts[id]], id)
	}
	var out []accountBatch
	var errs []error
	for _, name := range sortedKeys(byName) {
		cl, err := m.clientForAccountName(p.active, name)
		if err != nil {
			errs = append(errs, fmt.Errorf("%d message(s): %w", len(byName[name]), err))
			continue
		}
		out = append(out, accountBatch{client: cl, folder: cl.InboxFolder().String(), ids: byName[name]})
	}
	return out, errors.Join(errs...)
}

// batchesByFolder groups a saved set's ids by the folder each came from: the
// set spans folders, and a move names one source folder per call.
func (m *tuiModel) batchesByFolder(p actionPlan, ids []email.ID) ([]accountBatch, error) {
	if p.active == nil {
		return nil, errors.New("no active account")
	}
	byFolder := make(map[string][]email.ID)
	var errs []error
	for _, id := range ids {
		folder := p.folders[id]
		if folder == "" {
			errs = append(errs, fmt.Errorf("%s: %w", id, errNoSourceFolder))
			continue
		}
		byFolder[folder] = append(byFolder[folder], id)
	}
	out := make([]accountBatch, 0, len(byFolder))
	for _, folder := range sortedKeys(byFolder) {
		out = append(out, accountBatch{client: p.active, folder: folder, ids: byFolder[folder]})
	}
	return out, errors.Join(errs...)
}

func sortedKeys(groups map[string][]email.ID) []string {
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// bulkOutcome is what a bulk action achieved: the ids the servers accepted
// and every failure, so the status line never counts a refused id as done.
type bulkOutcome struct {
	done []email.ID
	err  error
	// undo is what each accepted batch moved, for the actions that move.
	undo []undoBatch
}

// eachBatch runs act on every account's batch and collects the outcome.
func (m *tuiModel) eachBatch(p actionPlan, ids []email.ID, act func(accountBatch) error) bulkOutcome {
	batches, err := m.batches(p, ids)
	errs := []error{err}
	var done []email.ID
	for _, b := range batches {
		if err := act(b); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", accountLabel(b.client), err))
			continue
		}
		done = append(done, b.ids...)
	}
	return bulkOutcome{done: done, err: errors.Join(errs...)}
}

func accountLabel(cl mailclient.MailClient) string {
	if cl == nil || cl.Config() == nil || cl.Config().SelectedAccount == "" {
		return "account"
	}
	return string(cl.Config().SelectedAccount)
}

func (m *tuiModel) dispatchBulkArchive(p actionPlan, ids []email.ID) bulkOutcome {
	return m.dispatchBulkMove(p, ids, m.bk.folders().ResolveArchiveTarget)
}

// folderTargetResolver names the folder a bulk move files mail into, per client.
type folderTargetResolver func(mailclient.MailClient) (label.Folder, error)

// dispatchBulkMove moves ids out of their folder. In a unified inbox each
// message's own account resolves the destination and receives its own ids;
// otherwise the active client takes the whole batch.
func (m *tuiModel) dispatchBulkMove(p actionPlan, ids []email.ID, resolve folderTargetResolver) bulkOutcome {
	var undo []undoBatch
	o := m.eachBatch(p, ids, func(b accountBatch) error {
		tgt, err := resolve(b.client)
		if err != nil {
			return err
		}
		if err := b.client.MoveEmail(b.ids, label.FromServer(b.folder), tgt); err != nil {
			return err
		}
		undo = append(undo, newUndoBatch(b.client, tgt, label.FromServer(b.folder), b.ids))
		return nil
	})
	o.undo = undo
	return o
}

func (m *tuiModel) dispatchBulkDelete(p actionPlan, ids []email.ID) bulkOutcome {
	return m.dispatchBulkMove(p, ids, m.bk.folders().ResolveTrashTarget)
}

// dispatchBulkMarkRead marks ids read on their servers and records it locally.
func (m *tuiModel) dispatchBulkMarkRead(p actionPlan, ids []email.ID, record bool) bulkOutcome {
	return m.eachBatch(p, ids, func(b accountBatch) error {
		return setReadOnServer(b, true, record)
	})
}

// dispatchBulkMarkUnread clears ids' read state on their servers and locally.
func (m *tuiModel) dispatchBulkMarkUnread(p actionPlan, ids []email.ID, record bool) bulkOutcome {
	return m.eachBatch(p, ids, func(b accountBatch) error {
		return setReadOnServer(b, false, record)
	})
}

// setReadOnServer asks the server to set each message's read state, and, when
// record is set, writes the same decision into the local read cache first. The
// local write comes first so a backend with no server-side read state -- an
// mbox file -- still remembers what the user chose; it is skipped under
// --read-only, where recording a change the server never saw is exactly what
// read-only forbids.
func setReadOnServer(b accountBatch, read, record bool) error {
	record = record && b.client.Config() != nil
	if record {
		account := b.client.Config().AccountCache
		if read {
			_ = cache.MarkIDsRead(account, b.ids, nil)
		} else {
			_ = cache.MarkIDsUnread(account, b.ids)
		}
	}
	var err error
	if read {
		err = b.client.MarkAsRead(b.ids)
	} else {
		err = b.client.MarkAsUnread(b.ids)
	}
	// A failed mark stays pending, for the next folder load to retry.
	if err == nil && record {
		_ = cache.ConfirmReads(b.client.Config().AccountCache, b.ids)
	}
	return err
}

// retryPendingReads sends the account's unconfirmed read marks to the server
// again, confirming those it accepts and counting a failure against the rest.
// It runs before a folder is listed, so the listing already reflects them.
func retryPendingReads(client mailclient.MailClient) {
	if client == nil || client.Config() == nil {
		return
	}
	account := client.Config().AccountCache
	var read, unread []email.ID
	for id, p := range cache.LoadPendingReads(account) {
		if p.Read {
			read = append(read, id)
		} else {
			unread = append(unread, id)
		}
	}
	retry := func(ids []email.ID, mark func([]email.ID) error) {
		if len(ids) == 0 {
			return
		}
		if err := mark(ids); err != nil {
			slog.Warn("retrying pending read marks failed", slog.Int("count", len(ids)), slog.Any("error", err))
			_ = cache.NoteReadRetryFailed(account, ids)
			return
		}
		_ = cache.ConfirmReads(account, ids)
	}
	retry(read, client.MarkAsRead)
	retry(unread, client.MarkAsUnread)
}

// dispatchBulkSpam reports ids as spam. An account whose spam folder cannot be
// named still reports; its batch is recorded as one undo cannot move back.
func (m *tuiModel) dispatchBulkSpam(p actionPlan, ids []email.ID) bulkOutcome {
	var undo []undoBatch
	o := m.eachBatch(p, ids, func(b accountBatch) error {
		if err := b.client.ReportSpam(b.ids, label.FromServer(b.folder)); err != nil {
			return err
		}
		dest, err := m.bk.folders().ResolveSpamTarget(b.client)
		batch := newUndoBatch(b.client, dest, label.FromServer(b.folder), b.ids)
		if err != nil {
			batch.blocked = "the spam folder is unknown: " + err.Error()
		}
		undo = append(undo, batch)
		return nil
	})
	o.undo = undo
	return o
}

// unifiedAccountRows is one account's inbox rows for the unified inbox, read
// from that account's cache. Each row carries the account's own read and
// replied state: the loaders used to set none, or the active account's, so a
// refresh showed every other account's mail as unread and never replied.
func unifiedAccountRows(accCache cachedir.Account, idx int, acc cfg_acc.AccountConfig, ids []email.ID) []email.Email {
	isRead := cache.LoadReadState(accCache)
	isReplied := cache.LoadRepliedState(accCache)
	rows := make([]email.Email, 0, len(ids))
	for _, id := range ids {
		data, err := msg.Read(accCache, id)
		if err != nil {
			continue
		}
		em := email.ParseReader(bytes.NewReader(data), id, "")
		if em == nil {
			continue
		}
		stampAccount(em, idx, acc)
		em.IsRead = isRead[id]
		em.IsReplied = isReplied[id]
		if info, err := msg.GetInfo(accCache, id); err == nil {
			em.IsSpam = info.IsSpam()
			em.IsPolitical = info.IsPolitical()
			em.IsBlacklisted = info.IsBlacklisted()
		}
		rows = append(rows, *em)
	}
	return rows
}

// rowFlags is the read and replied state of the unified inbox's rows, each
// taken from its own account's cache, for merging into the session's maps.
// Without it the next rebuild of the list laid the active account's older
// state over the fresh rows.
func rowFlags(emails []email.Email) (isRead, isReplied map[email.ID]bool) {
	isRead = make(map[email.ID]bool, len(emails))
	isReplied = make(map[email.ID]bool, len(emails))
	for _, em := range emails {
		isRead[em.ID] = em.IsRead
		isReplied[em.ID] = em.IsReplied
	}
	return isRead, isReplied
}

// mergeFlags writes src's entries into dst, allocating dst if needed, and
// returns it. Entries src does not name are kept.
func mergeFlags(dst, src map[email.ID]bool) map[email.ID]bool {
	if dst == nil {
		dst = make(map[email.ID]bool, len(src))
	}
	for id, v := range src {
		dst[id] = v
	}
	return dst
}
