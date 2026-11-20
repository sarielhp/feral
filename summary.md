# TUI Auto-Refresh & Inactivity New Mail Check

This document summarizes the changes implemented to automatically detect and display new incoming email in the TUI without requiring manual refreshes (`R`), as well as the edge-case review and fixes.

---

## 1. Periodic Refresh (Every Minute)

- **Configuration**: `autoRefreshInterval = 60 * time.Second` defined in [`tui/tui.go`](file:///home/sariel/prog/26/mail_cli/tui/tui.go).
- **Periodic Timer**: The TUI's internal 10-second ticker triggers [`handleTimeTick`](file:///home/sariel/prog/26/mail_cli/tui/tui_model.go), which checks whether 60 seconds have elapsed since the last check (`lastCheckTime`).
- **Non-blocking Execution**: When due and safe to refresh ([`canCheckNewMail`](file:///home/sariel/prog/26/mail_cli/tui/tui_model.go)), an asynchronous background command ([`bgCheckNewMailCmd`](file:///home/sariel/prog/26/mail_cli/tui/tui_model.go)) is dispatched via Bubble Tea commands. It fetches and downloads new messages without displaying a blocking modal or interrupting the UI.

---

## 2. Inactivity Refresh (Key Press After > 30s of Inactivity)

- **Configuration**: `idleActivityThreshold = 30 * time.Second` and `minRefreshInterval = 30 * time.Second`.
- **Activity Tracking**:
  - In [`key`](file:///home/sariel/prog/26/mail_cli/tui/tui_keys.go), whenever the user presses a key, the elapsed time since `m.lastActivityTime` is checked.
  - If $\ge 30$ seconds have passed without user input, a background mail check for the current folder is batched with the keypress action.
  - Keystrokes execute immediately without waiting for the network check.
  - Throttled by `minRefreshInterval` (30 seconds) to avoid redundant network requests if a check completed recently.
- **Mouse Activity**: [`handleMouse`](file:///home/sariel/prog/26/mail_cli/tui/tui_mouse.go) also updates `m.lastActivityTime` on wheel and click events.

---

## 3. Background Safety & Seamless Updates

- **Silent Errors**: Background checks log errors (`slog.Warn`) rather than popping up intrusive error dialogs or red error banners (`showError`).
- **View Preservation**: When new mail arrives, [`handleEmailsLoaded`](file:///home/sariel/prog/26/mail_cli/tui/tui_update.go) updates the visible email list and keeps the cursor on the currently selected message.
- **Stale Check Guards**:
  - `emailsLoadedMsg` records the target client and folder. If the user switched accounts or changed folders while the fetch was in flight, stale results are discarded safely.
  - Checks are suppressed if compose mode is active, a search view (`%search:...`) is open, account overlays are active, or another check is already running.

---

## 4. Code Review & Edge-Case Fixes

During the code review, the following edge cases were identified and resolved:

1. **`bgCheckingMail` State Leak Prevention**:
   - In [`loadEmailsWithSilentOption`](file:///home/sariel/prog/26/mail_cli/tui/tui_model.go), if `client` or `client.Config()` was nil inside the command goroutine, returning `nil` sent no message to the update loop, leaving `m.bgCheckingMail = true` indefinitely.
   - Fixed by returning `bgCheckDoneMsg{folder: folder}` so `Update` always receives the message and safely clears `bgCheckingMail`.

2. **Deduplicating Manual & Navigation Refreshes**:
   - When selecting a folder from the tree or pressing `R`, [`loadEmailsCmd`](file:///home/sariel/prog/26/mail_cli/tui/tui_model.go) is dispatched as a foreground command. If a user was idle for $\ge 30$ seconds prior to selecting a folder, pressing Enter could simultaneously dispatch both `loadEmailsCmd` and `bgCheckNewMailCmd`.
   - Fixed by having [`loadEmailsCmd`](file:///home/sariel/prog/26/mail_cli/tui/tui_model.go) update `m.lastCheckTime = time.Now()`, ensuring that `minRefreshInterval` (30s) prevents duplicate checks during navigation or manual refreshes.

3. **Overlay & Modal Guards in [`canCheckNewMail`](file:///home/sariel/prog/26/mail_cli/tui/tui_model.go)**:
   - Background checks are suppressed while drawers or modals are open (the folder tree drawer `m.treeOpen`, action menus `m.menuOpen` / `m.spamMenuOpen`, or confirmation dialogs `m.blockModalOpen`).

4. **Thread-Safe Captures in [`loadUnifiedEmailsCmd`](file:///home/sariel/prog/26/mail_cli/tui/tui_unified.go)**:
   - Reading `m.client`, `m.currentF`, and `m.getAccounts()` from within the background `tea.Cmd` goroutine could race with account switches on the main loop.
   - Fixed by capturing `client`, `folder`, `accounts`, and `cfg` before returning the goroutine closure.

---

## 5. Provider & Real-Time Push Considerations

- **Gmail & Outlook REST APIs**: Do not support long-lived client-side push streams (SSE/WebSockets) without external cloud infrastructure (Google Cloud Pub/Sub or public HTTPS webhooks). Client-side polling is required.
- **JMAP (Fastmail)**: Supports Server-Sent Events (SSE / EventSource, RFC 8620 §7.3).
- **IMAP IDLE**: Supported by raw IMAP connections, but `mail_cli` uses native REST APIs to avoid IMAP sync daemons and credential deprecations.
- **Polling Efficiency**: Polling sends lightweight ID-list queries; messages are only downloaded when they do not already exist in the local cache.
