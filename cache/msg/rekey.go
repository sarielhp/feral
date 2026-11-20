package msg

import (
	"fmt"

	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/cache/dirlock"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/message_info"
)

// RekeyReport says what Rekey carried.
type RekeyReport struct {
	Carried int // messages copied under their new ID
	Decided int // of those, how many carry the user's spam/not-spam decision
	Dropped int // messages rename had no new ID for
}

// Rekey copies every message cached in from into to under the ID rename gives
// it: the body, the index record -- date, scan evidence and the user's override
// -- and any LLM summary, which costs a request to make again. The parsed
// summary is left behind; it is derived from the body. A message rename does
// not map is dropped: it names a message the server no longer knows. A record
// with no body is carried only when it holds the user's decision, as Repair
// keeps it. from is not changed.
func Rekey(from, to cachedir.Account, rename map[email.ID]email.ID) (RekeyReport, error) {
	var rep RekeyReport
	var ids []email.ID
	ForEachID(from, func(id email.ID) bool {
		ids = append(ids, id)
		return false
	})
	for _, id := range ids {
		newID, ok := rename[id]
		if !ok {
			rep.Dropped++
			continue
		}
		decided, err := rekeyOne(from, to, id, newID)
		if err != nil {
			return rep, fmt.Errorf("carrying %s: %w", id, err)
		}
		rep.Carried++
		if decided {
			rep.Decided++
		}
	}
	return rep, nil
}

// rekeyOne copies one message, reporting whether it holds the user's decision.
func rekeyOne(from, to cachedir.Account, id, newID email.ID) (bool, error) {
	c := getCompact(from.Path(), id)
	if c == nil {
		return false, nil
	}
	decided := c.Override != message_info.OverrideNone
	raw, readErr := Read(from, id)
	if readErr != nil && !decided {
		return false, nil
	}
	if err := setCompact(to.Path(), newID, *c); err != nil {
		return decided, err
	}
	if readErr != nil {
		return decided, nil
	}
	p, err := lookupPath(to.Path(), newID)
	if err != nil {
		return decided, err
	}
	if err := dirlock.AtomicWrite(p, raw, 0600); err != nil {
		return decided, err
	}
	summary, sumErr := ReadSummary(from, id)
	if sumErr != nil {
		return decided, nil
	}
	return decided, StoreSummary(to, newID, summary)
}
