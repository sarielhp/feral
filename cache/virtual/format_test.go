package virtual

import (
	"os"
	"strings"
	"testing"

	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/email"
)

// A saved set is written with its format; one a newer build wrote is refused
// by Load and left out of List.
func TestMailboxFormat(t *testing.T) {
	dir := t.TempDir()
	acc := cachedir.At(dir)
	if err := Save(acc, &Mailbox{Name: "ok", MessageIDs: []email.ID{"a"}}); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(virtualPath(dir, "ok")); !strings.Contains(string(data), `"format":1`) {
		t.Errorf("not stamped: %s", data)
	}
	if err := os.WriteFile(virtualPath(dir, "new"), []byte(`{"format":2,"name":"new"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(acc, "new"); err == nil {
		t.Error("Load accepted a newer format")
	}
	sets, err := List(acc)
	if err != nil || len(sets) != 1 || sets[0].Name != "ok" {
		t.Errorf("List: %+v, %v", sets, err)
	}

}
