package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sarielhp/clihelp"
)

// The symlinks behind links. A link's symlink sits beside the running binary
// and points at it by its base name (`mt -> feral`), as tools/install does
// for mail_cli; one placed elsewhere with --dir points at the binary's full
// path, since a bare name there would point at nothing.

// runningBinary is the running executable with its symlinks resolved.
func runningBinary() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("cannot find the running binary: %w", err)
	}
	if resolved, evalErr := filepath.EvalSymlinks(exe); evalErr == nil {
		exe = resolved
	}
	return exe, nil
}

// linkPlace is where a link's symlink goes and what it points at.
type linkPlace struct {
	dir    string // the directory holding the symlink
	path   string // the symlink
	exe    string // the binary it must resolve to
	target string // what the symlink holds
}

// linkPlacement is the symlink for name: beside the running binary, or in
// dir when one is given. A binary that `go run` or `go test` built is about
// to disappear, so without dir it is refused rather than linked to.
func linkPlacement(dir, name string) (linkPlace, error) {
	exe, err := runningBinary()
	if err != nil {
		return linkPlace{}, err
	}
	return linkPlacementFor(exe, dir, name)
}

// linkPlacementFor is linkPlacement for the binary exe.
func linkPlacementFor(exe, dir, name string) (linkPlace, error) {
	var err error
	place := linkPlace{exe: exe, dir: filepath.Dir(exe), target: filepath.Base(exe)}
	if dir == "" && strings.Contains(exe, string(filepath.Separator)+"go-build") {
		return linkPlace{}, fmt.Errorf("%w: %s was built by go run or go test and will be removed; "+
			"install feral, or give --dir", clihelp.ErrUsage, exe)
	}
	if dir != "" {
		if place.dir, err = filepath.Abs(dir); err != nil {
			return linkPlace{}, err
		}
		if place.dir != filepath.Dir(exe) {
			place.target = exe
		}
	}
	place.path = filepath.Join(place.dir, name)
	return place, nil
}

// check refuses what create would refuse, without writing: a directory at
// the path, always, and any other file that is not already this feral
// without force.
func (p linkPlace) check(force bool) error {
	info, err := os.Lstat(p.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory; a link cannot replace it", p.path)
	}
	if p.isThisProgram() || force {
		return nil
	}
	return fmt.Errorf("%w: %s exists and is not this feral; --force replaces it", clihelp.ErrUsage, p.path)
}

// isThisProgram reports whether the path, followed through its symlinks,
// is the binary.
func (p linkPlace) isThisProgram() bool {
	return sameFile(p.path, p.exe)
}

// create puts the symlink in place. One that already resolves to the binary
// is kept as it is. The new one is made beside the path and renamed over it,
// so a crash leaves either the old file or the new link.
func (p linkPlace) create() error {
	if p.isThisProgram() {
		return nil
	}
	if err := os.MkdirAll(p.dir, 0o755); err != nil {
		return err
	}
	stage := fmt.Sprintf("%s.link-%d", p.path, os.Getpid())
	_ = os.Remove(stage)
	if err := os.Symlink(p.target, stage); err != nil {
		return err
	}
	if err := os.Rename(stage, p.path); err != nil {
		_ = os.Remove(stage)
		return err
	}
	return nil
}

// remove deletes the symlink if it is a symlink to this feral; anything else
// is left, and reported unless it is simply not there.
func (p linkPlace) remove() (bool, error) {
	info, err := os.Lstat(p.path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeSymlink == 0 || !p.isThisProgram() {
		return false, errors.New("it is not a symlink to this feral")
	}
	return true, os.Remove(p.path)
}

// symlinkState is how name stands on this machine, for `link list`: dir is
// where its symlink would be, beside the binary.
func symlinkState(name, exe, dir string) string {
	if exe == "" {
		return "unknown"
	}
	if found, err := exec.LookPath(name); err == nil && sameFile(found, exe) {
		return "ok"
	}
	path := filepath.Join(dir, name)
	if _, err := os.Lstat(path); err != nil {
		return "no symlink"
	}
	if sameFile(path, exe) {
		return "not on PATH"
	}
	return "symlink elsewhere"
}

// sameFile reports whether a and b, following symlinks, are one file.
func sameFile(a, b string) bool {
	ai, err := os.Stat(a)
	if err != nil {
		return false
	}
	bi, err := os.Stat(b)
	return err == nil && os.SameFile(ai, bi)
}

// dirOnPath reports whether dir is one of $PATH's directories.
func dirOnPath(dir string) bool {
	if dir == "" {
		return false
	}
	for _, entry := range filepath.SplitList(os.Getenv("PATH")) {
		if entry != "" && sameFile(entry, dir) {
			return true
		}
	}
	return false
}

// integrationInstalled reports whether `completion install` has written a
// shell integration file for any shell.
func integrationInstalled(a *clihelp.App) bool {
	for _, shell := range []string{"bash", "zsh", "fish"} {
		path, err := clihelp.IntegrationPath(a, shell)
		if err != nil {
			continue
		}
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}
