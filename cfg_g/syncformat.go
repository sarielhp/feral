package cfg_g

import (
	"fmt"
	"strconv"
	"strings"
)

// The configuration export (plans/config_sync.md) carries a format version
// of its own, in a file named SyncFormatFile at the top of sync_dir.
//
// The export is decoded into the fields this build knows. A machine running
// an older feral would import an export from a newer one, drop the fields it
// does not know and export without them -- which the merge reads as a
// deletion and carries back to the newer machine. So a build refuses to sync
// with a directory whose format is newer than its own, and a build exporting
// raises the directory's format to its own.
//
// SyncFormat is raised whenever the exported shape changes; a field added to
// what is exported counts. It is separate from fileformat.Current, which
// versions the live files: the export is a different file set and changes
// for different reasons.

// SyncFormat is the export format this build reads and writes.
const SyncFormat = 1

// SyncFormatFile is the file in sync_dir that holds the export's format.
const SyncFormatFile = "format"

// SyncFormatTooNewError is the refusal to sync with an export a newer feral
// wrote. Its text is the one line a run reports before going on without
// sync.
type SyncFormatTooNewError struct {
	Found int
}

func (e *SyncFormatTooNewError) Error() string {
	return fmt.Sprintf("config sync skipped: sync_dir was written by a newer feral (format %d, this is %d); upgrade feral on this machine",
		e.Found, SyncFormat)
}

// ParseSyncFormat reads the content of SyncFormatFile: one positive integer,
// surrounding white space allowed.
func ParseSyncFormat(data []byte) (int, error) {
	text := strings.TrimSpace(string(data))
	n, err := strconv.Atoi(text)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("%s holds %q, not a format number", SyncFormatFile, text)
	}
	return n, nil
}

// CheckSyncFormat refuses an export in a format newer than SyncFormat. An
// older one is accepted: exporting writes SyncFormat over it.
func CheckSyncFormat(found int) error {
	if found > SyncFormat {
		return &SyncFormatTooNewError{Found: found}
	}
	return nil
}

// SyncFormatBytes is what an export writes to SyncFormatFile.
func SyncFormatBytes() []byte {
	return []byte(strconv.Itoa(SyncFormat) + "\n")
}
