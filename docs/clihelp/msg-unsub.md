---
title: 'feral msg unsub'
parent: 'feral msg'
---

# feral msg unsub

Reads the message's List-Unsubscribe header and takes the first of these it offers: a one-click unsubscribe (RFC 8058: List-Unsubscribe-Post with an https link) is a POST through the guarded client that refuses loopback, private and metadata addresses; a mailto: is a short message from the account the mail arrived in, shown in full first; a plain web link is opened in your browser, never fetched by feral. It always shows what it will do and asks first. --read-only and --dry-run show it and send nothing. Each attempt is recorded in unsubscriptions.json beside config.json.

## Usage

```
feral msg unsub [flags] <message-id>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<message-id>` | The message's ID (short 8-character or full). |

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
| `-y, --yes` | Proceed without the confirmation prompt. |

## Examples

- `feral msg unsub abc123de` — Show how to leave the list, then ask
- `feral --dry-run msg unsub abc123de` — Only show what it would do

---

[↑ feral](index.md) — [nav](nav.md)
