package tui

import (
	"errors"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/cache/meta"
	"github.com/sarielhp/feral/cache/msg"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/label"
	"github.com/sarielhp/feral/mailclient"
	"github.com/sarielhp/feral/spam"
)

// Undo for the list's moves: archive, delete to trash, mark as spam and a
// move to a folder each file messages somewhere the server can move them back
// from, so z moves them back. The session keeps the last undoStackDepth
// actions; nothing is written down, so nothing survives a restart.
//
// An entry is recorded when the server has accepted the action -- from its
// done message, never at the key -- and holds only what moved. Undo moves the
// messages back through the same client decorators the action used, so the
// folder index is reconciled where every move reconciles it
// (CheckingMailClient), and restores by hand only what the action changed by
// hand: delete and spam drop the cached copy, and with it the user's spam
// override, which undo downloads again and puts back; a spam mark also
// trained bogofilter, which undo unregisters as many times as it registered.
//
// What cannot be moved back stands on the stack as a final entry, so z says
// so instead of silently reaching past it to an older action: a sent message,
// a blocked sender (a rule change), a move into another account, an Outlook
// account whose message IDs change on every move.

// undoKey undoes the last move. u was taken -- it unspams, and within three
// seconds of S undoes the spam mark -- and z is Gmail's undo key.
const undoKey = "z"

// undoStackDepth is how many actions the session can undo.
const undoStackDepth = 10

// undoBatch is what one account's part of an action moved, from to into
// from: from is where the action put the messages and to where they were,
// which is where undo returns them.
type undoBatch struct {
	// client acted; account names it, and undo acts through the client of
	// that name, which is this one while the session still has it.
	client  mailclient.MailClient
	account string
	from    label.Folder
	to      label.Folder
	ids     []email.ID
	// overrides is each message's spam override before an action that
	// dropped the cached copy along with its record.
	overrides map[email.ID]msg.Override
	// training is what a spam mark taught bogofilter about each message,
	// which undo takes back once the message is out of the spam folder.
	training map[email.ID]spam.Training
	// blocked says why this batch cannot be moved back, or is empty.
	blocked string
}

// undoEntry is one action on the stack.
type undoEntry struct {
	verb    string // "Archived", "Deleted", "Marked as spam", "Moved"
	batches []undoBatch
	// final says why the action cannot be undone; empty when it can.
	final string
	// count is how many messages a final entry concerned.
	count int
}

func (e undoEntry) size() int {
	if e.final != "" {
		return e.count
	}
	n := 0
	for _, b := range e.batches {
		n += len(b.ids)
	}
	return n
}

// title names the action in the status line: "Archived 3", or a final
// entry's verb alone ("Sent a message").
func (e undoEntry) title() string {
	if n := e.size(); n > 0 && e.final == "" {
		return fmt.Sprintf("%s %d", e.verb, n)
	}
	return e.verb
}

type undoDoneMsg struct {
	title string
	err   error
}

// newUndoBatch records that cl moved ids from to into from. An Outlook cache
// that still keys messages by Graph's default IDs cannot be undone: those IDs
// change with every move, so the ones recorded name nothing afterwards.
func newUndoBatch(cl mailclient.MailClient, from, to label.Folder, ids []email.ID) undoBatch {
	b := undoBatch{client: cl, from: from, to: to, ids: ids}
	if cl == nil || cl.Config() == nil {
		return b
	}
	cfg := cl.Config()
	b.account = string(cfg.SelectedAccount)
	if cl.BackendType() == "outlook" && !meta.ImmutableIDs(cfg.AccountCache) {
		b.blocked = "Outlook message IDs change on a move (utils cache outlook-ids keeps them)"
	}
	return b
}

// priorOverrides reads the override of each message an action is about to
// drop from the cache, keeping only those the user set.
func priorOverrides(account cachedir.Account, ids []email.ID) map[email.ID]msg.Override {
	var out map[email.ID]msg.Override
	for _, id := range ids {
		info, err := msg.GetInfo(account, id)
		if err != nil || info.Override == msg.OverrideNone {
			continue
		}
		if out == nil {
			out = make(map[email.ID]msg.Override)
		}
		out[id] = info.Override
	}
	return out
}

