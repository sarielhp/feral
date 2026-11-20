package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// The TUI's screens as one ordered table, the top first. Both the key handler
// and the view read it: the top open layer is the screen drawn and the screen
// that gets the next key. They used to keep two orders of their own --
// handleKey's checks and view's -- which drifted, so a screen could draw one
// thing while the keys went to another: the folder tree vanished while its
// search took the keys; a click behind the sort menu opened a message.
//
// A layer is open when its flags say so. Opening a screen is still a matter
// of setting its flags; leaving one goes through its helper (showList,
// closeTree, endListFilter, endDetailFilter), so no flag outlives its screen.

// layerGroup says which of the global keys reach a layer: each group sees
// the ones of the groups above it as well.
type layerGroup int

const (
	groupPrompt  layerGroup = iota // one-line prompts: every key is text
	groupExiting                   // the wait for a draft upload on the way out
	groupQuit                      // the quit dialog: Ctrl+C quits from here down
	groupHelp                      // help: F9 toggles the mouse from here down
	groupModal                     // compose, save and attachment screens: l signs in from here down
	groupOverlay                   // overlays: F1 opens help from here down
	groupBase                      // tree, searches, message, list: Ctrl+D opens diagnostics here
)

// layer is one screen in the table.
type layer struct {
	id    screen
	group layerGroup
	open  func(m *tuiModel) bool
	key   func(m *tuiModel, k tea.KeyMsg) (tea.Model, tea.Cmd)
	frame string                   // what a click is read against while it is drawn
	draw  func(m *tuiModel) string // nil: drawn by the layer beneath
}

// screenLayers is the table, the top first. It is built in init because the
// handlers in it reach handleKey, which reads it: a package-level initializer
// would be an initialization cycle.
var screenLayers []layer

func init() {
	screenLayers = joinLayers(promptLayers(), modalLayers(), overlayLayers(), baseLayers())
}

func joinLayers(groups ...[]layer) []layer {
	var out []layer
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}

// keyOnly adapts a handler that takes the key name.
func keyOnly(f func(m *tuiModel, s string) (tea.Model, tea.Cmd)) keyFunc {
	return func(m *tuiModel, k tea.KeyMsg) (tea.Model, tea.Cmd) { return f(m, k.String()) }
}

type (
	openFunc = func(m *tuiModel) bool
	keyFunc  = func(m *tuiModel, k tea.KeyMsg) (tea.Model, tea.Cmd)
	drawFunc = func(m *tuiModel) string
)

// drawBase draws the list or message itself; prompts, searches and the menu
// draw into it.
func drawBase(m *tuiModel) string { return m.renderBaseScreen() }

// dimmed draws content over the dimmed list or message.
func dimmed(m *tuiModel, content string) string {
	return renderDimmedOverlay(m.width, m.height, m.renderBaseScreen(), content)
}

// banner draws a screen with the error banner under it. The relay banner is
// the base screen's alone: it stays up for hours, and these screens do not
// give up a row for it.
func banner(render drawFunc) drawFunc {
	return func(m *tuiModel) string { return m.withErrorBanner(render(m)) }
}

// dimmedBanner draws a screen over the dimmed list, with the error banner.
func dimmedBanner(render drawFunc) drawFunc {
	return func(m *tuiModel) string { return m.withErrorBanner(dimmed(m, render(m))) }
}

// onBase is a layer that draws into the list or message: a prompt, a search,
// the menu panel.
func onBase(id screen, g layerGroup, open openFunc, key keyFunc) layer {
	return layer{id: id, group: g, open: open, key: key, frame: frameBase, draw: drawBase}
}

// drawn is a layer that draws a screen of its own.
func drawn(id screen, g layerGroup, open openFunc, key keyFunc, frame string, draw drawFunc) layer {
	return layer{id: id, group: g, open: open, key: key, frame: frame, draw: draw}
}

