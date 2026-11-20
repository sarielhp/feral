package mailops

import (
	"fmt"
	"strings"

	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/mailclient"
	"github.com/sarielhp/feral/report"
)

// AccountJob is called for each matching account with the resolved label prefix.
// Return stop=true to abort iteration early.
type AccountJob func(acc cfg_acc.AccountConfig, labelPrefix string) (stop bool, err error)

// AccountNames returns the list of display/canonical names for all configured accounts.
func AccountNames(config *cfg_g.Config) []string {
	names := make([]string, 0, len(config.Accounts))
	for _, a := range config.Accounts {
		displayName := a.GetDisplayName()
		if displayName != a.Name && displayName != "" {
			names = append(names, fmt.Sprintf("%s (%s)", displayName, a.Name))
		} else {
			names = append(names, a.Name)
		}
	}
	return names
}

// Span is what a reference that names no account -- no `account:` in it, no
// -A, no alias -- covers.
type Span int

const (
	// DefaultSpan is the default account (`account default`, else the
	// first). A folder reference reads it so: `feral %` used to scan the
	// inbox of every account, which is rarely what a bare `%` means.
	DefaultSpan Span = iota
	// EverySpan is every account: for a message ID, which is looked for
	// wherever it is.
	EverySpan
)

// ForEachAccount resolves a label reference -- `%[account:]label` -- and
// iterates over the accounts it names; `%all:` names every one, and a
// reference that names none covers unnamed.
func ForEachAccount(config *cfg_g.Config, labelSpec string, unnamed Span, fn AccountJob) error {
	spec, err := cfg_g.ParseLabelRef(labelSpec)
	if err != nil {
		return err
	}
	var accounts []cfg_acc.AccountConfig

	// Both named forms go through cfg_g.Lookup, which is the one place that
	// decides what an identifier means. These used to be two hand-rolled loops
	// taking the first account whose name, display name, alias *or backend
	// type* matched, so "outlook:INBOX" silently picked one of two Outlook
	// accounts and went to the network.
	switch {
	case strings.EqualFold(spec.Account, "all"):
		if len(config.Accounts) == 0 {
			return cfg_g.ErrNoAccounts
		}
		accounts = config.Accounts
	case spec.Account != "":
		acc, err := cfg_g.Lookup(config.Accounts, spec.Account)
		if err != nil {
			return err
		}
		accounts = []cfg_acc.AccountConfig{*acc}
	case config.SelectedAccount != "" || unnamed == DefaultSpan:
		// An empty selection is the default account (Lookup("")).
		acc, err := cfg_g.Lookup(config.Accounts, string(config.SelectedAccount))
		if err != nil {
			return err
		}
		accounts = []cfg_acc.AccountConfig{*acc}
	default:
		if len(config.Accounts) == 0 {
			return cfg_g.ErrNoAccounts
		}
		accounts = config.Accounts
	}

	for _, acc := range accounts {
		if stop, err := fn(acc, spec.Label); stop || err != nil {
			return err
		}
	}
	return nil
}

// GetClientForSelected returns a MailClient for the currently selected account (or the default account).
func GetClientForSelected(config *cfg_g.Config, rep report.Reporter) (mailclient.MailClient, error) {
	var err error
	var client mailclient.MailClient
	if forEachErr := ForEachAccount(config, "", DefaultSpan, func(acc cfg_acc.AccountConfig, _ string) (bool, error) {
		client, err = NewMailClient(acc, config, rep)
		return true, nil // stop after first
	}); forEachErr != nil {
		return nil, forEachErr
	}
	if err != nil {
		return nil, err
	}
	if client == nil {
		return nil, fmt.Errorf("no accounts configured in config.json")
	}
	return client, nil
}

// GetClientForAccountIndex returns a MailClient for the 1-based account index.
func GetClientForAccountIndex(config *cfg_g.Config, index int, rep report.Reporter) (mailclient.MailClient, error) {
	if index < 1 || index > len(config.Accounts) {
		return nil, fmt.Errorf("account index %d is out of range. You have %d configured account(s)", index, len(config.Accounts))
	}
	acc := config.Accounts[index-1]
	return NewMailClient(acc, config, rep)
}

// ResolveSendAccount determines which account should send an outbound message.
// Precedence:
//  1. Explicit override account (e.g. -A flag)
//  2. Selected account (e.g. from -1, -2, or program symlink alias)
//  3. DefaultSendAccount if configured in config
//  4. DefaultAccount (`account default`) if configured and it names an
//     account; a dangling one is passed over, as everywhere else
//  5. The sole account if exactly one account is configured
//  6. If multiple accounts exist and none specified, fails with ErrNoSenderAccount
func ResolveSendAccount(config *cfg_g.Config, overrideAccount string) (*cfg_acc.AccountConfig, error) {
	if config == nil || len(config.Accounts) == 0 {
		return nil, cfg_g.ErrNoAccounts
	}
	if overrideAccount != "" {
		return cfg_g.Lookup(config.Accounts, overrideAccount)
	}
	if config.SelectedAccount != "" {
		return cfg_g.Lookup(config.Accounts, string(config.SelectedAccount))
	}
	if config.DefaultSendAccount != "" {
		return cfg_g.Lookup(config.Accounts, config.DefaultSendAccount)
	}
	if config.DefaultAccount != "" && !cfg_g.DanglingDefault(config.Accounts, config.DefaultAccount) {
		return cfg_g.Lookup(config.Accounts, config.DefaultAccount)
	}
	if len(config.Accounts) == 1 {
		return &config.Accounts[0], nil
	}
	return nil, fmt.Errorf("%w (%d accounts configured)", cfg_g.ErrNoSenderAccount, len(config.Accounts))
}

// GetClientForSending returns a MailClient initialized for sending outbound mail and the resolved account.
func GetClientForSending(config *cfg_g.Config, overrideAccount string, rep report.Reporter) (mailclient.MailClient, *cfg_acc.AccountConfig, error) {
	acc, err := ResolveSendAccount(config, overrideAccount)
	if err != nil {
		return nil, nil, err
	}
	client, err := NewMailClient(*acc, config, rep)
	if err != nil {
		return nil, nil, err
	}
	return client, acc, nil
}
