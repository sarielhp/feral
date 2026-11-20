---
title: 'feral rule add'
parent: 'feral rule'
---

# feral rule add

Add a server filter rule to auto-label emails from a sender or from a cached message ID.

## Usage

```
feral rule add <email|message_id> [lbl]
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<email\|message_id>` | The sender email address or cached message ID. |
| `[lbl]` | Target label (optional with message_id). |

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

- `feral rule add notifications@github.com %Dev/GitHub`
- `feral rule add 12345 %Dev/GitHub`
- `feral rule add 12345`

---

[↑ feral](index.md) — [nav](nav.md)