func promptLayers() []layer {
	return []layer{
		onBase(scrPipePrompt, groupPrompt, func(m *tuiModel) bool { return m.inPipePrompt }, (*tuiModel).kPipePrompt),
		onBase(scrTagPattern, groupPrompt, func(m *tuiModel) bool { return m.inTagPatternPrompt }, (*tuiModel).kTagPattern),
		onBase(scrSaveName, groupPrompt, func(m *tuiModel) bool { return m.inSaveNamePrompt }, (*tuiModel).kSaveName),
		onBase(scrMovePrompt, groupPrompt, func(m *tuiModel) bool { return m.inMovePrompt }, (*tuiModel).kMovePrompt),
		onBase(scrSnoozePrompt, groupPrompt, func(m *tuiModel) bool { return m.inSnoozePrompt }, (*tuiModel).kSnoozePrompt),
		onBase(scrTagPrefix, groupPrompt, func(m *tuiModel) bool { return m.inTagPrefix }, (*tuiModel).kTagPrefix),
		onBase(scrRemoteSearch, groupPrompt, func(m *tuiModel) bool { return m.inRemoteSearch }, (*tuiModel).kRemoteSearchPrompt),
		{id: scrExiting, group: groupExiting, open: func(m *tuiModel) bool { return m.exitPending },
			key: keyOnly((*tuiModel).handleKeyWhileExiting)},
		drawn(scrQuitConfirm, groupQuit, func(m *tuiModel) bool { return m.quitConfirmOpen }, keyOnly((*tuiModel).kQuitConfirm),
			frameQuit, func(m *tuiModel) string { return dimmed(m, quitConfirmBox()) }),
		drawn(scrHelpSearch, groupHelp, func(m *tuiModel) bool { return m.showHelp && m.helpFind.typing }, (*tuiModel).kHelp,
			frameHelp, drawHelp),
		drawn(scrHelp, groupHelp, func(m *tuiModel) bool { return m.showHelp }, (*tuiModel).kHelp, frameHelp, drawHelp),
	}
}

// drawHelp draws the help overlay, searched or not, over the dimmed screen.
func drawHelp(m *tuiModel) string {
	return dimmed(m, m.withErrorBanner(renderHelp(m)))
}

