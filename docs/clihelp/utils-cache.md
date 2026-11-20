---
title: 'feral utils cache'
has_children: true
parent: 'feral utils'
---

# feral utils cache

Manage the local email download cache.

## Usage

```
feral utils cache <subcommand> [args...]
```

## Subcommands

| Command | Description |
|---------|-------------|
| [prune](utils-cache-prune.md) | Prune cached emails older than N days. |
| [clear (reset)](utils-cache-clear.md) | Recreate cache directory and wipe data. |
| [repair](utils-cache-repair.md) | Check and rewrite this account's index. |
| [outlook-ids](utils-cache-outlook-ids.md) | Key an Outlook cache by stable message IDs. |

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

- `feral utils cache prune`
- `feral utils cache prune 7`
- `feral utils cache prune --wipe`
- `feral utils cache clear`

---

[↑ feral](index.md) — [nav](nav.md)
