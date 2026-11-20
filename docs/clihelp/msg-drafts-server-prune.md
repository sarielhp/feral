---
title: 'feral msg drafts server prune'
parent: 'feral msg drafts server'
---

# feral msg drafts server prune

Trash superseded and already-sent drafts.

## Usage

```
feral msg drafts server prune [--sent] [--by-subject]
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
| `--sent` | Also trash drafts whose message is in the Sent folder. |
| `--by-subject` | Match by recipients and subject when a draft has no Message-ID. |

---

[↑ feral](index.md) — [nav](nav.md)