// modalLayers are compose with the pickers it draws over itself, the save
// dialogs and the attachment screens.
func modalLayers() []layer {
	compose := func(m *tuiModel) bool { return m.mode == modeCompose }
	inCompose := func(open openFunc) openFunc { return func(m *tuiModel) bool { return compose(m) && open(m) } }
	files := dimmedBanner(func(m *tuiModel) string { return m.fileBrowser.View() })
	fileBrowser := func(m *tuiModel) bool { return m.fileBrowserOpen }
	attModal := func(m *tuiModel) bool { return m.attModalOpen }
	accounts := func(m *tuiModel) bool { return m.accountOverlayOpen }
	aliases := func(m *tuiModel) bool { return m.aliasView != nil }
	return []layer{
		drawn(scrFileBrowser, groupModal, inCompose(fileBrowser), (*tuiModel).kFileBrowser, frameFiles, files),
		drawn(scrAttachment, groupModal, inCompose(attModal), (*tuiModel).kAttachmentModal, frameAttModal, banner(renderAttachmentModalOverlay)),
		drawn(scrAccounts, groupModal, inCompose(accounts), (*tuiModel).kAccountOverlay, frameAccount, banner(renderAccountOverlay)),
		drawn(scrAliases, groupModal, inCompose(aliases), (*tuiModel).kAliasBrowser, frameAliases, banner(renderAliasBrowser)),
		drawn(scrContacts, groupModal, inCompose(func(m *tuiModel) bool { return m.contactPick != nil }),
			(*tuiModel).kContactPicker, frameContacts, banner(renderContactPicker)),
		drawn(scrCompose, groupModal, compose, (*tuiModel).handleComposeKey, frameCompose, banner((*tuiModel).renderComposeScreen)),
		drawn(scrSaveHelp, groupModal, func(m *tuiModel) bool { return m.saveHelpOpen && m.saving() },
			(*tuiModel).kSaving, frameSaveHelp, banner(renderSaveHelp)),
		drawn(scrSaveToFolder, groupModal, func(m *tuiModel) bool { return m.savingMessageToFolder },
			(*tuiModel).kSaveToFolder, frameTree, dimmedBanner(renderSaveFolderDialog)),
		drawn(scrSaveToDisk, groupModal, func(m *tuiModel) bool { return m.savingMessageToDisk && m.fileBrowserOpen },
			(*tuiModel).kSaveToDisk, frameFiles, files),
		drawn(scrFileBrowser, groupModal, fileBrowser, (*tuiModel).kFileBrowser, frameFiles, files),
		drawn(scrAttachmentList, groupModal, func(m *tuiModel) bool { return m.attachmentListOpen },
			(*tuiModel).handleAttachmentListKey, frameAttList, banner((*tuiModel).renderAttachmentListView)),
		drawn(scrICS, groupModal, func(m *tuiModel) bool { return m.icsModalOpen },
			(*tuiModel).handleICSModalKey, frameICS, banner(renderICSModalOverlay)),
		drawn(scrAttachment, groupModal, attModal, (*tuiModel).kAttachmentModal, frameAttModal, banner(renderAttachmentModalOverlay)),
		drawn(scrAttachmentHelp, groupModal, func(m *tuiModel) bool { return m.attachmentHelpOpen }, (*tuiModel).kAttachmentHelp,
			frameAttHelp, func(m *tuiModel) string { return dimmed(m, m.withErrorBanner(renderAttachmentHelp(m))) }),
		onBase(scrAttachmentBar, groupModal, func(m *tuiModel) bool { return m.attachmentDrawer }, (*tuiModel).kAttachmentDrawer),
	}
}

// overlayLayers are the overlays opened from the list or a message, in the
// order the key handler has always checked them.
func overlayLayers() []layer {
	g := groupOverlay
	return []layer{
		onBase(scrMenu, g, func(m *tuiModel) bool { return m.menuOpen }, (*tuiModel).kMenu),
		drawn(scrSpamMenu, g, func(m *tuiModel) bool { return m.spamMenuOpen }, (*tuiModel).kSpamMenu, frameSpam, banner(renderSpamMenu)),
		drawn(scrDiag, g, func(m *tuiModel) bool { return m.showDiag }, (*tuiModel).kDiag, frameDiag, renderDiag),
		drawn(scrSummary, g, func(m *tuiModel) bool { return m.showSummary }, (*tuiModel).kSummary, frameSummary, banner(renderSummaryOverlay)),
		drawn(scrDeadlines, g, func(m *tuiModel) bool { return m.showDeadlinesAgenda }, (*tuiModel).kDeadlinesAgenda,
			frameDeadlines, banner(renderDeadlinesAgenda)),
		drawn(scrBlock, g, func(m *tuiModel) bool { return m.blockModalOpen }, (*tuiModel).kBlockModal, frameBlock, banner(renderBlockDialog)),
		drawn(scrUnsubscribe, g, func(m *tuiModel) bool { return m.unsubDialog != nil }, (*tuiModel).kUnsubscribe,
			frameUnsub, banner(renderUnsubscribeDialog)),
		drawn(scrAliases, g, func(m *tuiModel) bool { return m.aliasView != nil }, (*tuiModel).kAliasBrowser,
			frameAliases, dimmedBanner(renderAliasBrowser)),
		drawn(scrAccounts, g, func(m *tuiModel) bool { return m.accountOverlayOpen }, (*tuiModel).kAccountOverlay,
			frameAccount, banner(renderAccountOverlay)),
		drawn(scrSort, g, func(m *tuiModel) bool { return m.sortMenuOpen }, (*tuiModel).kSortMenu, frameSort, banner(renderSortMenu)),
		drawn(scrSavedSearches, g, func(m *tuiModel) bool { return m.virtualBrowserOpen }, (*tuiModel).kVirtualBrowser,
			frameVirtual, dimmedBanner(renderVirtualBrowser)),
		drawn(scrRecent, g, func(m *tuiModel) bool { return m.recentOpen }, (*tuiModel).kRecentFolders,
			frameRecent, dimmedBanner(renderRecentFolders)),
	}
}

