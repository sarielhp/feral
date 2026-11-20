---
title: 'feral account test'
parent: 'feral account'
---

# feral account test

Check an account's config and connection.

## Usage

```
feral account test [account_name]
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `[account_name]` | Account name to test (defaults to currently selected account). |

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

- `feral account test`
- `feral account test work-fastmail`

---

[↑ feral](index.md) — [nav](nav.md)
