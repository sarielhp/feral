package tui

import (
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime/debug"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/feral/testenv"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/cache/folderindex"
	"github.com/sarielhp/feral/cache/msg"
	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/mailclient"
)

// The TUI as a state machine: screen.go names its screens and how to tell
// which one is in front; this file reaches every screen by the keys a person
// would press, then presses random keys from there, checking after every
// message that the model is in a state it can be in (stateProblems) and that
// neither Update nor View panics.
//
// It runs in-process against a mock account, so it is fast enough for the
// gate and safe by construction: PATH is an empty directory, so nothing the
// model starts -- an editor, a browser, a shell behind the pipe prompt -- can
// run, and HOME, the config directory and the cache are temporary.

// fsmPaths is the transition table: the keys that reach each screen from the
// message list a fresh model opens on.
var fsmPaths = map[screen][]string{
	scrList:           {},
	scrMessage:        {"enter"},
	scrListSearch:     {"/"},
	scrMessageSearch:  {"enter", "/"},
	scrFolderTree:     {"f"},
	scrFolderSearch:   {"f", "/"},
	scrPipePrompt:     {"|"},
	scrTagPattern:     {"T"},
	scrTagPrefix:      {";"},
	scrSaveName:       {"t", ";", "v"},
	scrMovePrompt:     {"t", ";", "a"},
	scrSnoozePrompt:   {"H"},
	scrRemoteSearch:   {"ctrl+f"},
	scrQuitConfirm:    {"esc"},
	scrHelp:           {"f1"},
	scrHelpSearch:     {"f1", "/"},
	scrCompose:        {"c"},
	scrSaveToFolder:   {"s"},
	scrMenu:           {"m"},
	scrSpamMenu:       {"f5"},
	scrDiag:           {"ctrl+d"},
	scrDeadlines:      {"D"},
	scrBlock:          {"B"},
	scrUnsubscribe:    {"X"},
	scrAliases:        {"f6"},
	scrAccounts:       {"f3"},
	scrSort:           {"o"},
	scrSavedSearches:  {"f7"},
	scrRecent:         {"f8"},
	scrAttachmentList: {"/", "a", "t", "t", "a", "c", "h", "enter", "v"},
}

// fsmKeys is what the random walk presses. In-process nothing can escape, so
// it includes the keys the kitty monkey leaves out: compose, reply, the pipe
// prompt, sign-in and quit.
var fsmKeys = func() []string {
	keys := []string{
		"up", "down", "left", "right", "enter", "esc", "tab", "pgup", "pgdown", "home", "end",
		"backspace", " ", "shift+left", "shift+right", "f1", "f2", "f3", "f4", "f5", "f6", "f7",
		"f8", "ctrl+a", "ctrl+u", "ctrl+d", "ctrl+f", "ctrl+q", "ctrl+c",
	}
	for _, r := range "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789/~!;+-?|.,=שלום" {
		keys = append(keys, string(r))
	}
	return keys
}()

func fsmKey(name string) tea.KeyMsg {
	named := map[string]tea.KeyType{
		"up": tea.KeyUp, "down": tea.KeyDown, "left": tea.KeyLeft, "right": tea.KeyRight,
		"enter": tea.KeyEnter, "esc": tea.KeyEsc, "tab": tea.KeyTab, "pgup": tea.KeyPgUp,
		"pgdown": tea.KeyPgDown, "home": tea.KeyHome, "end": tea.KeyEnd, "backspace": tea.KeyBackspace,
		" ": tea.KeySpace, "shift+left": tea.KeyShiftLeft, "shift+right": tea.KeyShiftRight,
		"f1": tea.KeyF1, "f2": tea.KeyF2, "f3": tea.KeyF3, "f4": tea.KeyF4, "f5": tea.KeyF5,
		"f6": tea.KeyF6, "f7": tea.KeyF7, "f8": tea.KeyF8, "ctrl+a": tea.KeyCtrlA, "ctrl+u": tea.KeyCtrlU,
		"ctrl+d": tea.KeyCtrlD, "ctrl+f": tea.KeyCtrlF, "ctrl+q": tea.KeyCtrlQ, "ctrl+c": tea.KeyCtrlC,
	}
	if kt, ok := named[name]; ok {
		return tea.KeyMsg{Type: kt}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(name)}
}

