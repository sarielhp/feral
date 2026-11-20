package outbox

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRelabelRenamesOnlyTheAccount(t *testing.T) {
	dir := t.TempDir()
	mine, err := Enqueue(dir, "old", nil, "mine", []byte("x"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Enqueue(dir, "other", nil, "theirs", []byte("y")); err != nil {
		t.Fatal(err)
	}
	for _, want := range []int{1, 0} { // the second run has nothing left to do
		if n, relErr := Relabel(dir, "OLD", "new"); n != want || relErr != nil {
			t.Fatalf("Relabel = %d, %v; want %d", n, relErr, want)
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, mine.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var got Entry
	if err := json.Unmarshal(data, &got); err != nil || got.Account != "new" || got.Subject != "mine" {
		t.Errorf("relabelled entry = %+v, %v", got, err)
	}
	entries, _ := List(dir)
	for _, e := range entries {
		if e.Subject == "theirs" && e.Account != "other" {
			t.Errorf("another account's entry was relabelled: %+v", e)
		}
	}
}
