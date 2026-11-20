---
title: 'feral msg block'
parent: 'feral msg'
---

# feral msg block

Block the senders of messages and move them.

## Usage

```
feral msg block [flags] <message-id...>
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
| `--trash` | Route to Trash instead of Spam (no spam filter training) |
| `--push` | Immediately push updated rules to the server |

## Examples

- `feral msg block abc123de` — Block sender of one message to Spam
- `feral msg block --trash abc123de` — Block sender of message to Trash

---

[↑ feral](index.md) — [nav](nav.md)
