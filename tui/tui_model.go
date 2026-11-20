package tui

import (
	"bytes"
	"context"
	"fmt"
	"github.com/sarielhp/feral/cache"
	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/cache/folderindex"
	"github.com/sarielhp/feral/cache/msg"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/label"
	"github.com/sarielhp/feral/organize"
	"github.com/sarielhp/feral/pgp"
	"github.com/sarielhp/feral/report"
	"github.com/sarielhp/feral/scan"
	"github.com/sarielhp/feral/spam"
	"log/slog"
	"mime"
	"net/mail"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *tuiModel) Config() *cfg_g.Config {
	if m.client != nil {
		return m.client.Config()
	}
	return nil
}

func (m *tuiModel) isAccountReadOnly() bool {
	if m.Config() != nil && m.Config().ReadOnly {
		return true
	}
	if m.cfg != nil && m.cfg.ReadOnly {
		return true
	}
	accounts := m.getAccounts()
	idx := m.currentAccountIndex()
	if idx >= 0 && idx < len(accounts) {
		return accounts[idx].ReadOnly
	}
	return false
}

func (m *tuiModel) isCurrentFolderReadOnly() bool {
	if m.isAccountReadOnly() {
		return true
	}
	if m.lockedFolders != nil && m.lockedFolders[strings.ToLower(m.currentF)] {
		return true
	}
	accounts := m.getAccounts()
	idx := m.currentAccountIndex()
	if idx >= 0 && idx < len(accounts) {
		return accounts[idx].IsFolderReadOnly(m.currentF)
	}
	return false
}

// isDryRun reports whether the session reports changes instead of making
// them, local writes included.
func (m *tuiModel) isDryRun() bool {
	return (m.cfg != nil && m.cfg.DryRun) || (m.Config() != nil && m.Config().DryRun)
}

func (m *tuiModel) isReadOnly() bool {
	return m.isCurrentFolderReadOnly()
}

func (m *tuiModel) Init() tea.Cmd {
	if m.standaloneCompose {
		return m.initStandaloneComposeCmd()
	}
	m.lastTime = time.Now()
	m.lastCheckTime = time.Now()
	m.lastActivityTime = time.Now()
	var cmds []tea.Cmd
	cmds = append(cmds, tea.Tick(time.Second*10, func(time.Time) tea.Msg { return timeTickMsg{} }))
	cmds = append(cmds, m.bgFetchFoldersCmd())
	if m.currentF != "" {
		if len(m.emails) == 0 {
			m.showLoad = "Downloading..."
		}
		cmds = append(cmds, m.loadEmailsCmd(m.currentF))
	} else {
		m.showLoad = "Downloading..."
		cmds = append(cmds, func() tea.Msg {
			items, err := m.client.GetLabelItems()
			if err != nil {
				return errMsg{err}
			}
			return foldersLoadedMsg{folders: items}
		})
	}
	return tea.Batch(cmds...)
}

func (m *tuiModel) bgFetchFoldersCmd() tea.Cmd {
	return func() tea.Msg {
		items, err := m.client.GetLabelItems()
		if err != nil {
			return errMsg{err}
		}
		return foldersLoadedMsg{folders: items}
	}
}

func (m *tuiModel) loadEmailsCmd(folder string) tea.Cmd {
	m.lastCheckTime = time.Now()
	return m.loadEmailsWithSilentOption(folder, false)
}

func (m *tuiModel) bgCheckNewMailCmd(folder string) tea.Cmd {
	return m.loadEmailsWithSilentOption(folder, true)
}

