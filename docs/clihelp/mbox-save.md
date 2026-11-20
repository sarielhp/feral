---
title: 'feral mbox save'
parent: 'feral mbox'
---

# feral mbox save

Download a folder and append its messages to a local mbox file, creating the file when it does not exist. The file is appended to, never replaced, so repeated runs accumulate into one corpus; `mbox merge` is what deduplicates. The folder is named by a label prefix, exactly as `msg show` names it: in the default account unless the prefix names one (`%work:inbox`), and in every account with `%all:`.

## Usage

```
feral mbox save <path> <lbl_prefix>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<path>` | Destination mbox file (created if missing) |
| `<lbl_prefix>` | Folder or label prefix to download |

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

- `feral mbox save ~/archive/keep.mbox %keep`
- `feral mbox save backup.mbox INBOX`

---

[↑ feral](index.md) — [nav](nav.md)
