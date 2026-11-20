---
title: 'feral spam allow rm'
parent: 'feral spam allow'
---

# feral spam allow rm

Remove email addresses from allowlist.

## Usage

```
feral spam allow rm <email...>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<email...>` | Email addresses to remove from whitelist. |

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

- `feral spam allow rm friend@example.com`

---

[↑ feral](index.md) — [nav](nav.md)
