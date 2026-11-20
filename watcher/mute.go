package watcher

import (
	"context"

	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/mute"
	"github.com/sarielhp/feral/report"
)

// Archiver files messages out of an account's inbox. The watcher names no
// backend and holds only a reader, so Run's caller supplies it.
type Archiver func(ctx context.Context, account cfg_acc.AccountConfig, ids []email.ID) error

// archiveMuted takes the new mail of an account's muted threads out of
// arrivals and archives it, returning what is left to announce. A muted
// message is kept silent only once it is archived: when the list cannot be
// read, the session or the account may not change the server (--read-only,
// --dry-run, an account marked read-only, whose archive would be simulated), no
// Archive was supplied or the archive fails, every arrival is announced as
// usual -- the watermark records it as seen either way, and a message left
// in the inbox must never also be left unannounced.
func (d Deps) archiveMuted(ctx context.Context, rep report.Reporter, account cfg_acc.AccountConfig, arrivals []*email.Email) []*email.Email {
	if len(arrivals) == 0 || d.Config == nil || ctx.Err() != nil {
		return arrivals
	}
	path, err := mute.Path(d.Config.ConfigDir, account.Name)
	if err != nil {
		return arrivals // no configuration directory: nothing can have been muted
	}
	list, err := mute.Load(path)
	if err != nil {
		rep.Warnf("watch: %s: muted threads not applied: %v", account.Name, err)
		return arrivals
	}
	var keep []*email.Email
	var muted []email.ID
	for _, em := range arrivals {
		if _, ok := list.Match(em); ok {
			muted = append(muted, em.ID)
			continue
		}
		keep = append(keep, em)
	}
	if len(muted) == 0 {
		return arrivals
	}
	if d.Archive == nil || d.Config.ReadOnly || d.Config.DryRun || account.ReadOnly {
		rep.Infof("watch: %s: %d message(s) in muted threads left in the inbox: this session does not change the server",
			account.Name, len(muted))
		return arrivals
	}
	if err := d.Archive(ctx, account, muted); err != nil {
		rep.Warnf("watch: %s: archiving %d message(s) in muted threads failed: %v", account.Name, len(muted), err)
		return arrivals
	}
	rep.Infof("watch: %s: archived %d message(s) in muted threads", account.Name, len(muted))
	return keep
}
