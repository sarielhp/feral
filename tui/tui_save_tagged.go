package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/cache/virtual"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/mailclient"
)

// saveNameKind is what the name typed into the save-name prompt is for. The
// prompt is one overlay for both gestures, so the kind is what tells them
// apart once Enter arrives.
type saveNameKind int

const (
	saveNameNone saveNameKind = iota
	saveNameVirtualFolder
	saveNameMbox
)

// errSaveSpansAccounts refuses to freeze a tagged set that spans accounts: a
// saved search is one file in one account's cache, and splitting it would need
// the user to decide how. The mbox export has no such limit, so the message
// names it.
var errSaveSpansAccounts = errors.New("this set spans accounts, and a saved search belongs to one; use ;x to export it to an mbox file instead")

// errSaveNeedsFolder refuses to freeze a tagged set from a result-set view: a
// set records one source folder for the whole of it, which is the folder the
// tags were made in. A remote search's results span folders with nothing
// recording which, and a saved set's rows each carry their own, so neither view
// is the one folder a new set is frozen from.
var errSaveNeedsFolder = errors.New("a result-set view is not one folder to freeze a set from; save one from a folder, or use ;x to export it to an mbox file")

// beginSaveName opens the name prompt ;v and ;x share.
func (m *tuiModel) beginSaveName(kind saveNameKind) (tea.Model, tea.Cmd) {
	if m.countTagged() == 0 {
		return m, m.showNotice("Tag messages with t first, then " + saveNameKeyHint(kind))
	}
	m.saveNameKind = kind
	m.saveNameQuery = ""
	m.inSaveNamePrompt = true
	return m, nil
}

// saveNameKeyHint is the key the user pressed, for the "nothing tagged yet"
// notice, which has to say which gesture they are in.
func saveNameKeyHint(kind saveNameKind) string {
	if kind == saveNameMbox {
		return ";x"
	}
	return ";v"
}

// kSaveName drives the shared name prompt. Esc and an empty submit close it
// without acting; the kind is read before the call because kPromptLine clears
// the prompt on every path that ends it.
func (m *tuiModel) kSaveName(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	kind := m.saveNameKind
	name, ok := m.kPromptLine(k, &m.saveNameQuery, &m.inSaveNamePrompt)
	if !m.inSaveNamePrompt {
		m.saveNameKind = saveNameNone
	}
	if !ok {
		return m, nil
	}
	switch kind {
	case saveNameVirtualFolder:
		return m.saveTaggedAsVirtualFolder(name)
	case saveNameMbox:
		return m.beginMboxExport(name)
	}
	return m, nil
}

// saveTaggedAsVirtualFolder freezes the tagged set as a named saved search in
// the account the set belongs to, where F7 lists it.
//
// The tags are deliberately kept: nothing left the view, so the same set can
// be exported to an mbox file afterwards with ;x.
func (m *tuiModel) saveTaggedAsVirtualFolder(name string) (tea.Model, tea.Cmd) {
	ids := m.getTaggedIDs()
	if len(ids) == 0 {
		return m, nil
	}
	if isSearchView(m.currentF) || isVirtualView(m.currentF) {
		m.err = errSaveNeedsFolder
		m.showError = true
		return m, nil
	}

	plan := m.planAction(ids, m.currentF)
	batches, err := m.batches(plan, ids)
	if err != nil {
		m.err = err
		m.showError = true
		return m, nil
	}
	if len(batches) != 1 {
		m.err = errSaveSpansAccounts
		m.showError = true
		return m, nil
	}

	batch := batches[0]
	account := setCache(batch.client)
	if account.IsZero() {
		m.err = errors.New("no cache directory is configured to save into")
		m.showError = true
		return m, nil
	}
	// A curated set is the user's work, so a name that is taken is refused
	// rather than overwritten: virtual.Save replaces the file unconditionally,
	// and a typo would otherwise lose the set without a word. F7's `d` is how
	// one is removed, so the refusal names it.
	if _, err := virtual.Load(account, name); err == nil {
		m.err = fmt.Errorf("a saved search named %q already exists; delete it in F7 to save over it", name)
		m.showError = true
		return m, nil
	}
	folderMap := make(map[email.ID]string, len(ids))
	for _, id := range batch.ids {
		folderMap[id] = batch.folder
	}
	vm := &virtual.Mailbox{
		Name:        name,
		Description: fmt.Sprintf("Tagged set of %d message(s)", len(ids)),
		MessageIDs:  ids,
		FolderMap:   folderMap,
	}
	if err := virtual.Save(account, vm); err != nil {
		m.err = fmt.Errorf("cannot save %q: %w", name, err)
		m.showError = true
		return m, nil
	}
	return m, m.showNotice(fmt.Sprintf("Saved %q (%d message(s)); F7 lists it", name, len(ids)))
}

