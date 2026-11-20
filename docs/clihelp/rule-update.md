---
title: 'feral rule update'
parent: 'feral rule'
---

# feral rule update

Ensure all blacklisted senders have a corresponding local rule.

## Usage

```
feral rule update
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

- `feral rule update`

---

[↑ feral](index.md) — [nav](nav.md)
