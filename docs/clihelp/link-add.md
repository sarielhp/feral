---
title: 'feral link add'
parent: 'feral link'
---

# feral link add

Record a link in config.json and put a symlink of that name beside the feral binary (or in --dir). The arguments come after `--`, so that their flags are the link's and not this command's; with no flags among them the `--` may be left out. They are stored as they will run: the account named by -A, a display name or -N is stored by its name, and a command must be spelled in full.

## Usage

```
feral link add <name> [--description T] [--force] [--dir D] -- <args...>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<name>` | The name to type: letters, digits, - _ . or +. |
| `[args...]` | What it runs, as typed after feral. |

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
| `--description <text>` | What the link is for, for link list. |
| `--force` | Replace a link or file of the same name. |
| `--dir <dir>` | Put the symlink here, not beside feral. |

## Examples

- `feral link add mt -- -A fastmail tui`
- `feral link add work -- -A work`
- `feral link add mail_cli` — Make the old name complete

## Symlink

The symlink points at the running binary. An existing symlink to it is kept; any other file of that name -- a wrapper script, a symlink elsewhere -- is replaced only with --force, and a directory never. A directory not on $PATH gets the link and a warning.

## Dry run

--dry-run says what it would do and writes nothing.

---

[↑ feral](index.md) — [nav](nav.md)
