---
title: 'feral label count'
parent: 'feral label'
---

# feral label count

Count the messages in a label or folder.

## Usage

```
feral label count <label> [flags]
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<label>` | Label or folder, optionally account:label |

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
| `-q, --quiet` | Print only the number |
| `-u, --unread` | Output unread count instead of total count |

## Examples

- `feral label count inbox`
- `feral label count inbox -q`
- `feral label count FastMail:received_private -q`
- `feral label count work:Inbox -u -q`

---

[↑ feral](index.md) — [nav](nav.md)
