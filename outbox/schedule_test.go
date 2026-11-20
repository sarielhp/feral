package outbox

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/feral/fileformat"
)

// legacyEntry and legacyList are the outbox reader of every build before
// format 2, copied as it was: it reads the outbox directory's *.json files,
// checks no format, and its flush sends every entry it lists.
type legacyEntry struct {
	ID      string `json:"id"`
	Account string `json:"account"`
}

func legacyList(t *testing.T, dir string) []legacyEntry {
	t.Helper()
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []legacyEntry
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, f.Name()))
		if err != nil {
			continue
		}
		var item legacyEntry
		if json.Unmarshal(data, &item) != nil {
			continue
		}
		out = append(out, item)
	}
	return out
}

// An older build must never find a scheduled entry, or it would send it at
// once; an entry to be sent now stays exactly as older builds wrote it, so
// they still send it.
func TestOlderBuildsNeverSeeAScheduledEntry(t *testing.T) {
	dir := t.TempDir()
	later, err := EnqueueEntry(dir, Entry{Account: "work", Subject: "later",
		SendAfter: time.Now().Add(time.Hour)}, []byte("later body"))
	if err != nil {
		t.Fatal(err)
	}
	now, err := Enqueue(dir, "work", nil, "now", []byte("now body"))
	if err != nil {
		t.Fatal(err)
	}
	old := legacyList(t, dir)
	if len(old) != 1 || old[0].ID != now.ID {
		t.Fatalf("an older build lists %+v; want only %s, never the scheduled %s", old, now.ID, later.ID)
	}
	data, err := os.ReadFile(filepath.Join(dir, now.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "format") || strings.Contains(string(data), "send_after") {
		t.Errorf("an entry sent at once must stay format 1, as older builds wrote it:\n%s", data)
	}
	if later.Format != formatScheduled {
		t.Errorf("a scheduled entry is format %d, want %d", later.Format, formatScheduled)
	}
}

func TestScheduledEntryRoundTrips(t *testing.T) {
	dir := t.TempDir()
	due := time.Date(2026, 11, 21, 8, 0, 0, 0, time.UTC)
	e, err := EnqueueEntry(dir, Entry{Account: "work", To: []string{"a@example.com"}, Subject: "S", SendAfter: due}, []byte("body"))
	if err != nil {
		t.Fatal(err)
	}
	list, err := List(dir)
	if err != nil || len(list) != 1 {
		t.Fatalf("List = %+v, %v", list, err)
	}
	if !list[0].SendAfter.Equal(due) || list[0].Format != formatScheduled || list[0].TooNew() {
		t.Errorf("listed entry = %+v", list[0])
	}
	if list[0].Due(due.Add(-time.Second)) || !list[0].Due(due) {
		t.Errorf("Due is wrong around %v", due)
	}
	if err = RecordAttempt(dir, e.ID, errors.New("offline")); err != nil {
		t.Fatal(err)
	}
	if n, relErr := Relabel(dir, "work", "office"); n != 1 || relErr != nil {
		t.Fatalf("Relabel = %d, %v", n, relErr)
	}
	got, raw, err := Get(dir, e.ID)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "body" || got.Attempts != 1 || got.Account != "office" || !got.SendAfter.Equal(due) || got.Format != formatScheduled {
		t.Errorf("after an attempt and a relabel the entry is %+v, %q; it must keep its due time and format", got, raw)
	}
	if err := Remove(dir, e.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ = List(dir); len(list) != 0 {
		t.Errorf("Remove left %+v", list)
	}
	for _, ext := range []string{".json", ".eml"} {
		if _, err := os.Stat(filepath.Join(scheduledDir(dir), e.ID+ext)); !os.IsNotExist(err) {
			t.Errorf("Remove left %s%s", e.ID, ext)
		}
	}
}

// writeFutureEntry puts an entry in a format newer than this build's into the
// scheduled directory, with a payload, as a later build might.
func writeFutureEntry(t *testing.T, dir, id, meta string) {
	t.Helper()
	home := scheduledDir(dir)
	if err := os.MkdirAll(home, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, id+".json"), []byte(meta), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, id+".eml"), []byte("body"), 0600); err != nil {
		t.Fatal(err)
	}
}

// A newer build's entry is listed, so the user sees it, but refused by Get --
// which every send goes through -- and never rewritten.
func TestANewerFormatEntryIsListedAndRefused(t *testing.T) {
	cases := map[string]string{
		"readable": `{"id":"f3","account":"work","subject":"S","format":3,"send_after":"2026-11-21T08:00:00Z"}`,
		"reshaped": `{"id":"f3","account":{"name":"work"},"format":3}`,
	}
	for name, meta := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeFutureEntry(t, dir, "f3", meta)
			list, err := List(dir)
			if err != nil || len(list) != 1 || list[0].ID != "f3" || !list[0].TooNew() {
				t.Fatalf("List = %+v, %v; want the format-3 entry, marked too new", list, err)
			}
			var tooNew *fileformat.TooNewError
			if _, _, err := Get(dir, "f3"); !errors.As(err, &tooNew) {
				t.Errorf("Get: want TooNewError, got %v", err)
			}
			if err := RecordAttempt(dir, "f3", errors.New("x")); !errors.As(err, &tooNew) {
				t.Errorf("RecordAttempt: want TooNewError, got %v", err)
			}
			if _, err := Relabel(dir, "work", "office"); !errors.As(err, &tooNew) {
				t.Errorf("Relabel: want TooNewError, got %v", err)
			}
			data, _ := os.ReadFile(filepath.Join(scheduledDir(dir), "f3.json"))
			if string(data) != meta {
				t.Errorf("the newer entry was rewritten:\n%s", data)
			}
		})
	}
}
