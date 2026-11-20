---
title: 'feral link'
has_children: true
---

# feral link

A link is another name for feral that runs it with arguments put first: with `feral link add mt -- -A fastmail tui`, typing `mt` runs `feral -A fastmail tui`, and `mt -2` runs it on account 2, because the last account named on a line wins. A link is a symlink to the feral binary plus an entry under "links" in config.json; Tab completes it and Alt-H explains it like feral itself, once `feral completion install` has set up the shell.

## Usage

```
feral link <subcommand> [args...]
```

## Subcommands

| Command | Description |
|---------|-------------|
| [add](link-add.md) | Add a name that runs feral with arguments. |
| [list (ls)](link-list.md) | List link names and what each one runs. |
| [rm](link-rm.md) | Remove a link and its symlink. |

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

- `feral link add mt -- -A fastmail tui` — `mt` opens the TUI on fastmail
- `feral link list`
- `feral link rm mt`

## Aliases

An account's aliases[] used to select it when feral was started under that name. The first run that may write turns each such alias into a link of the same name; after that an alias is only a name -A accepts. A symlink named after an account that is not a link is refused rather than run against the default account.

---

[↑ feral](index.md) — [nav](nav.md)
