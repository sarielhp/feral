---
title: 'feral account stale'
parent: 'feral account'
---

# feral account stale

A relay account is one whose inbox something else empties on a schedule -- a fetcher that moves its mail into another account every few minutes. Set its limit with `account inbox-max-age`; mail older than that still in its inbox means the fetcher has stopped. This asks each relay's server, reading arrival times only, and exits non-zero when any inbox is stale or could not be checked, so it can run from cron. The TUI and `watch` make the same check every couple of minutes and say so on screen and on the desktop.

## Usage

```
feral account stale [account_name]
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `[account_name]` | Relay to check (defaults to every account with a limit). |

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

- `feral account stale`
- `feral account stale relay || notify-send 'mail relay stalled'`

---

[↑ feral](index.md) — [nav](nav.md)
