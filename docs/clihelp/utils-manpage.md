---
title: 'feral utils manpage'
parent: 'feral utils'
---

# feral utils manpage

Print the manual page, or install it for man(1)

## Usage

```
feral utils manpage [--man-install|--man-uninstall] [--man-force]
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
| `--man-install` | Install it under the user's data directory |
| `--man-uninstall` | Remove a manual page installed this way |
| `--man-force` | Install even though another page exists |

## Examples

- `feral utils manpage > feral.1` — Write the page for a package to install
- `feral utils manpage --man-install` — Install it for this user

## Installing

Installation refuses when a manual page for this program already exists elsewhere, because which of two pages man(1) shows is not predictable and the one that loses is invisible. --man-force installs anyway.

---

[↑ feral](index.md) — [nav](nav.md)
