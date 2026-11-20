---
title: 'feral spam empty'
parent: 'feral spam'
---

# feral spam empty

Permanently delete all messages currently residing in the remote Spam folder. Unlike archiving or trashing, this operation is immediate and irreversible on the server.

## Usage

```
feral spam empty [--yes]
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
| `-y, --yes` | Proceed without the confirmation prompt. |

## Examples

- `feral spam empty --yes` — Permanently purge the Spam folder

---

[↑ feral](index.md) — [nav](nav.md)
