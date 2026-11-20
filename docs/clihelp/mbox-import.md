---
title: 'feral mbox import'
parent: 'feral mbox'
---

# feral mbox import

Copy every message of a local mbox (or .eml) file into the account's received folder -- its `received_folder`, or the inbox when it has none -- or into the label given. A message the folder already holds (same Message-ID; Subject and Date when a side has none) is skipped, so an import can be run again, or resumed after a failure, without duplicating anything; that is the difference from `upload`, which uploads every message it reads. The file is only read, never changed. Under --read-only or --dry-run, or into a read-only account, it reports what it would import and uploads nothing.

## Usage

```
feral mbox import <file> [%label]
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<file>` | Local mbox or .eml file to read |
| `[%label]` | Destination (default: received folder) |

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

- `feral mbox import mbox.fl` — Into the selected account's received folder
- `feral mbox import mbox.fl %work:projects` — Into a label of another account

---

[↑ feral](index.md) — [nav](nav.md)
