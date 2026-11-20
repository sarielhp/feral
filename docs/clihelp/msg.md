---
title: 'feral msg'
has_children: true
---

# feral msg

Use msg show to read a message or folder, msg send for scripts and pipes, and msg reply to continue a thread. msg drafts manages saved work, msg last lists recent mail, and msg export saves messages or threads. Write new mail with write. Archive and block operate on message IDs; label archive acts on a whole folder.

## Usage

```
feral msg <subcommand> [args...]
```

## Subcommands

| Command | Description |
|---------|-------------|
| [show](msg-show.md) | Show emails by label prefix or message ID. |
| [send](msg-send.md) | Send an email directly or from stdin. |
| [reply](msg-reply.md) | Reply to an email preserving thread headers. |
| [drafts](msg-drafts.md) | List or remove locally saved drafts. |
| [last](msg-last.md) | Show recent emails across all folders. |
| [export](msg-export.md) | Export emails or threads to mbox or eml. |
| [pipe](msg-pipe.md) | Pipe message raw RFC822 or body to command. |
| [outbox](msg-outbox.md) | Manage offline outbound email queue. |
| [archive](msg-archive.md) | Archive one or more messages by ID. |
| [block](msg-block.md) | Block the senders of messages and move them. |
| [snooze](msg-snooze.md) | Hide messages until a time, then return them. |
| [unsub](msg-unsub.md) | Leave the mailing list a message came from. |
| [mute](msg-mute.md) | Archive a thread's new messages on arrival. |
| [unmute](msg-unmute.md) | Let a muted thread reach the inbox again. |
| [muted](msg-muted.md) | List the account's muted threads. |

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

- `feral msg show abc123de` — Read a message
- `feral msg last 20` — List recent messages across folders
- `feral msg archive abc123de` — Archive one message by ID
- `feral msg block abc123de` — Block sender of message to Spam
- `feral msg unsub abc123de` — Leave the mailing list of a message

---

[↑ feral](index.md) — [nav](nav.md)