// recordUndo puts an action on the stack and returns what the status line
// adds to its report of it. Only the latest action on a message can be
// undone: an older entry forgets the messages a newer one moved again, since
// moving them back from where it left them would act on a stale location.
func (m *tuiModel) recordUndo(e undoEntry) string {
	e.batches = movedBatches(e.batches)
	for _, b := range e.batches {
		if b.blocked != "" {
			e.final, e.count = b.blocked, e.size()
		}
	}
	if e.final == "" && e.size() == 0 {
		return ""
	}
	for _, b := range e.batches {
		m.forgetUndo(b.ids)
	}
	m.undoStack = append(m.undoStack, e)
	if len(m.undoStack) > undoStackDepth {
		m.undoStack = m.undoStack[len(m.undoStack)-undoStackDepth:]
	}
	if e.final != "" {
		return " — cannot be undone: " + e.final
	}
	return " — " + m.keyHint(m.screenGroup(), "undo") + " to undo"
}

// reportUndoable records a single message's action, when it moved, and says
// so in the status line.
func (m *tuiModel) reportUndoable(verb string, b *undoBatch) {
	if b == nil {
		return
	}
	e := undoEntry{verb: verb, batches: []undoBatch{*b}}
	if note := m.recordUndo(e); note != "" {
		m.saveStatus = e.title() + note
		m.saveStatusTime = time.Now()
	}
}

// movedBatches drops the batches that moved nothing: an empty one, or one
// whose messages were already where the action filed them. A blocked batch
// stays whatever its folders: across accounts the names may well match.
func movedBatches(batches []undoBatch) []undoBatch {
	kept := batches[:0:0]
	for _, b := range batches {
		if len(b.ids) == 0 || (b.blocked == "" && strings.EqualFold(b.from.String(), b.to.String())) {
			continue
		}
		kept = append(kept, b)
	}
	return kept
}

// recordFinal stands an action on count messages that cannot be undone on the
// stack and returns the status line's note of it; ids are the messages it
// moved, which no older entry knows the place of any more. An action that
// did nothing records nothing.
func (m *tuiModel) recordFinal(verb, why string, ids []email.ID, count int) string {
	if count == 0 {
		return ""
	}
	m.forgetUndo(ids)
	return m.recordUndo(undoEntry{verb: verb, final: why, count: count})
}

// forgetUndo drops ids from every entry: something other than an undoable
// action moved them, so no entry knows where they are.
func (m *tuiModel) forgetUndo(ids []email.ID) {
	if len(ids) == 0 || len(m.undoStack) == 0 {
		return
	}
	gone := make(map[email.ID]bool, len(ids))
	for _, id := range ids {
		gone[id] = true
	}
	kept := m.undoStack[:0]
	for _, e := range m.undoStack {
		if e.final == "" {
			e.batches = withoutIDs(e.batches, gone)
			if e.size() == 0 {
				continue
			}
		}
		kept = append(kept, e)
	}
	m.undoStack = kept
}

// withoutIDs returns batches less the ids in gone, dropping a batch emptied.
func withoutIDs(batches []undoBatch, gone map[email.ID]bool) []undoBatch {
	out := make([]undoBatch, 0, len(batches))
	for _, b := range batches {
		ids := make([]email.ID, 0, len(b.ids))
		for _, id := range b.ids {
			if !gone[id] {
				ids = append(ids, id)
			}
		}
		if len(ids) > 0 {
			b.ids = ids
			out = append(out, b)
		}
	}
	return out
}

// undoLast undoes the action on top of the stack. A final entry is taken off
// with a note of why it stays done, so the next z reaches the one beneath.
func (m *tuiModel) undoLast() (tea.Model, tea.Cmd) {
	if len(m.undoStack) == 0 {
		return m, m.undoStatus("Nothing to undo")
	}
	top := m.undoStack[len(m.undoStack)-1]
	if top.final != "" {
		m.undoStack = m.undoStack[:len(m.undoStack)-1]
		return m, m.undoStatus(fmt.Sprintf("Cannot undo %s: %s", top.title(), top.final))
	}
	if why := m.undoRefusal(top); why != "" {
		m.err = errors.New(why)
		m.showError = true
		return m, nil
	}
	m.undoStack = m.undoStack[:len(m.undoStack)-1]
	m.saveStatus = "Undoing: " + top.title() + "..."
	m.saveStatusTime = time.Now()
	return m, func() tea.Msg {
		var errs []error
		for _, b := range top.batches {
			if err := m.reverseBatch(b); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", b.account, err))
			}
		}
		return undoDoneMsg{title: top.title(), err: errors.Join(errs...)}
	}
}

