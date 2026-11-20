---
title: 'feral label rm'
parent: 'feral label'
---

# feral label rm

Delete a label or folder from the server.

## Usage

```
feral label rm %<name>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<lbl_name>` | The full path of the label to delete. |

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
| `-y, --yes` | Proceed without the confirmation prompt. |

## Examples

- `feral label del %Receipts/Old`

---

[↑ feral](index.md) — [nav](nav.md)
