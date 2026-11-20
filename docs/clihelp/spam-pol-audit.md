---
title: 'feral spam pol audit'
parent: 'feral spam pol'
---

# feral spam pol audit

Scan Spam folder for political fundraising emails and print scoring details.

## Usage

```
feral spam pol audit
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

- `feral spam pol audit` — Report political mail in Spam, changing nothing

---

[↑ feral](index.md) — [nav](nav.md)
