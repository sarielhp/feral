---
title: 'feral utils cache prune'
parent: 'feral utils cache'
---

# feral utils cache prune

Prune cached emails and spam scores older than the specified number of days (default: 30). With --wipe, purges all cached messages regardless of age.

## Usage

```
feral utils cache prune [days] [-w, --wipe]
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `[days]` | Age threshold in days for pruning cached emails (default: 30). |

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
| `-w, --wipe` | Purge all cached email files regardless of age. |

## Examples

- `feral utils cache prune`
- `feral utils cache prune 7`
- `feral utils cache prune --wipe`

---

[↑ feral](index.md) — [nav](nav.md)
