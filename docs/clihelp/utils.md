---
title: 'feral utils'
has_children: true
---

# feral utils

Use utils cache for local download maintenance, utils color to inspect terminal colors, utils keys to start a keys.json that moves the TUI's keys, and utils completion or utils manpage for shell integration. utils doctor verifies server writes by creating and deleting a temporary folder and message; it changes the real account and refuses --read-only and --dry-run.

## Usage

```
feral utils <subcommand>
```

## Subcommands

| Command | Description |
|---------|-------------|
| [cache](utils-cache.md) | Manage the local email download cache. |
| [color](utils-color.md) | Test the terminal's true-color support. |
| [doctor](utils-doctor.md) | Diagnose credentials, connectivity, mail flow. |
| [keys](utils-keys.md) | Print the default TUI keys as keys.json. |
| [completion](utils-completion.md) | Generate or install shell tab-completion scripts |
| [manpage](utils-manpage.md) | Print the manual page, or install it for man(1) |

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

- `feral utils cache prune 30` — Prune old local cached messages
- `feral utils completion install` — Install shell completion
- `feral utils manpage` — Print the manual page

---

[↑ feral](index.md) — [nav](nav.md)
