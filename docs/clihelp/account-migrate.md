---
title: 'feral account migrate'
parent: 'feral account'
---

# feral account migrate

Copy configuration and credentials to a remote machine via SSH/SCP.

## Usage

```
feral account migrate [user@]host
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `[user@]host` | Destination host, as [user@]host |

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

- `feral account migrate myserver.local`
- `feral account migrate user@myserver.local`

---

[↑ feral](index.md) — [nav](nav.md)