// fsmMailbox is the mock account: nested folders, a thread, a Hebrew subject,
// HTML, and a message with a PDF and a calendar invitation.
type fsmMailbox struct {
	client *mailclient.MockMailClient
	dir    string
}

func fsmRaw(i int, subject, extra, body string) []byte {
	date := time.Date(2026, 1, 1+i%28, 9, i%60, 0, 0, time.UTC).Format(time.RFC1123Z)
	return []byte(fmt.Sprintf("Message-ID: <m%d@fsm.example>\r\nDate: %s\r\nFrom: Sender %d <s%d@fsm.example>\r\n"+
		"To: Owner <owner@fsm.example>\r\nSubject: %s\r\n%sMIME-Version: 1.0\r\n", i, date, i%7, i%7, subject, extra) + body)
}

func fsmAttachmentBody() string {
	pdf := base64.StdEncoding.EncodeToString([]byte("%PDF-1.4 fake"))
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:fsm-1@fsm.example\r\nSUMMARY:FSM meeting\r\n" +
		"DTSTART:20260901T140000Z\r\nDTEND:20260901T150000Z\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	return "Content-Type: multipart/mixed; boundary=B\r\n\r\n--B\r\nContent-Type: text/plain\r\n\r\nSee attached.\r\n" +
		"--B\r\nContent-Type: application/pdf; name=r.pdf\r\nContent-Disposition: attachment; filename=r.pdf\r\n" +
		"Content-Transfer-Encoding: base64\r\n\r\n" + pdf + "\r\n--B\r\nContent-Type: text/calendar; method=REQUEST; name=i.ics\r\n" +
		"Content-Disposition: attachment; filename=i.ics\r\n\r\n" + ics + "--B--\r\n"
}

// fsmUnsubscribeHeaders are the ways a list asks to be left -- one-click, a
// mailto, a web link -- so X opens its dialog on most messages and a walk
// that presses y reaches each method (the POST is replaced in fsmIsolate).
var fsmUnsubscribeHeaders = []string{
	"List-Unsubscribe: <https://lists.fsm.example/u?t=1>\r\nList-Unsubscribe-Post: List-Unsubscribe=One-Click\r\n",
	"List-Unsubscribe: <mailto:leave@lists.fsm.example?subject=unsubscribe>\r\n",
	"List-Unsubscribe: <https://lists.fsm.example/manage>\r\n",
}

func newFSMMailbox(t *testing.T) *fsmMailbox {
	t.Helper()
	dir := t.TempDir()
	account := cachedir.At(dir)
	folders := map[string][]int{"INBOX": nil, "Archive/2025/Q1": nil, "Projects/Alpha": nil, "Spam": nil}
	raws := map[email.ID][]byte{}
	for i := 0; i < 40; i++ {
		folder := []string{"INBOX", "INBOX", "INBOX", "Archive/2025/Q1", "Projects/Alpha", "Spam"}[i%6]
		subject, extra, body := fmt.Sprintf("Note %d about item %d", i, i*7), "", "Content-Type: text/plain\r\n\r\nBody of note.\r\n"
		switch i {
		case 0:
			subject, body = "Attachments inside", fsmAttachmentBody()
		case 1:
			subject = "=?utf-8?B?" + base64.StdEncoding.EncodeToString([]byte("שלום עולם mixed")) + "?="
		case 2:
			body = "Content-Type: text/html\r\n\r\n<p>Hello <a href=\"https://fsm.example/x\">link</a></p>\r\n"
		case 6, 12:
			subject, extra = "Re: Note 0 about item 0", "In-Reply-To: <m0@fsm.example>\r\nReferences: <m0@fsm.example>\r\n"
		default:
			extra = fsmUnsubscribeHeaders[i%len(fsmUnsubscribeHeaders)]
		}
		id := email.ID(fmt.Sprintf("m%d", i))
		raws[id] = fsmRaw(i, subject, extra, body)
		if err := msg.Store(account, id, raws[id], time.Now()); err != nil {
			t.Fatal(err)
		}
		folders[folder] = append(folders[folder], i)
	}
	client := &mailclient.MockMailClient{RawMessages: raws, DownloadedIDs: map[string][]email.ID{}}
	for folder, nums := range folders {
		ids := make([]email.ID, 0, len(nums))
		for _, n := range nums {
			ids = append(ids, email.ID(fmt.Sprintf("m%d", n)))
		}
		if err := folderindex.ReplaceAll(account, folder, ids); err != nil {
			t.Fatal(err)
		}
		client.DownloadedIDs[folder] = ids
	}
	for _, f := range []string{"INBOX", "Archive", "Archive/2025", "Archive/2025/Q1", "Projects", "Projects/Alpha", "Spam", "Sent", "Drafts"} {
		client.Labels = append(client.Labels, f)
		client.LabelItems = append(client.LabelItems, cfg_acc.Item{Name: f[strings.LastIndex(f, "/")+1:], FullName: f, IsLabel: true})
	}
	return &fsmMailbox{client: client, dir: dir}
}

