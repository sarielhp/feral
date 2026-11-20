---
title: 'feral contacts search'
parent: 'feral contacts'
---

# feral contacts search

Search contacts across names, emails, nicknames, groups, and notes.

## Usage

```
feral contacts search <query>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<query>` | Search query words. |

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

- `feral contacts search algo`
- `feral contacts search "dave mount"`

---

[↑ feral](index.md) — [nav](nav.md)