func (m *tuiModel) loadEmailsWithSilentOption(folder string, silentErr bool) tea.Cmd {
	if isUnifiedInbox(folder) {
		return m.loadUnifiedEmailsCmd()
	}
	client := m.client
	// Never under read-only: the read-only client simulates the mark, and
	// confirming it would drop a mark the server never saw.
	retry := !m.isAccountReadOnly()
	gen := m.flagGen
	return func() tea.Msg {
		if client == nil || client.Config() == nil {
			return bgCheckDoneMsg{folder: folder, foreground: !silentErr}
		}
		if retry {
			retryPendingReads(client)
		}
		cacheDir := client.Config().AccountCache
		slog.Info("loadEmailsCmd: calling FetchAndDownloadEmails", slog.String("folder", folder), slog.String("cacheDir", cacheDir.Path()))
		ids, err := client.FetchAndDownloadEmails(label.FromServer(folder), "")
		if err != nil {
			if silentErr {
				slog.Warn("bgCheckNewMailCmd: FetchAndDownloadEmails failed", slog.String("folder", folder), slog.Any("error", err))
				return bgCheckDoneMsg{folder: folder, err: err}
			}
			slog.Error("loadEmailsCmd: FetchAndDownloadEmails failed", slog.String("folder", folder), slog.Any("error", err))
			return errMsg{err}
		}
		// Locals, not model fields: this runs on a tea.Cmd goroutine, and the key
		// handlers write these same maps from Update.
		isRead := cache.LoadReadState(cacheDir)
		isReplied := cache.LoadRepliedState(cacheDir)
		var msgs []email.Email
		var classifyIDs []email.ID
		// Sent and Drafts hold the user's own mail: no verdict, and any cached one is cleared.
		authored := organize.IsAuthoredFolder(client, label.FromServer(folder))
		stamp := scan.Stamp(client.Config())
		for _, id := range ids {
			if authored {
				_ = msg.ForgetEvidence(cacheDir, id)
			}
			// A message that will not load is left out rather than shown as an
			// error row: the TUI is a browser, not a listing.
			var em *email.Email
			var err error
			if authored {
				em, err = msg.CachedEmailRecipient(cacheDir, id)
			} else {
				em, err = msg.CachedEmail(cacheDir, id)
			}
			if err != nil {
				continue
			}
			em.IsAuthored = authored
			em.IsRead = isRead[id]
			em.IsReplied = isReplied[id]
			needsClassify := !authored
			if info, infoErr := msg.GetInfo(cacheDir, id); infoErr == nil && !authored {
				needsClassify = info.NeedsScan(stamp)
			}
			if needsClassify {
				classifyIDs = append(classifyIDs, id)
			}
			msgs = append(msgs, *em)
		}
		sort.Slice(msgs, func(i, j int) bool {
			return msgs[i].EmailDate.Before(msgs[j].EmailDate)
		})
		return emailsLoadedMsg{
			client:      client,
			folder:      folder,
			emails:      msgs,
			classifyIDs: classifyIDs,
			isRead:      isRead,
			isReplied:   isReplied,
			flagGen:     gen,
		}
	}
}

func (m *tuiModel) canCheckNewMail() bool {
	if m.client == nil || m.client.Config() == nil {
		return false
	}
	if m.standaloneCompose || m.done || m.exitPending {
		return false
	}
	if m.currentF == "" || isSearchView(m.currentF) || isVirtualView(m.currentF) {
		return false
	}
	if m.bgCheckingMail || m.showLoad != "" {
		return false
	}
	if m.mode == modeCompose || m.inSearch {
		return false
	}
	if m.treeOpen || m.accountOverlayOpen || m.menuOpen || m.spamMenuOpen || m.blockModalOpen {
		return false
	}
	return true
}

func (m *tuiModel) handleTimeTick() (tea.Model, tea.Cmd) {
	m.lastTime = time.Now()
	m.cleanupIsReadMap()
	cmds := []tea.Cmd{
		tea.Tick(time.Second*10, func(time.Time) tea.Msg { return timeTickMsg{} }),
	}
	if m.relayCheckDue(m.lastTime) {
		cmds = append(cmds, m.startRelayCheck(m.lastTime))
	}
	if time.Since(m.lastCheckTime) >= autoRefreshInterval && m.canCheckNewMail() {
		m.lastCheckTime = time.Now()
		m.bgCheckingMail = true
		cmds = append(cmds, m.bgCheckNewMailCmd(m.currentF))
	}
	return m, tea.Batch(cmds...)
}

func (m *tuiModel) bgClassifyCmd(ids []email.ID) tea.Cmd {
	return func() tea.Msg {
		if len(ids) == 0 {
			return classifyDoneMsg{}
		}
		slog.Info("Background classification starting", slog.Int("count", len(ids)))
		_, _, _, err := scan.Emails(report.Or(m.rep), ids, m.client.Config(), "")
		if err != nil {
			slog.Error("Background classification failed", slog.Any("error", err))
		} else {
			slog.Info("Background classification complete", slog.Int("count", len(ids)))
		}
		return classifyDoneMsg{}
	}
}

