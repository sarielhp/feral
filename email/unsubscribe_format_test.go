package email

import (
	"os"
	"path/filepath"
	"testing"
)

// A damaged unsubscription log, or one a newer build wrote, is left alone. It
// used to be read as empty and the one new entry written over it.
func TestLogUnsubscriptionKeepsAnUnreadableLog(t *testing.T) {
	for name, content := range map[string]string{"damaged": `[{"sender":`, "newer": `{"format":2,"entries":[]}`} {
		dir := t.TempDir()
		logPath := filepath.Join(dir, "unsubscriptions.json")
		if err := os.WriteFile(logPath, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		LogUnsubscription(dir, "s@example.com", "http", "https://example.com/u", nil)
		if got, _ := os.ReadFile(logPath); string(got) != content {
			t.Errorf("%s: the log was overwritten: %s", name, got)
		}
	}
	dir := t.TempDir()
	LogUnsubscription(dir, "s@example.com", "http", "https://example.com/u", nil)
	if _, err := os.Stat(filepath.Join(dir, "unsubscriptions.json")); err != nil {
		t.Errorf("a new log was not started: %v", err)
	}
}
