---
title: 'feral spam sweep'
parent: 'feral spam'
---

# feral spam sweep

Empty the Spam folder in four steps. Political fundraising mail is unsubscribed from, and its senders are blacklisted. On Gmail every blacklisted political sender -- including those from earlier sweeps -- is also blocked with a server filter that keeps their mail out of the inbox. Each message that was unsubscribed from is then deleted permanently on Gmail and JMAP (Outlook leaves it in place). The spam filter is trained on everything in Spam, those senders are dropped from the contacts history, and what remains in Spam is moved to the spam_learn folder (LearnSpam by default), not deleted. To delete Spam for good use spam empty.

## Usage

```
feral spam sweep
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

- `feral spam sweep` — Train on Spam and move it to LearnSpam

---

[↑ feral](index.md) — [nav](nav.md)
