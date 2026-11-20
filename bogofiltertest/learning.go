package bogofiltertest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Learning is a stand-in bogofilter that learns: it calls every message ham
// while its spam registrations (-s) do not outnumber its unregistrations (-S),
// and spam otherwise. It records the flag of every call, so a test can count
// what was trained and taken back.
type Learning struct {
	log string
}

// InstallLearning installs a Learning bogofilter on the test's PATH. The
// script uses shell builtins only, since PATH holds nothing else.
func InstallLearning(t *testing.T) *Learning {
	t.Helper()
	dir := t.TempDir()
	l := &Learning{log: filepath.Join(dir, "calls")}
	count := filepath.Join(dir, "registered")
	if err := os.WriteFile(count, []byte("0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	Install(t, "#!/bin/sh\nwhile read -r _; do :; done\n"+
		"echo \"$1\" >> '"+l.log+"'\n"+
		"read -r n < '"+count+"'\n"+
		"case \"$1\" in\n"+
		"-s) echo $((n + 1)) > '"+count+"' ;;\n"+
		"-S) echo $((n - 1)) > '"+count+"' ;;\n"+
		"-v) if [ \"$n\" -gt 0 ]; then echo 'X-Bogosity: Spam, tests=bogofilter, spamicity=0.990000'; "+
		"else echo 'X-Bogosity: Ham, tests=bogofilter, spamicity=0.010000'; fi ;;\n"+
		"esac\nexit 0\n")
	return l
}

// Count is how many calls passed flag.
func (l *Learning) Count(t *testing.T, flag string) int {
	t.Helper()
	data, err := os.ReadFile(l.log)
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, line := range strings.Split(string(data), "\n") {
		if line == flag {
			n++
		}
	}
	return n
}
