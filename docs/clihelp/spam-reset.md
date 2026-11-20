---
title: 'feral spam reset'
parent: 'feral spam'
---

# feral spam reset

Discard every local spam classification and score for this account.

## Usage

```
feral spam reset
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

- `feral spam reset` — Forget what the filter learned locally and re-classify

---

[↑ feral](index.md) — [nav](nav.md)