// baseLayers are the folder tree, the busy notice, the searches and the
// list or message beneath everything. The busy notice has no handler of its
// own: dispatchLayer drops the keys it blocks and passes the rest down.
func baseLayers() []layer {
	g := groupBase
	tree := banner(func(m *tuiModel) string { return renderFolderOverlay(m, renderFolderTree(m)) })
	detail := func(m *tuiModel) bool { return m.mode == modeDetail }
	return []layer{
		drawn(scrFolderSearch, g, func(m *tuiModel) bool { return m.treeOpen && m.inFolderSearch }, (*tuiModel).kFolder, frameTree, tree),
		drawn(scrFolderTree, g, func(m *tuiModel) bool { return m.treeOpen }, (*tuiModel).kGlobalKey, frameTree, tree),
		drawn(scrLoading, g, func(m *tuiModel) bool { return m.showLoad != "" && m.mode != modeCompose }, nil,
			frameLoading, banner((*tuiModel).renderLoadingOverlay)),
		onBase(scrListSearch, g, func(m *tuiModel) bool { return m.inSearch }, (*tuiModel).kSearch),
		onBase(scrRightPane, g, func(m *tuiModel) bool { return detail(m) && m.rightPaneFocus }, (*tuiModel).kRightPane),
		onBase(scrMessageSearch, g, func(m *tuiModel) bool { return detail(m) && m.inDetailSearch }, (*tuiModel).kDetailSearch),
		onBase(scrMessage, g, detail, (*tuiModel).kGlobalKey),
		onBase(scrList, g, func(*tuiModel) bool { return true }, (*tuiModel).kGlobalKey),
	}
}

// saving reports whether a save dialog is open, the ones whose help is
// scrSaveHelp.
func (m *tuiModel) saving() bool {
	return m.savingMessageToFolder || (m.savingMessageToDisk && m.fileBrowserOpen)
}

// kSaving gives a key to whichever save dialog is open; both close their help
// on the first key.
func (m *tuiModel) kSaving(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.savingMessageToFolder {
		return m.kSaveToFolder(k)
	}
	return m.kSaveToDisk(k)
}

// kAttachmentHelp gives "/" and what is typed after it to the help's search,
// closes the help on F1, Esc or q, and swallows every other key.
func (m *tuiModel) kAttachmentHelp(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := k.String()
	if s != "f1" && m.helpFind.key(k) {
		return m, nil
	}
	if s == "f1" || s == "esc" || s == "q" || k.Type == tea.KeyEsc {
		m.setAttachmentHelp(false)
	}
	return m, nil
}

// setAttachmentHelp opens or closes the attachment help, with no search set.
func (m *tuiModel) setAttachmentHelp(open bool) {
	m.attachmentHelpOpen = open
	m.helpFind = helpQuery{}
}

// topLayer is the first open layer at or below index from, and its index.
func (m *tuiModel) topLayer(from int) (int, *layer) {
	for i := from; i < len(screenLayers); i++ {
		if screenLayers[i].open(m) {
			return i, &screenLayers[i]
		}
	}
	last := len(screenLayers) - 1
	return last, &screenLayers[last]
}

// drawnLayer is the first open layer that draws itself: the exit wait draws
// what is beneath it.
func (m *tuiModel) drawnLayer() *layer {
	for i := range screenLayers {
		if l := &screenLayers[i]; l.open(m) && l.draw != nil {
			return l
		}
	}
	return &screenLayers[len(screenLayers)-1]
}
