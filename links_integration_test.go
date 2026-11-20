package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The program, run as the shell runs it: under a name on $PATH that is a
// symlink to this binary. The test binary stands in for feral -- run under
// the helper below it calls main() with the words it was given -- so the
// test needs no `go build`, and the binary's base name is the symlink's, as
// it is for feral and its links.

const linkHelperArgs = "FERAL_LINK_HELPER_ARGS"

// TestLinkProgramHelper is main() with the arguments from linkHelperArgs. It
// does nothing in an ordinary run.
func TestLinkProgramHelper(t *testing.T) {
	words, ok := os.LookupEnv(linkHelperArgs)
	if !ok {
		t.Skip("run by TestLinksEndToEnd")
	}
	var args []string
	if words != "" {
		args = strings.Split(words, "\x1f")
	}
	os.Args = append([]string{os.Args[0]}, args...)
	main()
}

// linkWorld is a scratch HOME with a PATH of one directory, binDir, where
// "feral" is a symlink to the test binary.
type linkWorld struct {
	home, binDir, exe string
	env               []string
}

func newLinkWorld(t *testing.T) linkWorld {
	t.Helper()
	home := linkTestHome(t, `{"accounts": [{"name": "acct", "type": "jmap"}], "links": {}}`)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	w := linkWorld{home: home, binDir: filepath.Join(home, "bin"), exe: exe}
	if err := os.MkdirAll(w.binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(exe, filepath.Join(w.binDir, "feral")); err != nil {
		t.Fatal(err)
	}
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		switch key {
		case "PATH", "SHELL", "CLIHELP_NO_AUTO_COMPLETION", "TERM", linkHelperArgs:
			continue
		}
		w.env = append(w.env, kv)
	}
	w.env = append(w.env, "PATH="+w.binDir, "SHELL=/bin/bash", "TERM=dumb")
	return w
}

// run runs name from binDir with args, and returns what it printed.
func (w linkWorld) run(t *testing.T, name string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(filepath.Join(w.binDir, name), "-test.run=^TestLinkProgramHelper$")
	cmd.Env = append(append([]string{}, w.env...), linkHelperArgs+"="+strings.Join(args, "\x1f"))
	cmd.Dir = w.home
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return out.String(), err
}

func (w linkWorld) mustRun(t *testing.T, name string, args ...string) string {
	t.Helper()
	out, err := w.run(t, name, args...)
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return out
}

var namesField = regexp.MustCompile(`names=([0-9a-f]+)`)

func TestLinksEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the program in a subprocess")
	}
	w := newLinkWorld(t)
	w.mustRun(t, "feral", "completion", "install")
	integration := filepath.Join(w.home, ".config", "feral", "shell", "bash")
	before, err := os.ReadFile(integration)
	if err != nil {
		t.Fatalf("completion install wrote no bash integration: %v", err)
	}

	out := w.mustRun(t, "feral", "link", "add", "mt", "--dir", w.binDir, "--", "-A", "acct", "tui")
	if !strings.Contains(out, "mt runs: feral -A acct tui") {
		t.Errorf("link add did not say what mt runs:\n%s", out)
	}
	if !sameTarget(filepath.Join(w.binDir, "mt"), w.exe) {
		t.Fatal("link add made no symlink to the program")
	}

	if out = w.mustRun(t, "mt", "--version"); !strings.Contains(out, "feral version") {
		t.Errorf("mt --version:\n%s", out)
	}
	if out = w.mustRun(t, "mt", "-h"); !strings.HasPrefix(out, "mt runs: feral -A acct tui") {
		t.Errorf("mt -h does not open with what it runs:\n%s", out)
	}
	words := map[string]bool{}
	for _, line := range strings.Split(w.mustRun(t, "mt", "__complete", ""), "\n") {
		words[strings.SplitN(line, "\t", 2)[0]] = true
	}
	if !words["write"] || words["scan"] {
		t.Errorf("mt completes as something other than tui: %v", words)
	}

	after, err := os.ReadFile(integration)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(after), "mt") || namesOf(before) == namesOf(after) {
		t.Errorf("the integration file was not refreshed for mt (names %s -> %s)", namesOf(before), namesOf(after))
	}
	if out = w.mustRun(t, "feral", "link", "list"); !strings.Contains(out, "ok") || !strings.Contains(out, "feral -A acct tui") {
		t.Errorf("link list:\n%s", out)
	}

	w.mustRun(t, "feral", "link", "rm", "mt", "--dir", w.binDir)
	if _, statErr := os.Lstat(filepath.Join(w.binDir, "mt")); !os.IsNotExist(statErr) {
		t.Error("link rm left the symlink")
	}
	final, err := os.ReadFile(integration)
	if err != nil {
		t.Fatal(err)
	}
	if namesOf(final) != namesOf(before) {
		t.Errorf("after link rm the integration names %s, want %s again", namesOf(final), namesOf(before))
	}
}

func namesOf(integration []byte) string {
	m := namesField.FindSubmatch(integration)
	if m == nil {
		return ""
	}
	return string(m[1])
}

func sameTarget(a, b string) bool {
	ai, err := os.Stat(a)
	if err != nil {
		return false
	}
	bi, err := os.Stat(b)
	return err == nil && os.SameFile(ai, bi)
}
