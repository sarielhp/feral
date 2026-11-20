---
title: 'feral config set'
parent: 'feral config'
---

# feral config set

Set a configuration parameter to a new value. Keys that belong to an account are written to the account -A selects.

## Usage

```
feral config set <key> <value>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<key>` | The configuration parameter name. |
| `<value>` | The value to set for the parameter. |

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

- `feral config set spam_folder Junk`
- `feral config set limit 50`
- `feral -A work config set badge 💼` — Tag the work account in All Inboxes
- `feral config set mutt_alias_file ~/.mutt/aliases`
- `feral config set llm_endpoint http://localhost:4000/v1/chat/completions`
- `feral config set llm_api_key sk-litellm-master-key`
- `feral config set llm_auto_summary true`
- `feral config set attachment_save_dir ~/info/misc/%YY/%MM/%DD/`
- `feral config set save_locations ~/Downloads,~/info/misc,~` — Jumps offered by the save dialog

---

[↑ feral](index.md) — [nav](nav.md)
