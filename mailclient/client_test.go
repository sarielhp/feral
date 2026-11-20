package mailclient

import (
	"errors"
	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/label"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/sarielhp/feral/cache"
	"github.com/sarielhp/feral/cache/folderindex"
	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
)

type mockFailingCacheStore struct {
	saveErr error
	saved   bool
}

func (m *mockFailingCacheStore) GetLabelItems() ([]cfg_acc.Item, error) {
	return nil, errors.New("no cache")
}

func (m *mockFailingCacheStore) SaveLabelItems(_ []cfg_acc.Item) error {
	m.saved = true
	return m.saveErr
}

func (m *mockFailingCacheStore) CachedLabelItemsAge() (time.Duration, error) {
	return 0, errors.New("no cache")
}

func (m *mockFailingCacheStore) ForgetLabelItems() error { return nil }

func (m *mockFailingCacheStore) IsActive() bool {
	return true
}

func TestCheckingMailClient_GetLabelItems_SaveError(t *testing.T) {
	expectedItems := []cfg_acc.Item{
		{Name: "INBOX", FullName: "INBOX", MessagesUnread: 2, MessagesTotal: 10},
	}
	mockBackend := &MockMailClient{
		Cfg:        &cfg_g.Config{},
		LabelItems: expectedItems,
	}
	store := &mockFailingCacheStore{
		saveErr: errors.New("disk full or unwritable"),
	}
	client := NewCheckingMailClient(mockBackend, "test-account", store)

	items, err := client.GetLabelItems()
	if err != nil {
		t.Fatalf("expected GetLabelItems to succeed despite save error, got: %v", err)
	}
	if !store.saved {
		t.Fatal("expected SaveLabelItems to be called")
	}
	if len(items) != len(expectedItems) {
		t.Fatalf("got %d items, want %d", len(items), len(expectedItems))
	}
}

// The embedded MailClient carries every call this decorator does not handle, so
// it must be the same client as Delegate. A struct literal that sets only
// Delegate compiles and then panics on the first unhandled method.
func TestNewCheckingMailClientWiresBothReferences(t *testing.T) {
	mock := &MockMailClient{}
	c := NewCheckingMailClient(mock, "acc", &cache.NoOpCacheStore{})

	if c.MailClient == nil {
		t.Fatal("embedded MailClient was not set")
	}
	if c.MailClient != c.Delegate {
		t.Error("embedded MailClient and Delegate are different clients")
	}
}

// An unhandled call reaches the backend through the embedding.
func TestCheckingMailClientForwardsUnhandledCalls(t *testing.T) {
	mock := &MockMailClient{Labels: []string{"INBOX"}}
	c := NewCheckingMailClient(mock, "acc", &cache.NoOpCacheStore{})

	if got := c.BackendType(); got != mock.BackendType() {
		t.Errorf("BackendType = %q, want the delegate's %q", got, mock.BackendType())
	}
}

// assertReceivedFolderRejected checks that Validate rejects a config whose
// received_folder is "inbox", with want as the exact error text; an account
// name, when known, is named in the message.
func assertReceivedFolderRejected(t *testing.T, account, want string) {
	t.Helper()
	mockBackend := &MockMailClient{
		Cfg: &cfg_g.Config{
			Policy: cfg_g.Policy{ReceivedFolder: "inbox"},
		},
	}
	client := NewCheckingMailClient(mockBackend, account, &cache.NoOpCacheStore{})

	err := client.Validate()
	if err == nil {
		t.Fatal("expected error when received_folder is inbox, got nil")
	}
	if err.Error() != want {
		t.Errorf("got error %q, want %q", err.Error(), want)
	}
}

func TestCheckingMailClient_FixReceivedFolder_ErrorWithAccount(t *testing.T) {
	want := `received_folder cannot be inbox for account "personal"; run 'feral config check --fix' to repair it automatically, or edit received_folder in config.json`
	assertReceivedFolderRejected(t, "personal", want)
}

func TestCheckingMailClient_FixReceivedFolder_ErrorFallbackWithoutAccount(t *testing.T) {
	want := "received_folder cannot be inbox; run 'feral config check --fix' to repair it automatically, or edit received_folder in config.json"
	assertReceivedFolderRejected(t, "", want)
}

