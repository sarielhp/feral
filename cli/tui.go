package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/sarielhp/feral/app"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/watcher"

	"github.com/sarielhp/clihelp"
)

// TuiCmd returns the clihelp.Command for the tui command.
func TuiCmd(session *Session) clihelp.Command {
	write := rewriteCommandHelp(ComposeCmd(session), strings.NewReplacer("feral write", "feral tui write"))
	write.Group = ""
	return clihelp.Command{
		Name:            "tui",
		Group:           "Daily",
		Description:     "Open interactive terminal email browser.",
		LongDescription: "Open the interactive terminal email browser. With an optional label_prefix argument, open the TUI with the matching label as the initial folder. The prefix is matched case-insensitively as a substring against the full label path. If exactly one label matches, the TUI opens on that label. If multiple match, all matching labels are printed and the program exits. Given an existing .eml file instead, open that one message read-only, as mbox view does. Use tui write to compose a message; it accepts the same arguments and flags as write, including --resume.",
		UsageLine:       "feral tui [label_prefix]\n  feral tui <file.eml>\n  feral tui write [flags] [recipient...]",
		Subcommands:     []clihelp.Command{write},
		Parameters: []clihelp.Param{
			{Name: "[label_prefix]", Description: "Folder/label prefix to open into, or a .eml file to view.", Complete: CompleteLabelRefs},
		},
		Options: []clihelp.Option{
			clihelp.Bool(&flagTuiNoWatch, "--no-watch", false, "Do not watch for new mail while open"),
		},
		Notes: []clihelp.Note{
			{Heading: "New-mail notifications", Text: "While the TUI is open it runs the same watcher as feral watch, over every account's inbox once a minute, and shows a desktop notification when new mail arrives there; it stops when the TUI closes. A check is one small request per account. Its messages go to the diagnostics panel (Ctrl+D). Unlike feral watch it never sends the offline outbox. Only one watcher runs per download directory, so a second TUI, or a TUI beside feral watch, leaves the watching to the one already running. Turn it off with --no-watch, or for good with \"tui_watch\": false in config.json."},
		},
		Examples: []clihelp.Example{
			{Line: "feral tui"},
			{Line: "feral tui %receipts"},
			{Line: "feral tui saved.eml", Description: "View one saved message, read-only"},
			{Line: "feral tui write boss@example.com -s Update", Description: "Compose through a wrapper that opens the TUI"},
		},
		Args: clihelp.MaximumNArgs(1),
		Run: func(ctx *clihelp.Context) error {
			if len(ctx.Args) > 0 && isEmlFile(ctx.Args[0]) {
				return runMboxView(session, ctx.Args[0])
			}
			if session != nil && session.Config != nil && cfg_g.NeedsOnboarding(session.Config) {
				if term.IsTerminal(int(os.Stdin.Fd())) {
					fmt.Fprintf(session.errOut(), "%s No email accounts configured yet.\n", app.PrefixWarn)
					if promptYesNo("Would you like to run the setup wizard now?", true) {
						if err := runSetup(session); err != nil {
							return err
						}
					} else {
						return cfg_g.ErrNoAccounts
					}
				} else {
					return fmt.Errorf("%w: run 'feral account setup' to configure an account", cfg_g.ErrNoAccounts)
				}
			}

			labelPrefix := ""
			initialNotice := ""
			if len(ctx.Args) > 0 && ctx.Args[0] != "" {
				// The argument is a label reference like everywhere else. This
				// used to take it raw, so completion -- which offers the %
				// form -- suggested arguments the command then rejected.
				ref, err := cfg_g.ParseLabelRef(ctx.Args[0])
				if err != nil {
					return err
				}
				prefix, notice, err := resolveTuiFolderPrefix(session, ref.Label)
				if err != nil {
					return err
				}
				labelPrefix = prefix
				initialNotice = notice
			}

			stop := startTuiWatch(ctx.Context, session)
			defer stop()
			if initialNotice != "" {
				return session.tui().Run(session.Config, labelPrefix, initialNotice)
			}
			return session.tui().Run(session.Config, labelPrefix)
		},
	}
}

