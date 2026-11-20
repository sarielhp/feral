package cfg_g

import (
	"strings"

	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/cfg_acc"
)

// Credentials is how to reach an account's server. Per account, not global:
// BuildAccountLocalConfig overwrites these from the AccountConfig.
type Credentials struct {
	IMAPHost string
	Username string
	Password string
}

// Paths is where this run keeps its files.
type Paths struct {
	// CacheRoot is the configured download_dir: drafts, mbox views and the
	// contacts history live here, and each account's cache beneath it. It is
	// never an account's own directory; that is Config.AccountCache.
	CacheRoot   string
	ConfigDir   string
	MuttAliases string
}

// Policy is what the user has decided should happen to mail: which folders
// spam moves to, what counts as spam, which senders bypass the checks.
type Policy struct {
	Limit            int
	ScoreThreshold   float64
	LearnSpam        bool
	ForceLearn       bool
	SpamFolder       string
	ReceivedFolder   string
	SpamLearn        string
	UnspamLearn      string
	AllowedLanguages []string
	BlockPolitical   bool
	DeleteAllSpam    bool
	AutoUnsubscribe  bool
	Whitelist        []string
	Blacklist        []string
	Rules            []cfg_acc.Rule
}

// Prefs is presentation: which browser and editor to launch, how much to say,
// how to draw attachments. Nothing below the user interface reads these, and
// grouping them says so.
type Prefs struct {
	Browser           string
	Editor            string
	EditorArgs        []string
	Quiet             bool
	Verbose           bool
	HideZeroLabels    bool
	AttachmentSaveDir string
	SaveLocations     []string // the directories the save dialog jumps to; nil means the default pair
	GitHubURL         string
	// TuiWatch runs the new-mail watcher for as long as the TUI is open.
	TuiWatch bool
}

// Config is one run's settings: the four groups above, plus the session state
// that says which account is selected and how this invocation may behave.
//
// The groups are embedded, so config.Username and config.CacheRoot still
// resolve. The point is not to change how they are read but to say which layer
// each field belongs to -- and to make it possible to hand a backend its
// Credentials and Policy without also handing it the user's editor.
type Config struct {
	Credentials
	Paths
	Policy
	Prefs

	Accounts        []cfg_acc.AccountConfig
	SelectedAccount AccountName
	// AccountCache is the selected account's cache directory. Only the
	// client factory sets it, in the per-account configs it builds; the
	// session's own config leaves it zero, and the cache functions refuse a
	// zero one rather than reading the root.
	AccountCache       cachedir.Account
	DefaultSendAccount string
	// DefaultAccount is config.json's default_account, as written; the
	// account it names carries IsDefault. See DefaultOf.
	DefaultAccount string
	// MyEmails are the user's own addresses besides each account's
	// user_email; see OwnAddresses.
	MyEmails    []string
	AccountType string
	FixConfig   bool
	ReadOnly    bool

	// DryRun reports every change instead of making it, local writes included.
	// ReadOnly is the narrower flag: it forbids server mutations but permits
	// caching, because a message cannot be displayed without its body. DryRun
	// implies ReadOnly; the reverse does not hold.
	DryRun       bool
	LLM          LLMConfig
	SaveSentCopy bool

	// Links is config.json's links, as loaded (and migrated in memory).
	Links map[string]Link
	// LinksDeclared records that config.json had a "links" key when it was
	// read, before any migration: until then, starting feral under an
	// account's alias still selects that account.
	LinksDeclared bool
}

// LLMConfig stores settings for connecting to a local or remote LLM endpoint.
type LLMConfig struct {
	Endpoint     string   `json:"endpoint,omitempty"`
	APIKey       string   `json:"api_key,omitempty"`
	Model        string   `json:"model,omitempty"`
	Temperature  *float64 `json:"temperature,omitempty"`
	MaxTokens    *int     `json:"max_tokens,omitempty"`
	SystemPrompt string   `json:"system_prompt,omitempty"`
	// ReasoningEffort turns a hybrid model's thinking on or off through the
	// OpenAI-compatible "reasoning_effort" field: "none" disables it, while
	// "low"/"medium"/"high" enable it. Empty sends nothing, so the same binary
	// still works against a backend that does not know the field.
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
	AutoSummary     *bool  `json:"auto_summary,omitempty"`
	// AutoSubject lets a message sent with an empty subject get one written
	// by the model from its body and attachment names; nil means enabled.
	AutoSubject *bool `json:"auto_subject,omitempty"`
}

