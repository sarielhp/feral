---
title: 'feral label move'
parent: 'feral label'
---

# feral label move

Move emails matching pattern to target label.

## Usage

```
feral label move <source_label> <pattern> <target_label> [--do]
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<source_label>` | Source folder/label containing messages to split. |
| `<pattern>` | Subject pattern with wildcards (*, ?) to match. |
| `<target_label>` | Destination folder/label to move matched messages to. |

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
| `--do` | Perform the actual move operations |

## Examples

- `feral label move Inbox "*invoice*" Receipts`
- `feral label move Inbox "*invoice*" Receipts --do`

---

[↑ feral](index.md) — [nav](nav.md)
