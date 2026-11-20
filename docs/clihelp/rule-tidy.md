---
title: 'feral rule tidy'
parent: 'feral rule'
---

# feral rule tidy

Remove duplicate routing rules, keeping the last rule for each sender or subject.

## Usage

```
feral rule tidy
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

- `feral rule tidy`

---

[↑ feral](index.md) — [nav](nav.md)
