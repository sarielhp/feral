---
title: 'feral account signature'
parent: 'feral account'
---

# feral account signature

An account's signature goes under what it sends -- `write`, `msg reply`, `msg send` and the TUI's compose, reply and forward -- below a line holding "-- ", and above the quoted text of a reply. It is part of the body, so a PGP-signed or encrypted message covers it. Without an option the signature is printed. It is kept as signature.txt in the account's directory beside config.json, so `account rename` takes it along.

## Usage

```
feral account signature <account> [--file <path> | --edit | --clear]
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<account>` | The account. |

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
| `--file <path>` | Set the signature from a file (- reads stdin). |
| `--edit` | Edit the signature in the configured editor. |
| `--clear` | Sign nothing from this account. |

## Examples

- `feral account signature work` — Print the signature
- `feral account signature work --file ~/sig-work.txt`
- `feral account signature work --edit`
- `feral account signature work --clear`

## Not signed

`msg send --raw` sends the message as it is piped in. A body that already has a "-- " line -- a resumed draft, or text written with a signature of its own -- is not signed a second time.

## ~/.signature

An account with no signature of its own is signed in the TUI with ~/.signature, if there is one, as before signatures were per account. --clear stops that too.

## Modes

--dry-run refuses to set one; --read-only does not, since a signature changes nothing on the server.

---

[↑ feral](index.md) — [nav](nav.md)
