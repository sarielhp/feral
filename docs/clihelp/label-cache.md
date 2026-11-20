---
title: 'feral label cache'
has_children: true
parent: 'feral label'
---

# feral label cache

Manage the local labels cache: fetch and refresh cached label/folder data from the server.

## Usage

```
feral label cache <subcommand>
```

## Subcommands

| Command | Description |
|---------|-------------|
| [update (up, sync)](label-cache-update.md) | Refresh local cache from server. |

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

- `feral label cache update`

---

[↑ feral](index.md) — [nav](nav.md)
