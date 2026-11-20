---
title: 'feral account setup'
parent: 'feral account'
---

# feral account setup

Configure an email account interactively.

## Usage

```
feral account setup
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

- `feral account setup` — Launch guided account setup wizard

---

[↑ feral](index.md) — [nav](nav.md)
