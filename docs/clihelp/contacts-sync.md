---
title: 'feral contacts sync'
parent: 'feral contacts'
---

# feral contacts sync

Fetch and update contacts from Google People API into local contacts.json.

## Usage

```
feral contacts sync [account]
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `[account]` | Name of the Google account to sync (defaults to selected account). |

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

- `feral contacts sync`
- `feral contacts sync work`

---

[↑ feral](index.md) — [nav](nav.md)
