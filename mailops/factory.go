package mailops

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/sarielhp/feral/app"
	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/cache/meta"
	"github.com/sarielhp/feral/email"

	"github.com/sarielhp/feral/backend/gmail"
	"github.com/sarielhp/feral/backend/jmap"
	"github.com/sarielhp/feral/backend/mbox"
	"github.com/sarielhp/feral/backend/outlook"
	"github.com/sarielhp/feral/cache"
	"github.com/sarielhp/feral/cache/folderindex"
	"github.com/sarielhp/feral/cache/virtual"
	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/mailclient"
	"github.com/sarielhp/feral/report"
)

// BuildAccountLocalConfig creates a per-account configuration struct.
func BuildAccountLocalConfig(acc cfg_acc.AccountConfig, config *cfg_g.Config) cfg_g.Config {
	localCfg := *config
	if acc.Username != "" {
		localCfg.Username = acc.Username
	}
	if acc.Password != "" {
		localCfg.Password = acc.Password
	}
	if acc.IMAPHost != "" {
		localCfg.IMAPHost = acc.IMAPHost
	}
	if acc.SpamFolder != "" {
		localCfg.SpamFolder = acc.SpamFolder
	}
	if acc.ReceivedFolder != "" {
		localCfg.ReceivedFolder = acc.ReceivedFolder
	}
	// SpamLearn and UnspamLearn are per-account settings in the config file, and
	// were the only such overrides this function did not carry. So scan/scan.go
	// moved blacklisted mail to the *global* folder while cfg_g/helpers.go filed
	// the matching rule against the *account's* — two paths disagreeing about
	// where the same message belongs, in a program that may hold several accounts
	// with different folder names.
	if acc.SpamLearn != "" {
		localCfg.SpamLearn = acc.SpamLearn
	}
	if acc.UnspamLearn != "" {
		localCfg.UnspamLearn = acc.UnspamLearn
	}
	localCfg.SelectedAccount = cfg_g.NameOf(acc)
	localCfg.AccountType = acc.AccountType
	if localCfg.AccountType == "" {
		localCfg.AccountType = "regular"
	}
	localCfg.Rules = []cfg_acc.Rule{}
	if acc.Rules != nil {
		localCfg.Rules = acc.Rules
	}
	localCfg.Whitelist = []string{}
	if acc.Whitelist != nil {
		localCfg.Whitelist = acc.Whitelist
	}
	localCfg.Blacklist = []string{}
	if acc.Blacklist != nil {
		localCfg.Blacklist = acc.Blacklist
	}
	// The account's cache directory, beside the root it lives under. These
	// used to be one field that this function rewrote from the root to the
	// account directory, so code holding the session's config read the root
	// where it meant the account.
	localCfg.AccountCache = config.AccountCacheFor(acc)
	if acc.ReadOnly {
		localCfg.ReadOnly = true
	}
	return localCfg
}

// InstantiateBackendClient instantiates the underlying MailClient for the given
// account type. rep is where that backend reports progress; a nil rep discards.
func InstantiateBackendClient(acc cfg_acc.AccountConfig, localCfg *cfg_g.Config, rep report.Reporter) (mailclient.MailClient, error) {
	switch strings.ToLower(acc.Type) {
	case "gmail":
		return gmail.NewClient(acc, localCfg, rep), nil
	case "jmap":
		return jmap.NewClient(acc, localCfg, rep), nil
	case "outlook":
		return outlook.NewClient(acc, localCfg, rep), nil
	case "mbox":
		path, err := acc.MboxPath()
		if err != nil {
			return nil, err
		}
		return mbox.NewClient(acc, localCfg, path), nil
	default:
		return nil, fmt.Errorf("unsupported account type: %q", acc.Type)
	}
}

// IsAccountSelected checks if the account matches the currently active account.
func IsAccountSelected(acc cfg_acc.AccountConfig, config *cfg_g.Config) bool {
	selected, err := cfg_g.Lookup(config.Accounts, string(config.SelectedAccount))
	if err != nil {
		return false
	}
	return strings.EqualFold(selected.Name, acc.Name)
}

