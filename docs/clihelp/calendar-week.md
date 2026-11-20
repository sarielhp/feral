---
title: 'feral calendar week'
parent: 'feral calendar'
---

# feral calendar week

Show all calendar events in the upcoming week or specified number of days.

## Usage

```
feral calendar week [days]
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `[days]` | Number of days to display (defaults to 7). |

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

- `feral calendar week`
- `feral calendar week 14`

---

[↑ feral](index.md) — [nav](nav.md)
