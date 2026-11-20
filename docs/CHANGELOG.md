# 📝 Changelog

All notable changes to feral (called `mail_cli` until 0.7.77) are documented in
this file.

> **This file was not maintained between 0.2.20 and 0.7.0.** The entries below
> resume it. The gap is real and is not reconstructed here; `git log` is the
> record for that stretch.

## [Unreleased]

### Development

- **The test opt-in variables are `FERAL_*`**: `FERAL_LIVE`, `FERAL_INTERACTIVE`,
  `FERAL_LIVE_MUTATE`, `FERAL_MUTATE_ACCOUNT`, `FERAL_LIVE_ACCOUNT`, `FERAL_LIVE_SEND`,
  `FERAL_LIVE_MONKEY` and the rest; the Makefile targets set them. A run that sets
  only a `MAIL_CLI_*` name fails and names the new one, rather than skipping. The
  `--on-mail` hook still exports `MAIL_CLI_*` beside `FERAL_*`.
- **The repositories are `sarielhp/feral` and `sarielhp/feral_dev`**; GitHub
  redirects the old `mail_cli` names.

### Undo in the TUI

- **`z` undoes the last move.** Archive, delete to trash, mark as spam and a
  move to a folder (`;s`, or `;a` within one account) -- one message or tagged
  -- report "Archived 3 — z to undo", and `z` moves the messages back to the
  folder they came from, on the server and in the folder index. The session
  keeps the last ten; nothing survives a restart. `u` was taken: it unspams.
- **Undo puts back what the action dropped.** Delete and spam remove the
  cached copy, and with it the user's spam override; undo downloads the
  message again and restores the override.
- **Undoing a spam mark takes back its training.** The mark registers the
  message with bogofilter until it scores as spam; undo unregisters it once
  per registration (`bogofilter -S`) and drops it from the trained-message
  record if the mark put it there. It is not trained as ham: that would be a
  verdict the user did not give. Tagged spam (`;S`) trains nothing, so there
  is nothing to take back.
- **What cannot be undone says so**, and stays on the stack so `z` does not
  silently reach past it: a sent message, a blocked sender (a rule change),
  the spam menu's move of every [SPAM] message, a move into another account,
  and any move in an Outlook account whose cache still keys messages by
  Graph's default IDs, which change on every move.
- **Gmail: moving archived mail back works without an "Archive" label.**
  Archive there removes the inbox label and adds nothing; the move back used
  to fail resolving "Archive" as its source.

### Leaving a mailing list

- **`msg unsub <id>`** leaves the list a message came from, using its
  `List-Unsubscribe` header: a one-click unsubscribe (RFC 8058) is a POST
  through the SSRF-guarded client, a `mailto:` is a short message sent from the
  account the mail arrived in and shown in full first, and a plain web link is
  opened in the browser, never fetched. It always shows the plan and asks
  (`--yes` skips the question); `--read-only` and `--dry-run` only show it.
  Every attempt goes into `unsubscriptions.json`, the log `spam pol unsub`
  already keeps.
- **`X` in the TUI** does the same for the message under the cursor or on
  screen: a dialog shows the plan (and a mailto message in full), `y` carries
  it out, `n` or Esc cancels. Under read-only the dialog says so and `y` only
  reports. (`U` was taken by unspam.)

### Mailing lists

- **A message from a list shows the list's tag** in the TUI's message list:
  `[linux-kernel]` ahead of the subject, from the message's `List-Id`
  (RFC 2919). A subject that already carries the tag is not tagged twice.
  Summaries cached before this release get their list from the message's
  header on first use, once.
- **Reply to the list**: `G` in the TUI, beside `r` and `g`, and
  `msg reply --list` (`-l`) address the list's `List-Post` (RFC 2369) alone.
  A message from no list, a list whose `List-Post` is `NO` and a list that
  takes posts only on the web are refused with a message saying which -- the
  reply never goes to the sender instead. `--list` and `--group` exclude each
  other.