// migrateAccountCacheLayout is H4's cache-layout migration for what lives
// inside one account's (already-migrated) cache directory: the folder
// indexes named by the account's own cached label listing, and every
// self-describing virtual mailbox. It is the step that takes a cache with no
// manifest to version 1 (see cache/meta), so it runs once per cache rather
// than on every start. A missing or unreadable label cache just means there
// is nothing yet to migrate; a rename that fails is returned, and the cache
// is left unrecorded so the next start tries again.
func migrateAccountCacheLayout(downloadDir cachedir.Account) error {
	if downloadDir.IsZero() {
		return nil
	}

	store := &cache.DiskCacheStore{Dir: downloadDir}
	if items, err := store.GetLabelItems(); err == nil && len(items) > 0 {
		names := make([]string, 0, len(items))
		for _, item := range items {
			name := item.FullName
			if name == "" {
				name = item.Name
			}
			if name != "" {
				names = append(names, name)
			}
		}
		if _, err := folderindex.MigrateSanitizedNames(downloadDir, names); err != nil {
			slog.Debug("folder index sanitization migration failed",
				slog.String("dir", downloadDir.Path()), slog.Any("error", err))
			return err
		}
	}

	if _, err := virtual.MigrateSanitizedNames(downloadDir); err != nil {
		slog.Debug("virtual mailbox sanitization migration failed",
			slog.String("dir", downloadDir.Path()), slog.Any("error", err))
		return err
	}
	return nil
}

// openAccountCache is the manifest check every client is built behind: it
// refuses a cache this build does not understand, and records the manifest
// of one that has none, running the legacy layout step first.
func openAccountCache(acct cachedir.Account, backend string, config *cfg_g.Config) error {
	_, err := meta.Open(acct, backend, meta.Options{
		DryRun:       config.DryRun,
		Build:        app.Version,
		LegacyLayout: func() error { return migrateAccountCacheLayout(acct) },
	})
	return err
}

// NewMailClient constructs a fully initialized and wrapped MailClient for the
// account, reporting its progress through rep.
func NewMailClient(acc cfg_acc.AccountConfig, config *cfg_g.Config, rep report.Reporter) (mailclient.MailClient, error) {
	// The account's own cache directory has to be migrated before
	// BuildAccountLocalConfig computes localCfg.AccountCache from it (H4):
	// that computation joins config.CacheRoot with the *current*
	// sanitization of acc.Name, which is exactly the path this call moves
	// the pre-H4 directory to if it is still sitting under the old one.
	cfg_g.MigrateDirSanitization(config.CacheRoot, acc.Name)
	localCfg := BuildAccountLocalConfig(acc, config)
	if err := openAccountCache(localCfg.AccountCache, acc.Type, &localCfg); err != nil {
		return nil, fmt.Errorf("account %s: %w", acc.Name, err)
	}

	client, err := InstantiateBackendClient(acc, &localCfg, rep)
	if err != nil {
		return nil, err
	}

	if localCfg.ReadOnly || acc.ReadOnly {
		client = mailclient.NewReadOnlyMailClient(client, acc.Name)
	}

	// Every account caches into its own directory -- BuildAccountLocalConfig
	// gives each one config.CacheRoot/<account> -- so there is nothing for one
	// to collide with in another, and dirlock guards concurrent writers. A
	// non-selected account used to get a no-op store, which CheckingMailClient
	// then read as "this client is inactive" and used to short-circuit every
	// fetch to an empty result.
	cs := &cache.DiskCacheStore{Dir: localCfg.AccountCache}

	return mailclient.NewCheckingMailClient(client, acc.Name, cs), nil
}

// The three places a command has to name a specific backend. They live here,
// behind the factory, so nothing above has to import gmail or outlook.

