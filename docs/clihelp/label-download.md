---
title: 'feral label download'
parent: 'feral label'
---

# feral label download

Downloads a folder into an mbox file. Safe to repeat: if the file already exists, only messages it does not yet contain are appended, and a run with nothing new leaves it untouched. Messages are matched by Message-ID (Subject and Date when there is none). The folder is also filled into the local cache. Use -d to see what would be added, or -w to replace the file instead.

## Usage

```
feral label download <label_or_prefix> <destination> [flags]
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<label_or_prefix>` | Label, or hierarchy prefix, to download. |
| `<destination>` | The local destination file (or directory if using -r). |

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
| `-r, --recursive` | Export sublabels into a mirrored tree |
| `-d` | Preview; create or change no files |
| `-w, --overwrite` | Overwrite files instead of appending |

## Examples

- `feral label download receipts ~/archive/receipts` — Create the mbox, or append what is missing
- `feral label download -d receipts ~/archive/receipts`
- `feral label download -r research ~/archive/research/`
- `feral label download -r -d FastMail:research ~/archive/research/`

---

[↑ feral](index.md) — [nav](nav.md)
