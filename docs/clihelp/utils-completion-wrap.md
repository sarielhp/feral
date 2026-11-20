---
title: 'feral utils completion wrap'
parent: 'feral utils completion'
---

# feral utils completion wrap

Generate a wrapper script with preset arguments

## Usage

```
feral utils completion wrap [--completion-from <path>] <name> [--] [<args>...]
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<name>` | Name of the wrapper script |
| `[<args>...]` | Preset arguments prepended to wrapped command |

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
| `--completion-from <path>` | Inspect an existing wrapper to extract its arguments |

## Examples

- `feral utils completion wrap pd deploy` — Generate wrapper 'pd' for 'feral deploy'
- `feral utils completion wrap --completion-from ~/bin/mt` — Inspect existing script and generate wrapper

---

[↑ feral](index.md) — [nav](nav.md)
