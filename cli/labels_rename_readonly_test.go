package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/mailclient"
)

// snapshotDir reads every file under dir.
func snapshotDir(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		data, err := os.ReadFile(p)
		out[p] = string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// Under --read-only the rename is simulated, so the rules must keep the old
// name. The command used to rewrite them anyway and save the config, leaving
// every rule aimed at a label that did not exist.
func TestLabelRenameSavesNoRulesUnderReadOnly(t *testing.T) {
	cfgDir := t.TempDir()
	path := filepath.Join(cfgDir, "config.json")
	accounts := []cfg_acc.AccountConfig{{Name: "a", Type: "gmail", Username: "a@example.com",
		Rules: []cfg_acc.Rule{{Sender: "news@example.com", Label: "Old"}}}}
	if err := cfg_g.SaveConfigFile(path, &cfg_g.FileConfig{Accounts: &accounts}); err != nil {
		t.Fatal(err)
	}
	before := snapshotDir(t, cfgDir)

	client := &mailclient.MockMailClient{Labels: []string{"Old"}, Cfg: &cfg_g.Config{}}
	session := &Session{
		Config: &cfg_g.Config{ReadOnly: true, Paths: cfg_g.Paths{ConfigDir: cfgDir}, Accounts: accounts, SelectedAccount: "a"},
		Deps: Deps{Clients: stubClients{ForSelectedFn: func(*cfg_g.Config) (mailclient.MailClient, error) {
			return mailclient.NewReadOnlyMailClient(client, "a"), nil
		}}},
	}
	if _, err := captureOutput(func() error { return InitCLI(session).Execute([]string{"label", "rename", "Old", "New"}) }); err != nil {
		t.Fatal(err)
	}
	after := snapshotDir(t, cfgDir)
	for p, data := range before {
		if after[p] != data {
			t.Errorf("%s changed under --read-only", p)
		}
	}
	if len(after) != len(before) {
		t.Errorf("files under the config directory went from %d to %d", len(before), len(after))
	}
}
