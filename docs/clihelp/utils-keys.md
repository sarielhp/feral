---
title: 'feral utils keys'
parent: 'feral utils'
---

# feral utils keys

The TUI reads keys.json from the configuration directory, beside config.json, when it starts. It maps action names to keys for two groups of screens: list (the message list) and message (an open message). An action left out keeps its default; one that is named runs on the keys given, a key or a list of keys, and on no other. The TUI refuses to start on an unknown group or action, a key that is not a key, a key two actions of one group would share, or a key the screens keep for themselves -- q, Esc, Tab, f, F, h, ?, Enter, the arrows, PgUp, PgDn, Home, End, Shift+← and Shift+→, F1-F4, F8, F9, Ctrl+C, Ctrl+D, Ctrl+Q and every Alt chord -- and names the line. The help (F1) shows the keys in effect. This command prints every action with its default keys and writes nothing.

## Usage

```
feral utils keys
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

- `feral utils keys > ~/.config/feral/keys.json` — Start a keys file to edit

---

[↑ feral](index.md) — [nav](nav.md)
