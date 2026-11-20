package cache

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/email"
)

// A read state or pending-reads ledger a newer build wrapped is refused by
// every update, and left as it was rather than overwritten.
func TestStateFilesRefuseWrappedFormat(t *testing.T) {
	for _, name := range []string{"seen_cache.json", "replied_cache.json", "pending_reads.json"} {
		dir := t.TempDir()
		acc := cachedir.At(dir)
		content := `{"format":2,"entries":{}}`
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		var err error
		switch name {
		case "replied_cache.json":
			err = MarkIDsReplied(acc, []email.ID{"x"}, nil)
		default:
			err = MarkIDsRead(acc, []email.ID{"x"}, nil)
		}
		if err == nil {
			t.Errorf("%s: an update went ahead over a newer file", name)
		}
		if got, _ := os.ReadFile(filepath.Join(dir, name)); string(got) != content {
			t.Errorf("%s: the file was overwritten", name)
		}
	}
}
