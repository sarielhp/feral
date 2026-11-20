---
title: 'feral mbox check'
parent: 'feral mbox'
---

# feral mbox check

Check if a file (or all files in a directory) is a valid mbox file.

## Usage

```
feral mbox check <file>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<file>` | Mbox file or directory to validate |

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

- `feral mbox check inbox.mbox`
- `feral mbox check ~/archive/`

---

[↑ feral](index.md) — [nav](nav.md)
