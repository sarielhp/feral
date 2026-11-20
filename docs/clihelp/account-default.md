---
title: 'feral account default'
parent: 'feral account'
---

# feral account default

The default account is the one a command acts on when no account is named -- by -A, by a numeric shortcut such as -2, or by a link's preset (`feral link`). It is stored as default_account in config.json; with none set, the first account is the default. A folder reference that names no account -- `feral %`, `scan %inbox`, `msg show %inbox` -- is read there; `%all:inbox` reads every account. `watch` still covers every account.

## Usage

```
feral account default [account]
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `[account]` | The account to make the default. |

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

- `feral account default` — Name the default account
- `feral account default fastmail`

## Clearing

`feral config unset default_account` returns to the first account.

---

[↑ feral](index.md) — [nav](nav.md)
