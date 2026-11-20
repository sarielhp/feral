---
title: 'feral config add-email'
parent: 'feral config'
---

# feral config add-email

Add to my_emails, the global list of your own addresses; each account's user_email counts already. In the TUI index a message from one of them shows its recipient (→ To) instead of its sender, and a reply to it goes to that recipient. `config emails` lists every address that counts, `config remove-email` takes one out, and `config unset my_emails` clears the list.

## Usage

```
feral config add-email <address>...
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<address>` | An address of yours. |

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

- `feral config add-email me@old-job.edu me@example.org`
- `feral config emails`

---

[↑ feral](index.md) — [nav](nav.md)
