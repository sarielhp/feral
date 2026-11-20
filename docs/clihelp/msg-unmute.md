---
title: 'feral msg unmute'
parent: 'feral msg'
---

# feral msg unmute

Name the thread by any cached message of it, or by the <key> msg muted prints (the brackets may be left off). New mail in the thread reaches the inbox and is announced again; what the watcher already archived stays archived. Unmuting is allowed under --read-only, since it records nothing the server did not do; --dry-run reports.

## Usage

```
feral msg unmute <message-id|key...>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<message-id\|key...>` | A message of the thread, or a <key> from msg muted. |

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

- `feral msg unmute abc123de` — Unmute the thread of one message
- `feral msg unmute '<root@example.com>'` — Unmute a thread by its listed key

---

[↑ feral](index.md) — [nav](nav.md)