// loadDetailCmd plans on the Update goroutine which account holds id; the
// command reads neither m.currentF nor the rows.
func (m *tuiModel) loadDetailCmd(id email.ID) tea.Cmd {
	// A message-view filter was typed for the message being replaced.
	m.endDetailFilter()
	plan := m.planAction([]email.ID{id}, m.currentF)
	return func() tea.Msg {
		client, err := m.targetClient(plan, id)
		if err != nil {
			return detailMsg{headers: "(Unavailable)", body: "(Could not resolve this message's account: " + err.Error() + ")"}
		}
		data, err := client.GetRawMessage(id)
		if err != nil {
			return detailMsg{headers: "(Unavailable)", body: "(Could not retrieve this message: " + err.Error() + ")"}
		}
		msg, err := mail.ReadMessage(bytes.NewReader(data))
		if err != nil {
			return detailMsg{headers: "(Parse error)", body: err.Error()}
		}
		dec := new(mime.WordDecoder)
		subj := email.DecDecode(dec, msg.Header.Get("Subject"))
		frm := email.DecDecode(dec, msg.Header.Get("From"))
		dS := msg.Header.Get("Date")
		to := email.FormatAddressListVerbose(dec, msg.Header.Get("To"))
		cc := email.FormatAddressListNames(dec, msg.Header.Get("Cc"))
		bcc := email.FormatAddressListNames(dec, msg.Header.Get("Bcc"))

		var hdrLines []string
		hdrLines = append(hdrLines, fmt.Sprintf("From: %s", frm))
		if to != "" {
			hdrLines = append(hdrLines, fmt.Sprintf("To: %s", to))
		}
		if cc != "" {
			hdrLines = append(hdrLines, fmt.Sprintf("Cc: %s", cc))
		}
		if bcc != "" {
			hdrLines = append(hdrLines, fmt.Sprintf("Bcc: %s", bcc))
		}
		hdrLines = append(hdrLines, fmt.Sprintf("Subject: %s", subj), fmt.Sprintf("Date: %s", dS))
		hdrs := strings.Join(hdrLines, "\n")

		// PGP runs before the body is read: for an encrypted message there is
		// no body to read until it has been decrypted, and the decrypted
		// entity -- not the multipart envelope -- is what carries the
		// attachments.
		processed := pgp.Process(context.Background(), data, pgp.Options{Verify: true, Decrypt: true})
		atts, _ := email.ExtractAttachments(processed.Raw)
		// The detail renderer wraps at the current viewport width after
		// compacting links; prewrapping here can damage URLs and spacing.
		return detailMsg{headers: hdrs, body: detailBody(processed), attachments: atts, crypto: processed.Status}
	}
}

// detailBody is the text of a message after the PGP layer: the plaintext when
// the message was decrypted or was an inline signature, and the message's own
// plain part otherwise.
func detailBody(processed pgp.Result) string {
	if processed.Body != "" {
		return email.EmbedOSC8Hyperlinks(email.StripEscapeSequences(processed.Body))
	}
	msg, err := mail.ReadMessage(bytes.NewReader(processed.Raw))
	if err != nil {
		return ""
	}
	body, _ := email.ExtractPlainBodyText(msg)
	return email.EmbedOSC8Hyperlinks(body)
}

// cmdArchive takes a plan made on the Update goroutine rather than reading
// m.currentF or the rows inside the closure. This runs on a tea.Cmd goroutine,
// so reading the field there picked up whatever folder the user had navigated
// to by the time the network call ran -- and that value is sent as the move's
// *source* label, which on Gmail removes the wrong label.
func (m *tuiModel) cmdArchive(id email.ID, plan actionPlan) tea.Cmd {
	return func() tea.Msg {
		client, realSrc, err := m.target(plan, id)
		if err != nil {
			return errMsg{err: err}
		}
		tgt, err := m.bk.folders().ResolveArchiveTarget(client)
		if err != nil {
			return errMsg{err: err}
		}
		// The index and the stale classification are reconciled by
		// CheckingMailClient, which wraps this client and owns that job for
		// every caller. Doing it again here was not merely redundant: the
		// decorator skips reconciliation under --read-only, because the move
		// was only simulated, and this copy did not -- so the cache recorded a
		// move the server never made.
		if err := client.MoveEmail([]email.ID{id}, label.FromServer(realSrc), tgt); err != nil {
			return errMsg{err: err}
		}
		undo := newUndoBatch(client, tgt, label.FromServer(realSrc), []email.ID{id})
		return archiveDoneMsg{id: id, undo: &undo}
	}
}

