---
title: 'feral rule add-domain'
parent: 'feral rule'
---

# feral rule add-domain

Add an auto-labeling rule for a sender's domain from a cached message ID.

## Usage

```
feral rule add-domain <message_id> [lbl]
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<message_id>` | The message ID or short ID of the email. |
| `[lbl]` | Target label (default: message's folder). |

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

- `feral rule add-domain 12345 %Sort/Newsletters`
- `feral rule add-domain 12345`

---

[↑ feral](index.md) — [nav](nav.md)
