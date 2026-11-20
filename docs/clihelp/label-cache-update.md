---
title: 'feral label cache update'
parent: 'feral label cache'
---

# feral label cache update

Fetch the latest labels and folder hierarchies from the server and refresh the local cache.

## Usage

```
feral label cache update
```

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
