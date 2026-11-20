---
title: 'feral label repair'
parent: 'feral label'
---

# feral label repair

Ensure that all parent folders exist for every nested folder path in the account.

## Usage

```
feral label repair
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

- `feral label fix`

---

[↑ feral](index.md) — [nav](nav.md)