func (m *tuiModel) undoStatus(text string) tea.Cmd {
	m.saveStatus = text
	m.saveStatusTime = time.Now()
	return nil
}

// undoRefusal says why the entry may not be undone now: undo is a server move,
// refused as every other one is under --read-only, and refused into a folder
// locked read-only since the action.
func (m *tuiModel) undoRefusal(e undoEntry) string {
	if m.isAccountReadOnly() {
		return "🔒 Read-Only Mode: nothing to undo on the server"
	}
	for _, b := range e.batches {
		if m.lockedFolders[strings.ToLower(b.to.String())] {
			return fmt.Sprintf("🔒 Folder %q is locked read-only; unlock it (L) to undo", b.to)
		}
		if acc, err := cfg_g.Lookup(m.getAccounts(), b.account); err == nil && b.account != "" && acc.IsFolderReadOnly(b.to.String()) {
			return fmt.Sprintf("🔒 %s:%s is read-only; nothing can be moved back into it", acc.Name, b.to)
		}
	}
	return ""
}

// reverseBatch moves one batch back, puts back the overrides the action
// dropped and takes back the training a spam mark gave. Off the Update
// goroutine.
func (m *tuiModel) reverseBatch(b undoBatch) error {
	cl := b.client
	if b.account != "" {
		named, err := m.clientForAccountName(b.client, b.account)
		if err != nil {
			return err
		}
		cl = named
	}
	if cl == nil {
		return errors.New("no client for the account")
	}
	if err := cl.MoveEmail(b.ids, b.from, b.to); err != nil {
		return fmt.Errorf("moving back to %s: %w", b.to, err)
	}
	return errors.Join(restoreOverrides(cl, b.overrides), untrainSpam(cl, b.training))
}

// untrainSpam takes back what a spam mark taught bogofilter about each
// message, reading the message again: the mark removed it from the cache.
func untrainSpam(cl mailclient.MailClient, training map[email.ID]spam.Training) error {
	if len(training) == 0 || cl.Config() == nil {
		return nil
	}
	var errs []error
	for id, t := range training {
		if t == (spam.Training{}) {
			// Nothing was taught, so nothing is worth a download.
			continue
		}
		raw, err := cl.GetRawMessage(id)
		if err == nil {
			err = spam.UnmarkTraining(cl.Config().ConfigDir, id, raw, t)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", id, err))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("moved back, but the spam filter keeps its training: %w", err)
	}
	return nil
}

// restoreOverrides downloads each message back into the cache -- the action
// removed it -- and sets the override it had.
func restoreOverrides(cl mailclient.MailClient, overrides map[email.ID]msg.Override) error {
	if len(overrides) == 0 || cl.Config() == nil {
		return nil
	}
	account := cl.Config().AccountCache
	var errs []error
	for id, o := range overrides {
		if _, err := cl.GetRawMessage(id); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", id, err))
			continue
		}
		if err := msg.SetOverride(account, id, o); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", id, err))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("moved back, but the spam override was not restored: %w", err)
	}
	return nil
}

// handleUndoDone reports the undo and reloads the folder on screen, where the
// messages may have come back. A saved set does not list them again: the set
// stopped listing them when they moved.
func (m *tuiModel) handleUndoDone(msg undoDoneMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.err = fmt.Errorf("undo %s: %w", msg.title, msg.err)
		m.showError = true
	} else {
		m.saveStatus = "Undone: " + msg.title
		m.saveStatusTime = time.Now()
	}
	if m.currentF == "" || isSearchView(m.currentF) || isVirtualView(m.currentF) {
		return m, nil
	}
	return m, m.loadEmailsCmd(m.currentF)
}
