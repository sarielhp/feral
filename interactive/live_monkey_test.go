package interactive

import (
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/feral/testenv"
)

// TestLiveMonkey presses random keys at the TUI on the seeded test account
// and checks, every few keys, that nothing went wrong in a way a person would
// notice: a panic on screen, the program ending with an error, or an external
// program started behind the user's back.
//
// What keeps it safe to run against a real account:
//
//   - --read-only: the server is never changed and no mail is sent, which
//     ReadOnlyMailClient blocks; calendar writes refuse on their own.
//   - the sandbox (sandbox_test.go): feral can start only w3m, bogofilter
//     and gpg; any other program it reaches for is a stub that records the
//     attempt, and the test fails on it.
//   - the private HOME, cache and working directory every interactive test
//     has, so no local write lands outside the test directory.
//   - keys whose purpose is to start an editor or a shell -- compose, reply,
//     forward, the pipe prompt -- are not pressed; quitting is, and a clean
//     quit just starts the next round.
//
//	FERAL_INTERACTIVE=1 FERAL_LIVE=1 FERAL_LIVE_ACCOUNT=sariel_test \
//	    FERAL_LIVE_MONKEY=2000 [FERAL_MONKEY_SEED=n] \
//	    go test ./interactive/ -run LiveMonkey -v -count=1 -timeout 60m

const (
	monkeyVar     = "FERAL_LIVE_MONKEY"
	monkeySeedVar = "FERAL_MONKEY_SEED"
	monkeyCheck   = 15 // keys between checks
	monkeyPace    = 40 * time.Millisecond
)

// monkeyKey is one thing the monkey can press: a kitty key name, or text.
type monkeyKey struct {
	key    string
	text   string
	weight int
}

func (k monkeyKey) String() string {
	if k.text != "" {
		return strconv.Quote(k.text)
	}
	return k.key
}

// monkeyKeys favours moving around, so the program visits many states
// rather than typing long queries into one prompt.
var monkeyKeys = func() []monkeyKey {
	keys := []monkeyKey{
		{key: "up", weight: 8}, {key: "down", weight: 8}, {key: "left", weight: 4},
		{key: "right", weight: 4}, {key: "enter", weight: 8}, {key: "escape", weight: 6},
		{key: "tab", weight: 3}, {key: "pageup", weight: 2}, {key: "pagedown", weight: 2},
		{key: "home", weight: 1}, {key: "end", weight: 1}, {key: "backspace", weight: 2},
		{key: "space", weight: 2}, {key: "shift+left", weight: 2}, {key: "shift+right", weight: 2},
		{key: "f1", weight: 1}, {key: "f2", weight: 1}, {key: "f4", weight: 1},
		{key: "f5", weight: 1}, {key: "f6", weight: 1}, {key: "f7", weight: 1}, {key: "f8", weight: 1},
		{key: "ctrl+a", weight: 1}, {key: "ctrl+u", weight: 1}, {key: "ctrl+d", weight: 1},
		{key: "ctrl+f", weight: 1},
	}
	// c, r, g and b start the editor and | runs a shell command; q and Q
	// quit, which the monkey survives but should not do every few keys. l and
	// L are left out, and F3 above: the account list -- also reachable from
	// the menu -- signs in on "l", handing the terminal to a device-code
	// login that waits on a browser. A 3000-key run spent all but its first
	// 25 keys typing into it. G replies to the list, so it is out with r and g;
	// H (snooze), M (mute), X (unsubscribe) and = (list filter) are in, and
	// under --read-only each refuses or only reports.
	for _, r := range "adefhijkmnopstuvwxyzABDEFHMNRSTUWX=/~!;+-?0123456789 " {
		keys = append(keys, monkeyKey{text: string(r), weight: 2})
	}
	keys = append(keys, monkeyKey{text: "q", weight: 1})
	return keys
}()

