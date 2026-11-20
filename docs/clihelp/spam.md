---
title: 'feral spam'
has_children: true
---

# feral spam

Manage Spam folder, train filters, and unsubscribe from political mail.

## Usage

```
feral spam <subcommand>
feral spam <message_id...>
```

## Subcommands

| Command | Description |
|---------|-------------|
| [empty (del, delete)](spam-empty.md) | Permanently purge all emails in Spam. |
| [sweep (bye)](spam-sweep.md) | Unsubscribe, train, file Spam to LearnSpam. |
| [learn](spam-learn.md) | Train Bogofilter on Spam messages. |
| [pol](spam-pol.md) | Manage political spam and opt-outs. |
| [allow](spam-allow.md) | Manage sender allowlist to bypass spam. |
| [block](spam-block.md) | Manage sender blocklist to classify as spam. |
| [unmark](spam-unmark.md) | Mark email as ham and return to Inbox. |
| [reset](spam-reset.md) | Discard local spam scores for account. |
| [mark](spam-mark.md) | Mark messages as spam and move to Spam. |
| [forget](spam-forget.md) | Let the filter judge messages again. |

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<message_id...>` | One or more message IDs to mark as spam. |

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

- `feral spam empty --yes` — Permanently purge the Spam folder
- `feral spam sweep` — Train on Spam and move it to LearnSpam
- `feral spam learn` — Train the filter on everything in the Spam folder
- `feral spam learn --ham %received` — Train on a folder of known-good mail
- `feral spam unmark abc123de` — This one was not spam: train as ham and move it back
- `feral spam allow add friend@example.com` — Never treat this sender as spam
- `feral spam block add spammer@example.com` — Always treat this sender as spam
- `feral spam pol audit` — Report political mail in Spam, changing nothing
- `feral spam pol unsub` — Send opt-outs and delete what was unsubscribed from
- `feral spam mark abc123de` — Mark one message as spam

---

[↑ feral](index.md) — [nav](nav.md)
