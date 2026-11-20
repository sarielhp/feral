---
title: 'feral label search'
parent: 'feral label'
---

# feral label search

Search cached labels by one or more substring patterns (all must match).

## Usage

```
feral label search <substring> [additional_substrings...]
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<substring>` | Substring to search for in folder paths. |
| `[additional_substrings...]` | Extra substrings; all must match. |

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

- `feral label search receipts`
- `feral label search archive 2026`

---

[↑ feral](index.md) — [nav](nav.md)
