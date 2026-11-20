---
title: 'feral label archive'
parent: 'feral label'
---

# feral label archive

Archive every message in a folder.

## Usage

```
feral label archive [%label]
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `[%label]` | The folder to empty into the archive. Defaults to the inbox. |

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

- `feral label archive` — Archive everything in the inbox
- `feral label archive %receipts` — Archive everything in one folder

---

[↑ feral](index.md) — [nav](nav.md)