func pickKey(rng *rand.Rand) monkeyKey {
	total := 0
	for _, k := range monkeyKeys {
		total += k.weight
	}
	n := rng.IntN(total)
	for _, k := range monkeyKeys {
		if n < k.weight {
			return k
		}
		n -= k.weight
	}
	return monkeyKeys[0]
}

// startLiveSandboxed is startLive with the sandbox.
func startLiveSandboxed(t *testing.T) *session {
	t.Helper()
	return startLiveWith(t, true)
}

func TestLiveMonkey(t *testing.T) {
	budget, _ := strconv.Atoi(testenv.Getenv(t, monkeyVar))
	if budget <= 0 {
		t.Skipf("set %s to the number of keys to press", monkeyVar)
	}
	seed, err := strconv.ParseUint(testenv.Getenv(t, monkeySeedVar), 10, 64)
	if err != nil {
		seed = uint64(time.Now().UnixNano())
	}
	t.Logf("seed %d (rerun with %s=%d)", seed, monkeySeedVar, seed)
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))

	share := max(1, budget/len(monkeyStarts))
	for _, start := range monkeyStarts {
		pressed := 0
		for round := 1; pressed < share; round++ {
			ok := t.Run(fmt.Sprintf("%s/round%d", start.screen, round), func(t *testing.T) {
				pressed += monkeyRound(t, rng, start.keys, share-pressed)
			})
			if !ok {
				return
			}
		}
	}
}

// monkeyStart is a screen to start a stretch of random keys from, and the
// keys that open it from the message list. It is the kitty-side copy of the
// TUI's state-machine table (tui/tui_fsm_test.go), which the in-process
// walks use; this one spends the key budget evenly across the screens.
type monkeyStart struct {
	screen string
	keys   []string // kitty key names; one character is typed as text
}

var monkeyStarts = []monkeyStart{
	{"list", nil}, {"message", []string{"enter"}}, {"list-search", []string{"/"}},
	{"message-search", []string{"enter", "/"}}, {"folder-tree", []string{"f"}},
	{"folder-search", []string{"f", "/"}}, {"menu", []string{"m"}}, {"help", []string{"f1"}},
	{"diag", []string{"ctrl+d"}}, {"spam-menu", []string{"f5"}}, {"saved-searches", []string{"f7"}},
	{"recent", []string{"f8"}}, {"sort", []string{"o"}}, {"deadlines", []string{"D"}},
	{"tag-prefix", []string{";"}}, {"tag-pattern", []string{"T"}}, {"remote-search", []string{"ctrl+f"}},
	{"aliases", []string{"f6"}}, {"save-to-folder", []string{"s"}},
}

// monkeyRound runs one program until it quits cleanly or budget keys are
// pressed, and returns how many were.
func monkeyRound(t *testing.T, rng *rand.Rand, start []string, budget int) int {
	s := startLiveSandboxed(t)
	for _, k := range start {
		if len([]rune(k)) == 1 {
			s.Type(k)
		} else {
			s.Key(k)
		}
		time.Sleep(300 * time.Millisecond)
	}
	var trail []string
	states := map[string]int{}
	defer func() { logStates(t, states) }()
	for n := 1; n <= budget; n++ {
		k := pickKey(rng)
		trail = append(trail, k.String())
		if k.text != "" {
			s.Type(k.text)
		} else {
			s.Key(k.key)
		}
		time.Sleep(monkeyPace)
		// An ended program is checked at once: no key goes to whatever
		// holds the window after it.
		_, ended := s.Exited()
		if !ended && n%monkeyCheck != 0 && n != budget {
			continue
		}
		states[screenState(s.Text())]++
		if s.monkeyCheck(trail) {
			t.Logf("quit cleanly after %d keys", n)
			return n
		}
	}
	return budget
}

