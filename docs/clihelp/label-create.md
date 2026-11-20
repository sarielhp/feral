---
title: 'feral label create'
parent: 'feral label'
---

# feral label create

Create a label or folder on the server.

## Usage

```
feral label create <lbl_name>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<lbl_name>` | The name or path of the label/folder to create. |

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

- `feral labels create "Work/Projects"`

---

[↑ feral](index.md) — [nav](nav.md)