func TestCheckingMailClient_FixReceivedFolder_SelectedAccountFallback(t *testing.T) {
	mockBackend := &MockMailClient{
		Cfg: &cfg_g.Config{
			SelectedAccount: "work",
			Policy:          cfg_g.Policy{ReceivedFolder: "inbox"},
		},
	}
	client := NewCheckingMailClient(mockBackend, "", &cache.NoOpCacheStore{})

	err := client.Validate()
	if err == nil {
		t.Fatal("expected error when received_folder is inbox, got nil")
	}
	want := `received_folder cannot be inbox for account "work"; run 'feral config check --fix' to repair it automatically, or edit received_folder in config.json`
	if err.Error() != want {
		t.Errorf("got error %q, want %q", err.Error(), want)
	}
}

// receivedFolderFixCase is one auto-fix scenario: a config file holding a
// single account whose received_folder is initial and must become expected.
type receivedFolderFixCase struct {
	account  string
	accType  string
	initial  string
	expected string
}

// assertReceivedFolderAutoFix seeds the case's config file, runs Validate with
// FixConfig set, and checks received_folder is rewritten in memory and on disk.
func assertReceivedFolderAutoFix(t *testing.T, c receivedFolderFixCase) {
	t.Helper()
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	fc := &cfg_g.FileConfig{
		Accounts: &[]cfg_acc.AccountConfig{
			{Name: c.account, Type: c.accType, ReceivedFolder: c.initial},
		},
	}
	if err := cfg_g.SaveConfigFile(configPath, fc); err != nil {
		t.Fatalf("failed to save initial test config: %v", err)
	}

	cfg := &cfg_g.Config{
		SelectedAccount: cfg_g.AccountName(c.account),
		FixConfig:       true,
		Paths:           cfg_g.Paths{ConfigDir: tmpDir},
		Policy:          cfg_g.Policy{ReceivedFolder: c.initial},
	}
	mockBackend := &MockMailClient{Cfg: cfg}
	client := NewCheckingMailClient(mockBackend, c.account, &cache.NoOpCacheStore{})

	if err := client.Validate(); err != nil {
		t.Fatalf("expected Validate() with FixConfig to succeed, got: %v", err)
	}

	if cfg.ReceivedFolder != c.expected {
		t.Errorf("in-memory ReceivedFolder = %q, want %q", cfg.ReceivedFolder, c.expected)
	}

	reloaded, err := cfg_g.LoadConfigFile(configPath)
	if err != nil {
		t.Fatalf("failed to reload config: %v", err)
	}
	if len(*reloaded.Accounts) == 0 || (*reloaded.Accounts)[0].ReceivedFolder != c.expected {
		t.Errorf("persisted ReceivedFolder = %q, want %q", (*reloaded.Accounts)[0].ReceivedFolder, c.expected)
	}
}

func TestCheckingMailClient_FixReceivedFolder_AutoFix_Gmail(t *testing.T) {
	assertReceivedFolderAutoFix(t, receivedFolderFixCase{
		account:  "personal",
		accType:  "gmail",
		initial:  "inbox",
		expected: "received",
	})
}

func TestCheckingMailClient_FixReceivedFolder_AutoFix_Outlook(t *testing.T) {
	assertReceivedFolderAutoFix(t, receivedFolderFixCase{
		account:  "work-outlook",
		accType:  "outlook",
		initial:  "Inbox",
		expected: "Archive",
	})
}

func TestCheckingMailClient_FixReceivedFolder_ReadOnly_SkipsFix(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	fc := &cfg_g.FileConfig{
		Accounts: &[]cfg_acc.AccountConfig{
			{Name: "personal", Type: "gmail", ReceivedFolder: "inbox"},
		},
	}
	if err := cfg_g.SaveConfigFile(configPath, fc); err != nil {
		t.Fatalf("failed to save initial test config: %v", err)
	}

	cfg := &cfg_g.Config{
		SelectedAccount: "personal",
		FixConfig:       true,
		ReadOnly:        true,
		Paths:           cfg_g.Paths{ConfigDir: tmpDir},
		Policy:          cfg_g.Policy{ReceivedFolder: "inbox"},
	}
	mockBackend := &MockMailClient{Cfg: cfg}
	client := NewCheckingMailClient(mockBackend, "personal", &cache.NoOpCacheStore{})

	if err := client.Validate(); err != nil {
		t.Fatalf("expected Validate() under ReadOnly to skip and succeed, got: %v", err)
	}

	if cfg.ReceivedFolder != "inbox" {
		t.Errorf("in-memory ReceivedFolder changed under ReadOnly to %q", cfg.ReceivedFolder)
	}

	reloaded, err := cfg_g.LoadConfigFile(configPath)
	if err != nil {
		t.Fatalf("failed to reload config: %v", err)
	}
	if len(*reloaded.Accounts) == 0 || (*reloaded.Accounts)[0].ReceivedFolder != "inbox" {
		t.Errorf("persisted ReceivedFolder changed under ReadOnly to %q", (*reloaded.Accounts)[0].ReceivedFolder)
	}
}

