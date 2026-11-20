---
title: 'feral contacts suggest'
parent: 'feral contacts'
---

# feral contacts suggest

Suggest ranked email recipients combining curated contacts and interaction history.

## Usage

```
feral contacts suggest [query]
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `[query]` | Optional prefix or search string to match recipients. |

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

- `feral contacts suggest`
- `feral contacts suggest alice`
- `feral contacts suggest algo`

---

[↑ feral](index.md) — [nav](nav.md)
