---
title: 'feral contacts'
has_children: true
---

# feral contacts

View and manage email contacts and aliases from Google and Mutt.

## Usage

```
feral contacts [subcommand]
```

## Subcommands

| Command | Description |
|---------|-------------|
| [list](contacts-list.md) | List stored contacts and aliases. |
| [tui](contacts-tui.md) | Manage contacts full screen. |
| [pick](contacts-pick.md) | Choose contacts full screen; print them on stdout. |
| [search](contacts-search.md) | Search contacts by name, email, or note. |
| [suggest](contacts-suggest.md) | Suggest ranked recipients from history. |
| [harvest](contacts-harvest.md) | Build suggestion index from cached mail. |
| [import-mutt](contacts-import-mutt.md) | Import contacts from a Mutt alias file. |
| [sync](contacts-sync.md) | Sync contacts from Google People API. |
| [path](contacts-path.md) | Print the file path to local contacts store. |

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

- `feral contacts` — List all contacts and aliases
- `feral contacts tui` — Manage contacts full screen
- `feral contacts search algo` — Search contacts and groups
- `feral contacts suggest alice` — Suggest ranked email recipients
- `feral contacts harvest` — Harvest suggested recipients from cached mail
- `feral contacts sync` — Sync contacts from Google Contacts
- `feral contacts import-mutt` — Import Mutt aliases configured in config.json
- `feral contacts import-mutt ~/.mutt/aliases` — Import Mutt aliases from a specific file

---

[↑ feral](index.md) — [nav](nav.md)
