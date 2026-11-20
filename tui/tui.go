package tui

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sarielhp/feral/cache"
	"github.com/sarielhp/feral/cache/virtual"
	"github.com/sarielhp/feral/calendar"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/filebrowser"
	"github.com/sarielhp/feral/keymap"
	"github.com/sarielhp/feral/label"
	"github.com/sarielhp/feral/llm"
	"github.com/sarielhp/feral/mailclient"
	"github.com/sarielhp/feral/report"
	"github.com/sarielhp/feral/tui/render"

	"github.com/charmbracelet/bubbles/viewport"
)

const modeIndex int = 0
const modeDetail = 1
const modeCompose = 2

type pendingOpType int

const (
	pendingSpam    pendingOpType = 1
	pendingArchive pendingOpType = 2
	pendingDelete  pendingOpType = 3
	pendingUnspam  pendingOpType = 4
)

const (
	autoRefreshInterval   = 60 * time.Second
	idleActivityThreshold = 30 * time.Second
	minRefreshInterval    = 30 * time.Second
)

type timeTickMsg struct{}

// bgCheckDoneMsg ends a mail check. foreground marks a load the user asked
// for, which owns the loading indicator; a background poll must not clear one
// that belongs to an operation started since.
type bgCheckDoneMsg struct {
	folder     string
	err        error
	foreground bool
}

type errMsg struct {
	err error
}

type foldersLoadedMsg struct {
	folders []label.Item
}

type emailsLoadedMsg struct {
	client      mailclient.MailClient
	folder      string
	emails      []email.Email
	classifyIDs []email.ID
	// Update. Assigning them to the model from inside the goroutine raced the
	// key handlers that write the same maps, which can abort the process with a
	// non-recoverable "concurrent map writes" fatal error.
	isRead    map[email.ID]bool
	isReplied map[email.ID]bool
	// mergeFlags says the maps cover only these rows -- the unified inbox,
	// drawn from several accounts -- so they are merged into the session's
	// rather than replacing them, which would leave every other folder's
	// messages unread to the cached loader.
	mergeFlags bool
	// flagGen is the model's flagGen when the load was dispatched: a mark
	// made after it is laid back over the maps the load brings.
	flagGen uint64
}

type detailMsg struct {
	headers     string
	body        string
	attachments []email.Attachment
	crypto      *email.CryptoStatus
}

type doneMsg struct {
	id email.ID
}

type spamDoneMsg struct {
	id email.ID
	// undo is what moved, when the server accepted the move.
	undo *undoBatch
}

type archiveDoneMsg struct {
	id email.ID
	// undo is what moved, when the server accepted the move.
	undo *undoBatch
}

type deleteDoneMsg struct {
	id email.ID
	// undo is what moved, when the server accepted the move.
	undo *undoBatch
}

type unspamDoneMsg struct {
	id email.ID
}

// markedSpamDoneMsg reports the outcome of moving [SPAM] messages to the spam
// folder. Only the moved ids are dropped from the index; failed maps each id
// that stayed put to the reason.
type markedSpamDoneMsg struct {
	ids    []email.ID
	failed map[email.ID]error
}

// spamClassifiedMsg reports whether a message was learned as spam. On success
// it carries the [SPAM] badge in place; on failure err is set and the optimistic
// badge is rolled back.
type spamClassifiedMsg struct {
	id  email.ID
	err error
}

type classifyDoneMsg struct{}

type clearSpamUndoMsg struct {
	time time.Time
}

type deadlineEntry struct {
	EmailID     email.ID
	Sender      string
	Subject     string
	DateText    string
	Description string
	EmailDate   time.Time
}

func (m *tuiModel) isPending(id email.ID) bool {
	_, ok := m.pendingEmails[id]
	return ok
}

func (m *tuiModel) addPending(id email.ID, op pendingOpType) {
	m.pendingEmails[id] = op
}

func (m *tuiModel) removePending(id email.ID) {
	delete(m.pendingEmails, id)
}