// TokenPath is where an account's OAuth token lives, or "" for a backend that
// has none.
func TokenPath(configDir string, acc cfg_acc.AccountConfig) (string, error) {
	switch strings.ToLower(acc.Type) {
	case "gmail":
		return gmail.GetTokenPath(&cfg_g.Config{
			SelectedAccount: cfg_g.NameOf(acc),
			Paths:           cfg_g.Paths{ConfigDir: configDir},
		}), nil
	case "outlook":
		return outlook.GetTokenPath(&cfg_g.Config{Paths: cfg_g.Paths{ConfigDir: configDir}}, acc.Name)
	default:
		return "", nil
	}
}

// TokenFiles are the paths in configDir an account's OAuth token may be under,
// current and legacy, each with the copy a login sets aside (".prev"). They
// are named after the account, so renaming it moves each one. Nothing is
// written to compute them, unlike TokenPath, which migrates a legacy name.
func TokenFiles(configDir string, acc cfg_acc.AccountConfig) []string {
	var names []string
	switch strings.ToLower(acc.Type) {
	case "gmail":
		names = gmail.TokenFileNames(acc.Name)
	case "outlook":
		names = outlook.TokenFileNames(acc.Name)
	}
	paths := make([]string, 0, 2*len(names))
	for _, n := range names {
		p := filepath.Join(configDir, n)
		paths = append(paths, p, p+".prev")
	}
	return paths
}

// RunBackendTests exercises the Gmail REST surface. It is a Gmail-specific
// diagnostic, which is why it is named here rather than on the interface.
func RunBackendTests(rep report.Reporter, config *cfg_g.Config) error {
	return gmail.RunRESTTests(rep, config)
}

// NewGoogleOAuthClient is the OAuth client provider calendar/google is wired
// with. Gmail and Google Calendar share one token and one scope list.
func NewGoogleOAuthClient(rep report.Reporter, config *cfg_g.Config) (*http.Client, error) {
	return gmail.NewOAuthHTTPClient(rep, config)
}

// OpenMbox builds the read-only, caching client stack over a local mbox file.
func OpenMbox(config *cfg_g.Config, mboxPath string, cacheDir cachedir.Account) (mailclient.MailClient, error) {
	if len(config.Accounts) == 0 {
		return nil, fmt.Errorf("mbox view requires a synthetic account in the config")
	}
	acc := config.Accounts[0]
	if err := openAccountCache(cacheDir, "mbox", config); err != nil {
		return nil, fmt.Errorf("mbox view %s: %w", mboxPath, err)
	}

	raw := mbox.NewClient(acc, config, mboxPath)
	readOnly := mailclient.NewReadOnlyMailClient(raw, acc.Name)
	return NewCheckingClient(readOnly, acc.Name, &cache.DiskCacheStore{Dir: cacheDir}), nil
}

// NewCheckingClient wraps a client in the label cache.
func NewCheckingClient(delegate mailclient.MailClient, accName string, store cache.Store) mailclient.MailClient {
	return mailclient.NewCheckingMailClient(delegate, accName, store)
}

// OutlookIDs is what migrating an Outlook account's cache to immutable IDs
// needs from the backend.
type OutlookIDs struct {
	// Translate maps default Graph message IDs to immutable ones.
	Translate func(context.Context, []email.ID) (map[email.ID]email.ID, error)
	// Verify proves on the account that Translate agrees with Graph's own
	// immutable IDs.
	Verify  func(context.Context) error
	Account cachedir.Account // the account's cache
}

// OutlookIDTranslator returns the account's ID translation, built with the
// per-account configuration it needs. It refuses an account that is not
// Outlook.
func OutlookIDTranslator(acc cfg_acc.AccountConfig, config *cfg_g.Config, rep report.Reporter) (OutlookIDs, error) {
	if !strings.EqualFold(acc.Type, "outlook") {
		return OutlookIDs{}, fmt.Errorf("account %q is %s, not outlook: only Outlook message IDs change on a move", acc.Name, acc.Type)
	}
	cfg_g.MigrateDirSanitization(config.CacheRoot, acc.Name)
	localCfg := BuildAccountLocalConfig(acc, config)
	t := outlook.NewIDTranslator(acc, &localCfg, rep)
	return OutlookIDs{Translate: t.Translate, Verify: t.Verify, Account: localCfg.AccountCache}, nil
}
