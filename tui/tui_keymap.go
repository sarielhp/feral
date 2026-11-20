package tui

import (
	"fmt"

	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/keymap"

	tea "github.com/charmbracelet/bubbletea"
)

// The keys of the message list and the message view are named actions, so a
// person can move them (keys.json, package keymap). Each action is one row
// below: its name, what it runs, and its default keys. The dispatchers --
// kIndex, kIndexActions and kDetail -- ask the keymap which action a key
// runs; a row with no handler is one a dispatcher handles inline, because it
// depends on the state around it (navigation, the split preview, the panes).
//
// Adding an action is adding a row. Keys the screens handle before their
// actions -- the global keys, kGlobalKey's, the arrows -- are reservedKeys,
// and no action may take them.

// The groups of screens a keys file can change.
const (
	groupList    = "list"    // the message list, with or without the split preview
	groupMessage = "message" // the open message
)

// The actions a dispatcher handles inline or a hint names.
const (
	actDown        = "down"
	actUp          = "up"
	actOpen        = "open"
	actSplit       = "split_preview"
	actSort        = "sort"
	actSortReverse = "sort_reverse"
	actScrollDown  = "scroll_down"
	actScrollUp    = "scroll_up"
	actFocusPane   = "focus_pane"
	actHidePane    = "hide_pane"
	actUnspam      = "unspam"
	actAttachments = "attachments"
	actLoadMore    = "load_more"
)

// keyHandler runs an action.
type keyHandler = func(m *tuiModel) (tea.Model, tea.Cmd)

// actionDef is one row of the table.
type actionDef struct {
	name string
	run  keyHandler // nil: the dispatcher handles it inline
	keys []string
}

func act(name string, run keyHandler, keys ...string) actionDef {
	return actionDef{name: name, run: run, keys: keys}
}

// listActions are the message list's own actions.
func listActions() []actionDef {
	return []actionDef{
		act(actDown, nil, "j"),
		act(actUp, nil, "k"),
		act(actOpen, nil, "e"),
		act(actSplit, nil, " "),
		act(actSort, nil, "o"),
		act(actSortReverse, nil, "O"),
		act("search", (*tuiModel).startListSearch, "/"),
		act("fuzzy_all", (*tuiModel).startFuzzyAll, "~"),
		act("fuzzy_folder", (*tuiModel).startFuzzyFolder, "!"),
		act(actLoadMore, (*tuiModel).loadMoreKey, "+"),
		act("refresh", (*tuiModel).refreshCurrentView, "R"),
		act("reply", (*tuiModel).replyFromList, "r"),
		act("reply_all", (*tuiModel).replyAllFromList, "g"),
		act("reply_list", (*tuiModel).replyListFromList, "G"),
		act("filter_list", (*tuiModel).filterBySelectedList, "="),
		act("forward", (*tuiModel).forwardFromList, "b"),
		act("aliases", (*tuiModel).openAliasesFromList, "f6"),
		act("tag", (*tuiModel).toggleTagAndAdvance, "t"),
		act("tag_pattern", (*tuiModel).openTagPatternPrompt, "T"),
		act("tag_prefix", (*tuiModel).openTagPrefix, ";"),
		act("tag_all", (*tuiModel).tagAllVisible, "ctrl+a"),
		act("clear_tags", (*tuiModel).clearTags, "ctrl+u"),
	}
}

// messageActions are the open message's own actions.
func messageActions() []actionDef {
	return []actionDef{
		act(actScrollDown, nil, "j"),
		act(actScrollUp, nil, "k"),
		act(actFocusPane, nil, "l"),
		act(actHidePane, nil, "p", "P"),
		act("search", (*tuiModel).startDetailSearch, "/"),
		act("reply", (*tuiModel).replyFromMessage, "r"),
		act("reply_all", (*tuiModel).replyAllFromMessage, "g"),
		act("reply_list", (*tuiModel).replyListFromMessage, "G"),
		act("forward", (*tuiModel).forwardFromMessage, "b"),
		act("aliases", (*tuiModel).openAliasesFromMessage, "f6"),
	}
}

