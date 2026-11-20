---
title: 'feral link list'
parent: 'feral link'
---

# feral link list

Each link with what it runs and its state: ok; no symlink; symlink elsewhere (a file of that name that is not this feral); not on PATH (the symlink is there, but typing the name does not reach it); account missing (its account was removed -- running it fails until an account of that name exists again).

## Usage

```
feral link list
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

- `feral link list`
- `feral --json link list`

---

[↑ feral](index.md) — [nav](nav.md)
