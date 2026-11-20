---
title: 'feral contacts harvest'
parent: 'feral contacts'
---

# feral contacts harvest

Scan cached sent and archived messages across all accounts to build the recipient suggestion index.

## Usage

```
feral contacts harvest
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

- `feral contacts harvest`

---

[↑ feral](index.md) — [nav](nav.md)
