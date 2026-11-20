---
title: 'feral msg show'
parent: 'feral msg'
---

# feral msg show

Show emails by label prefix or message ID.

## Usage

```
feral msg show <lbl_prefix|message-id> [message-id] [flags]
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<lbl_prefix\|message-id>` | Folder prefix, message ID, or virtual/<name> |
| `[message-id]` | Message ID or prefix within the folder |

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
| `-w, --web` | Open the HTML body of the email in your configured browser |
| `--raw` | Output verbatim unformatted RFC822 message to stdout |

## Examples

- `feral msg show receipts`
- `feral msg show receipts 1234abcd`
- `feral msg show receipts 1234abcd --web`
- `feral msg show 1234abcd --raw` — Output raw RFC822 message for piping
- `feral msg show 19f3a8216d52dfc6` — Show a message by ID alone, downloading it if needed
- `feral msg show virtual/last` — List a saved result set from the local cache

## Saved searches

A search, `last`, or a contact's latest mail saves its result set under the account that produced it, named after the query. `msg show virtual/<name>` lists one from the local cache -- no such folder exists on the server -- and F7 in the TUI browses them all.

## Which account

A folder is looked for in the default account (`account default`) unless the reference names one -- `%work:inbox`, or -A -- and in every account with `%all:inbox`. A message ID alone is looked for in every account.

## JSON output

With --json a folder listing is one JSON array with an element per matched folder in each account covered -- {account, folder, message_ids, total} -- even when only one folder matches, so the output always parses. A message shown by ID is a single JSON object.

---

[↑ feral](index.md) — [nav](nav.md)