type tuiModel struct {
	mode             int
	folders          []label.Item
	emails           []email.Email
	rawEmails        []email.Email
	expandedThreads  map[threadKey]bool
	lastSaveDir      string // where the last save dialog of this session saved, offered as a jump
	eIdx             int
	tree             []*label.TreeNode
	entries          []label.TreeEntry
	cursor           int
	currentF         string
	treeOpen         bool
	showHelp         bool
	showLoad         string
	loadEscNoted     bool // Esc was pressed over a load that cannot be canceled
	vp               viewport.Model
	done             bool
	width            int
	height           int
	lpr              string
	selectedID       email.ID
	client           mailclient.MailClient
	pendingEmails    map[email.ID]pendingOpType
	detailH          string
	detail           string
	detailCrypto     *email.CryptoStatus
	selectedRow      render.ColoredString
	detailVpDirty    bool
	theme            *render.ThemeManager
	lastTime         time.Time
	lastCheckTime    time.Time
	lastActivityTime time.Time
	bgCheckingMail   bool
	// relay is the state of the relay-account check; see tui_relay.go.
	relay              relayState
	folderSearch       string
	inFolderSearch     bool
	inSearch           bool
	searchQuery        string
	fuzzySearch        bool
	fuzzyGlobal        bool
	savedRawEmails     []email.Email
	savedCurrentF      string
	detailSearch       string
	inDetailSearch     bool
	inPipePrompt       bool
	pipeCommand        string
	taggedIDs          map[email.ID]bool
	inTagPrefix        bool
	inTagPatternPrompt bool
	tagPatternQuery    string
	bulkMovingToFolder bool
	inRemoteSearch     bool
	remoteSearchQuery  string
	remoteSearchLimit  int  // limit the search view was fetched with
	remoteSearchMore   bool // the search view filled its limit
	splitPreview       bool
	indexStart         int
	err                error
	globalFolders      []label.Item
	globalTree         []*label.TreeNode
	showGlobalTree     bool
	folderJustChanged  bool
	isRead             map[email.ID]bool
	isReplied          map[email.ID]bool
	flagGen            uint64                // counts the read and replied marks made here
	flagEdits          map[email.ID]flagEdit // marks a load in flight may not have seen
	bk                 *Backend
	rep                report.Reporter
	showError          bool
	showDiag           bool
	diagVp             viewport.Model
	attachments        []email.Attachment
	attachmentDrawer   bool
	attCursor          int
	saveStatus         string
	// exitNotice is printed once the program has exited, for an exit that
	// saved a draft: where the local copy is and whether the upload landed.
	exitNotice string
	// draftSyncs is the server-side draft work of each compose session,
	// keyed by its Message-ID; it outlives the session (tui_compose_draft.go).
	draftSyncs map[string]*draftSync
	// exitPending is set while the program waits for a session's server work
	// before exiting; abandonArmed is the first Ctrl+C of an abandon.
	exitPending bool
	// contactPick is the open contacts picker (F5 on the compose screen).
	contactPick *contactPicker
	// aliasView is the open aliases browser (F6), over any screen.
	aliasView *aliasBrowser
	// The help tree's state: helpSection indexes helpSections, with -1 the
	// section menu; helpCursor is the menu's row, helpScroll the page's first
	// body line, and helpDirect marks a page its screen opened (Esc closes).
	helpSection int
	helpCursor  int
	helpScroll  int
	helpDirect  bool
	// helpFind is the search typed into whichever help screen is open.
	helpFind helpQuery
	// keyMap is the keys of the list and the message view, from keys.json;
	// nil is the defaults (keys()).
	keyMap         *keymap.Keymap
	abandonArmed   bool
	saveStatusTime time.Time
	// notice is a transient confirmation shown under the index and detail
	// views, such as where F2 saved a message.
	notice     string
	noticeTime time.Time
	menuOpen   bool
	menuCursor int
	menuItems  []string
	// spamMenuOpen is the F5 spam dialog in the index view; spamMenuCursor is
	// its selection.
	spamMenuOpen       bool
	spamMenuCursor     int
	accountOverlayOpen bool
	accountCursor      int
	// virtualBrowserOpen is the F7 dialog listing the account's saved result
	// sets; virtualCursor is its selection and virtualMailboxes what it lists.
	virtualBrowserOpen bool
	virtualCursor      int
	virtualMailboxes   []virtual.Mailbox
	// recentOpen is the F8 dialog listing the folders opened most recently on
	// this account; recentFolders is what it lists, newest first, without the
	// folder on screen, and recentCursor its selection.
	recentOpen    bool
	recentCursor  int
	recentFolders []string
	// folderPositions is the message each folder had selected when it was
	// left this session, keyed by account cache and folder, so that coming
	// back selects it again.
	folderPositions map[string]email.ID
	// history and forward are the views Back and Forward reopen, oldest
	// first. quitConfirmOpen is the dialog Esc opens with nothing to go back
	// to, opened at quitConfirmAt; sentToastUntil ends "Message sent".
	history         []historyEntry
	forward         []historyEntry
	quitConfirmOpen bool
	quitConfirmAt   time.Time
	sentToastUntil  time.Time
	// virtualBox is the saved set the view was opened from, kept because only
	// it records the folder each of its rows came from, which is the source
	// folder an action on a row must name. It is checked against the folder on
	// screen rather than cleared, because currentF moves in a dozen places.
	virtualBox         *virtual.Mailbox
	cfg                *cfg_g.Config
	showSummary        bool
	summaryContent     string
	summaryEmailID     email.ID
	summaryFrom        string
	summarySubject     string
	summaryDate        string
	summaryVp          viewport.Model
	summaryCache       map[email.ID]string
	attachmentHelpOpen bool
	fileBrowserOpen    bool
	fileBrowser        filebrowser.Model
	pendingSaveAtt     email.Attachment
	// pendingSaveStem is set when the save dialog is saving a whole message
	// (F2): its name is shortened against the directory finally chosen.
	pendingSaveStem string
	// pendingMbox is set between ;x's name prompt and the directory the save
	// dialog chooses: the tagged set waiting to be written.
	pendingMbox *pendingMboxExport
	// inSaveNamePrompt is the name prompt ;v and ;x share, with saveNameKind
	// saying what the name is for.
	inSaveNamePrompt bool
	// inMovePrompt is ;a's "account:folder" prompt, moveQuery its text.
	inMovePrompt bool
	moveQuery    string
	// inSnoozePrompt is H's "until when" prompt, snoozeQuery its text and
	// snoozeTarget the message it was opened on.
	inSnoozePrompt            bool
	snoozeQuery               string
	snoozeTarget              *snoozeTarget
	saveNameKind              saveNameKind
	saveNameQuery             string
	lastSpammedID             email.ID
	lastSpammedTime           time.Time
	undoSpamBanner            string
	summaryQueue              []email.ID
	summaryInFlight           bool
	currentSummarizingID      email.ID
	waitingUserSummaryID      email.ID
	savingMessageToFolder     bool
	savingMessageID           email.ID
	savingMessageSourceFolder string
	savingMessageToDisk       bool
	lastSaveTargetFolder      bool
	saveHelpOpen              bool
	sortOption                SortOption
	sortReverse               bool
	sortMenuOpen              bool
	sortMenuReverse           bool // the menu was opened with O: its choice sorts descending
	sortMenuCursor            int
	summaryParsedCache        map[email.ID]llm.ParsedSummary
	showDeadlinesAgenda       bool
	deadlinesVp               viewport.Model
	deadlinesCursor           int
	deadlinesList             []deadlineEntry
	icsModalOpen              bool
	icsFilename               string
	icsData                   []byte
	icsEvent                  *calendar.ICSEvent
	icsEvents                 []*calendar.ICSEvent
	icsEventIdx               int
	icsCheckStatus            icsCheckStatus
	blockModalOpen            bool
	blockSenderName           string
	blockSenderEmail          string
	blockMessageID            email.ID
	unsubDialog               *unsubPrompt // the unsubscribe confirmation (X), open while set
	icsInCalendar             bool
	icsDiscrepancy            string
	icsCheckErr               string
	icsAddStatus              string
	icsVp                     viewport.Model
	attModalOpen              bool
	attPreviewAtt             email.Attachment
	attPreviewVp              viewport.Model
	attPreviewStatus          string
	attPreviewStatusTime      time.Time
	attachmentLineOffset      int
	detailSplitOpen           bool
	rightPaneFocus            bool
	hideRightPane             bool
	rightVp                   viewport.Model
	rightVpAttFilename        string
	attachmentListOpen        bool
	attachmentViewerOpen      bool
	attachmentViewerVp        viewport.Model
	attachmentDeleted         map[int]bool
	attachmentListCursor      int
	compose                   *composeState
	standaloneCompose         bool
	// mouseOff is set while F9 has handed the mouse back to the terminal.
	mouseOff bool
	// lastFrame is the screen as last drawn, and frameKind what it was -- the
	// index or the reader with nothing over it (frameBase), or which overlay;
	// a click is read against them. See tui_mouse_overlay.go.
	lastFrame string
	frameKind string
	// summaryShown is the summary as last put in its viewport; see
	// setupSummaryViewport.
	summaryShown  string
	composeOpts   ComposeOptions
	lockedFolders map[string]bool
	// undoStack is the session's undoable actions, the latest last; see
	// tui_undo.go.
	undoStack []undoEntry
}

