---
title: 'feral config remove-email'
parent: 'feral config'
---

# feral config remove-email

Remove addresses from my_emails, the global list `config add-email` adds to. An address that comes from an account -- its user_email or its login -- is not in that list; the command refuses it, says which account sets it, and changes nothing.

## Usage

```
feral config remove-email <address>...
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<address>` | An address in my_emails. |

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

- `feral config remove-email me@old-job.edu`
- `feral config emails` — See what is left

---

[↑ feral](index.md) — [nav](nav.md)
