---
title: 'feral spam allow'
has_children: true
parent: 'feral spam'
---

# feral spam allow

Manage the personal sender whitelist to bypass spam checks.

## Usage

```
feral spam allow <subcommand> [args...]
```

## Subcommands

| Command | Description |
|---------|-------------|
| [add](spam-allow-add.md) | Add email addresses to allowlist. |
| [rm (del)](spam-allow-rm.md) | Remove email addresses from allowlist. |
| [list](spam-allow-list.md) | List allowed email addresses. |

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

- `feral spam allow list`
- `feral spam allow add friend@example.com`

---

[↑ feral](index.md) — [nav](nav.md)
