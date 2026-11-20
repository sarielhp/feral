package mbox

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// The locks an mbox file is changed under. Mail delivery agents (procmail,
// postfix, exim, dovecot) and readers (mutt) agree on two: a dotlock -- a
// file named <mbox>.lock created exclusively -- and an fcntl write lock on the
// mbox itself. cache/dirlock is flock only, which no MDA honors, so it would
// not stop a delivery from landing in the middle of a rewrite.
var (
	lockTimeout    = 30 * time.Second
	lockRetry      = 100 * time.Millisecond
	dotlockStaleAt = 10 * time.Minute
)

// pathLocks serializes this process's own access to each mbox file. fcntl
// locks belong to the process, and closing *any* descriptor of the file drops
// them all, so nothing else in the process may open and close the file while
// a rewrite holds its lock: IngestMbox takes the same mutex.
var pathLocks sync.Map // path -> *sync.Mutex

func pathMutex(path string) *sync.Mutex {
	mu, _ := pathLocks.LoadOrStore(path, &sync.Mutex{})
	return mu.(*sync.Mutex)
}

// lockedMbox is an mbox file open for reading and writing under both locks.
type lockedMbox struct {
	target  string // the file itself, a symlink resolved
	file    *os.File
	dotlock string // "" when the directory refused one
	mu      *sync.Mutex
}

// lockMbox opens path read-write and takes the dotlock and then the fcntl
// lock, waiting up to lockTimeout for each, in the order MDAs take them. A
// directory that will not hold a dotlock (a spool directory the user cannot
// write) leaves the fcntl lock alone to guard the file, as mutt does without
// setgid mail.
//
// key is the path as configured, which keys this process's mutex (IngestMbox
// takes it by the same name); target is the file it resolves to, beside which
// the dotlock goes, since that is where a delivery agent looks for it.
func lockMbox(key, target string) (*lockedMbox, error) {
	mu := pathMutex(key)
	mu.Lock()
	l := &lockedMbox{target: target, mu: mu}
	dot, err := takeDotlock(target + ".lock")
	if err != nil {
		mu.Unlock()
		return nil, err
	}
	l.dotlock = dot
	f, err := os.OpenFile(target, os.O_RDWR, 0)
	if err != nil {
		l.release()
		return nil, fmt.Errorf("open mbox %q for writing: %w", target, err)
	}
	l.file = f
	if err := waitFcntl(f); err != nil {
		l.release()
		return nil, fmt.Errorf("lock mbox %q: %w", target, err)
	}
	return l, nil
}

// release drops the fcntl lock (by closing the file), then the dotlock, then
// the process mutex.
func (l *lockedMbox) release() {
	if l.file != nil {
		_ = l.file.Close()
		l.file = nil
	}
	if l.dotlock != "" {
		if err := os.Remove(l.dotlock); err != nil && !errors.Is(err, os.ErrNotExist) {
			slog.Warn("mbox: could not remove dotlock", slog.String("path", l.dotlock), slog.Any("error", err))
		}
		l.dotlock = ""
	}
	if l.mu != nil {
		l.mu.Unlock()
		l.mu = nil
	}
}

// takeDotlock creates lockPath exclusively, retrying until lockTimeout. A
// dotlock older than dotlockStaleAt is a crashed holder's and is broken. It
// returns "" without error when the directory refuses the file (permission),
// and an error when another holder keeps it past the timeout.
func takeDotlock(lockPath string) (string, error) {
	deadline := time.Now().Add(lockTimeout)
	for {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			_, _ = f.WriteString(strconv.Itoa(os.Getpid()) + "\n")
			_ = f.Close()
			return lockPath, nil
		}
		if errors.Is(err, os.ErrPermission) {
			slog.Info("mbox: directory refuses a dotlock; relying on fcntl", slog.String("path", lockPath))
			return "", nil
		}
		if !errors.Is(err, os.ErrExist) {
			return "", fmt.Errorf("dotlock %q: %w", lockPath, err)
		}
		if fi, statErr := os.Stat(lockPath); statErr == nil && time.Since(fi.ModTime()) > dotlockStaleAt {
			slog.Warn("mbox: breaking a stale dotlock", slog.String("path", lockPath), slog.Time("since", fi.ModTime()))
			_ = os.Remove(lockPath)
			continue
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("dotlock %q is held by another program (mail delivery?); try again shortly", lockPath)
		}
		time.Sleep(lockRetry)
	}
}

// waitFcntl takes a whole-file fcntl write lock on f, retrying until
// lockTimeout rather than blocking forever on a stuck holder.
func waitFcntl(f *os.File) error {
	deadline := time.Now().Add(lockTimeout)
	lk := syscall.Flock_t{Type: syscall.F_WRLCK, Whence: 0, Start: 0, Len: 0}
	for {
		err := syscall.FcntlFlock(f.Fd(), syscall.F_SETLK, &lk)
		if err == nil {
			return nil
		}
		if !errors.Is(err, syscall.EAGAIN) && !errors.Is(err, syscall.EACCES) {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("held by another program (mail delivery?); try again shortly")
		}
		time.Sleep(lockRetry)
	}
}