// sharedActions do the same on the list and in a message. Each group keeps
// its own binding of them: moving archive on the list leaves it where it was
// in a message.
func sharedActions() []actionDef {
	return []actionDef{
		act("archive", (*tuiModel).archiveCurrentEmail, "E"),
		act("undo", (*tuiModel).undoLast, undoKey),
		act("snooze", (*tuiModel).beginSnooze, "H"),
		act("unsubscribe", (*tuiModel).promptUnsubscribe, "X"),
		act("mute", (*tuiModel).toggleMuteSelectedThread, "M"),
		act("delete", (*tuiModel).deleteCurrentEmail, "d"),
		act("spam", (*tuiModel).spamCurrentEmail, "S"),
		act(actUnspam, (*tuiModel).unspamCurrentEmail, "U", "u"),
		act("block", (*tuiModel).promptBlockSender, "B"),
		act("toggle_read", (*tuiModel).toggleReadCurrentEmail, "N"),
		act("mark_all_read", (*tuiModel).markFolderRead, "x"),
		act("save_to_folder", (*tuiModel).promptSaveMessageToFolder, "s"),
		act("browser", (*tuiModel).openCurrentEmailInBrowser, "w", "W"),
		act("compose", (*tuiModel).composeNew, "c"),
		act(actAttachments, (*tuiModel).openAttachmentList, "v"),
		act("alias_sender", (*tuiModel).aliasCurrentSender, "a"),
		act("lock_folder", (*tuiModel).toggleFolderLock, "L"),
		act("menu", (*tuiModel).openMenu, "m"),
		act("pipe", (*tuiModel).openPipePrompt, "|"),
		act("server_search", (*tuiModel).openRemoteSearch, "ctrl+f"),
		act("spam_menu", (*tuiModel).openSpamMenu, "f5"),
		act("saved_searches", (*tuiModel).openSavedSearches, "f7"),
		act("deadlines", (*tuiModel).openDeadlinesAgenda, "D"),
	}
}

// reservedKeys are the keys a screen of either group handles before asking
// the keymap: the global keys, kGlobalKey's, and the fixed navigation.
func reservedKeys() map[string]string {
	nav := "fixed navigation"
	return map[string]string{
		"ctrl+c": "quit", quitKey: "quit", "f1": "help", mouseToggleKey: "the mouse switch",
		"ctrl+d": "the diagnostics panel", "q": "back, then quit", "esc": "back", "tab": "threads and panes",
		"f": "the folder tree", "F": "the folder tree", "h": "help", "?": "help",
		"f2": "save message", "f3": "accounts", "f4": "summarize", keyRecentFolders: "recent folders",
		keyBack: "back a view", keyForward: "forward a view", "enter": "open", "alt": "the menu",
		"up": nav, "down": nav, "left": nav, "right": nav, "pgup": nav, "pgdown": nav, "home": nav, "end": nav,
	}
}

// The table, built in init for the reason screenLayers is: its handlers
// reach handleKey.
var (
	keyTable    *keymap.Table
	keyHandlers map[string]map[string]keyHandler
	defaultKeys *keymap.Keymap
)

func init() {
	keyTable = &keymap.Table{Reserved: reservedKeys(), ReservedPrefixes: map[string]string{"alt+": "the menu"}}
	keyHandlers = map[string]map[string]keyHandler{}
	groups := []struct {
		name string
		defs []actionDef
	}{
		{groupList, append(listActions(), sharedActions()...)},
		{groupMessage, append(messageActions(), sharedActions()...)},
	}
	for _, g := range groups {
		group := keymap.Group{Name: g.name}
		keyHandlers[g.name] = map[string]keyHandler{}
		for _, d := range g.defs {
			group.Bindings = append(group.Bindings, keymap.Binding{Action: d.name, Keys: d.keys})
			keyHandlers[g.name][d.name] = d.run
		}
		keyTable.Groups = append(keyTable.Groups, group)
	}
	defaultKeys = keymap.Default(keyTable)
}

// DefaultKeysJSON is the default keymap as a keys file (feral utils keys).
func DefaultKeysJSON() []byte { return keyTable.JSON() }

// loadKeymap reads keys.json from the configuration directory. A broken one
// is an error, and the TUI does not start: half a keymap is worse than none.
func loadKeymap(cfg *cfg_g.Config) (*keymap.Keymap, error) {
	dir := ""
	if cfg != nil {
		dir = cfg.ConfigDir
	}
	if dir == "" {
		var err error
		if dir, err = cfg_g.UserConfigDir(); err != nil {
			return nil, fmt.Errorf("find the keys file: %w", err)
		}
	}
	return keymap.Load(keyTable, dir)
}

// keys is the keymap in effect; a model built without one has the defaults.
func (m *tuiModel) keys() *keymap.Keymap {
	if m.keyMap != nil {
		return m.keyMap
	}
	return defaultKeys
}

// keyAction is the action k runs in group, or "".
func (m *tuiModel) keyAction(group string, k tea.KeyMsg) string {
	return m.keys().Action(group, k.String())
}

// runKeyAction runs the handler of the action k names in group, if it has
// one; a key that names none does nothing.
func (m *tuiModel) runKeyAction(group string, k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if run := keyHandlers[group][m.keyAction(group, k)]; run != nil {
		return run(m)
	}
	return m, nil
}

// screenGroup is the group whose keys are live: a message's while one is
// open, else the list's (the split preview draws a message under the list).
func (m *tuiModel) screenGroup() string {
	if m.mode == modeDetail {
		return groupMessage
	}
	return groupList
}

// keyHint is the first key of action in group as a hint shows it.
func (m *tuiModel) keyHint(group, action string) string {
	keys := m.keys().Keys(group, action)
	if len(keys) == 0 {
		return ""
	}
	return keymap.Display(keys[0])
}
