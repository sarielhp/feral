---
title: 'feral spam unmark folder'
parent: 'feral spam unmark'
---

# feral spam unmark folder

Mark all messages in the specified folder as ham and move them back to Inbox.

## Usage

```
feral spam unmark folder <folder_name>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<folder_name>` | Folder name containing emails to unspam. |

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

- `feral spam unmark folder %spam`

---

[↑ feral](index.md) — [nav](nav.md)
