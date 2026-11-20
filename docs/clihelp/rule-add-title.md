---
title: 'feral rule add-title'
parent: 'feral rule'
---

# feral rule add-title

Add a server filter rule to auto-label emails with matching subject prefix.

## Usage

```
feral rule add-title <title> <lbl>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<title>` | Subject prefix/substring to match. |
| `<lbl>` | The target label/folder hierarchy. |

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

- `feral rule add-title "[JIRA]" %Work/Jira`

---

[↑ feral](index.md) — [nav](nav.md)
