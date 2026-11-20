---
title: 'feral utils cache clear'
parent: 'feral utils cache'
---

# feral utils cache clear

Recreate the cache directory for the current account, wiping all cached data.

## Usage

```
feral utils cache clear [--yes]
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
| `-y, --yes` | Proceed without the confirmation prompt. |

## Examples

- `feral utils cache clear`
- `feral utils cache repair`

---

[↑ feral](index.md) — [nav](nav.md)
