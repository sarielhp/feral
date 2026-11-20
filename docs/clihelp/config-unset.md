---
title: 'feral config unset'
parent: 'feral config'
---

# feral config unset

Reset a configuration parameter to default.

## Usage

```
feral config unset <key>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<key>` | The configuration parameter name to reset. |

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

- `feral config unset score_threshold`
- `feral config unset mutt_alias_file`

---

[↑ feral](index.md) — [nav](nav.md)
