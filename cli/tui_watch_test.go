package cli

import (
	"context"
	"testing"
	"time"

	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/watcher"
)

// The TUI runs the watcher beside itself -- on a copy of the configuration,
// without flushing the outbox -- and stops it when it closes.
func TestStartTuiWatchRunsUntilStopped(t *testing.T) {
	prev := flagTuiNoWatch
	t.Cleanup(func() { flagTuiNoWatch = prev })
	flagTuiNoWatch = false

	config := &cfg_g.Config{Paths: cfg_g.Paths{ConfigDir: t.TempDir()}, Prefs: cfg_g.Prefs{TuiWatch: true}, Accounts: []cfg_acc.AccountConfig{{Name: "work"}}}
	started := make(chan *cfg_g.Config, 1)
	var opts watcher.Options
	stopped := make(chan struct{})
	session := &Session{Config: config}
	session.Deps.Watch = stubWatch{BackgroundFn: func(ctx context.Context, c *cfg_g.Config, o watcher.Options) error {
		opts = o
		started <- c
		<-ctx.Done()
		close(stopped)
		return nil
	}}

	stop := startTuiWatch(context.Background(), session)
	var got *cfg_g.Config
	select {
	case got = <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("the TUI did not start a watcher")
	}
	if got == config || len(got.Accounts) != 1 || &got.Accounts[0] == &config.Accounts[0] {
		t.Error("the watcher shares the TUI's configuration instead of a copy")
	}
	stop()
	select {
	case <-stopped:
	default:
		t.Fatal("stop returned before the watcher stopped")
	}
	if !opts.NoFlush || !opts.QuietFirstPoll || opts.Interval != tuiWatchInterval {
		t.Errorf("options = %+v, want no outbox flush, a quiet first poll and the TUI's interval", opts)
	}
}

func TestStartTuiWatchHonoursOptOuts(t *testing.T) {
	prev := flagTuiNoWatch
	t.Cleanup(func() { flagTuiNoWatch = prev })
	cases := []struct {
		name    string
		flag    bool
		enabled bool
	}{
		{"--no-watch", true, true},
		{"tui_watch false", false, false},
	}
	for _, tc := range cases {
		flagTuiNoWatch = tc.flag
		called := make(chan struct{}, 1)
		session := &Session{Config: &cfg_g.Config{Prefs: cfg_g.Prefs{TuiWatch: tc.enabled}}}
		session.Deps.Watch = stubWatch{BackgroundFn: func(context.Context, *cfg_g.Config, watcher.Options) error {
			called <- struct{}{}
			return nil
		}}
		startTuiWatch(context.Background(), session)()
		select {
		case <-called:
			t.Errorf("%s: a watcher started anyway", tc.name)
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// A TUI that found another watcher running takes over once it stops, and
// announces from its first poll, since nobody recorded the mail since.
func TestRunTuiWatchTakesOverWhenTheOtherStops(t *testing.T) {
	prev := tuiWatchRetry
	t.Cleanup(func() { tuiWatchRetry = prev })
	tuiWatchRetry = time.Millisecond

	var calls []watcher.Options
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ops := stubWatch{BackgroundFn: func(_ context.Context, _ *cfg_g.Config, o watcher.Options) error {
		calls = append(calls, o)
		if len(calls) < 3 {
			return watcher.ErrAlreadyWatching
		}
		cancel() // took over; the TUI closes
		return nil
	}}
	runTuiWatch(ctx, ops, &cfg_g.Config{})
	if len(calls) != 3 {
		t.Fatalf("watcher started %d time(s), want retries until it took over (3)", len(calls))
	}
	if !calls[0].QuietFirstPoll || calls[2].QuietFirstPoll {
		t.Errorf("quiet first poll = %v then %v; a takeover must announce", calls[0].QuietFirstPoll, calls[2].QuietFirstPoll)
	}
}
