---
title: 'feral link rm'
parent: 'feral link'
---

# feral link rm

Remove the link from config.json, and its symlink beside the feral binary (or in --dir) if that symlink is this feral; any other file of that name is left alone and reported.

## Usage

```
feral link rm <name> [--dir D]
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<name>` | The link to remove. |

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
| `--dir <dir>` | Look for the symlink here, not beside feral. |

## Examples

- `feral link rm mt`

---

[↑ feral](index.md) — [nav](nav.md)
