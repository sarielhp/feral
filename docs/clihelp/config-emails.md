---
title: 'feral config emails'
parent: 'feral config'
---

# feral config emails

List every address feral treats as your own -- the my_emails list, each account's user_email, and each account's login when it is an address -- with where each one is set, followed by the commands that change them. A message from one of these addresses is mail you wrote: the TUI index shows its recipient instead of its sender.

## Usage

```
feral config emails
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

- `feral config emails`

---

[↑ feral](index.md) — [nav](nav.md)
