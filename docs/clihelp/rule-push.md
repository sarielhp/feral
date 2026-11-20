---
title: 'feral rule push'
parent: 'feral rule'
---

# feral rule push

Export local rules to mail server filters (e.g. Gmail filters or Sieve script).

## Usage

```
feral rule push [force] [flags]
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `[force]` | Overwrite existing remote filters. |

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
| `--sieve <file>` | Export rules as a Sieve script to file path |
| `-f, --force` | Force overwrite conflicting remote filters |

## Examples

- `feral rule push`
- `feral rule push --sieve ~/rules.sieve`

---

[↑ feral](index.md) — [nav](nav.md)
