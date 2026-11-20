---
title: 'feral contacts tui'
parent: 'feral contacts'
---

# feral contacts tui

Open the full-screen contacts manager: search, view and edit contacts, add people and groups, merge duplicates, hide imported contacts, and turn harvested addresses into contacts. Edits to a contact imported from Google or mutt are kept beside what the source says and survive every re-sync; `r` in the detail view reverts to the source. Press ? inside for every key. Under --read-only nothing is saved; under --dry-run each change is checked and reported but not saved.

## Usage

```
feral contacts tui
```

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

- `feral contacts tui`

---

[↑ feral](index.md) — [nav](nav.md)
