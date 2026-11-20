---
title: 'feral account calendar'
parent: 'feral account'
---

# feral account calendar

Show or set the calendar manager account.

## Usage

```
feral account calendar [account_name]
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `[account_name]` | Account name to configure calendar for. |

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

- `feral account calendar`
- `feral account calendar personal-gmail`

---

[↑ feral](index.md) — [nav](nav.md)