// fsmFrames is the frame View must record for each drawn screen: the mouse
// reads clicks against it, so a stale one sends a click to a screen that is
// not there.
var fsmFrames = map[screen]string{
	scrList: frameBase, scrMessage: frameBase, scrLoading: frameLoading, scrHelp: frameHelp, scrHelpSearch: frameHelp,
	scrAliases: frameAliases, scrCompose: frameCompose, scrContacts: frameContacts,
	scrAccounts: frameAccount, scrDeadlines: frameDeadlines, scrFileBrowser: frameFiles,
	scrSaveToFolder: frameTree, scrAttachmentList: frameAttList, scrICS: frameICS,
	scrAttachment: frameAttModal, scrAttachmentHelp: frameAttHelp, scrSummary: frameSummary,
	scrDiag: frameDiag, scrSavedSearches: frameVirtual, scrRecent: frameRecent,
	scrFolderTree: frameTree, scrBlock: frameBlock, scrUnsubscribe: frameUnsub, scrSort: frameSort, scrSpamMenu: frameSpam,
	scrQuitConfirm: frameQuit, scrSaveHelp: frameSaveHelp,
}

// fsmDriver feeds a model keys and the messages its commands produce.
type fsmDriver struct {
	t     *testing.T
	box   *fsmMailbox
	m     *tuiModel
	trail []string
	seen  map[string][]string // problem -> the first trail that showed it
	msgs  int                 // messages delivered for the current key
}

func newFSMDriver(t *testing.T, box *fsmMailbox) *fsmDriver {
	d := &fsmDriver{t: t, box: box, seen: map[string][]string{}}
	d.reset()
	return d
}

// reset starts a fresh program on the mailbox.
func (d *fsmDriver) reset() {
	cfgDir := d.t.TempDir()
	cfg := &cfg_g.Config{
		Paths:        cfg_g.Paths{CacheRoot: d.box.dir, ConfigDir: cfgDir},
		AccountCache: cachedir.At(d.box.dir),
		Prefs:        cfg_g.Prefs{Browser: "/nonexistent/browser", Editor: "/nonexistent/editor"},
		Accounts:     []cfg_acc.AccountConfig{{Name: "fsm", Type: "imap"}},
	}
	d.box.client.Cfg = cfg
	// The folders resolve, so archive, delete and their undo (z) act on the
	// mock instead of failing before they start.
	d.m = newTuiModel(d.box.client, "INBOX", &Backend{Folders: stubFolders{
		ArchiveFn: fixedFolder("Archive"), TrashFn: fixedFolder("Trash"), SpamFn: fixedFolder("Spam")}})
	d.m.width, d.m.height = 120, 40
	d.trail = nil
	d.deliver(tea.WindowSizeMsg{Width: 120, Height: 40})
	d.run(d.m.Init(), 0)
	d.run(d.m.loadEmailsCmd("INBOX"), 0)
}

// press sends one key, delivers what follows from it, and checks the state.
// In a file browser a path is being typed, so the keys that could leave the
// test directory -- an absolute path, "..", the home directory -- become a
// plain letter: a walk once saved messages into the repository through "../".
func (d *fsmDriver) press(key string) {
	if s := d.m.keyScreen(); (s == scrFileBrowser || s == scrSaveToDisk) && strings.ContainsAny(key, "/.~") {
		key = "x"
	}
	d.trail = append(d.trail, key)
	d.msgs = 0
	d.deliver(fsmKey(key))
}

