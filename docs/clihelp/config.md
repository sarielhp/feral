---
title: 'feral config'
has_children: true
---

# feral config

Use config show, get, set, and unset for preferences. Run config check to validate configuration, or config llm test to check the configured model endpoint. Account credentials are managed under account; local maintenance lives under utils.

## Usage

```
feral config [subcommand]
```

## Subcommands

| Command | Description |
|---------|-------------|
| [show](config-show.md) | Display the active configuration values. |
| [get](config-get.md) | Print a configuration parameter's value. |
| [set](config-set.md) | Set a configuration parameter value. |
| [add-email](config-add-email.md) | Add addresses that are your own. |
| [remove-email](config-remove-email.md) | Remove addresses from my_emails. |
| [emails](config-emails.md) | List the addresses that count as yours. |
| [unset (reset)](config-unset.md) | Reset a configuration parameter to default. |
| [check (validate)](config-check.md) | Validate config, DNS, and Bogofilter. |
| [llm](config-llm.md) | Test the configured LLM endpoint. |

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

- `feral config show`
- `feral config get mutt_alias_file`
- `feral config set mutt_alias_file ~/.mutt/aliases`
- `feral config set spam_folder Junk`
- `feral config unset score_threshold`
- `feral config check`

---

[↑ feral](index.md) — [nav](nav.md)
