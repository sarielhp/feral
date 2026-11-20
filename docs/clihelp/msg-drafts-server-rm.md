---
title: 'feral msg drafts server rm'
parent: 'feral msg drafts server'
---

# feral msg drafts server rm

'all' moves every draft in the Drafts folder to the trash, unreadable ones included, and lists each one it moves. The trash keeps them until the server expires it, so nothing is lost at once; run with --dry-run to see the list first.

## Usage

```
feral msg drafts server rm <id>... | all
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

---

[↑ feral](index.md) — [nav](nav.md)
