---
title: 'feral contacts import-mutt'
parent: 'feral contacts'
---

# feral contacts import-mutt

Import contacts and distribution lists from a Mutt alias file.

## Usage

```
feral contacts import-mutt [file_path]
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `[file_path]` | Mutt alias file (default: from config) |

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

- `feral contacts import-mutt`
- `feral contacts import-mutt ~/.mutt/aliases`

---

[↑ feral](index.md) — [nav](nav.md)
