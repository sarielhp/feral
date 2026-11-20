---
title: 'feral watch'
---

# feral watch

Poll every account in the background, notify on messages that arrived since the last poll, run a script per message, and flush the offline outbox. It runs in the foreground until interrupted; put it under systemd --user or in a tmux window to keep it alive. The first poll of an account records what is already there without notifying.

## Usage

```
feral watch [flags]
```

## Flags

| Flag | Description |
|------|-------------|
| `-A, --account <name>` | Target account name |
| `--account-index N` | Target account by config index (also -N) |
| `--read-only` | Block server writes (cache still updates) |
| `-D, --dry-run` | Report changes only; implies --read-only |
| `-v, --verbose` | Enable verbose output |
| `--json` | Emit result as JSON on stdout |
| `--logappend` | Append to existing logs instead of truncating |
| `--version` | Print the version number and exit |
| `-i, --interval <duration>` | How often to poll (minimum 30s) (default: 2m0s) |
| `--on-mail <path>` | Run this program once per new message |
| `--no-notify` | Do not send desktop notifications |
| `--no-flush` | Do not flush the offline outbox |

## Examples

- `feral watch` — Watch every account, notifying on new mail
- `feral watch -A work --no-notify` — Watch one account, log instead of notifying
- `feral watch --on-mail ~/bin/on-mail.sh` — Run a script for each new message

## Which accounts

Every account is watched by default; pass the global -A/--account to watch just one.

## Notifications

New mail is announced as a desktop notification when notify-send is available -- one per account per poll, listing the messages -- and as a log line otherwise. The TUI runs this watcher too while it is open; only one runs per download directory, and a second watch exits saying another is running.

## The --on-mail hook

--on-mail passes the message in the environment: FERAL_ACCOUNT, FERAL_FROM, FERAL_SUBJECT, FERAL_DATE, FERAL_ID (also as MAIL_CLI_*, the names before the rename). A hook still running after 30 seconds is killed.

## The first poll

The first poll of an account records existing messages as seen and announces nothing, so starting the watcher does not notify you about the whole inbox.

## What is announced

Only the inbox is watched: each poll asks every account for the IDs of the newest messages in its inbox, one small request, and downloads only those it has not seen. Mail filed elsewhere -- spam, sent, drafts, a label a filter skips the inbox for -- is never announced, nor is mail dated more than six hours before the previous poll: an older message that came into view because newer ones were deleted or moved back.

## Failures

After a failed poll it waits longer each time, up to ten minutes, and reports the failure instead of exiting.

---

[↑ feral](index.md) — [nav](nav.md)
