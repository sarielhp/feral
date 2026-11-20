package interactive

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// A sandboxed session controls which programs feral can start. The binary
// runs with PATH set to one directory holding:
//
//   - the few tools it needs to work, linked to the real ones: w3m renders
//     HTML, bogofilter scores spam, gpg checks signatures and bat colours an
//     HTML or code part (it reads and prints, nothing more), each against
//     the private HOME's own (empty) state;
//   - for every other program it can be made to start -- an editor, a shell
//     behind the pipe command, a pager, an opener, a notifier, ssh, a
//     clipboard tool -- a stub that records its name and arguments and fails.
//
// feral starts every external program by name, never by absolute path, so
// nothing reaches past this directory. A test that wants no external program
// at all checks Tools() is empty.

// sandboxReal are linked to the real programs.
var sandboxReal = []string{"w3m", "bogofilter", "gpg", "bat", "batcat"}

// sandboxStubs record and refuse. The list covers every program feral
// starts by name (see the exec.Command calls) and the usual editors, pagers
// and openers a configuration could name.
var sandboxStubs = []string{
	"sh", "bash", "dash", "zsh", "fish",
	"editor", "sensible-editor", "vi", "vim", "nvim", "nano", "emacs", "emacsclient", "ed",
	"pager", "less", "more",
	"xdg-open", "open", "sensible-browser", "x-www-browser", "www-browser", "firefox",
	"firefox-esr", "google-chrome", "chromium", "brave", "lynx", "links",
	"notify-send", "ssh", "scp", "rsync", "curl", "wget",
	"xclip", "xsel", "wl-copy", "wl-paste", "mail", "sendmail", "msmtp",
}

// runner runs the program with the sandbox PATH, records how it ended, and
// then holds the window with sleep, so a panic stays on screen and a test can
// tell a quit from a crash. It used kitty --hold, which on this kitty drops
// into an interactive shell -- with the real PATH -- and a random-key run
// went on typing into it. Keys sent to sleep go nowhere.
const sandboxRunner = `#!/bin/sh
exitfile=$1
sandbox=$2
shift 2
PATH=$sandbox "$@"
echo $? > "$exitfile"
exec sleep 2147483647
`

// buildSandbox makes the sandbox directory and returns it.
func (s *session) buildSandbox() string {
	dir := filepath.Join(s.dir, "sandbox-bin")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		s.t.Fatal(err)
	}
	for _, name := range sandboxReal {
		path, err := exec.LookPath(name)
		if err != nil {
			continue // a missing optional tool only disables what needs it
		}
		if err := os.Symlink(path, filepath.Join(dir, name)); err != nil {
			s.t.Fatal(err)
		}
	}
	// ${0##*/} rather than basename: the stub runs on the sandbox PATH, where
	// basename is not, and every line came out blank.
	stub := "#!/bin/sh\nprintf '%s\\n' \"${0##*/} $*\" >> '" + s.toolLog + "'\nexit 1\n"
	for _, name := range sandboxStubs {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(stub), 0o700); err != nil {
			s.t.Fatal(err)
		}
	}
	return dir
}

// sandboxProgram is the kitty command line that runs bin in the sandbox; the
// runner holds the window when the program ends, so a panic stays readable.
func (s *session) sandboxProgram(bin string, args []string) []string {
	s.toolLog = filepath.Join(s.dir, "tools.log")
	s.exitFile = filepath.Join(s.dir, "exit.code")
	runner := filepath.Join(s.dir, "sandbox-run.sh")
	if err := os.WriteFile(runner, []byte(sandboxRunner), 0o700); err != nil {
		s.t.Fatal(err)
	}
	return append([]string{runner, s.exitFile, s.buildSandbox(), bin}, args...)
}

// Tools lists the stubbed programs feral tried to start, one per line.
func (s *session) Tools() []string {
	data, err := os.ReadFile(s.toolLog)
	if err != nil {
		return nil
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

// Exited reports whether the program has ended, and its exit status.
func (s *session) Exited() (string, bool) {
	data, err := os.ReadFile(s.exitFile)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(data)), true
}
