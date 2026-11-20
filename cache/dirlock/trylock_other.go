//go:build !unix && !windows

package dirlock

import "os"

func tryLockFile(_ *os.File) error {
	return nil // no advisory locks here: every caller gets the lock
}