// TestCheckingMailClient_MoveEmail_ReindexesAndRespectsReadOnly is the
// unit-level coverage for reindexMoved's own ReadOnly gate, now the only
// place that gate lives: cli/splice.go, cli/split.go and cli/msg.go used to
// each reindex a move manually (redundantly duplicating this exact logic,
// once with the wrong ReadOnly source), and were simplified to rely on this
// method once it was confirmed to already do the same work correctly.
func TestCheckingMailClient_MoveEmail_ReindexesAndRespectsReadOnly(t *testing.T) {
	t.Run("reindexes on a real move", func(t *testing.T) {
		dir := t.TempDir()
		if err := folderindex.Add(cachedir.At(dir), "INBOX", "msg1"); err != nil {
			t.Fatalf("folderindex.Add: %v", err)
		}
		cfg := &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: dir}, AccountCache: cachedir.At(dir)}
		client := NewCheckingMailClient(&MockMailClient{Cfg: cfg}, "acc1", &cache.NoOpCacheStore{})

		if err := client.MoveEmail([]email.ID{"msg1"}, label.FromServer("INBOX"), label.FromServer("Archive")); err != nil {
			t.Fatalf("MoveEmail: %v", err)
		}

		archiveIDs, err := folderindex.IDs(cachedir.At(dir), "Archive")
		if err != nil || !slices.Contains(archiveIDs, "msg1") {
			t.Errorf("expected msg1 reindexed into Archive, got %v (err %v)", archiveIDs, err)
		}
	})

	t.Run("a read-only client's config does not reindex", func(t *testing.T) {
		dir := t.TempDir()
		if err := folderindex.Add(cachedir.At(dir), "INBOX", "msg1"); err != nil {
			t.Fatalf("folderindex.Add: %v", err)
		}
		// The account itself is read-only (as mailops.BuildAccountLocalConfig
		// sets it from cfg_acc.AccountConfig.ReadOnly), independent of any
		// global --read-only/--dry-run flag.
		cfg := &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: dir}, AccountCache: cachedir.At(dir), ReadOnly: true}
		client := NewCheckingMailClient(&MockMailClient{Cfg: cfg}, "acc1", &cache.NoOpCacheStore{})

		if err := client.MoveEmail([]email.ID{"msg1"}, label.FromServer("INBOX"), label.FromServer("Archive")); err != nil {
			t.Fatalf("MoveEmail: %v", err)
		}

		archiveIDs, _ := folderindex.IDs(cachedir.At(dir), "Archive")
		if slices.Contains(archiveIDs, "msg1") {
			t.Errorf("expected msg1 NOT reindexed into Archive under ReadOnly, got %v", archiveIDs)
		}
		inboxIDs, err := folderindex.IDs(cachedir.At(dir), "INBOX")
		if err != nil || !slices.Contains(inboxIDs, "msg1") {
			t.Errorf("expected msg1 to remain in INBOX under ReadOnly, got %v (err %v)", inboxIDs, err)
		}
	})
}

type mockItemStore struct {
	cache.NoOpCacheStore
	items []cfg_acc.Item
	err   error
	age   time.Duration
}

func (m *mockItemStore) CachedLabelItemsAge() (time.Duration, error) {
	if m.err != nil {
		return 0, m.err
	}
	return m.age, nil
}

func (m *mockItemStore) GetLabelItems() ([]cfg_acc.Item, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.items, nil
}

