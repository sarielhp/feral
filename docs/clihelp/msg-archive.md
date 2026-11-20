---
title: 'feral msg archive'
parent: 'feral msg'
---

# feral msg archive

Archive one or more messages by ID.

## Usage

```
feral msg archive <message-id...>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<message-id...>` | One or more message IDs (short 8-character or full). |

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

- `feral msg archive abc123de` — Archive one message
- `feral msg archive abc123de 9f8e7d6c` — Archive several at once

---

[↑ feral](index.md) — [nav](nav.md)
