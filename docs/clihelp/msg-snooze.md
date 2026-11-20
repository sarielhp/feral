---
title: 'feral msg snooze'
parent: 'feral msg'
---

# feral msg snooze

msg snooze --until <time> moves messages into a Snoozed folder on their account, created the first time, and records when each is due in snooze.json in the download directory (H, for hide until, in the TUI, on the list or an open message, does the same for that message). When the time comes a watcher puts it back in the inbox, unread: feral watch, or the one the TUI runs while it is open. With neither running the message waits; msg snooze --wake brings back what is due, or the messages named, at once. Without flags, msg snooze lists what is snoozed.

## Usage

```
feral msg snooze [flags] [<message-id>...]
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `[<message-id>...]` | Message IDs (short 8-character or full). |

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
| `-u, --until <time>` | Snooze until a time: +2h, tomorrow 8am. |
| `-l, --list` | List snoozed messages (the default). |
| `--wake` | Return messages now; with no IDs, those due. |
| `--forget` | Drop the records; move nothing. |

## Examples

- `feral msg snooze --until "tomorrow 8am" abc123de` — Hide a message until the morning
- `feral msg snooze -u +2h abc123de 9f8e7d6c` — Hide two messages for two hours
- `feral msg snooze` — List what is snoozed and when it returns
- `feral msg snooze --wake abc123de` — Bring one back now

## Times

Local time, as msg send --at reads it: "2026-11-20 08:00", "+2h", "+30m", "+1d", "today 17:00" or "tomorrow 8am". A time already past is refused.

## When a message cannot come back

A message deleted meanwhile, or one whose account was removed, keeps its record and is reported by the watcher; msg snooze --forget drops the record and moves nothing. An Outlook account whose cache still holds default message IDs is refused, because they change on the move: run feral utils cache outlook-ids first.

## Read-only and dry runs

Snoozing and waking are refused under --read-only, since the record would describe a move that did not happen; --dry-run says what would move and changes nothing.

---

[↑ feral](index.md) — [nav](nav.md)
