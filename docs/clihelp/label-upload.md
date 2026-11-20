---
title: 'feral label upload'
parent: 'feral label'
---

# feral label upload

Upload messages from mbox file to folder.

## Usage

```
feral label upload <label> <file_name>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<label>` | The target label/folder to upload messages into. |
| `<file_name>` | The local mbox file path to read messages from. |

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

- `feral label upload %receipts ~/archive/receipts.mbox`

---

[↑ feral](index.md) — [nav](nav.md)
