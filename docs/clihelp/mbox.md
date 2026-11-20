---
title: 'feral mbox'
has_children: true
---

# feral mbox

Inspect, merge and import local mbox files.

## Usage

```
feral mbox <subcommand> [args...]
```

## Subcommands

| Command | Description |
|---------|-------------|
| [check](mbox-check.md) | Validate mbox file or directory. |
| [save](mbox-save.md) | Append a folder to an mbox file. |
| [merge](mbox-merge.md) | Merge mbox file x into y. |
| [info](mbox-info.md) | Display message count and file size. |
| [view](mbox-view.md) | Browse a local mbox or .eml file in the TUI. |
| [import](mbox-import.md) | Import an mbox file as received mail. |

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

- `feral mbox check archive.mbox` — Validate a single mbox file
- `feral mbox check ~/archive/` — Validate all mbox files in a directory
- `feral mbox merge new.mbox archive.mbox` — Merge new.mbox into archive.mbox deduplicating messages
- `feral mbox info archive.mbox` — Show message count, file size, and attachment statistics
- `feral mbox view archive.mbox` — Open and browse an mbox file in the TUI (read-only)
- `feral mbox import archive.mbox` — Copy its messages into the received folder

---

[↑ feral](index.md) — [nav](nav.md)
