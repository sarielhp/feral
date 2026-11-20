---
title: 'feral utils cache outlook-ids'
parent: 'feral utils cache'
---

# feral utils cache outlook-ids

Outlook gives a message a new ID whenever it moves to another folder, so archiving or filing one orphaned what feral keeps by ID: the replied mark, your spam/not-spam decision, the cached copy. Graph also offers IDs that survive a move. A new Outlook account uses them from the start; this command moves an existing account's cache over. It asks the server to translate every cached ID, builds a new cache beside the old one -- message copies with your decisions and LLM summaries, read, replied and pending marks, training ledgers, saved result sets -- and swaps it in. The old cache is kept, renamed with a .pre-immutable-<time> suffix, and can be deleted once the account looks right. Folder indexes are rebuilt by the next listing. It changes only local files -- the server is only asked to translate. Close every feral TUI and `feral watch` first: it refuses while a watcher runs, but a TUI started with --no-watch holds no lock, and would go on writing the old IDs into the new cache.

## Usage

```
feral -A <account> utils cache outlook-ids
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

- `feral -A work utils cache outlook-ids` — Migrate the account named work

## Safety

Refused under --dry-run and --read-only. If the server translates none of the IDs, nothing is changed.

## Interrupted runs

Run it again: a finished new cache is swapped in, an unfinished one is rebuilt.

## Older builds

A feral from before this command does not know the new IDs and writes the old ones into a migrated cache. Install this build everywhere the account is used before migrating it.

## Undoing it

Move the account's cache directory away and rename <account>.pre-immutable-<time> back to <account>.

---

[↑ feral](index.md) — [nav](nav.md)
