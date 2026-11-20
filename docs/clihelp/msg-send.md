---
title: 'feral msg send'
parent: 'feral msg'
---

# feral msg send

Send an email via configured account or standard input pipe, with optional attachments and Cc/Bcc recipients. Without -s, and with an LLM endpoint configured, the subject line is written by the model from the body and the attachment names and printed before the send; set llm_auto_subject to false to keep sending without one. The account's signature (`account signature`) is appended under a "-- " line unless the body already has such a line or --no-signature is given; a --raw message is sent as it is.

## Usage

```
feral msg send [flags] <recipient...>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<recipient...>` | One or more recipient email addresses or contact aliases. |

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
| `-s, --subject <text>` | Email subject line. |
| `-c, --cc <address>` | Carbon copy (Cc) recipient address (comma-separated). |
| `-b, --bcc <address>` | Bcc recipient (comma-separated). |
| `-a, --attach <path>` | Attach files; repeat, comma-separate, or quote a glob. |
| `-i, --include <path>` | Path to file containing email body. |
| `--body <text>` | Inline email body text. |
| `-t, --raw` | Read a full RFC822 message from stdin. |
| `-q, --queue` | Queue message in outbox without sending. |
| `--at <time>` | Queue it to send at a time: +2h, tomorrow 8am. |
| `--no-signature` | Send without the account's signature. |
| `--sign` | PGP-sign the message body. |
| `--encrypt` | PGP-encrypt; fails unless all recipients have keys. |
| `--pgp-key <id>` | Key to sign with (default: account key, else gpg's). |

## Examples

- `feral msg send -s "Meeting notes" user@example.com < notes.txt` — Send email with body from stdin pipe
- `feral msg send -s "Quick update" --body "Everything on track" boss@company.com` — Send quick email with inline body
- `feral msg send -s "Report" -a report.pdf -c team@company.com boss < body.txt` — Send with attachment, CC, and alias
- `feral msg send -s "Dry run test" --dry-run user@example.com < body.txt` — Preview RFC822 message without sending
- `feral msg send -a report.pdf boss` — Send a file with "See attachment." as the body and an LLM-written subject
- `feral msg send --raw < patch.eml` — Send pre-formatted RFC822 message directly (git send-email mode)

## Sending later (--at)

With --at the message is built now, queued in the outbox with a due time, and sent by the first outbox flush at or after it: feral watch flushes after every clean poll, or run msg outbox flush from cron. The TUI never flushes, so with neither running the message waits; msg outbox ls shows when each one is due. Times are local: "2026-11-20 08:00", "+2h", "+30m", "+1d", "today 17:00", "tomorrow 8am" -- as in feral msg send --at "tomorrow 8am" -s Status boss < status.txt. A time already past is refused. Like --queue, --at is refused under --read-only.

---

[↑ feral](index.md) — [nav](nav.md)
