---
title: 'feral msg last'
parent: 'feral msg'
---

# feral msg last

Check all folders in the account, collect the last N emails received, save them to a virtual mailbox, and print each email with its folder name.

## Usage

```
feral msg last <N>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<N>` | Number of most recent emails to retrieve and display. |

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

- `feral msg last 10`
- `feral msg last 25`
- `feral -A work-jmap msg last 20`

---

[↑ feral](index.md) — [nav](nav.md)
