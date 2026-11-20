---
title: 'feral account rename'
parent: 'feral account'
---

# feral account rename

Change an account's name -- what -A takes, what %name:label means, and what its files are named after. The account's settings directory, its OAuth token, its cache, the watcher's record of it and any queued outgoing message move to the new name, and default_account, default_send_account and every link that runs it follow it, so nothing is downloaded again and no sign-in is needed. A name is letters, digits, '-' and '_', starting with a letter; "default" and "all" are reserved. Only the name shown in listings is changed by `account display-name`.

## Usage

```
feral account rename <account> <new_name>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<account>` | The account to rename. |
| `<new_name>` | Its new name. |

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

- `feral --dry-run account rename default personal` — List what would move
- `feral account rename default personal`

## Interrupted

The rename writes account_rename.json beside config.json while it runs, and every other command refuses to start until it is gone. Running the same rename again finishes it.

## Other machines

Each machine keeps its own configuration: rename the account on each one. --dry-run lists every file it would move.

## Watchers

Refused while `feral watch` or a TUI's watcher runs; close them first.

---

[↑ feral](index.md) — [nav](nav.md)
