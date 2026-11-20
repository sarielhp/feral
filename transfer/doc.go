// Package transfer moves and copies messages between accounts, or between
// folders of one: the dedup-upload-confirm-remove protocol behind splice and
// the TUI's cross-account move.
//
// A message is matched on the destination by Message-ID (Subject and Date only
// when a side lacks one), uploaded only when absent, confirmed by listing the
// destination from the server, and only then removed from the source. Each
// confirmed message is recorded in the source account's journal until it is
// removed, so an interrupted run's owed removals are finished by the next.
// Dedup makes the upload idempotent, so a rerun after a crash at any point
// never duplicates a message.
//
// It prints nothing: a Mover returns a Result per message for the caller to
// render, and journal recovery reports through report.Reporter. It names no
// backend; how a source removes a message is the caller's Remover.
package transfer
