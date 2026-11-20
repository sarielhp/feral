---
title: 'feral config llm test'
parent: 'feral config llm'
---

# feral config llm test

Send a harmless sample message to the configured chat endpoint and report whether the reply carries the urgency and TL;DR fields the summarizer parses. Use it to confirm an endpoint, model or reasoning-effort change before scanning real mail.

## Usage

```
feral config llm test
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

- `feral config llm test` — Check the endpoint, model and answer format

---

[↑ feral](index.md) — [nav](nav.md)
