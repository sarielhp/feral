package interactive

import (
	"strings"
	"testing"
	"time"
)

// TestTuiWatcherRunsBesideTheTUI runs the TUI with its new-mail watcher on,
// against a fake JMAP server and in the program sandbox. The watcher must
// report to the diagnostics panel rather than over the screen, record the
// mailbox it finds without a notification, and stop with the TUI: the run
// ends promptly, nothing it started outlives it (reapStrays), and
// notify-send was never started.
func TestTuiWatcherRunsBesideTheTUI(t *testing.T) {
	day := time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
	fake := newFakeJMAP(t, "alpha@example.com",
		fakeMessage{"alpha-1", "Watched one", "Ann <ann@example.com>", day},
		fakeMessage{"alpha-2", "Watched two", "Ann <ann@example.com>", day.Add(time.Hour)})
	s := startHome(t, homeSpec{accounts: []map[string]any{fake.account("alpha")}, sandbox: true, watch: true}, "tui")
	s.WaitFor("Watched two")

	s.Key("ctrl+d")
	s.WaitFor("watch: polling 1 account(s)")
	s.WaitFor("first run; recorded 2 existing")
	s.Key("ctrl+d")
	screen := s.WaitFor("Watched one")
	if strings.Contains(screen, "watch:") {
		t.Errorf("the watcher wrote over the message list:\n%s", screen)
	}

	s.Key("ctrl+q")
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if code, ended := s.Exited(); ended {
			if code != "0" {
				t.Errorf("feral exited with status %s", code)
			}
			for _, tool := range s.Tools() {
				if strings.HasPrefix(tool, "notify-send") {
					t.Errorf("the first poll notified: %s", tool)
				}
			}
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("feral did not exit within 10s of Ctrl+Q:\n%s", s.Text())
}
