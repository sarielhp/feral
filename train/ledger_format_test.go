package train

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/sarielhp/feral/fileformat"
)

// A ledger a newer build wrapped is refused, never read as "nothing trained"
// -- which would retrain the corpus -- and left as it was.
func TestOpenRefusesWrappedLedger(t *testing.T) {
	p := filepath.Join(t.TempDir(), "trained_uids.json")
	content := `{"format":2,"ids":["a"]}`
	if err := os.WriteFile(p, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	var tooNew *fileformat.TooNewError
	if _, err := Open(p); !errors.As(err, &tooNew) {
		t.Errorf("Open = %v, want TooNewError", err)
	}
	if got, _ := os.ReadFile(p); string(got) != content {
		t.Error("the ledger was changed")
	}
	for _, shape := range []string{`["a","b"]`, `{"a":true,"b":true}`} {
		if err := os.WriteFile(p, []byte(shape), 0600); err != nil {
			t.Fatal(err)
		}
		if l, err := Open(p); err != nil || l.Len() != 2 {
			t.Errorf("historical shape %s: %v", shape, err)
		}
	}
}
