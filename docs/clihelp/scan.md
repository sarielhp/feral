---
title: 'feral scan'
---

# feral scan

Scan all folders starting with the given label prefix (case-insensitive) for spam emails using Bogofilter. The folders are in the default account (`account default`) unless the prefix names one (`%work:inbox`, or -A); `%all:inbox` scans every account. `feral %` is this command for the inbox.

## Usage

```
feral scan <lbl_prefix> [flags]
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<lbl_prefix>` | Label prefix to scan (e.g. inbox) |

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
| `-p, --pattern <pattern>` | Only messages whose subject has this |
| `-m, --move` | Move identified spam emails to the Spam folder |
| `--move-from <From>` | Move the lone message from this sender to Spam |
| `--inbox-move <From>` | Move mail from this sender back to Inbox |

## Examples

- `feral scan inbox`
- `feral scan inbox -m`
- `feral scan receipts --move-from spammer@example.com`

---

[↑ feral](index.md) — [nav](nav.md)