// click sends a mouse event at a random cell: a left click (press and
// release), a right click or a wheel turn.
func (d *fsmDriver) click(rng *rand.Rand) {
	x, y := rng.IntN(d.m.width), rng.IntN(d.m.height)
	d.msgs = 0
	switch rng.IntN(4) {
	case 0:
		d.trail = append(d.trail, fmt.Sprintf("wheel@%d,%d", x, y))
		d.deliver(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	case 1:
		d.trail = append(d.trail, fmt.Sprintf("right@%d,%d", x, y))
		d.deliver(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonRight, Action: tea.MouseActionPress})
	default:
		d.trail = append(d.trail, fmt.Sprintf("click@%d,%d", x, y))
		d.deliver(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		d.deliver(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})
	}
}

func (d *fsmDriver) deliver(message tea.Msg) {
	if d.m.done {
		return
	}
	d.msgs++
	if d.msgs > 60 {
		return // a command chain that keeps producing; enough for one key
	}
	model, cmd := d.safely(func() (tea.Model, tea.Cmd) { return d.m.Update(message) })
	if tm, ok := model.(*tuiModel); ok {
		d.m = tm
	}
	d.safely(func() (tea.Model, tea.Cmd) { d.m.View(); return nil, nil })
	problems := d.m.stateProblems()
	if want, ok := fsmFrames[d.m.drawnScreen()]; ok && d.m.frameKind != want {
		problems = append(problems, fmt.Sprintf("%s is drawn but clicks are read as %q", d.m.drawnScreen(), d.m.frameKind))
	}
	for _, p := range problems {
		if _, ok := d.seen[p]; !ok {
			d.seen[p] = append([]string(nil), d.trail...)
		}
	}
	d.run(cmd, 0)
}

// safely runs f, turning a panic into a recorded problem.
func (d *fsmDriver) safely(f func() (tea.Model, tea.Cmd)) (model tea.Model, cmd tea.Cmd) {
	defer func() {
		if r := recover(); r != nil {
			p := fmt.Sprintf("panic: %v", r)
			if _, ok := d.seen[p]; !ok {
				d.seen[p] = append(append([]string(nil), d.trail...), "\n"+string(debug.Stack()))
			}
			model, cmd = d.m, nil
		}
	}()
	return f()
}

// run executes a command and delivers its message; a batch or sequence is
// unpacked. A command still running after a moment -- a tick, a timer -- is
// left behind: its message would arrive after the next key anyway.
func (d *fsmDriver) run(cmd tea.Cmd, depth int) {
	if cmd == nil || depth > 6 || d.m.done {
		return
	}
	done := make(chan tea.Msg, 1)
	go func() {
		defer func() { _ = recover() }()
		done <- cmd()
	}()
	var message tea.Msg
	select {
	case message = <-done:
	case <-time.After(150 * time.Millisecond):
		return
	}
	if message == nil {
		return
	}
	if batch, ok := message.(tea.BatchMsg); ok {
		for _, c := range batch {
			d.run(c, depth+1)
		}
		return
	}
	if v := reflect.ValueOf(message); v.Kind() == reflect.Slice && v.Type().Elem() == reflect.TypeOf(tea.Cmd(nil)) {
		for i := 0; i < v.Len(); i++ { // tea.Sequence's unexported message
			c, _ := v.Index(i).Interface().(tea.Cmd)
			d.run(c, depth+1)
		}
		return
	}
	d.deliver(message)
}

