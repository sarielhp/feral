package cfg_g

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/feral/fileformat"
)

func writeConfigFixture(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"accounts":[{"name":"acc","type":"gmail"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	accDir := filepath.Join(dir, "accounts", SanitizeLabelForCache("acc"))
	if err := os.MkdirAll(accDir, 0700); err != nil {
		t.Fatal(err)
	}
	return configPath, accDir
}

// A damaged white or black list, rules file or account.json stops the load and
// is left as it was. They used to load as empty, and the next config save --
// any of some thirty commands -- wrote the empty list back over them.
func TestLoadConfigFileRefusesDamagedAccountFiles(t *testing.T) {
	for _, name := range []string{"blacklist.json", "whitelist.json", "rules.json", "account.json"} {
		configPath, accDir := writeConfigFixture(t)
		damaged := []byte(`["spam@example.com", `)
		if err := os.WriteFile(filepath.Join(accDir, name), damaged, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadConfigFile(configPath); err == nil || !strings.Contains(err.Error(), "damaged") {
			t.Errorf("%s: LoadConfigFile = %v, want a refusal naming the damaged file", name, err)
		}
		if got, _ := os.ReadFile(filepath.Join(accDir, name)); string(got) != string(damaged) {
			t.Errorf("%s: the damaged file was changed", name)
		}
	}
}

// A file a newer build wrapped, or a config whose format is newer, is
// refused rather than misread.
func TestLoadConfigFileRefusesNewerFormats(t *testing.T) {
	configPath, accDir := writeConfigFixture(t)
	if err := os.WriteFile(filepath.Join(accDir, "blacklist.json"), []byte(`{"format":2,"list":["x"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	var tooNew *fileformat.TooNewError
	if _, err := LoadConfigFile(configPath); !errors.As(err, &tooNew) {
		t.Errorf("wrapped blacklist: %v", err)
	}

	configPath, accDir = writeConfigFixture(t)
	if err := os.WriteFile(filepath.Join(accDir, "account.json"), []byte(`{"name":"acc","type":"gmail","format":2}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfigFile(configPath); !errors.As(err, &tooNew) {
		t.Errorf("newer account.json: %v", err)
	}

	configPath, _ = writeConfigFixture(t)
	if err := os.WriteFile(configPath, []byte(`{"format":2,"accounts":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfigFile(configPath); !errors.As(err, &tooNew) {
		t.Errorf("newer config.json: %v", err)
	}
}

// Saving stamps config.json and account.json with the current format; the
// bare lists keep their shape.
func TestSaveConfigFileStampsFormat(t *testing.T) {
	configPath, accDir := writeConfigFixture(t)
	fc, err := LoadConfigFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	(*fc.Accounts)[0].Blacklist = []string{"spam@example.com"}
	if err := SaveConfigFile(configPath, fc); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{configPath, filepath.Join(accDir, "account.json")} {
		if data, _ := os.ReadFile(p); !strings.Contains(string(data), `"format": 1`) {
			t.Errorf("%s not stamped: %s", filepath.Base(p), data)
		}
	}
	if data, _ := os.ReadFile(filepath.Join(accDir, "blacklist.json")); !strings.HasPrefix(strings.TrimSpace(string(data)), "[") {
		t.Errorf("the black list changed shape: %s", data)
	}
	if again, err := LoadConfigFile(configPath); err != nil || (*again.Accounts)[0].Blacklist[0] != "spam@example.com" {
		t.Errorf("round trip: %v", err)
	}
}
