---
title: 'feral contacts pick'
parent: 'feral contacts'
---

# feral contacts pick

Open a full-screen contacts picker on the terminal and print the chosen contacts on stdout, for other programs to use (a calendar invite, a script). The list is ranked as Tab-completion ranks it: stored contacts and aliases, then the addresses harvested from mail. Space marks, Enter chooses the marked contacts (or the one under the cursor), Esc cancels. The picker draws on /dev/tty, so stdin and stdout may be pipes. Exit status: 0 with the choice on stdout, 130 when cancelled or nothing was chosen (no output), 2 for a bad command line, 1 for any other failure.

## Usage

```
feral contacts pick [flags]
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
| `-f, --format <fmt>` | Output: rfc (Name <addr>), addr, or json. (default: rfc) |
| `-1, --single` | Choose one contact; no marking. |
| `-q, --query <text>` | Start with this search. |
| `-p, --preselect <addr>` | Start with these addresses marked (repeatable). |
| `-g, --expand-groups` | In json, list a group's members in its place. |
| `--title <text>` | Title shown above the list. |

## Examples

- `feral contacts pick` — Pick contacts; print Name <addr> per line
- `feral contacts pick --format=json` — Print the choice as JSON for a program
- `feral contacts pick -p "a@x.org,b@y.org" --title "Invitees"` — Edit an existing list of invitees
- `feral contacts pick -1 -f addr` — Choose one address

---

[↑ feral](index.md) — [nav](nav.md)
