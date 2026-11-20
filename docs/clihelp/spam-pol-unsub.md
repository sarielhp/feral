---
title: 'feral spam pol unsub'
parent: 'feral spam pol'
---

# feral spam pol unsub

Scan the Spam folder for political messages, send their opt-outs, and blacklist the senders that are safe to blacklist -- a list mailer on a non-public domain, scored well past the threshold. On Gmail every blacklisted political sender, including those from earlier runs, is also blocked with a server filter that files their mail under the spam_learn label, skipping the inbox; a sender some filter already names is left alone. Each message unsubscribed from is deleted permanently on Gmail and JMAP.

## Usage

```
feral spam pol unsub
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

- `feral spam pol unsub` — Send opt-outs and delete what was unsubscribed from

---

[↑ feral](index.md) — [nav](nav.md)
