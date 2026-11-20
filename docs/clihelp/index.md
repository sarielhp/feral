---
title: 'feral'
has_children: true
---

# feral

feral 🐕 -- a terminal mail client for Linux: Gmail, Outlook, Fastmail.

## Commands

| Command | Description |
|---------|-------------|
| [scan](scan.md) | Scan folders matching label prefix for spam. |
| [search](search.md) | Search email history or export to mbox. |
| [write](write.md) | Write a message in your editor, then review. |
| [tui](tui.md) | Open interactive terminal email browser. |
| [watch](watch.md) | Watch accounts and notify on new mail. |
| [msg](msg.md) | Read, send, and manage messages. |
| [label](label.md) | Manage labels: download, upload, sort, move. |
| [mbox](mbox.md) | Inspect, merge and import local mbox files. |
| [spam](spam.md) | Manage Spam folder and train filters. |
| [rule](rule.md) | Manage routing rules and server filters. |
| [calendar](calendar.md) | Manage calendar events from attachments. |
| [contacts](contacts.md) | Manage contacts and aliases. |
| [account](account.md) | Configure accounts and sign in. |
| [link](link.md) | Manage other names that run feral. |
| [config](config.md) | Show or manage configuration options. |
| [utils](utils.md) | Maintain cache and shell integration. |
| [docs](docs.md) | Generate Markdown help pages |

## Global Flags

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

- `feral %` — Scan the inbox -- a label is written %name, and % alone is the inbox
- `feral %work:receipts` — Scan one folder on one account
- `feral tui` — Browse mail interactively
- `feral label sort %receipts` — File a folder into the dated keep/ hierarchy
- `feral spam sweep` — Move spam out of the inbox and train the filter
- `feral -E <command>` — Show the examples for one command

## Version

0.7.79

## Config file location

`~/.config/feral/config.json`

## About

A wild dog to take care of your email. What could possibly go wrong?

Not much: --dry-run shows what any command would do, --read-only never touches the server, spam is moved to a folder rather than deleted, and emptying that folder asks first.

GitHub: [sarielhp/feral](https://github.com/sarielhp/feral)