// cmdSpam marks a message as spam, through the same path as `spam mark`. The
// undo it reports moves the message back from the spam folder to the folder
// it was taken from, named as archive and delete name it, and puts back the
// override the mark dropped with the cached copy. Whether the mark moved it
// at all is decided as the mark decides it, from the folder the cache places
// it in.
func (m *tuiModel) cmdSpam(id email.ID, plan actionPlan) tea.Cmd {
	return func() tea.Msg {
		client, err := m.targetClient(plan, id)
		if err != nil {
			return errMsg{err: err}
		}
		_, source, srcErr := m.target(plan, id)
		cfg := client.Config()
		// Existence is all this needs; reading the body to find out wasted the
		// read.
		if ok, existsErr := msg.Exists(cfg.AccountCache, id); existsErr != nil || !ok {
			return spamDoneMsg{id: id}
		}
		_, from, findErr := cache.FindCachedEmailByID(cfg.AccountCache, string(id))
		prior := priorOverrides(cfg.AccountCache, []email.ID{id})
		trained, err := spam.MarkByIDTrained(report.Or(m.rep), client, cfg, string(id))
		if err != nil {
			slog.Warn("marking message as spam failed", slog.String("id", string(id)), slog.Any("error", err))
			return spamDoneMsg{id: id}
		}
		dest, destErr := m.bk.folders().ResolveSpamTarget(client)
		if findErr != nil || srcErr != nil || destErr != nil || cfg.ReadOnly || isSpamFolderName(cfg, from) {
			// Nothing moved, or nothing says where it went.
			return spamDoneMsg{id: id}
		}
		undo := newUndoBatch(client, dest, label.FromServer(source), []email.ID{id})
		undo.overrides = prior
		undo.training = map[email.ID]spam.Training{id: trained}
		return spamDoneMsg{id: id, undo: &undo}
	}
}

// isSpamFolderName reports whether folder is the account's spam folder, as
// spam.MarkByID decides it: a message already there is learned, not moved.
func isSpamFolderName(cfg *cfg_g.Config, folder string) bool {
	return (cfg.SpamFolder != "" && strings.EqualFold(folder, cfg.SpamFolder)) || strings.EqualFold(folder, "spam")
}

func (m *tuiModel) cmdUnspam(id email.ID) tea.Cmd {
	return func() tea.Msg {
		startTime := time.Now()
		cfg := m.client.Config()
		err := spam.UnspamByID(report.Or(m.rep), m.client, cfg, string(id))
		elapsed := time.Since(startTime)
		if elapsed < time.Second {
			time.Sleep(time.Second - elapsed)
		}
		if err != nil {
			return errMsg{err: err}
		}
		return unspamDoneMsg{id: id}
	}
}

// cmdDelete takes a plan; see cmdArchive.
func (m *tuiModel) cmdDelete(id email.ID, plan actionPlan) tea.Cmd {
	return func() tea.Msg {
		client, realSrc, err := m.target(plan, id)
		if err != nil {
			return errMsg{err: err}
		}
		cfg := client.Config()
		cd := cfg.AccountCache
		prior := priorOverrides(cd, []email.ID{id})
		tgt, err := m.bk.folders().ResolveTrashTarget(client)
		moved := false
		if err == nil {
			// As in cmdArchive, the index is CheckingMailClient's job.
			moved = client.MoveEmail([]email.ID{id}, label.FromServer(realSrc), tgt) == nil
		}
		// The cached body goes only when the server accepted the move; dropping
		// it after a refusal would hide a message that is still in the folder.
		if !moved {
			return deleteDoneMsg{id: id}
		}
		_ = msg.Delete(cd, id)
		undo := newUndoBatch(client, tgt, label.FromServer(realSrc), []email.ID{id})
		undo.overrides = prior
		return deleteDoneMsg{id: id, undo: &undo}
	}
}

func (m *tuiModel) loadCachedEmailsForFolder(folder string) {
	if isUnifiedInbox(folder) {
		m.loadUnifiedCachedEmails()
		return
	}
	if m.client == nil {
		slog.Warn("loadCachedEmailsForFolder: client is nil")
		return
	}
	if m.client.Config() == nil {
		slog.Warn("loadCachedEmailsForFolder: client config is nil")
		return
	}
	cacheDir := m.client.Config().AccountCache
	slog.Info("loadCachedEmailsForFolder starting", slog.String("folder", folder), slog.String("cacheDir", cacheDir.Path()))

	if !folderindex.HasStructure(cacheDir) {
		slog.Info("loadCachedEmailsForFolder: cache structure not found, skipping")
		return
	}

	ids, err := folderindex.IDs(cacheDir, folder)
	if err != nil {
		slog.Info("loadCachedEmailsForFolder: no folder index", slog.String("folder", folder))
		return
	}
	var cachedEmails []email.Email
	authored := organize.IsAuthoredFolder(m.client, label.FromServer(folder))
	for _, id := range ids {
		if em, ok := m.loadCachedEmail(cacheDir, id, authored); ok {
			cachedEmails = append(cachedEmails, em)
		}
	}

	prevSelectedID := m.selectedID
	prevFolder := m.currentF

	m.rawEmails = cachedEmails
	m.rebuildVisibleEmails()

	if prevFolder == folder && prevSelectedID != "" {
		found := false
		for i, e := range m.emails {
			if e.ID == prevSelectedID {
				m.eIdx = i
				m.selectedID = prevSelectedID
				found = true
				break
			}
		}
		if !found {
			if len(m.emails) > 0 {
				m.eIdx = min(m.eIdx, len(m.emails)-1)
				m.selectedID = m.emails[m.eIdx].ID
			} else {
				m.eIdx = 0
				m.selectedID = ""
			}
		}
	} else {
		m.folderJustChanged = true
		if len(m.emails) > 0 {
			m.eIdx = m.openingIndex()
			m.selectedID = m.emails[m.eIdx].ID
		} else {
			m.eIdx = 0
			m.selectedID = ""
		}
	}
	m.adjustScrollOffset()
}