- **Filter the list by mailing list**: `=` in the TUI shows only the selected
  message's list (again, or Esc, to show everything). It is the ordinary list
  filter with the query `list:<id>`, which can also be typed after `/`; it
  matches the `List-Id` alone, never a subject.
- **Mute a thread**: `M` in the TUI (again to unmute), `msg mute <id>`,
  `msg unmute <id|key>` and `msg muted`. A muted thread's new messages skip
  the inbox: the watcher -- `feral watch`, or the one the TUI runs --
  archives them as they arrive and does not announce them. A thread is
  recognized by the Message-ID at its root (the first `References` entry,
  else `In-Reply-To`, else the message's own), so a reply naming any message
  of the thread belongs to it, on every backend alike. The list is kept per
  account in `muted_threads.json` in the account's settings directory, which
  `account rename` moves. Muting is refused under `--read-only`, whose
  watcher could not archive; `--dry-run` reports what it would do. A muted
  message the watcher cannot archive -- the archive failed, the session is
  read-only or the account is, the list is damaged -- is announced like any
  other, never left
  in the inbox unannounced.

### The program is feral

- **The binary is `feral`.** `tools/install` installs `~/bin/feral` and turns
  `~/bin/mail_cli` into a symlink to it, so scripts and alias links that run
  `mail_cli` keep working and get the new build.
- **The configuration directory is `~/.config/feral`.** The first run that may
  write moves `~/.config/mail_cli` there and leaves a `mail_cli` symlink, so a
  build from before the rename still finds the accounts. `--read-only` and
  `--dry-run` read the old directory where it is. If both exist as real
  directories, feral refuses rather than choose.
- **Caches are not moved.** An existing `~/.cache/mail_cli` is still used when
  config.json names no `download_dir`.
- **`watch --on-mail` hooks** get `FERAL_ACCOUNT`, `FERAL_FROM`,
  `FERAL_SUBJECT`, `FERAL_DATE` and `FERAL_ID`, and the same values under the old
  `MAIL_CLI_*` names.
- **Account names.** `account rename` renames an account and everything kept
  under its name; `account display-name` sets only the label; `account default`
  chooses the account a command acts on when none is named. `default` and `all`
  are no longer valid account names.
- **A bare folder reference (`feral %`) reads the default account**, not every
  account; `%all:` still covers all of them.

### Send later

- **`msg send --at <time>`** builds the message now and queues it in the
  outbox to be sent at a local time: `"2026-11-20 08:00"`, `"+2h"`, `"+30m"`,
  `"+1d"`, `"today 17:00"` or `"tomorrow 8am"`. A time already past is refused,
  and so is `--at` under `--read-only`, as `--queue` is; so is a blank `--at`,
  which must not mean "now". It works with `--raw`. A message `send` builds is
  dated at its due time; a `--raw` message keeps its own `Date:`.
- **What sends it** is the first outbox flush at or after the due time: `feral
  watch` after each clean poll, or `msg outbox flush` run by hand or from cron.
  The TUI's own watcher never flushes, so with neither running the message waits.
- **`outbox ls` shows when each entry goes out**; `outbox flush` sends only what
  is due, says how many wait and when the next is due, and does not fail because
  something is scheduled; `--dry-run` names what is not due yet.
- **Older builds never send a scheduled message early.** A scheduled entry is
  format 2 and is kept in `outbox/scheduled/`, which builds before this one do
  not read; an entry to be sent now is written exactly as before. An entry in a
  newer format still is listed, refused by flush and never rewritten.

### Snooze

- **`msg snooze --until <time> <id>...`** hides messages until a time, read as
  `msg send --at` reads one. Each moves to a `Snoozed` folder on its own
  account, created the first time, and is recorded in `snooze.json` in the
  download directory.
- **`H` ("hide until") in the TUI**, on the list or an open message, asks until when and
  snoozes that message the same way; a time it cannot read leaves it in place.
- **A watcher brings it back**, to the inbox and unread, on its first poll at
  or after the time: `feral watch`, or the one the TUI runs while it is open.
  With neither running it waits; `msg snooze --wake` brings back what is due,
  or the messages named, at once.
- **`msg snooze`** lists what is snoozed and when each returns; `--forget`
  drops a record and moves nothing.
- **Nothing snoozed is lost silently.** A message deleted meanwhile, or one
  whose account is gone, keeps its record and is reported once by the watcher.
  A damaged `snooze.json` is an error, never an empty list. The record is
  written before the move and removed only after the move back.
- **Refused where it cannot be undone safely**: under `--read-only` (the record
  would describe a move that did not happen), on an mbox account, and on an
  Outlook account whose cache still holds default message IDs, which change on
  a move (`utils cache outlook-ids` converts it). `--dry-run` says what would
  move. `account rename` renames the account in the record.

### Configuration: groundwork for sync across machines

- **One writer for the shared configuration.** config.json and each account's
  `account.json`, `whitelist.json`, `blacklist.json` and `rules.json` are
  written in one place (`cfg_g.writeShared`), which `--dry-run` refuses for
  every caller, an account rename's in-place edit included. A test fails when
  new code writes one of them directly.
- **The shared files have one layout.** Keys sorted at every depth, one
  array element per line, a final newline, and the lists that are sets
  (`my_emails`, `allowed_languages`, white and black lists) sorted; rules,
  accounts and every other list keep their order. The same configuration is
  now the same bytes, whichever command wrote it, so a diff of config.json
  shows only what changed. The first save reorders an existing file once.
- **The configuration export has a format version** (`cfg_g.SyncFormat`,
  kept in `<sync_dir>/format`), with the check that will refuse to sync with
  an export a newer feral wrote. Nothing exports yet.
- **A save no longer reverts another process's change.** Two feral processes
  that loaded the configuration and both saved used to leave only the second
  one's copy: a TUI kept open across `account default` in another terminal
  put the old default back on its next save. A save now holds a lock on
  config.json, and a file that changed since it was loaded is merged: what
  this run did not change keeps the value on disk, a list such as the black
  list keeps both runs' additions and removals, and two runs that each add
  an account keep both. When both changed the same setting, the later save
  wins, as before, and the log says so. A run that loaded the configuration
  before an account was renamed no longer recreates the old name.

### Links: other names for feral

- **`feral link add mt -- -A fastmail tui`** makes `mt` a command that runs
  `feral -A fastmail tui`: a symlink beside the binary (or in `--dir`) and an
  entry under `links` in config.json. A link completes with `<Tab>` and
  explains with Alt-H like feral, and `mt -h` opens with `mt runs: …`.
  `link list` shows each link and its state; `link rm` removes it and its
  symlink (only a symlink that is this feral).
- **A link is stored as it will run**: the account by its name (a display
  name, alias or `-2` is resolved when the link is added), and a command
  spelled in full.
- **`aliases[]` become links.** The first run that may write turns each
  account alias into a link of the same name, so `~/bin/gmail` keeps
  selecting the gmail account; after that an alias is only a name `-A`
  accepts. A name on the command line that is an account's but not a link is
  refused instead of running against the default account.
- **The last account named on a line wins.** `-A` and `-N`/`--account-index`
  are one choice: `feral -A work -2` means account 2 (it used to be an
  error), so `mt -2` runs the link's command on account 2.
- `account rename` carries the links that run the account; `account rm`
  keeps them and lists them with a warning.

### Signatures, per account

- **`account signature <account>`** prints an account's signature;
  `--file <path>`, `--edit` (the configured editor) and `--clear` set it. It is
  kept as `signature.txt` in the account's directory, so `account rename`
  moves it. `--dry-run` refuses to set one; `--read-only` allows it.
- **It is appended under a `-- ` line** by `write`, `msg reply` (headless or
  not), `msg send` and the TUI's compose, reply and forward -- above the quoted
  text of a reply, under the forwarded text of a forward. `msg send --raw` is
  never signed, nor is a body that already has a `-- ` line, so a resumed
  draft is not signed twice. The signature is part of the body, so `--sign`
  and the TUI's PGP signing cover it. `msg send --no-signature` and
  `msg reply --headless --no-signature` send without it, for scripts.
- **Changing the sending account while composing swaps the signature**, if
  the old one is still there unedited.
- `~/.signature`, which the TUI used to append below the quote, is still used
  for an account that has no signature of its own; `--clear` turns that off
  for the account.

### The TUI's keys can be moved

- **`keys.json` beside `config.json` rebinds the message list's and the
  message view's keys.** Each key runs a named action (`archive`, `reply_all`,
  `toggle_read`, ...), per group: `{"format": 1, "list": {"archive": "A"},
  "message": {...}}`. An action left out keeps its defaults; a key or a list
  of keys replaces them. `feral utils keys` prints every action with its
  default keys, a file ready to edit.
