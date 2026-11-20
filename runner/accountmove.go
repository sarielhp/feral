package runner

import (
	"fmt"

	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/label"
	"github.com/sarielhp/feral/mailclient"
	"github.com/sarielhp/feral/organize"
	"github.com/sarielhp/feral/report"
	"github.com/sarielhp/feral/transfer"
)

// TransferService moves messages between accounts for the TUI, through the
// same protocol as splice: deduplicated on the destination, confirmed there,
// journaled, and only then removed from the source.
type TransferService struct {
	Rep report.Reporter
}

func (s TransferService) rep() report.Reporter { return report.Or(s.Rep) }

// MoveAcross moves ids out of source on src into dest on dst, two different
// accounts, and returns those that left src. A server account gives a message
// up to its trash; an mbox file deletes it, in one rewrite for the whole set,
// and only a message whose bytes are still the ones uploaded.
func (s TransferService) MoveAcross(src, dst mailclient.MailClient, source label.Folder, ids []email.ID, dest label.Folder) ([]email.ID, error) {
	if transfer.SameAccount(src, dst) {
		return nil, fmt.Errorf("%s is the account the messages are in; a move within it is not a transfer", dst.Config().SelectedAccount)
	}
	// Refused before anything is uploaded: on a read-only source the removal
	// would only be simulated, and the message would be copied while reported
	// as moved.
	for _, side := range []struct {
		name   string
		client mailclient.MailClient
	}{{"source", src}, {"destination", dst}} {
		if mailclient.Refuses(side.client) {
			return nil, fmt.Errorf("the %s account %s is read-only; nothing was moved", side.name, side.client.Config().SelectedAccount)
		}
	}
	opts := transfer.Options{Reporter: s.rep()}
	if src.BackendType() == "mbox" {
		opts.Remove, opts.Defer = transfer.DeleteRemover(src, s.rep()), true
	} else {
		trash, err := organize.ResolveTrashTarget(src)
		if err != nil {
			return nil, fmt.Errorf("find the trash to move the originals to: %w", err)
		}
		if trash.IsZero() {
			return nil, fmt.Errorf("the source account has no trash folder to move the originals to")
		}
		opts.Remove = transfer.TrashRemover(src, trash)
	}
	m, err := transfer.NewMover(src, dst, source, opts)
	if err != nil {
		return nil, err
	}
	moved, err := m.MoveAll(ids, dest)
	if closeErr := m.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	return moved, err
}
