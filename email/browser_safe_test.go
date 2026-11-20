package email

import (
	"strings"
	"testing"
)

// A message opened in the browser loads nothing remote and runs no script:
// the sender's HTML used to be written verbatim, so a tracking pixel reported
// the open and scripts ran.
func TestForBrowser(t *testing.T) {
	for _, tc := range []struct {
		name, in string
	}{
		{"with doctype", "<!DOCTYPE html>\n<html><head><title>x</title></head><body><img src=\"https://t.example/p.gif\"></body></html>"},
		{"no doctype", "<html><body><p>hi</p><script>alert(1)</script></body></html>"},
		{"fragment", "<p>Just a fragment</p>"},
		{"meta refresh", `<html><head><META HTTP-EQUIV="Refresh" content="0;url=https://t.example/opened"></head><body>x</body></html>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := ForBrowser(tc.in)
			policy := strings.Index(out, `http-equiv="Content-Security-Policy"`)
			if policy < 0 || !strings.Contains(out, "default-src 'none'") {
				t.Fatalf("no policy:\n%s", out)
			}
			if doctype := strings.Index(strings.ToLower(out), "<!doctype"); doctype > policy {
				t.Errorf("the policy comes before the doctype, which puts the page in quirks mode:\n%s", out)
			}
			if strings.Contains(strings.ToLower(out), "refresh") {
				t.Errorf("a meta refresh survived:\n%s", out)
			}
			if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(out)), "<!doctype") && strings.Contains(strings.ToLower(tc.in), "<!doctype") {
				t.Errorf("the doctype is no longer first:\n%s", out)
			}
		})
	}
}