- **A broken keys file stops the TUI before it starts**, naming the file, the
  line and the entry: an unknown group or action, a key that is not a key, a
  key two actions of one group would share, or a key the screens keep for
  themselves (q, Esc, Tab, f, F, h, ?, Enter, the arrows, PgUp, PgDn, Home,
  End, Shift+← and Shift+→, F1-F4, F8, F9, Ctrl+C, Ctrl+D, Ctrl+Q, every Alt
  chord).
  No file means the defaults, unchanged.
- **The help (F1, h, ?) shows the keys in effect**, and so do the hints that
  name one (the spam undo banner, the attachment badge, "+: more", "Press R
  to retry").
- **Alt+F4 to Alt+F7 open the menu in a message**, as they already
  did on the list: the F-key arms used to be matched by key type ahead of
  the Alt check there, and a key is now looked up by name.

### Fixed

- **Saving the configuration no longer drops settings it does not know.** A
  key in config.json or an account.json that the running build does not
  declare -- written by a newer build, or added by hand -- used to vanish on
  the next save; it is now kept as it was.

### Command line

- **A mistyped flag gets a suggestion** (clihelp 0.3.63): `--hlep` suggests
  `--help`, and a flag given to the wrong command says whose it is.
- **`<Tab>` at the top level offers the global options too.**

## [0.7.66 – 0.7.69] - 2026-11-03

> 0.7.1 to 0.7.65 have no entries; `git log` is the record for them.

Found with a seeded test account, live TUI tests in kitty, a random-key
monkey and a state machine of the TUI's screens.

### Changed behavior

- **`--read-only` refuses more.** `msg block` and `spam mark` refuse, as their
  TUI keys do: their blacklist, filter training and override would record a
  change the server never saw. `send --queue` refuses (the next run would send
  it). `label rename` no longer saves rules rewritten for a rename that was
  only simulated. `account migrate` prints what it would copy instead of
  copying the config and tokens over ssh. Calendar writes are refused (they
  used to reach Google Calendar). The TUI writes no LLM summaries; F4 still
  shows one already on disk.
- **`--dry-run` writes nothing.** No command writes `config.json`;
  `spam learn --ham`, `spam unmark`, `spam reset`, `cache clear`,
  `cache prune` and `mbox download` report instead of acting.
- **Messages opened in the browser load nothing remote** (`w`, `show`, HTML
  attachments): no remote images, so no tracking pixels, and no scripts.
- **Busy dialogs are modal.** Under "Unspamming…", "Downloading…" and the like,
  keys no longer act on the hidden list; Esc, q and Ctrl+C still work.
- **`account login` keeps the old token** until a new one is written;
  abandoning a sign-in used to leave the account with none.
- HTML attachments open in the configured browser, as `w` does.

### Fixed

- A search typed in a message stayed open after Esc and replaced every
  message body afterwards; it also survived 17 other ways back to the list.
- `~` (whole-cache) search: Esc kept the cache's rows; `/` or `~` during it lost
  the folder; a refresh or delete under it was lost or came back.
- Searches leaked into the next folder, history step or account.
- Backspace removed a byte, not a character, in all three searches (Hebrew).
- The folder tree vanished on a search that matched nothing; its search
  outlived it.
- Clicks behind the sort menu, the quit dialog or the save help opened a
  message hidden underneath.
- `label rm` and `label upload` right after `label create` answered
  "no label matches" (a cached folder list was kept for a day).
- Outlook: throttled reads are retried; a failed subfolder listing no longer
  silently drops part of the folder tree.

## [0.7.0] - 2026-09-19

The largest release so far: a layering rewrite, a redesigned command tree, and
the whole backlog of a deep code review. 83 commits.

Read the **Breaking changes** section before upgrading. Most of it is renames
with a mechanical fix, but labels now carry a sigil, which touches nearly every
command line.

### Breaking changes

**Labels are written `%label`.** A bare folder name is no longer accepted; the
error names the spelling that would have worked.

```
scan inbox              ->  scan %inbox
show receipts 1234abcd  ->  show %receipts 1234abcd
label sort work:old     ->  label sort %work:old
```

`%` on its own means the inbox, so `mail_cli %` scans it and `mail_cli %inbox`
is the same thing. `%%foo` is the label literally named `%foo`. The sigil exists
because without it the tool has to guess whether an argument is a folder, a
message ID or an address — and guessing is what it used to do.

**Commands moved under the noun they belong to.** The old top-level spellings
are gone, not deprecated:

| was | now |
|---|---|
| `splice` | `label sort` |
| `split` | `label move` |
| `download` | `label export` |
| `upload` | `label import` |
| `unspam` | `spam unmark` |
| `whitelist` | `spam allow` |
| `blacklist` | `spam block` |
| `learning reset` | `spam reset` |
| `learn-ham` | `spam learn --ham` |
| `filter` | `rule remote` |
| `caladd` | `calendar sync` |
| `test` | `doctor` |
| `label list full ext real` | `label list --full --ext --real` |

**Kept, because they are in everyone's fingers:** `labels`, `arc`, `archive`,
`ss`, `sb`, `wlist`, `blist`, `del`, `delete`, `fix`, `ls`, and the `-1` / `-2`
account-index flags all still work.

**Flags.**

- `--move=<addr>` is now `--move-from <addr>`. `-m` / `--move` is a plain
  boolean. One string flag used to carry both a boolean and an address, with the
  absent value recorded as the literal `"true"`.
- `--ro` is removed. Use `--read-only`.
- `-p` / `--pattern` and `--fix` are no longer accepted by every command. They
  are declared by the three commands that read the pattern (`scan`, `label sort`
  and the bare `%label` form) and by `config check --fix`.
- `config set` takes exactly two arguments. The third selected an
  account-specific override and was read nowhere; use `-A` to choose the account.

**Semantics.**

- `--dry-run` and `--read-only` are different contracts and both are documented
  as such. `--read-only` forbids server mutations but still caches, because
  without downloaded bodies a message cannot be shown at all; `--dry-run` also
  forbids the local config and cache writes. `--dry-run` implies `--read-only`.
- `label move`'s wildcard-free patterns are substring matches, not exact ones —
  the same rule `label sort` and `scan` already used. `label move src invoice
  dst` previously matched only a subject *equal* to "invoice", so it silently
  moved nothing.
- `doctor` is refused under `--read-only` and `--dry-run`. It creates a label,
  inserts a message, moves it and deletes both — that is how it proves the
  credentials can write.
- Irreversible commands prompt, and require `--yes` when stdin is not a
  terminal: `spam empty`, `label rm`, `rule rm-all`, `account rm`, `cache clear`.
  A cron line that would have prompted fails loudly rather than guessing.

### Added

- **`%` label references**, with `%account:label` for cross-account work.
- **Tab completion for label arguments**, not just flag values, on fourteen
  positionals. With nothing typed it offers the `%` form.
- **`-E` / `--examples`** shows every example in the program on one page; add a
  command to narrow it.
- **`--json`** on the listings, for scripts.
- **`spam reset`, `label repair`, `rule tidy`, `msg archive`, `label archive`**
  as first-class commands.
- **`report.Reporter`**, an output port: status goes to it, a command's product
  goes to an `io.Writer` the caller supplies. Suppressing progress no longer
  suppresses the answer.
- **A generated package inventory** in `docs/ARCHITECTURE.md`, with a drift test.
- **An architecture check** in the quality gate, enforcing four layering rules.

### Fixed

**Correctness**

- **Ambiguous label prefixes resolved to a string naming no folder.** With
  folders `Work` and `Worklog`, `scan wor` passed `"wor"` to the backend: Gmail
  matched nothing and did nothing, JMAP *created a new empty mailbox* called
  `wor` and filed messages into it. There is one resolver now and it refuses to
  guess.
- **Bulk spam operations processed one page and reported success.** JMAP queried
  1000 messages with no loop; Outlook took `$top=100` and never followed
  `@odata.nextLink`. Both now converge.
- **A partly-failed batch move discarded the part that succeeded**, leaving
  messages listed in a folder they had left.
- **`--dry-run` written before the command was silently downgraded** to
  `--read-only`, so the local writes it exists to prevent went ahead.
- **A cache store that writes nothing silenced the network.** Reaching a
  non-selected account showed every folder as empty and reported success.
- **One overlong line discarded an entire mbox import.**
- **`scan %inbox` stopped clearing cached classifications**, because the check
  compared the raw argument against `"inbox"`.
- **`label rm` deleted a server label with no confirmation**, the one row of the
  confirmation matrix that had never been implemented. It also passed its
  argument to the backend unparsed, so the `%` form named a label that does not
  exist while the bare form deleted one without being checked at all.

**Security and robustness**

- **`sieve` export could be broken out of.** A rule whose sender, subject or
  label contained a newline injected script into a file the user is told to
  upload to their mail server.
- **`StripHTML` leaked the contents of an unclosed `<script>` or `<style>`** into
  the reading pane and into the text the spam classifier scores. Numeric
  character references are decoded now, so `&#118;&#105;agra` no longer hides a
  keyword from the classifier.
- **MIME nesting is depth-capped** in the two attachment walks, which had no
  limit and run automatically on every message opened or scanned.
- **`doctor` mutated the account under `--read-only`** — it builds its own
  client, so the read-only decorator never saw it.
- **A closed stdin was treated as consent to every default**, so
  `account new jmap work < /dev/null` stored the literal placeholder
  `your-jmap-api-token` as the account password.
- **Every piped answer after the first was discarded** during `account new`,
  because each prompt built its own buffered reader.

**Interface**

- **The TUI's spam key never trained the classifier**, so the action a user
  repeats most often taught it nothing while `spam mark` taught it everything.
- **Attachment previews were unbounded**: one external process per attachment,
  none cancelled when the user navigated away, and no redraw when they finished.
- **`-A nonesuch` ran against the first configured account** and exited 0. Both
  the unknown and the ambiguous case are errors now.
- **`rule list` deleted duplicate rules and saved the config** — a read command
  with a write side effect. That is `rule tidy` now.
- **A refreshed Gmail OAuth token was discarded**, so every process re-derived
  one against a provider that rate-limits refreshes.
- **An Outlook account with a lapsed token started a browser sign-in from any
  process**, including a cron scan, and then waited up to fifteen minutes for
  someone to type a code. It refuses without a terminal now and names
  `account login`.
- **An unresolvable calendar `TZID` silently read the time as UTC.** It is
  reported now, because that instant is what an event is filed at.

### Changed

- **The layering is explicit and enforced.** `main` composes; `cli` and `tui`
  declare the interfaces they need; `runner` implements them; `mailops` is the
  only package that may name a backend. 454 `fmt.Print*` calls below the UI are
  gone, along with the `app.TuiActive` global that existed to mute them.
- **`docs/ARCHITECTURE.md` is rewritten.** It described a flat `package main`
  layout and instructed the reader never to create subdirectories, against a
  tree of forty-one packages.
- Every package has a doc comment.

## [0.2.20] - 2026-06-22

### Changed
* **TUI Message Detail View:** Redesigned the detail view when pressing Enter on an email: the top bar is now always shown, the second line displays the selected index row with highlighting, headers and message body use natural text coloring (yellow labels with default foreground body on dark background), and a special `< End of message >` line with green background appears at the bottom.

## [0.2.19] - 2026-06-06

### Fixed
* **JMAP Filter Compilation Error:** Implemented the missing `Requires() []jmap.URI` method on the custom `JMAPFilterSetMethod` struct to satisfy the `jmap.Method` interface.

## [0.2.18] - 2026-06-06

### Added
* **JMAP Server-Side Rule Migration:** Implemented server-side rule migration for JMAP accounts using the JMAP Filters extension (`Filter/set` method). This uploads local routing rules to the JMAP server so they apply automatically to incoming emails.

## [0.2.17] - 2026-06-06

### Added
* **JMAP Rule Export Persistence:** Enhanced `ExportRules` for JMAP accounts to persist the `Exported` status of rules back to `config.json` and update the in-memory configuration.

## [0.2.16] - 2026-06-06

### Added
* **JMAP Rule Export Support:** Implemented `ExportRules` for JMAP accounts. Since JMAP rules are processed locally by `mail_cli` during scans, they do not need to be uploaded to the server. The command now marks local rules as exported and prints a success message instead of failing.

## [0.2.15] - 2026-06-06

### Fixed
* **Cobra Optional Flag Argument Parsing:** Added logic to `preprocessArgs` to automatically rewrite `-m <argument>` / `--move <argument>` to use the assignment operator (`-m=<argument>` / `--move=<argument>`), preventing Cobra from interpreting the flag value as a positional argument when `NoOptDefVal` is active.

## [0.2.10] - 2026-06-06

### Added
* **Unique Message Move-to-Spam:** Enabled specifying a sender email address directly to `-m` / `--move` flag (e.g. `-m spammer@example.com`). If exactly one message from this sender is found in the scanned folder, it is moved to the spam folder and removed from the remainder of the scan.

## [0.2.9] - 2026-06-06

### Added
* **Version Command:** Added an explicit `version` command to the CLI that prints the active program version.

## [0.2.8] - 2026-06-06

### Fixed
* **ProgName Auto-Selection:** Removed the legacy exclusion of `"gmail"` from the binary name/symlink auto-selection check in `config.go`, enabling correct auto-selection of the associated account when the tool is executed via a `gmail` symbolic link.

## [0.2.7] - 2026-06-06

### Added
* **Command display in account list:** Modified the `account list` command output to display the associated CLI command/alias for each account on a new line.

## [0.2.6] - 2026-06-06

### Fixed
* **JMAP Spam Learning Responses:** Updated `LearnSpam` in `jmap_client.go` to properly handle HTTP status codes 204 (ignored) and 208 (already trained) returned by the Rspamd controller. This ensures that the local metadata cache is successfully cleared and trained message IDs are marked in the database.

## [0.2.5] - 2026-06-06

### Changed
* **Documentation update:** Updated HISTORY.md for the Rspamd Bayes statistics turn.

## [0.2.4] - 2026-06-06

### Changed
* **Default Scan Limit raised to 1000:** Increased the global default scan and download limit on everything to 1000 (up from 50) for users with smaller inboxes who want to process all messages.

## [0.2.3] - 2026-06-06

### Changed
* **Increased Ham Learning Limit:** Raised the default download/scan limit specifically for the `learn_ham` subcommand to 1000 messages (up from 50) to allow more comprehensive training of the Bayes classifier.

## [0.2.2] - 2026-06-06

### Added
* **Stat Warning for Ham Learning:** Added stat warnings to main.go in verbose mode when local email cache files cannot be found.
* **Automatic versioning policy:** Added rule 6 to GEMINI.md enforcing 0.0.1 version increments on all code changes.

### Fixed
* **Ham Learning Path Bug:** Passed account-specific client configuration down to performHamLearning so Rspamd learns emails using the correct account cache directory.

## [0.2.1] - 2026-06-06

### Added
* **Fuzzy Suffix Label Matching:** Enabled specifying target folders/labels without parent and grandparent hierarchies. If a unique leaf folder matches the suffix, the CLI prints the resolved full path and executes the command as usual.

### Fixed
* **JMAP Error Handling Type Bug:** Fixed a compilation failure in `jmap_client.go` where pointer strings (`*string`) were printed directly using `%s`.

## [0.1.1] - 2026-05-30

### Added
* **Personal Whitelist System**: Implemented a local, configuration-driven whitelist (`-wlist add <email>`, `-wlist del <email>`, and `-wlist list` subcommands) stored in `config.json`.
* **Automated Ham Training**: Integrated automatic local Rspamd HAM (non-spam) learning for whitelisted emails to improve Rspamd's statistical classifier accuracy.

### Changed
* **Visual Cleanup**: Cleaned up the inbox scan (`-i`) console output by hiding the red "SPAM EMAILS DETECTED" banner block when zero spam emails are found, replacing it with a clean green success status.

---

## [1.2.0] - 2026-05-29

This major release refactors the command-line interface, implements structured JSON configuration auto-population and bootstrapping, integrates standard Linux system directories, and enhances the language validation architecture.

### Added
* **Flag `-i` (Inbox Scan)**: The default inbox scan mode has been assigned to the new `-i` flag.
* **Flag `-spam` (Spam Auditor)**: Connects securely to the Spam folder, downloads/caches spam emails, and color-prints sender and subject headers.
* **Flag `-spamdel` (Permanent Purge)**: Connects in read-write mode, tags spam messages with `\Deleted`, and expunges them, permanently purging them from Google's servers forever (bypassing Trash).
* **Length-Constrained NLP Validation**: Restored `whatlanggo` statistical NLP language detection to audit Latin-script emails. To eliminate false positives on sparse texts, it is **only run if the email body has at least 20 words**.
* **Auto-Bootstrapping Config**: If `config.json` does not exist, a credentials template is written automatically on startup.
* **Dynamic Config Auto-Population**: Automatically updates existing configuration files with any missing default properties (such as whitelisted languages, thresholds, and limits) without modifying user credentials.
* **Standard System Caching**: Caching directories default to standard Linux user caches (`~/.cache/gmail-spam-checker/`).
* **Cache Auto-Migration**: Automatically detects legacy `./downloaded_emails` configurations and migrates them to the standard `~/.cache` directory.
* **Descriptive Operational Menu**: Displays a detailed operational guide of all commands and options when the program is run with no arguments.
* **Command Validation**: Alerts the user and presents the usage guide if arguments are parsed but no action/operation mode is specified.
* **Graceful Pre-flight Stack Check**: Automatically audits local Redis and Rspamd service ports before making remote IMAP connections, failing early with detailed instructions and exact, colorized Debian/Kali startup commands (`sudo systemctl start redis-server rspamd`).
* **MIME Normalization**: Decodes malformed and non-standard UTF-8 subject headers (e.g. `=?UTF8?Q?` which are missing the hyphen).

### Changed
* **Simplified CLI**: Moved all configuration-centric command-line options (e.g., `-host`, `-user`, `-pass`, `-dir`, `-limit`, `-languages`) entirely into `config.json` to declutter the user interface.
* **Binary Renaming**: Renamed the compiled binary to `gmail_spam_checker`.
* **Usage Outputs**: Extended `flag.Usage` to explicitly output the absolute file path of the configuration file at the bottom of the CLI usage block.

### Dependencies
* Added `github.com/abadojack/whatlanggo` (v1.0.1) for statistical trigram language analysis.
* Retained `github.com/emersion/go-imap` (v1.2.1) and `github.com/fatih/color` (v1.19.0).
