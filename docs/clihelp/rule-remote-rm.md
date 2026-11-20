---
title: 'feral rule remote rm'
parent: 'feral rule remote'
---

# feral rule remote rm

Delete a server filter and its local rule.

## Usage

```
feral rule remote rm <target> [flags]
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<target>` | The filter index, filter ID, or sender/subject to delete. |

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
| `-y, --yes` | Proceed without the confirmation prompt. |
| `--keep-local` | Keep matching local rule in config.json (mark as unexported). |

## Examples

- `feral rule remote rm 1`
- `feral rule remote rm AN63b9f`
- `feral rule remote rm alerts@github.com`

---

[↑ feral](index.md) — [nav](nav.md)