// pendingMboxExport is a tagged set waiting for a directory: the name to write
// under, and each account's slice of the set with the client that reads it.
type pendingMboxExport struct {
	name    string
	sources []MboxSource
}

// beginMboxExport asks where to put the tagged set's mbox file. The name was
// given already, so the save dialog only chooses a directory.
func (m *tuiModel) beginMboxExport(name string) (tea.Model, tea.Cmd) {
	ids := m.getTaggedIDs()
	if len(ids) == 0 {
		return m, nil
	}
	plan := m.planAction(ids, m.currentF)
	batches, err := m.clientBatches(plan, ids)
	if err != nil {
		m.err = err
		m.showError = true
		return m, nil
	}
	sources := make([]MboxSource, 0, len(batches))
	for _, b := range batches {
		sources = append(sources, MboxSource{Client: b.client, IDs: b.ids})
	}
	m.pendingMbox = &pendingMboxExport{name: name, sources: sources}

	dir, err := os.Getwd()
	if err != nil {
		dir, _ = os.UserHomeDir()
	}
	m.openSaveDialog(dir, email.Attachment{Filename: mboxFileName(name)})
	return m, nil
}

// appendPendingMbox writes the tagged set into the directory the save dialog
// chose, appending to <name>.mbox so repeated exports accumulate rather than
// replace. The write is off the Update goroutine because it downloads.
func (m *tuiModel) appendPendingMbox() tea.Cmd {
	pend := m.pendingMbox
	m.pendingMbox = nil
	if pend == nil {
		return nil
	}
	path := filepath.Join(m.fileBrowser.SelectedDir, mboxFileName(pend.name))
	mbox := m.bk.mbox()
	sources := pend.sources
	return func() tea.Msg {
		n, err := mbox.AppendMessagesToMbox(path, sources)
		return mboxExportDoneMsg{path: path, count: n, err: err}
	}
}

// mboxExportDoneMsg reports the outcome of an mbox export.
type mboxExportDoneMsg struct {
	path  string
	count int
	err   error
}

func (m *tuiModel) handleMboxExportDone(msg mboxExportDoneMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.err = fmt.Errorf("writing %s: %w", msg.path, msg.err)
		m.showError = true
		return m, nil
	}
	if msg.count == 0 {
		return m, m.showNotice("Nothing to write to " + msg.path)
	}
	return m, m.showNotice(fmt.Sprintf("Appended %d message(s) to %s", msg.count, msg.path))
}

// mboxFileName is the file a set is written to. The name is the user's, so it
// is sanitized the way every other cache-bound name is.
func mboxFileName(name string) string {
	return cfg_g.SanitizeLabelForCache(name) + ".mbox"
}

// setCache is the account cache a set's client writes to, or the zero Account
// when the client cannot name one.
func setCache(cl mailclient.MailClient) cachedir.Account {
	if cl == nil || cl.Config() == nil {
		return cachedir.Account{}
	}
	return cl.Config().AccountCache
}