// monkeyCheck fails the test on a panic, an error exit or a started program,
// and reports whether the program has quit cleanly.
func (s *session) monkeyCheck(trail []string) bool {
	s.t.Helper()
	time.Sleep(300 * time.Millisecond) // let the last keys land
	screen := s.Text()
	last := trail[max(0, len(trail)-60):]
	for _, marker := range []string{"panic:", "goroutine ", "runtime error", "fatal error:"} {
		if strings.Contains(screen, marker) {
			s.t.Fatalf("%q on screen after keys %v:\n%s", marker, last, screen)
		}
	}
	if strings.Contains(screen, "DEVICE AUTHENTICATION") {
		s.t.Fatalf("the keys reached the sign-in flow, so nothing after them tests the TUI; last keys %v:\n%s", last, screen)
	}
	if tools := unexpectedTools(s.t, s.Tools()); len(tools) > 0 {
		s.t.Fatalf("feral started external programs %q; last keys %v", tools, last)
	}
	code, exited := s.Exited()
	if !exited {
		return false
	}
	if code != "0" {
		s.t.Fatalf("feral exited with status %s after keys %v:\n%s", code, last, screen)
	}
	return true
}

// The sandbox is worth trusting only if it catches a program. Piping the
// selected message to less reaches for a pager, which here is a stub.
func TestLiveSandboxRecordsPrograms(t *testing.T) {
	s := startLiveSandboxed(t)
	s.WaitForWithin("Canary Sender", serverWait) // a selected message to pipe
	s.Type("|")
	s.typeText("less")
	s.Key("enter")
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		for _, tool := range s.Tools() {
			if strings.HasPrefix(tool, "less") {
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("piping to less was not recorded; tools %q, screen:\n%s", s.Tools(), s.Text())
}

// screenState names the screen by its top line with the numbers taken out,
// so a run can show how many different states it reached.
func screenState(screen string) string {
	top, _, _ := strings.Cut(screen, "\n")
	top = strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return -1
		}
		return r
	}, top)
	top = strings.Join(strings.Fields(top), " ")
	if len([]rune(top)) > 70 {
		top = string([]rune(top)[:70])
	}
	return top
}

// logStates prints the screens a round saw, commonest first: a run stuck on
// one screen tests far less than its key count says.
func logStates(t *testing.T, states map[string]int) {
	type seen struct {
		state string
		n     int
	}
	var all []seen
	for k, v := range states {
		all = append(all, seen{k, v})
	}
	slices.SortFunc(all, func(a, b seen) int { return b.n - a.n })
	t.Logf("%d distinct screens", len(all))
	for _, x := range all[:min(len(all), 20)] {
		t.Logf("  %4d  %s", x.n, x.state)
	}
}

// After the program ends, keys must reach nothing. kitty --hold dropped into
// an interactive shell with the real PATH, and a random-key run typed into it;
// the runner now holds the window with sleep.
func TestLiveSandboxQuitReachesNoShell(t *testing.T) {
	s := startLiveSandboxed(t)
	s.Type("q")
	deadline := time.Now().Add(15 * time.Second)
	for {
		if _, ended := s.Exited(); ended {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("q did not end the program:\n%s", s.Text())
		}
		time.Sleep(200 * time.Millisecond)
	}
	marker := filepath.Join(s.dir, "shell-ran")
	s.typeText("touch " + marker)
	s.Key("enter")
	time.Sleep(2 * time.Second)
	if _, err := os.Stat(marker); err == nil {
		t.Fatalf("a command typed after the program ended was run by a shell")
	}
}

// deliberateOpeners are what opening an attachment starts -- a key the user
// presses for that purpose -- so a run logs them rather than failing on them.
var deliberateOpeners = map[string]bool{"xdg-open": true, "gio": true, "gnome-open": true, "kde-open": true, "open": true}

// unexpectedTools is the programs started that no key asked for by name.
func unexpectedTools(t *testing.T, tools []string) []string {
	var out []string
	for _, tool := range tools {
		name, _, _ := strings.Cut(tool, " ")
		if deliberateOpeners[name] {
			t.Logf("opened an attachment: %s", tool)
			continue
		}
		out = append(out, tool)
	}
	return out
}
