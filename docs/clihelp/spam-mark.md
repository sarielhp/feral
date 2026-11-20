---
title: 'feral spam mark'
parent: 'feral spam'
---

# feral spam mark

Mark one or more messages as spam: train the filter and move them to the Spam folder.

## Usage

```
feral spam mark <message_id...>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<message_id...>` | One or more message IDs to mark as spam. |

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

- `feral spam mark abc123de` — Mark one message as spam

---

[↑ feral](index.md) — [nav](nav.md)
