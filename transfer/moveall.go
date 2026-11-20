package transfer

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/label"
	"github.com/sarielhp/feral/mailclient"
)

// String names the step, for an error that says where a transfer stopped.
func (s Stage) String() string {
	switch s {
	case StageEnsure:
		return "create the destination folder"
	case StageUpload:
		return "upload"
	case StageConfirm:
		return "confirm on the destination"
	case StageRemove:
		return "remove from the source"
	default:
		return "transfer"
	}
}

// TrashRemover removes by moving to trash on the source account, which is how
// a server account gives up a message that now lives in another.
func TrashRemover(src mailclient.MailClient, trash label.Folder) Remover {
	return func(folder label.Folder, msgs []Removal) ([]email.ID, error) {
		ids := make([]email.ID, len(msgs))
		for i, m := range msgs {
			ids[i] = m.ID
		}
		return nil, src.MoveEmail(ids, folder, trash)
	}
}

// MoveAll moves ids out of the Mover's source folder into dest and returns
// those that left the source. Each is read from the source, uploaded unless
// the destination already holds it, confirmed, and only then removed -- all
// together at the end, under Defer. A message that fails at any step stays in
// the source, and err says which step it was; a removal that fails stays owed
// in the journal, for the next Mover over this account to finish.
func (m *Mover) MoveAll(ids []email.ID, dest label.Folder) (moved []email.ID, err error) {
	var errs []error
	for _, id := range ids {
		raw, readErr := m.src.GetRawMessage(id)
		if readErr != nil {
			errs = append(errs, fmt.Errorf("%s: read: %w", id, readErr))
			continue
		}
		parsed := email.ParseReader(bytes.NewReader(raw), id, "")
		res := m.Move(Item{ID: id, Parsed: parsed, Raw: raw, Dest: dest})
		if !res.OK() {
			errs = append(errs, fmt.Errorf("%s: %s: %w", id, res.Failed, errOr(res.Err)))
			continue
		}
		if !m.opts.Defer {
			moved = append(moved, id)
		}
	}
	removed, kept, flushErr := m.Flush()
	moved = append(moved, removed...)
	for _, id := range kept {
		errs = append(errs, fmt.Errorf("%s: %w", id, ErrLeftInPlace))
	}
	if flushErr != nil {
		errs = append(errs, fmt.Errorf("remove from %s: %w", m.source, flushErr))
	}
	return moved, errors.Join(errs...)
}

// errOr is err, or a generic one for a step that failed without saying why
// (Confirm reports "not there yet" as false with no error).
func errOr(err error) error {
	if err != nil {
		return err
	}
	return errors.New("not confirmed")
}
