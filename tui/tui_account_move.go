package tui

import (
	"errors"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/label"
	"github.com/sarielhp/feral/mailclient"
)

// The move prompt (;a, or : in the folder tree ;s opens) files the tagged
// messages into a folder named as splice names one: "folder" in the account
// they are in, "account:folder" in another, "account:" for its inbox. Within
// one account it is the server's own move. Between two it is a transfer --
// upload, confirm on the destination, and only then remove from the source
// (to its trash, or out of an mbox file by rewriting it) -- and it holds the
// screen under a busy notice until it is done, since it cannot be recalled.

const loadMovingAccounts = "Moving to another account..."

// accountMoveDoneMsg reports a move from the prompt: the IDs that left their
// folder and the failures of the rest.
type accountMoveDoneMsg struct {
	moved []email.ID
	dest  string
	err   error
	undo  []undoBatch
}

// beginAccountMove opens the move prompt. In an mbox account it starts filled
// with the first server account's inbox: filing local mail into a real
// account is what an mbox account is for.
func (m *tuiModel) beginAccountMove() (tea.Model, tea.Cmd) {
	if m.countTagged() == 0 {
		return m, m.showNotice("Tag messages with t first, then ;a")
	}
	m.moveQuery = m.defaultMoveTarget()
	m.inMovePrompt = true
	return m, nil
}

func (m *tuiModel) defaultMoveTarget() string {
	if m.client == nil || m.client.BackendType() != "mbox" {
		return ""
	}
	for _, acc := range m.getAccounts() {
		if !acc.IsMbox() && !acc.ReadOnly {
			return acc.Name + ":INBOX"
		}
	}
	return ""
}

func (m *tuiModel) kMovePrompt(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	target, ok := m.kPromptLine(k, &m.moveQuery, &m.inMovePrompt)
	if !ok {
		return m, nil
	}
	return m.executeAccountMove(target)
}

// moveTarget is where the prompt said to move: an account name ("" for the
// account each message is in) and a folder name, resolved on that account.
type moveTarget struct {
	account string
	folder  string
}

func (t moveTarget) String() string {
	if t.account == "" {
		return t.folder
	}
	return t.account + ":" + t.folder
}

// parseMoveTarget reads the prompt's text as a label reference, with or
// without its % sigil, and names the account it is in.
func (m *tuiModel) parseMoveTarget(text string) (moveTarget, error) {
	ref, err := cfg_g.ParseLabelRef(cfg_g.LabelSigil + strings.TrimPrefix(text, cfg_g.LabelSigil))
	if err != nil {
		return moveTarget{}, err
	}
	if ref.Account == "" {
		return moveTarget{folder: ref.Label}, nil
	}
	acc, err := cfg_g.Lookup(m.getAccounts(), ref.Account)
	if err != nil {
		return moveTarget{}, err
	}
	if acc.ReadOnly || m.isAccountReadOnly() {
		return moveTarget{}, fmt.Errorf("account %s is read-only; nothing can be moved into it", acc.Name)
	}
	if acc.IsFolderReadOnly(ref.Label) {
		return moveTarget{}, fmt.Errorf("%s:%s is locked read-only; nothing can be moved into it", acc.Name, ref.Label)
	}
	return moveTarget{account: acc.Name, folder: ref.Label}, nil
}

func (m *tuiModel) executeAccountMove(text string) (tea.Model, tea.Cmd) {
	ids := m.getTaggedIDs()
	if len(ids) == 0 || m.refuseInSearchView() || m.refuseReadOnly() {
		return m, nil
	}
	if m.isDryRun() {
		m.err = fmt.Errorf("--dry-run: nothing was moved to %s", text)
		m.showError = true
		return m, nil
	}
	target, err := m.parseMoveTarget(text)
	if err != nil {
		m.err = err
		m.showError = true
		return m, nil
	}
	m.taggedIDs = nil
	plan := m.planAction(ids, m.currentF)
	m.showList()
	m.showLoad = loadMovingAccounts
	m.loadEscNoted = false
	return m, func() tea.Msg {
		undo, dest, err := m.dispatchAccountMove(plan, ids, target)
		var moved []email.ID
		for _, b := range undo {
			moved = append(moved, b.ids...)
		}
		return accountMoveDoneMsg{moved: moved, dest: dest, err: err, undo: undo}
	}
}

