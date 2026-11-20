package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// The drafts commands fall back to $HOME/.cache/feral/drafts when a
// session has no download directory. No test may reach the developer's real
// cache that way, so the whole package runs with HOME in a scratch directory.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp(testTempRoot(), "feral_cli_home_")
	if err != nil {
		fmt.Fprintln(os.Stderr, "cannot create a scratch HOME:", err)
		os.Exit(1)
	}
	os.Setenv("HOME", home)
	// clihelp writes shell integration under XDG_CONFIG_HOME before HOME, and
	// `link add` and `completion install` reach it: keep both in the scratch home.
	os.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	os.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	os.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	os.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	// write, send and reply read a redirected stdin as the body; under a
	// gate that pipes stdin, that would feed or hang them.
	if null, err := os.Open(os.DevNull); err == nil {
		os.Stdin = null
	}
	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}
