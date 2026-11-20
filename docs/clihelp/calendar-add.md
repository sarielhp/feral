---
title: 'feral calendar add'
parent: 'feral calendar'
---

# feral calendar add

Add a calendar event from an .ics attachment in a specific email.

## Usage

```
feral calendar add [label_prefix] <message_id>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `[label_prefix]` | Label prefix to locate the message (defaults to inbox). |
| `<message_id>` | Message ID or prefix containing the .ics attachment. |

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

- `feral calendar add 1234abcd`
- `feral calendar add %receipts 5678efgh`

---

[↑ feral](index.md) — [nav](nav.md)
