---
title: 'feral account link'
parent: 'feral account'
---

# feral account link

Add an alias to an account: a name -A accepts for it, besides its name and display name. An alias used to select its account when feral was started under that name; that is now a link's job (`feral link add <name> -- -A <account>`), and the first run that may write turns every alias into a link once.

## Usage

```
feral account link <account_name> <program_name>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<account_name>` | The existing account name to associate with. |
| `<program_name>` | The alias: another name for the account. |

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

- `feral account link work-account work_mail`

---

[↑ feral](index.md) — [nav](nav.md)
