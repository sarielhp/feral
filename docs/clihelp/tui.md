---
title: 'feral tui'
has_children: true
---

# feral tui

Open the interactive terminal email browser. With an optional label_prefix argument, open the TUI with the matching label as the initial folder. The prefix is matched case-insensitively as a substring against the full label path. If exactly one label matches, the TUI opens on that label. If multiple match, all matching labels are printed and the program exits. Given an existing .eml file instead, open that one message read-only, as mbox view does. Use tui write to compose a message; it accepts the same arguments and flags as write, including --resume.

## Usage

```
feral tui [label_prefix]
  feral tui <file.eml>
  feral tui write [flags] [recipient...]
```

## Subcommands

| Command | Description |
|---------|-------------|
| [write](tui-write.md) | Write a message in your editor, then review. |

## Parameters

| Parameter | Description |
|-----------|-------------|
| `[label_prefix]` | Folder/label prefix to open into, or a .eml file to view. |

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
| `--no-watch` | Do not watch for new mail while open |

## Examples

- `feral tui`
- `feral tui %receipts`
- `feral tui saved.eml` — View one saved message, read-only
- `feral tui write boss@example.com -s Update` — Compose through a wrapper that opens the TUI

## New-mail notifications

While the TUI is open it runs the same watcher as feral watch, over every account's inbox once a minute, and shows a desktop notification when new mail arrives there; it stops when the TUI closes. A check is one small request per account. Its messages go to the diagnostics panel (Ctrl+D). Unlike feral watch it never sends the offline outbox. Only one watcher runs per download directory, so a second TUI, or a TUI beside feral watch, leaves the watching to the one already running. Turn it off with --no-watch, or for good with "tui_watch": false in config.json.

---

[↑ feral](index.md) — [nav](nav.md)