// tuiWatchExitWait is how long closing the TUI waits for its watcher to stop.
// A poll blocked on the network may not notice at once; the process is
// exiting anyway, and the kernel drops the watcher's lock with it.
const tuiWatchExitWait = 2 * time.Second

// tuiWatchInterval is how often the TUI checks for new mail. A check is one
// request per account for the inbox's newest IDs, so a minute is cheap.
const tuiWatchInterval = time.Minute

// tuiWatchRetry is how often a TUI whose watching was left to another
// watcher checks whether that one has gone, to take over from it.
var tuiWatchRetry = watcher.DefaultInterval

// startTuiWatch runs the new-mail watcher for as long as the TUI is open, and
// returns what stops it. It watches a copy of the configuration, so the TUI may
// change its own -- the selected account, Quiet -- while the watcher reads.
func startTuiWatch(base context.Context, session *Session) (stop func()) {
	if flagTuiNoWatch || session == nil || session.Config == nil || !session.Config.TuiWatch {
		return func() {}
	}
	if base == nil {
		base = context.Background()
	}
	snapshot := *session.Config
	snapshot.Accounts = slices.Clone(snapshot.Accounts)
	// The watcher logs from its first poll, which is before the TUI is up:
	// open the TUI's log now, so every line reaches its diagnostics panel.
	app.InitTuiLogger(snapshot.ConfigDir)
	ctx, cancel := context.WithCancel(base)
	done := make(chan struct{})
	go func() {
		defer close(done)
		runTuiWatch(ctx, session.watch(), &snapshot)
	}()
	return func() {
		cancel()
		select {
		case <-done:
		case <-time.After(tuiWatchExitWait):
		}
		app.CloseTuiLogger()
	}
}

// runTuiWatch is the TUI's watcher. It sends nothing from the outbox -- that
// is feral watch's job, and a TUI that quietly sent queued mail would
// surprise -- and its first poll records rather than announces, since the
// user is about to see that mail on screen. While another watcher runs it
// waits, and takes over when that one stops, announcing from its first poll:
// the mail since is what the other would have announced.
func runTuiWatch(ctx context.Context, ops WatchOps, config *cfg_g.Config) {
	opts := watcher.Options{Interval: tuiWatchInterval, NoFlush: true, QuietFirstPoll: true}
	for {
		err := ops.WatchInBackground(ctx, config, opts)
		switch {
		case ctx.Err() != nil:
			return
		case errors.Is(err, watcher.ErrAlreadyWatching):
			if opts.QuietFirstPoll {
				slog.Info("watch: another watcher is running; this TUI takes over when it stops")
			}
			opts.QuietFirstPoll = false
		case err != nil:
			slog.Warn("watch: the new-mail watcher stopped", slog.Any("error", err))
			return
		default:
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(tuiWatchRetry):
		}
	}
}

func resolveTuiFolderPrefix(session *Session, arg string) (string, string, error) {
	matches, err := SearchLabels(session.Config, []string{arg})
	if err != nil {
		return "", fmt.Sprintf("Labels cache not available; opening TUI without folder filter %q", arg), nil
	}
	if len(matches) == 0 {
		return "", "", fmt.Errorf("%w: no folder matches %q. Run %q to list folders",
			clihelp.ErrUsage, arg, app.AppName+" labels")
	}
	resolved := ResolveUniqueMatch(arg, matches)
	if resolved == "" {
		return "", "", fmt.Errorf("%w: %d folders match %q: %s",
			clihelp.ErrUsage, len(matches), arg, strings.Join(matches, ", "))
	}
	return resolved, "", nil
}

// isEmlFile reports whether arg names an existing .eml file, which tui opens
// in place of a folder. Requiring the file to exist keeps label prefixes
// working: a label that happens to end in .eml still resolves as one.
func isEmlFile(arg string) bool {
	if !strings.EqualFold(filepath.Ext(arg), ".eml") {
		return false
	}
	fi, err := os.Stat(arg)
	return err == nil && fi.Mode().IsRegular()
}
