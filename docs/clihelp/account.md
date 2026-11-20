---
title: 'feral account'
has_children: true
---

# feral account

Start with account setup for guided onboarding, or account new to add an account explicitly. Use account login to renew credentials and account migrate to copy configuration to another machine over SSH.

## Usage

```
feral account <subcommand> [args...]
```

## Subcommands

| Command | Description |
|---------|-------------|
| [list](account-list.md) | List configured accounts. |
| [add (new)](account-add.md) | Add a new account template to config.json. |
| [link (associate)](account-link.md) | Add a name -A accepts for an account. |
| [test](account-test.md) | Check an account's config and connection. |
| [calendar (cal)](account-calendar.md) | Show or set the calendar manager account. |
| [login](account-login.md) | OAuth login for Gmail or Outlook. |
| [rename](account-rename.md) | Rename an account and its files. |
| [display-name](account-display-name.md) | Set the name an account is shown under. |
| [signature](account-signature.md) | Show or set an account's signature. |
| [default](account-default.md) | Show or set the default account. |
| [user-name](account-user-name.md) | Set the name shown in outgoing mail. |
| [user-email](account-user-email.md) | Set the address outgoing mail is sent from. |
| [stale](account-stale.md) | Check relay inboxes for mail left too long. |
| [inbox-max-age](account-inbox-max-age.md) | Mark an account as a relay to watch. |
| [rm (delete, del)](account-rm.md) | Delete an account and its credentials. |
| [setup](account-setup.md) | Configure an email account interactively. |
| [migrate](account-migrate.md) | Copy config and credentials via SSH. |

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

- `feral account list`
- `feral account new personal-gmail`
- `feral account test`
- `feral account rename old-gmail new-gmail`
- `feral account rm temp-account`

---

[↑ feral](index.md) — [nav](nav.md)