// loadCachedEmail reads one message from the account cache and overlays the
// session's read/replied state and the stored spam classification. authored
// marks the folder it was read from as one the owner wrote, which the row
// renderer shows as a recipient. ok is false when the message is absent or
// unparseable.
func (m *tuiModel) loadCachedEmail(cacheDir cachedir.Account, id email.ID, authored bool) (email.Email, bool) {
	data, err := msg.Read(cacheDir, id)
	if err != nil {
		return email.Email{}, false
	}
	em := email.ParseReader(bytes.NewReader(data), id, "")
	if em == nil {
		return email.Email{}, false
	}
	em.IsAuthored = authored
	em.IsRead = m.isRead[id]
	em.IsReplied = m.isReplied[id]
	if info, err := msg.GetInfo(cacheDir, id); err == nil {
		em.IsSpam = info.IsSpam()
		em.IsPolitical = info.IsPolitical()
		em.IsBlacklisted = info.IsBlacklisted()
	}
	return *em, true
}

func (m *tuiModel) removeEmailByID(id email.ID) {
	m.rawEmails = withoutEmail(m.rawEmails, id)
	if m.savedRawEmails != nil {
		// Under "~" the folder rows wait here; a removed message must not return with them.
		m.savedRawEmails = withoutEmail(m.savedRawEmails, id)
	}
	m.rebuildVisibleEmails()
	if len(m.emails) > 0 {
		m.eIdx = min(m.eIdx, len(m.emails)-1)
		m.selectedID = m.emails[m.eIdx].ID
	} else {
		m.eIdx = 0
		m.selectedID = ""
	}
	m.adjustScrollOffset()
}

// withoutEmail returns rows less the message id.
func withoutEmail(rows []email.Email, id email.ID) []email.Email {
	kept := make([]email.Email, 0, len(rows))
	for _, e := range rows {
		if e.ID != id {
			kept = append(kept, e)
		}
	}
	return kept
}

func (m *tuiModel) cleanupIsReadMap() {
	if len(m.isRead) > 10000 {
		slog.Info("Cleaning up isRead map", slog.Int("currentSize", len(m.isRead)))
		ids := make(map[email.ID]bool)
		for _, e := range m.rawEmails {
			ids[e.ID] = true
		}
		for id := range m.isRead {
			if !ids[id] {
				delete(m.isRead, id)
			}
		}
		slog.Info("Cleaned up isRead map", slog.Int("newSize", len(m.isRead)))
	}
}

func (m *tuiModel) loadAllCachedEmails() {
	if m.client == nil || m.client.Config() == nil {
		return
	}
	cacheDir := m.client.Config().AccountCache
	if !folderindex.HasStructure(cacheDir) {
		return
	}

	var allEmails []email.Email
	if len(m.folders) == 0 {
		return
	}

	for _, f := range m.folders {
		ids, err := folderindex.IDs(cacheDir, f.FullName)
		if err != nil {
			continue
		}
		authored := organize.IsAuthoredFolder(m.client, label.FromServer(f.FullName))
		for _, id := range ids {
			if em, ok := m.loadCachedEmail(cacheDir, id, authored); ok {
				allEmails = append(allEmails, em)
			}
		}
	}

	sort.Slice(allEmails, func(i, j int) bool {
		return allEmails[i].EmailDate.Before(allEmails[j].EmailDate)
	})

	m.savedRawEmails = m.rawEmails
	m.savedCurrentF = m.currentF
	m.rawEmails = allEmails
}
