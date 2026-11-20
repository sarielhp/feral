package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/feral/cfg_g"
)

// Under --read-only (and --dry-run, which implies it) migrate lists what it
// would copy and copies nothing. It used to run ssh and scp regardless,
// sending the config and every OAuth token to another machine.
func TestMigrateRefusesUnderReadOnly(t *testing.T) {
	bin := t.TempDir()
	calls := filepath.Join(t.TempDir(), "calls")
	for _, name := range []string{"ssh", "scp"} {
		script := "#!/bin/sh\necho " + name + " >> '" + calls + "'\n"
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	cfgDir := t.TempDir()
	for _, f := range []string{"config.json", "token_x.json"} {
		if err := os.WriteFile(filepath.Join(cfgDir, f), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	session := &Session{Out: &out, Config: &cfg_g.Config{ReadOnly: true, Paths: cfg_g.Paths{ConfigDir: cfgDir}}}

	if err := migrateRun(session, "someone@elsewhere"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(calls); err == nil {
		data, _ := os.ReadFile(calls)
		t.Fatalf("migrate ran %q under --read-only", strings.TrimSpace(string(data)))
	}
	if !strings.Contains(out.String(), "would copy 2 file(s)") || !strings.Contains(out.String(), "token_x.json") {
		t.Errorf("migrate did not say what it would copy:\n%s", out.String())
	}
}
