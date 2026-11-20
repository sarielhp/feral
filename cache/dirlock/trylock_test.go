//go:build unix || windows

package dirlock

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestTryLockIsExclusiveUntilReleased(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "x.lock")
	release, err := TryLock(path)
	if err != nil {
		t.Fatalf("first TryLock: %v", err)
	}
	if _, again := TryLock(path); !errors.Is(again, ErrHeld) {
		t.Fatalf("second TryLock while held = %v, want ErrHeld", again)
	}
	release()
	again, err := TryLock(path)
	if err != nil {
		t.Fatalf("TryLock after release: %v", err)
	}
	again()
}