// IsAutoSummaryEnabled returns true unless explicitly disabled in config.
func (l LLMConfig) IsAutoSummaryEnabled() bool {
	if l.AutoSummary == nil {
		return true
	}
	return *l.AutoSummary
}

// IsAutoSubjectEnabled reports whether an empty subject is filled in by the
// model: on unless explicitly disabled, but only meaningful with an endpoint.
func (l LLMConfig) IsAutoSubjectEnabled() bool {
	if strings.TrimSpace(l.Endpoint) == "" {
		return false
	}
	if l.AutoSubject == nil {
		return true
	}
	return *l.AutoSubject
}

// FileConfig defines the fields structured inside ~/.config/feral/config.json
type FileConfig struct {
	// Format is config.json's file format (see fileformat); absent means 1.
	Format             int                      `json:"format,omitempty"`
	Username           string                   `json:"username"`
	Password           string                   `json:"password"`
	IMAPHost           string                   `json:"imap_host"`
	DownloadDir        string                   `json:"download_dir"`
	Limit              *int                     `json:"limit"`
	ScoreThreshold     *float64                 `json:"score_threshold"`
	SpamFolder         string                   `json:"spam_folder"`
	ReceivedFolder     string                   `json:"received_folder"`
	SpamLearn          string                   `json:"spam_learn,omitempty"`
	UnspamLearn        string                   `json:"unspam_learn,omitempty"`
	AllowedLanguages   *[]string                `json:"allowed_languages"`
	BlockPolitical     *bool                    `json:"block_political"`
	AutoUnsubscribe    *bool                    `json:"auto_unsubscribe"`
	Whitelist          *[]string                `json:"whitelist,omitempty"`
	Blacklist          *[]string                `json:"blacklist,omitempty"`
	Rules              *[]cfg_acc.Rule          `json:"rules,omitempty"`
	Accounts           *[]cfg_acc.AccountConfig `json:"accounts"`
	Browser            string                   `json:"browser,omitempty"`
	Editor             string                   `json:"editor,omitempty"`
	EditorArgs         []string                 `json:"editor_args,omitempty"`
	ReadOnly           *bool                    `json:"read_only,omitempty"`
	LLM                *LLMConfig               `json:"llm,omitempty"`
	AttachmentSaveDir  string                   `json:"attachment_save_dir,omitempty"`
	SaveLocations      []string                 `json:"save_locations,omitempty"`
	SaveSentCopy       *bool                    `json:"save_sent_copy,omitempty"`
	GitHubURL          string                   `json:"github_url,omitempty"`
	TuiWatch           *bool                    `json:"tui_watch,omitempty"`
	MuttAliases        string                   `json:"mutt_aliases,omitempty"`
	DefaultSendAccount string                   `json:"default_send_account,omitempty"`
	MyEmails           []string                 `json:"my_emails,omitempty"`
	// DefaultAccount names the account a command acts on when it names none
	// (`account default`). Empty means the first account.
	DefaultAccount string `json:"default_account,omitempty"`
	// Links are the other names feral runs under (see Link). A pointer, so
	// that a config.json without the key -- one the aliases migration has not
	// reached -- differs from one with no links.
	Links *map[string]Link `json:"links,omitempty"`

	// loaded is what this FileConfig saw of each shared file when it was
	// loaded or last saved: what SaveConfigFile needs to merge with a change
	// another process made underneath (see fileState). Nil for a FileConfig
	// that was built rather than loaded, which a save writes as it is.
	loaded loadState
}

// IsSaveSentCopyEnabled returns whether saving a sent copy is enabled for the selected account or global default.
func (c *Config) IsSaveSentCopyEnabled() bool {
	if c == nil {
		return true
	}
	for _, acc := range c.Accounts {
		if c.IsSelected(acc) {
			return acc.IsSaveSentCopyEnabled(c.SaveSentCopy)
		}
	}
	return c.SaveSentCopy
}

// GetSelectedAccountDisplayName returns the display name of the selected account, or SelectedAccount if not found.
func (c *Config) GetSelectedAccountDisplayName() string {
	for _, acc := range c.Accounts {
		if c.IsSelected(acc) {
			return acc.GetDisplayName()
		}
	}
	return string(c.SelectedAccount)
}
