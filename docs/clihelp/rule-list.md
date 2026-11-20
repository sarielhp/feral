---
title: 'feral rule list'
parent: 'feral rule'
---

# feral rule list

List custom routing rules with their target labels and export status.

## Usage

```
feral rule list [flags]
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
| `-a, --all` | List all custom routing rules, including those already exported |

## Examples

- `feral rule list`
- `feral rule list --all`

---

[↑ feral](index.md) — [nav](nav.md)
