// Package watcher polls mail accounts in the background and announces what is
// new: a desktop notification, a user script, and a flush of the offline
// outbox. Each poll also brings back the snoozed messages that are due (see
// package snooze), the TUI's watcher included.
//
// It is a foreground process by design. systemd --user, tmux or a desktop
// session manager supervises it; the watcher never double-forks, never writes a
// PID file, and exits when its context is cancelled. A supervisor that restarts
// it and a log it writes to are the two things a daemon needs, and both already
// exist outside the program. The TUI runs the same loop on a goroutine for as
// long as it is open (`feral tui`, unless --no-watch or "tui_watch": false),
// reporting to its log and never flushing the outbox.
//
// One watcher runs per download directory: Run holds a lock beside the
// watermark for as long as it polls, and a second Run returns
// ErrAlreadyWatching, because two would both announce the same new message.
// A poll is one request per account -- LatestInboxIDs, the inbox's newest
// IDs -- on a client kept from poll to poll; only the inbox is watched. A
// poll's new mail in one account is one desktop notification, however many
// messages it is; the --on-mail hook still runs once per message.
//
// The watermark it keeps -- which message IDs it has already seen, per account,
// under <download_dir>/watcher/state.json -- is what makes the difference
// between "there is new mail" and "here is what arrived since you last looked".
// An account's first poll records everything it finds as seen and announces
// nothing, so a fresh install does not fire a notification per message already
// sitting in the inbox.
//
// It does not hold the cache lock across a poll. dirlock serializes a
// read-modify-write of one cache index, and the download path takes it from
// worker goroutines; holding it here, in the polling goroutine, would block
// those workers on a mutex this goroutine owns and deadlock the fetch. What the
// watcher does need it for -- a consistent read and write of its own watermark
// -- it takes, briefly, around the state file alone.
package watcher