// fsmIsolate keeps the model from reaching anything outside the test. An
// external program the model hands the terminal to -- the editor, a pipe
// command, a sign-in -- is recorded and reported as having succeeded without
// running, which is what lets a walk pass through compose.
func fsmIsolate(t *testing.T) {
	guardWorkingTree(t)
	t.Chdir(t.TempDir())
	previous := runProgram
	runProgram = func(c *exec.Cmd, fn tea.ExecCallback) tea.Cmd {
		// An editor is handed a file; write a line into it, as a person would.
		if last := c.Args[len(c.Args)-1]; strings.HasSuffix(c.Path, "editor") {
			if f, err := os.OpenFile(last, os.O_APPEND|os.O_WRONLY, 0); err == nil {
				_, _ = f.WriteString("\nTyped in the editor.\n")
				_ = f.Close()
			}
		}
		return func() tea.Msg { return fn(nil) }
	}
	t.Cleanup(func() { runProgram = previous })
	previousPost := unsubscribePost
	unsubscribePost = func(string) error { return nil }
	t.Cleanup(func() { unsubscribePost = previousPost })
	logger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(func() { slog.SetDefault(logger) })
	t.Setenv("PATH", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("EDITOR", "/nonexistent/editor")
	t.Setenv("VISUAL", "/nonexistent/editor")
	t.Setenv("BROWSER", "/nonexistent/browser")
}

// guardWorkingTree fails the test if anything new appears in the package
// directory or the repository root while it runs: a random walk that saved a
// message to disk once left a hundred files there.
func guardWorkingTree(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dirs := []string{wd, filepath.Dir(wd)}
	before := map[string]bool{}
	for _, dir := range dirs {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			before[filepath.Join(dir, e.Name())] = true
		}
	}
	t.Cleanup(func() {
		for _, dir := range dirs {
			entries, _ := os.ReadDir(dir)
			for _, e := range entries {
				if p := filepath.Join(dir, e.Name()); !before[p] {
					t.Errorf("the walk created %s outside its temporary directory", p)
				}
			}
		}
	})
}

// Every screen in the table is reached by its keys, so the table -- and the
// random walks that start from it -- describe the program as it is.
func TestFSM_EveryScreenReachable(t *testing.T) {
	fsmIsolate(t)
	box := newFSMMailbox(t)
	for target, path := range fsmPaths {
		d := newFSMDriver(t, box)
		for _, k := range path {
			d.press(k)
		}
		if got := d.m.keyScreen(); got != target {
			t.Errorf("%s: keys %q reached %s", target, path, got)
		}
	}
}

// From every screen, random keys never leave the model in a state it cannot
// be in, and nothing panics.
func TestFSM_RandomWalkFromEveryScreen(t *testing.T) {
	fsmIsolate(t)
	box := newFSMMailbox(t)
	steps := 150
	if n, err := fmt.Sscan(testenv.Getenv(t, "FERAL_FSM_STEPS"), &steps); n != 1 || err != nil {
		steps = 150
	}
	seed := uint64(time.Now().UnixNano())
	if n, _ := fmt.Sscan(testenv.Getenv(t, "FERAL_FSM_SEED"), &seed); n == 1 {
		t.Logf("seed %d", seed)
	}
	rng := rand.New(rand.NewPCG(seed, seed>>1))
	seen := map[string][]string{}
	visited := map[screen]int{}
	for target, path := range fsmPaths {
		d := newFSMDriver(t, box)
		for _, k := range path {
			d.press(k)
		}
		for i := 0; i < steps; i++ {
			if d.m.done {
				d.reset()
			}
			if rng.IntN(8) == 0 {
				d.click(rng)
			} else {
				d.press(fsmKeys[rng.IntN(len(fsmKeys))])
			}
			visited[d.m.keyScreen()]++
		}
		for p, trail := range d.seen {
			if _, ok := seen[p]; !ok {
				seen[p] = append([]string{"from " + string(target) + ":"}, trail...)
			}
		}
	}
	t.Logf("seed %d; screens visited: %v", seed, visited)
	problems := make([]string, 0, len(seen))
	for p := range seen {
		problems = append(problems, p)
	}
	sort.Strings(problems)
	for _, p := range problems {
		trail := seen[p]
		t.Errorf("%s\n    after %q", p, trail[max(0, len(trail)-25):])
	}
}

// z after E puts the archived message back where the walk found it, and the
// model is in a state it can be in after each key.
func TestFSM_UndoArchive(t *testing.T) {
	fsmIsolate(t)
	box := newFSMMailbox(t)
	d := newFSMDriver(t, box)
	first := d.m.selectedID
	moves := len(box.client.MovedEmails)
	d.press("E")
	d.press("z")
	got := box.client.MovedEmails[moves:]
	if len(got) != 2 || got[1].SourceLabel != "Archive" || got[1].DestLabel != "INBOX" ||
		len(got[1].MessageIDs) != 1 || got[1].MessageIDs[0] != first {
		t.Fatalf("E then z moved %+v; want %s to Archive and back", got, first)
	}
	if d.m.saveStatus != "Undone: Archived 1" {
		t.Errorf("status %q", d.m.saveStatus)
	}
	for p, trail := range d.seen {
		t.Errorf("%s after %q", p, trail)
	}
}
