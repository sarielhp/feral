package dirlock

import (
	"errors"
	"os"
	"path/filepath"
)

// ErrHeld is TryLock's answer when another holder has the lock.
var ErrHeld = errors.New("lock held elsewhere")

// TryLock takes an exclusive advisory lock on path without waiting: it
// returns ErrHeld at once when another process -- or another TryLock in this
// one, since each call opens its own descriptor -- holds it. It is for "only
// one of us should do this" rather than for serializing writes, which is what
// Guard is for. The kernel drops the lock when the process dies, so a crash
// never leaves it held.
func TryLock(path string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := tryLockFile(f); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() {
		_ = unlockFile(f)
		_ = f.Close()
	}, nil
}