// dispatchAccountMove moves each account's batch to the target and returns
// what left its folder, batch by batch, and the folder as it resolved -- which is
// what the status names, so a partial name's match is visible. Off the
// Update goroutine.
func (m *tuiModel) dispatchAccountMove(p actionPlan, ids []email.ID, target moveTarget) ([]undoBatch, string, error) {
	batches, err := m.batches(p, ids)
	errs := []error{err}
	var moved []undoBatch
	shown := target.String()
	for _, b := range batches {
		done, err := m.moveBatch(b, target)
		if len(done.ids) > 0 {
			moved = append(moved, done)
		}
		if !done.from.IsZero() {
			shown = moveTarget{account: target.account, folder: done.from.String()}.String()
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", accountLabel(b.client), err))
		}
	}
	return moved, shown, errors.Join(errs...)
}

// moveBatch moves one account's batch: by the server's move when the target
// is in the same account -- compared by account name, never by client, since
// clients are built afresh -- and by a transfer when it is in another. It
// returns what moved as undo would move it back, from the folder the target
// resolved to; a transfer cannot be moved back, since the copy it leaves is in
// the source account's trash or, from an mbox file, nowhere.
func (m *tuiModel) moveBatch(b accountBatch, target moveTarget) (undoBatch, error) {
	dst := b.client
	within := target.account == "" || sameAccountName(b.client, target.account)
	if !within {
		var err error
		if dst, err = m.clientForAccountName(b.client, target.account); err != nil {
			return undoBatch{}, err
		}
	}
	dest, err := mailclient.ResolveOne(dst, target.folder)
	if err != nil {
		return undoBatch{}, err
	}
	source := label.FromServer(b.folder)
	if !within {
		moved, err := m.bk.transfer().MoveAcross(b.client, dst, source, b.ids, dest)
		done := newUndoBatch(b.client, dest, source, moved)
		done.blocked = "a move to another account is not undone"
		return done, err
	}
	if strings.EqualFold(source.String(), dest.String()) {
		// Already there. A server move onto itself is not a no-op
		// everywhere: Gmail adds the label and then removes it.
		return undoBatch{from: dest}, nil
	}
	if err := b.client.MoveEmail(b.ids, source, dest); err != nil {
		return undoBatch{from: dest}, err
	}
	return newUndoBatch(b.client, dest, source, b.ids), nil
}

func sameAccountName(cl mailclient.MailClient, name string) bool {
	return cl != nil && cl.Config() != nil && strings.EqualFold(string(cl.Config().SelectedAccount), name)
}

func (m *tuiModel) handleAccountMoveDone(msg accountMoveDoneMsg) (tea.Model, tea.Cmd) {
	m.showLoad = ""
	m.loadEscNoted = false
	for _, id := range msg.moved {
		m.removeEmailByID(id)
	}
	m.dropFromSavedSet(msg.moved)
	if msg.err == nil && len(msg.moved) == 0 {
		m.saveStatus = fmt.Sprintf("Nothing moved: already in %s", msg.dest)
		m.saveStatusTime = time.Now()
		return m, nil
	}
	note := m.recordUndo(undoEntry{verb: "Moved", batches: msg.undo})
	m.reportActionOutcome(msg.err,
		fmt.Sprintf("moved %d message(s) to %s; the rest stayed", len(msg.moved), msg.dest),
		fmt.Sprintf("Moved %d message(s) to %s", len(msg.moved), msg.dest)+note)
	return m, nil
}
