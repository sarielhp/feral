---
title: 'feral calendar sync'
parent: 'feral calendar'
---

# feral calendar sync

Scan all messages in the inbox for .ics attachments, extract meeting invites, and synchronize them into the configured calendar account.

## Usage

```
feral calendar sync
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

## Examples

- `feral calendar sync` — Add every .ics attachment in the inbox to the calendar

---

[↑ feral](index.md) — [nav](nav.md)
