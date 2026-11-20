package cache

import (
	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/email"
)

// RekeyFlagState copies the read, replied and pending-read state from one
// account cache to another, renaming each message ID through rename. An ID
// rename does not map is dropped: it names a message the server no longer
// knows. It returns how many entries were carried and dropped.
func RekeyFlagState(from, to cachedir.Account, rename map[email.ID]email.ID) (carried, dropped int, err error) {
	for _, sf := range []stateFile{readStateFile, repliedStateFile} {
		old, loadErr := loadStateStrict(from.Path(), sf)
		if loadErr != nil {
			return carried, dropped, loadErr
		}
		c, d := 0, 0
		if upErr := updateState(to, sf, func(state map[email.ID]bool) {
			c, d = rekeyInto(state, old, rename)
		}); upErr != nil {
			return carried, dropped, upErr
		}
		carried, dropped = carried+c, dropped+d
	}
	oldPending, err := loadPending(from.Path())
	if err != nil {
		return carried, dropped, err
	}
	c, d := 0, 0
	err = updateReadAndPending(to, func(_ map[email.ID]bool, pending map[email.ID]PendingRead) {
		c, d = rekeyInto(pending, oldPending, rename)
	})
	return carried + c, dropped + d, err
}

func rekeyInto[V any](dst, src map[email.ID]V, rename map[email.ID]email.ID) (carried, dropped int) {
	for id, v := range src {
		newID, ok := rename[id]
		if !ok {
			dropped++
			continue
		}
		dst[newID] = v
		carried++
	}
	return carried, dropped
}