func TestCheckingMailClient_GetMatchingLabels(t *testing.T) {
	t.Run("returns from cache store when present", func(t *testing.T) {
		mockBackend := &MockMailClient{
			Labels: []string{"ServerOnly"},
		}
		store := &mockItemStore{
			items: []cfg_acc.Item{
				{Name: "INBOX", FullName: "INBOX"},
				{Name: "Sent", FullName: "Sent Items"},
				{Name: "Archive", FullName: "Archive"},
			},
		}
		client := NewCheckingMailClient(mockBackend, "test-acc", store)

		matched, err := client.GetMatchingLabels("sent")
		if err != nil {
			t.Fatalf("GetMatchingLabels failed: %v", err)
		}
		if len(matched) != 1 || matched[0] != "Sent Items" {
			t.Errorf("GetMatchingLabels = %v, want [\"Sent Items\"]", matched)
		}
	})

	t.Run("delegates when cache store has error or empty", func(t *testing.T) {
		mockBackend := &MockMailClient{
			Labels: []string{"INBOX", "Sent", "Trash"},
		}
		store := &mockItemStore{err: errors.New("not found")}
		client := NewCheckingMailClient(mockBackend, "test-acc", store)

		matched, err := client.GetMatchingLabels("in")
		if err != nil {
			t.Fatalf("GetMatchingLabels failed: %v", err)
		}
		if len(matched) != 1 || matched[0] != "INBOX" {
			t.Errorf("GetMatchingLabels = %v, want [\"INBOX\"]", matched)
		}
	})
	// A stale cache, or one that predates a new label, must not answer "absent".
	for _, tc := range []struct {
		name string
		age  time.Duration
	}{
		{"delegates when the cache is stale", 48 * time.Hour},
		{"delegates when the cache has no match", time.Hour},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mockBackend := &MockMailClient{Labels: []string{"NewLabel"}}
			store := &mockItemStore{items: []cfg_acc.Item{{Name: "NewLabel2", FullName: "Old"}}, age: tc.age}
			if tc.age > maxLabelCacheAge {
				store.items = []cfg_acc.Item{{Name: "NewLabel", FullName: "NewLabel"}}
			}
			client := NewCheckingMailClient(mockBackend, "test-acc", store)
			matched, err := client.GetMatchingLabels("New")
			if err != nil {
				t.Fatalf("GetMatchingLabels failed: %v", err)
			}
			if len(matched) != 1 || matched[0] != "NewLabel" {
				t.Errorf("GetMatchingLabels = %v, want the backend's [\"NewLabel\"]", matched)
			}
		})
	}

	t.Run("sorts cached matches", func(t *testing.T) {
		store := &mockItemStore{items: []cfg_acc.Item{{FullName: "b/2"}, {FullName: "b/1"}}}
		client := NewCheckingMailClient(&MockMailClient{}, "test-acc", store)
		matched, _ := client.GetMatchingLabels("b")
		if len(matched) != 2 || matched[0] != "b/1" {
			t.Errorf("GetMatchingLabels = %v, want sorted", matched)
		}
	})
}

// A folder change drops the cached listing. It used to stay for up to a day,
// so `label create X` followed by `label rm %X` reported "no label matches":
// the lookup read a listing taken before X existed.
func TestCheckingMailClient_FolderChangesForgetCachedListing(t *testing.T) {
	changes := map[string]func(c *CheckingMailClient) error{
		"EnsureLabelExists": func(c *CheckingMailClient) error { return c.EnsureLabelExists(label.FromServer("New")) },
		"DeleteLabel":       func(c *CheckingMailClient) error { return c.DeleteLabel(label.FromServer("Old")) },
		"RenameLabel": func(c *CheckingMailClient) error {
			return c.RenameLabel(label.FromServer("Old"), label.FromServer("New"))
		},
		"FixLabels": func(c *CheckingMailClient) error { return c.FixLabels() },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			backend := &MockMailClient{
				Labels:     []string{"INBOX", "Old"},
				LabelItems: []cfg_acc.Item{{Name: "INBOX", FullName: "INBOX"}, {Name: "Old", FullName: "Old"}},
			}
			store := &cache.DiskCacheStore{Dir: cachedir.At(t.TempDir())}
			client := NewCheckingMailClient(backend, "test-acc", store)
			if _, err := client.GetLabelItems(); err != nil { // caches INBOX, Old
				t.Fatal(err)
			}
			backend.Labels = []string{"INBOX", "New"} // the server after the change
			if err := change(client); err != nil {
				t.Fatal(err)
			}
			all, err := client.GetMatchingLabels("")
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Contains(all, "New") || slices.Contains(all, "Old") {
				t.Errorf("listing after %s = %v, want the server's [INBOX New]", name, all)
			}
		})
	}
}
