---
title: 'feral label rename'
parent: 'feral label'
---

# feral label rename

Rename a label/folder and update all existing routing rules that reference it.

## Usage

```
feral label rename <old_name> <new_name>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<old_name>` | The full path of the label to rename. |
| `<new_name>` | The new full path for the label. |

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

- `feral labels rename "Archive/2025" "Archive/2026"`

---

[↑ feral](index.md) — [nav](nav.md)
