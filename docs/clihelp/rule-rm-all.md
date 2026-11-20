---
title: 'feral rule rm-all'
parent: 'feral rule'
---

# feral rule rm-all

Delete all routing rules for the selected account.

## Usage

```
feral rule rm-all [--yes]
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

- `feral rule rm-all`

---

[↑ feral](index.md) — [nav](nav.md)
