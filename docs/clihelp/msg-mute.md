---
title: 'feral msg mute'
parent: 'feral msg'
---

# feral msg mute

A muted thread's new messages skip the inbox: the watcher (feral watch, or the one feral tui runs) archives them as they arrive and does not announce them. Mail already in the inbox stays where it is. A thread is recognized by the Message-ID at its root, so a reply that names any message of the thread in References or In-Reply-To belongs to it. The list is kept per account in muted_threads.json in the account's settings directory. Muting is refused under --read-only, where the archiving it asks for could not happen; --dry-run reports what would change and writes nothing.

## Usage

```
feral msg mute <message-id...>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<message-id...>` | A message of each thread to mute (ID or short ID). |

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

- `feral msg mute abc123de` — Mute the thread of one message

---

[↑ feral](index.md) — [nav](nav.md)
