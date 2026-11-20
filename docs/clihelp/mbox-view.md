---
title: 'feral mbox view'
parent: 'feral mbox'
---

# feral mbox view

Browse a local mbox or .eml file in the TUI.

## Usage

```
feral mbox view <file>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<file>` | A local mbox file, or a single message ending in .eml |

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

- `feral mbox view archive.mbox`
- `feral mbox view ~/mail/backup.mbox`
- `feral mbox view message.eml` — Open one saved message

---

[↑ feral](index.md) — [nav](nav.md)
