---
title: 'feral config check'
parent: 'feral config'
---

# feral config check

Validate configurations, account parameters, DNS reachability, and Bogofilter service.

## Usage

```
feral config check [--fix]
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
| `--fix` | Repair a received_folder set to "inbox" instead of only reporting it. |

## Examples

- `feral config check`
- `feral config check --fix` — Repair what the check reports

---

[↑ feral](index.md) — [nav](nav.md)
