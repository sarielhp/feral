---
title: 'feral label sort'
has_children: true
parent: 'feral label'
---

# feral label sort

Organize and archive messages from a source folder/label into the date-based keep/YYYY/MM/<folder> structure (root "keep/" is fixed). Runs in dry-run mode by default; use --move to perform actual moves or --copy to copy without deleting from source. Use -f, -F, or -Y to customize destination folder naming. The source may be a local mbox file -- an mbox account, or a path such as /var/mail/you or ./old.mbox -- whose messages are copied into another account (name it with -f, -F or -Y %account:name) and never removed from the file: --move acts as --copy, and a message already in the destination is skipped.

## Usage

```
feral label sort <folder> [flags]
feral label sort examples
```

## Subcommands

| Command | Description |
|---------|-------------|
| [examples](label-sort-examples.md) | Show archive mapping patterns and examples. |

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<folder>` | Source folder (e.g. "GMail:receipts"), or an mbox file path. |

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
| `--move` | Move the messages (default: dry run) |
| `--copy` | Copy the messages (default: dry run) |
| `-n <num>` | Number of messages to process (default: 10) (default: 10) |
| `-p, --pattern <pattern>` | Only messages whose subject has this |
| `-f, --folder <name>` | Folder keep/YYYY/MM/<name> |
| `-F, --folder-suffix <name>` | Folder keep/YYYY/MM/<name>-YYYY-MM |
| `-Y, --folder-year <name>` | Folder keep/YYYY/<name>-YYYY-MM |
| `--allow` | Allow sourcing from folders that start with keep/ |

## Examples

- `feral label sort %receipts` — Dry run preview of date-based archive for receipts folder
- `feral label sort %receipts --move` — Move receipts messages to keep/YYYY/MM/receipts
- `feral label sort %receipts -f Archive --move` — Move receipts messages to keep/YYYY/MM/Archive
- `feral label sort %receipts -F Receipts --move` — Move receipts messages to keep/YYYY/MM/Receipts-YYYY-MM
- `feral label sort %GMail:received -Y %FastMail:services --move` — Cross-account move from GMail to FastMail keep/YYYY/services-YYYY-MM
- `feral label sort %receipts -n 50 --copy` — Copy up to 50 receipts messages without deleting source
- `feral label sort %receipts -p "*order*" --move` — Move only receipts messages matching pattern
- `feral label sort examples` — Show detailed structured examples and mapping patterns

---

[↑ feral](index.md) — [nav](nav.md)
