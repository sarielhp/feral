---
title: 'feral account rm'
parent: 'feral account'
---

# feral account rm

Delete an account and its credentials.

## Usage

```
feral account rm <account_name>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<account_name>` | The name of the account to delete. |

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
| `-y, --yes` | Proceed without the confirmation prompt. |

## Examples

- `feral account rm temp-account`

---

[↑ feral](index.md) — [nav](nav.md)
