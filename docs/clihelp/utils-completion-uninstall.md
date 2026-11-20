---
title: 'feral utils completion uninstall'
parent: 'feral utils completion'
---

# feral utils completion uninstall

Remove the installed tab completion and key binding

## Usage

```
feral utils completion uninstall [<shell>]
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `[<shell>]` | Shell type ('bash', 'zsh', or 'fish'; defaults to current shell) |

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

---

[↑ feral](index.md) — [nav](nav.md)