func newTuiModel(cl mailclient.MailClient, lp string, bk *Backend) *tuiModel {
	// rep is replaced with the program reporter once the TUI is running.
	m := &tuiModel{
		mode:             modeIndex,
		client:           cl,
		lpr:              lp,
		theme:            render.NewThemeManager(),
		expandedThreads:  make(map[threadKey]bool),
		pendingEmails:    make(map[email.ID]pendingOpType),
		lockedFolders:    make(map[string]bool),
		showGlobalTree:   true,
		bk:               bk,
		menuItems:        []string{"Move to Spam", "Switch Account", "Exit"},
		helpSection:      -1, // the section menu
		lastTime:         time.Now(),
		lastCheckTime:    time.Now(),
		lastActivityTime: time.Now(),
	}
	if cl != nil && cl.Config() != nil {
		m.isRead = cache.LoadReadState(cl.Config().AccountCache)
		m.isReplied = cache.LoadRepliedState(cl.Config().AccountCache)
	}
	if m.isRead == nil {
		m.isRead = make(map[email.ID]bool)
	}
	if m.isReplied == nil {
		m.isReplied = make(map[email.ID]bool)
	}
	m.vp = viewport.New(70, 20)
	m.diagVp = viewport.New(70, 20)
	m.summaryVp = viewport.New(70, 20)
	m.icsVp = viewport.New(70, 8)
	m.summaryCache = make(map[email.ID]string)
	m.summaryParsedCache = make(map[email.ID]llm.ParsedSummary)
	m.deadlinesVp = viewport.New(70, 20)
	targetFolder := "INBOX"
	if cl != nil {
		targetFolder = cl.InboxFolder().String()
	}
	if lp != "" {
		targetFolder = lp
	}
	slog.Info("newTuiModel initializing", slog.String("labelPrefix", lp), slog.String("targetFolderDefault", targetFolder))

	var cachedFolders []label.Item
	if cl != nil && cl.Config() != nil {
		dir := cl.Config().AccountCache
		if !dir.IsZero() {
			cachePath := filepath.Join(dir.Path(), "labels_cache.json")
			slog.Info("newTuiModel loading cached folders", slog.String("path", cachePath))
			if data, err := os.ReadFile(cachePath); err == nil {
				if unmarshalErr := json.Unmarshal(data, &cachedFolders); unmarshalErr != nil {
					slog.Error("newTuiModel failed to unmarshal cached folders", slog.Any("error", unmarshalErr))
				} else {
					var names []string
					for _, f := range cachedFolders {
						names = append(names, f.FullName)
					}
					slog.Info("newTuiModel loaded cached folders successfully", slog.Int("count", len(cachedFolders)), slog.Any("folderNames", names))
				}
			} else {
				slog.Warn("newTuiModel failed to read cached folders file", slog.Any("error", err))
			}
		}
	}
	if len(cachedFolders) > 0 {
		m.globalFolders = cachedFolders
		m.globalTree = label.BuildLabelTree(cachedFolders)
		if lp != "" {
			lower := strings.ToLower(lp)
			var filtered []label.Item
			for _, f := range cachedFolders {
				if strings.Contains(strings.ToLower(f.FullName), lower) {
					filtered = append(filtered, f)
				}
			}
			slog.Info("newTuiModel filtered cached folders by prefix", slog.Int("filteredCount", len(filtered)), slog.String("prefix", lp))
			cachedFolders = filtered
		}
	}
	if len(cachedFolders) > 0 {
		m.folders = cachedFolders
		m.tree = label.BuildLabelTree(cachedFolders)
		m.buildEntries()
		resolved := label.ResolveFolder(cachedFolders, targetFolder, "INBOX")
		slog.Info("newTuiModel resolved target folder from cache", slog.String("resolvedFolder", resolved), slog.Bool("found", resolved != ""))
		m.currentF = resolved
	} else {
		slog.Warn("newTuiModel found no cached folders; defaulting directly to INBOX")
		m.currentF = targetFolder
	}
	m.loadCachedEmailsForFolder(m.currentF)
	return m
}
