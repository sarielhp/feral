---
title: 'feral msg export'
parent: 'feral msg'
---

# feral msg export

Export an email or conversation thread to standard mbox or eml format.

## Usage

```
feral msg export [flags] <message-id>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<message-id>` | Message ID or prefix hash of an email in the thread to export. |

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
| `-o, --output <path>` | Output file (mbox) or directory (eml). |
| `-f, --format <format>` | Export format: 'mbox' (default) or 'eml'. (default: mbox) |
| `--single` | Export only this message, not its thread. |

## Examples

- `feral msg export 18fb1234abcd > thread.mbox` — Export full thread to mbox file via stdout
- `feral msg export -o thread.mbox 18fb1234abcd` — Export full thread directly to mbox file
- `feral msg export --format eml -o ./emails/ 18fb1234abcd` — Export each message in thread to individual .eml files
- `feral msg export --single 18fb1234abcd | git am` — Pipe single message patch directly into git am
- `feral msg export --dry-run 18fb1234abcd` — Preview thread messages and sizes without writing

---

[↑ feral](index.md) — [nav](nav.md)
