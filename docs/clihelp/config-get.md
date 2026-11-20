---
title: 'feral config get'
parent: 'feral config'
---

# feral config get

Print a configuration parameter's value.

## Usage

```
feral config get <key>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<key>` | The configuration parameter name. |

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

- `feral config get mutt_alias_file`
- `feral config get browser`
- `feral config get download_dir`

---

[↑ feral](index.md) — [nav](nav.md)
