package cfg_g

import (
	"errors"
	"testing"
)

// An export from a newer feral is refused with the line the run reports; the
// same format and an older one are accepted, an older one to be raised on
// export.
func TestCheckSyncFormat(t *testing.T) {
	for _, found := range []int{1, SyncFormat} {
		if err := CheckSyncFormat(found); err != nil {
			t.Errorf("format %d refused: %v", found, err)
		}
	}
	err := CheckSyncFormat(SyncFormat + 6)
	var tooNew *SyncFormatTooNewError
	if !errors.As(err, &tooNew) || tooNew.Found != SyncFormat+6 {
		t.Fatalf("a newer format was not refused: %v", err)
	}
	want := "config sync skipped: sync_dir was written by a newer feral (format 7, this is 1); upgrade feral on this machine"
	if SyncFormat == 1 && err.Error() != want {
		t.Errorf("got %q\nwant %q", err.Error(), want)
	}
}

// The format file holds one number; what an export writes reads back as
// SyncFormat, and anything else is an error, never a format.
func TestParseSyncFormat(t *testing.T) {
	got, err := ParseSyncFormat(SyncFormatBytes())
	if err != nil || got != SyncFormat {
		t.Errorf("round trip: %d, %v", got, err)
	}
	if got, err := ParseSyncFormat([]byte("  7\n")); err != nil || got != 7 {
		t.Errorf("\"  7\\n\": %d, %v", got, err)
	}
	for _, bad := range []string{"", "seven", "0", "-1", "1.5", "1 2"} {
		if _, err := ParseSyncFormat([]byte(bad)); err == nil {
			t.Errorf("%q was read as a format", bad)
		}
	}
}
