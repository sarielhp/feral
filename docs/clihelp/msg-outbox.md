---
title: 'feral msg outbox'
has_children: true
parent: 'feral msg'
---

# feral msg outbox

Inspect, transmit, or delete messages spooled to the local outbox when offline, queued via msg send --queue, or scheduled via msg send --at. A scheduled message is sent by the first flush at or after its due time: feral watch flushes after every clean poll, and so does outbox flush run by hand or from cron. Nothing else sends it -- the TUI's own watcher never flushes -- so with neither running it waits.

## Usage

```
feral msg outbox <ls | flush | rm <id>...>
```

## Subcommands

| Command | Description |
|---------|-------------|
| [ls (list)](msg-outbox-ls.md) | List queued messages in outbox. |
| [flush](msg-outbox-flush.md) | Transmit queued messages that are due. |
| [rm](msg-outbox-rm.md) | Delete queued messages from outbox. |

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

## Examples

- `feral msg outbox ls` — List all spooled messages
- `feral msg outbox flush` — Transmit every queued message that is due
- `feral msg outbox rm 20261011-134500-0001` — Delete a queued message

---

[↑ feral](index.md) — [nav](nav.md)
