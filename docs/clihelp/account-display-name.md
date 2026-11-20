---
title: 'feral account display-name'
parent: 'feral account'
---

# feral account display-name

Set the label an account carries in listings, prompts and the TUI. It is also accepted wherever an account is named, so it may not be another account's name or display name. It changes no file; `account rename` changes the name itself.

## Usage

```
feral account display-name <account> <display_name>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<account>` | The account. |
| `<display_name>` | The name to show it under. |

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

- `feral account display-name work "Work Mail"`

---

[↑ feral](index.md) — [nav](nav.md)
