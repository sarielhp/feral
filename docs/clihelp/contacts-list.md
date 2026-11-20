---
title: 'feral contacts list'
parent: 'feral contacts'
---

# feral contacts list

List stored contacts and aliases with an optional substring filter.

## Usage

```
feral contacts list [filter]
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `[filter]` | Optional substring to filter by name, email, or alias. |

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

- `feral contacts list`
- `feral contacts list john`

---

[↑ feral](index.md) — [nav](nav.md)
