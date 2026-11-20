---
title: 'feral utils doctor run'
parent: 'feral utils doctor'
---

# feral utils doctor run

Execute connection and integration tests.

## Usage

```
feral utils doctor run
```

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

- `feral utils doctor run`

---

[↑ feral](index.md) — [nav](nav.md)
