---
title: 'feral spam unmark'
has_children: true
parent: 'feral spam'
---

# feral spam unmark

Mark a message as not being spam: train Bogofilter as ham and move it from Spam back to Inbox on the server.

## Usage

```
feral spam unmark <message_id...>
  feral unspam folder <folder_name>
```

## Subcommands

| Command | Description |
|---------|-------------|
| [folder](spam-unmark-folder.md) | Mark all messages in folder as ham. |

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<message_id...>` | One or more message IDs to unspam (short 8-char or full). |

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

- `feral spam unmark abc123de`
- `feral spam unmark folder %spam`

---

[↑ feral](index.md) — [nav](nav.md)
