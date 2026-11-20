package transfer

import (
	"errors"
	"fmt"
	"os"

	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/cache/folderindex"
	"github.com/sarielhp/feral/cache/msg"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/label"
	"github.com/sarielhp/feral/mailclient"
	"github.com/sarielhp/feral/report"
)

// DeleteRemover removes by deleting outright, in one DeleteMessages call per
// batch: for an mbox source, where deleting is rewriting the file and there is
// no trash to move to.
//
// An mbox's message IDs are not stable: the -N suffix of a duplicate
// Message-ID depends on what precedes it, so a file re-read after a crash can
// give an ID to a different copy. A message is therefore deleted only if its
// cached copy still hashes to what was uploaded. One that does not is left in
// the source and reported -- the copy that went to the destination is safe
// there, and the one still in the file is not it. One the source no longer
// lists is already gone.
func DeleteRemover(src mailclient.MailClient, rep report.Reporter) Remover {
	if rep == nil {
		rep = report.Discard()
	}
	return func(folder label.Folder, msgs []Removal) ([]email.ID, error) {
		acct := src.Config().AccountCache
		listed, err := folderindex.IDs(acct, folder.String())
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("list %s before deleting from it: %w", folder, err)
		}
		present := make(map[email.ID]bool, len(listed))
		for _, id := range listed {
			present[id] = true
		}
		var ids, kept []email.ID
		for _, m := range msgs {
			if !present[m.ID] {
				continue
			}
			if !sameContent(acct, m) {
				rep.Warnf("%s in %s is not the message that was moved (its content differs); left in place", m.ID, folder)
				kept = append(kept, m.ID)
				continue
			}
			ids = append(ids, m.ID)
		}
		if len(ids) == 0 {
			return kept, nil
		}
		return kept, src.DeleteMessages(ids, folder)
	}
}

// sameContent reports whether the cached copy of m.ID hashes to m.Hash. A
// removal with no hash never matches: deleting blind is what this guards.
func sameContent(acct cachedir.Account, m Removal) bool {
	if m.Hash == "" {
		return false
	}
	raw, err := msg.Read(acct, m.ID)
	return err == nil && ContentHash(raw) == m.Hash
}
