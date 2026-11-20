---
title: 'feral account inbox-max-age'
parent: 'feral account'
---

# feral account inbox-max-age

Set how long mail may sit in the account's inbox before feral reports that whatever empties it has stopped. Use it for an account whose mail a fetcher moves elsewhere every few minutes. `off` removes the limit. See `account stale`.

## Usage

```
feral account inbox-max-age <account_name> <duration>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<account_name>` | The relay account. |
| `<duration>` | A duration of at least 1m, such as 15m, or off. |

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

- `feral account inbox-max-age relay 15m`

---

[↑ feral](index.md) — [nav](nav.md)
