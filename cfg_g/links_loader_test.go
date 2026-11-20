package cfg_g

import (
	"os"
	"path/filepath"
	"testing"
)

// loaderHome is a HOME whose config.json has two accounts, gmail with the
// alias "gmail" in its account.json -- a configuration from before links.
func loaderHome(t *testing.T) string {
	t.Helper()
	ForbidConfigWrites(false)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	configDir := filepath.Join(home, ".config", AppName)
	writeJSON(t, filepath.Join(configDir, "config.json"), map[string]any{
		"accounts": []map[string]string{{"name": "gmail", "type": "gmail"}, {"name": "work", "type": "jmap"}},
	})
	writeJSON(t, filepath.Join(AccountDir(configDir, "gmail"), "account.json"), map[string]any{
		"name": "gmail", "type": "gmail", "aliases": []string{"gmail"},
	})
	prev := ProgramNamesBesideBinary
	t.Cleanup(func() { ProgramNamesBesideBinary = prev })
	ProgramNamesBesideBinary = func() []string { return nil }
	return configDir
}

// withProgramName runs the loader as if the binary were started as name.
func withProgramName(t *testing.T, name string) {
	t.Helper()
	prev := os.Args[0]
	t.Cleanup(func() { os.Args[0] = prev })
	os.Args[0] = filepath.Join("/somewhere", name)
}

func TestLoaderMigratesAliasesOnce(t *testing.T) {
	configDir := loaderHome(t)
	withProgramName(t, "gmail")

	// --read-only: computed in memory, written nowhere, and the alias still
	// selects its account by program name.
	config, err := LoadConfigNoWrite()
	if err != nil {
		t.Fatal(err)
	}
	if _, present, _ := ReadLinks(configDir); present {
		t.Fatal("a load that may not write wrote links")
	}
	if config.LinksDeclared || config.Links["gmail"].Account != "gmail" || config.SelectedAccount != "gmail" {
		t.Fatalf("no-write load: declared=%v links=%v selected=%q", config.LinksDeclared, config.Links, config.SelectedAccount)
	}

	// The first run that may write migrates, and still selects by alias.
	config, err = LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	links, present, err := ReadLinks(configDir)
	if err != nil || !present || links["gmail"].Account != "gmail" {
		t.Fatalf("after the migrating load: links=%v present=%v err=%v", links, present, err)
	}
	if config.LinksDeclared || config.SelectedAccount != "gmail" {
		t.Errorf("the migrating run: declared=%v selected=%q", config.LinksDeclared, config.SelectedAccount)
	}

	// After it, the link selects (through its preset), not the alias.
	config, err = LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !config.LinksDeclared || config.SelectedAccount != "" {
		t.Errorf("after the migration: declared=%v selected=%q", config.LinksDeclared, config.SelectedAccount)
	}
}

// A removed link is not brought back: an empty links is the migration's
// record, not something to fill again.
func TestLoaderKeepsAnEmptyLinks(t *testing.T) {
	configDir := loaderHome(t)
	path := filepath.Join(configDir, "config.json")
	if _, err := LoadConfig(); err != nil {
		t.Fatal(err)
	}
	fc, err := LoadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}
	*fc.Links = map[string]Link{}
	if err = SaveConfigFile(path, fc); err != nil {
		t.Fatal(err)
	}
	if _, err = LoadConfig(); err != nil {
		t.Fatal(err)
	}
	if links, present, _ := ReadLinks(configDir); !present || len(links) != 0 {
		t.Errorf("links after a load = %v (present %v), want the empty object kept", links, present)
	}
}
