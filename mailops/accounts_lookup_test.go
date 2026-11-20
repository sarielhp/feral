package mailops

import (
	"errors"
	"testing"

	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
)

func twoOutlookAccounts() *cfg_g.Config {
	return &cfg_g.Config{
		Accounts: []cfg_acc.AccountConfig{
			{Name: "default", DisplayName: "GMail", Type: "gmail"},
			{Name: "work", DisplayName: "Work", Type: "outlook"},
			{Name: "test_acct", Type: "outlook"},
		},
	}
}

// ForEachAccount used to hand-roll the lookup and take the first account whose
// name, display name, alias or backend type matched. "%outlook:INBOX" therefore
// picked one of two Outlook accounts and went to the network.
func TestForEachAccountRejectsAnAmbiguousSpec(t *testing.T) {
	var visited []string
	err := ForEachAccount(twoOutlookAccounts(), "%outlook:INBOX", DefaultSpan,
		func(acc cfg_acc.AccountConfig, _ string) (bool, error) {
			visited = append(visited, acc.Name)
			return false, nil
		})

	var ambiguous *cfg_g.AmbiguousAccountError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("expected an ambiguity error, got %v", err)
	}
	if len(visited) != 0 {
		t.Errorf("no account should have been visited, got %v", visited)
	}
}

func TestForEachAccountResolvesADisplayName(t *testing.T) {
	var visited []string
	err := ForEachAccount(twoOutlookAccounts(), "%Work:INBOX", DefaultSpan,
		func(acc cfg_acc.AccountConfig, label string) (bool, error) {
			visited = append(visited, acc.Name+":"+label)
			return false, nil
		})
	if err != nil {
		t.Fatalf("ForEachAccount: %v", err)
	}
	if len(visited) != 1 || visited[0] != "work:INBOX" {
		t.Errorf("expected the work account and a clean label, got %v", visited)
	}
}

// A selected account that names nothing must not silently become account one.
func TestForEachAccountRejectsAnUnknownSelection(t *testing.T) {
	config := twoOutlookAccounts()
	config.SelectedAccount = "nonesuch"

	var visited []string
	err := ForEachAccount(config, "", DefaultSpan, func(acc cfg_acc.AccountConfig, _ string) (bool, error) {
		visited = append(visited, acc.Name)
		return false, nil
	})

	var unknown *cfg_g.UnknownAccountError
	if !errors.As(err, &unknown) {
		t.Fatalf("expected an unknown-account error, got %v", err)
	}
	if len(visited) != 0 {
		t.Errorf("no account should have been visited, got %v", visited)
	}
}

// WithAccount must not disturb the config it was given.
func TestWithAccountLeavesTheCallersConfigAlone(t *testing.T) {
	config := twoOutlookAccounts()
	config.SelectedAccount = "default"

	scoped, err := cfg_g.SelectAccount(config, "work")
	if err != nil {
		t.Fatalf("SelectAccount: %v", err)
	}

	if scoped.SelectedAccount != "work" {
		t.Errorf("copy selects %q, want work", scoped.SelectedAccount)
	}
	if config.SelectedAccount != "default" {
		t.Errorf("original was mutated: selects %q", config.SelectedAccount)
	}
}

func TestResolveSendAccount(t *testing.T) {
	// Case 1: No accounts
	emptyCfg := &cfg_g.Config{}
	if _, err := ResolveSendAccount(emptyCfg, ""); !errors.Is(err, cfg_g.ErrNoAccounts) {
		t.Errorf("expected ErrNoAccounts, got %v", err)
	}

	// Case 2: Single account returns itself
	singleAcc := &cfg_g.Config{
		Accounts: []cfg_acc.AccountConfig{{Name: "only", Username: "only@example.com"}},
	}
	resolved, err := ResolveSendAccount(singleAcc, "")
	if err != nil || resolved.Name != "only" {
		t.Errorf("expected only, got %v (err: %v)", resolved, err)
	}

	// Case 3: Multiple accounts with no default -> ErrNoSenderAccount
	multiAcc := twoOutlookAccounts()
	if _, resolveErr := ResolveSendAccount(multiAcc, ""); !errors.Is(resolveErr, cfg_g.ErrNoSenderAccount) {
		t.Errorf("expected ErrNoSenderAccount, got %v", resolveErr)
	}

	// Case 3b: default_account is the sender when nothing else names one; a
	// dangling one is passed over, not reported as an unknown account.
	multiAcc.DefaultAccount = "work"
	resolved, err = ResolveSendAccount(multiAcc, "")
	if err != nil || resolved.Name != "work" {
		t.Errorf("expected default_account work, got %v (err: %v)", resolved, err)
	}
	multiAcc.DefaultAccount = "gone"
	if _, resolveErr := ResolveSendAccount(multiAcc, ""); !errors.Is(resolveErr, cfg_g.ErrNoSenderAccount) {
		t.Errorf("a dangling default_account: expected ErrNoSenderAccount, got %v", resolveErr)
	}
	soleDangling := *singleAcc
	soleDangling.DefaultAccount = "gone"
	if one, resolveErr := ResolveSendAccount(&soleDangling, ""); resolveErr != nil || one.Name != "only" {
		t.Errorf("a dangling default_account must not stop the sole account sending: %v, %v", one, resolveErr)
	}

	// Case 4: DefaultSendAccount, which wins over default_account
	multiAcc.DefaultSendAccount = "default"
	resolved, err = ResolveSendAccount(multiAcc, "")
	if err != nil || resolved.Name != "default" {
		t.Errorf("expected default, got %v (err: %v)", resolved, err)
	}

	// Case 5: SelectedAccount overrides DefaultSendAccount
	multiAcc.SelectedAccount = "work"
	resolved, err = ResolveSendAccount(multiAcc, "")
	if err != nil || resolved.Name != "work" {
		t.Errorf("expected work, got %v (err: %v)", resolved, err)
	}

	// Case 6: Explicit override wins over everything
	resolved, err = ResolveSendAccount(multiAcc, "test_acct")
	if err != nil || resolved.Name != "test_acct" {
		t.Errorf("expected test_acct, got %v (err: %v)", resolved, err)
	}
}

func TestForEachAccount_AllSigil(t *testing.T) {
	var visited []string
	err := ForEachAccount(twoOutlookAccounts(), "%all:inbox", DefaultSpan,
		func(acc cfg_acc.AccountConfig, label string) (bool, error) {
			visited = append(visited, acc.Name+":"+label)
			return false, nil
		})
	if err != nil {
		t.Fatalf("ForEachAccount failed: %v", err)
	}
	if len(visited) != 3 {
		t.Fatalf("expected 3 accounts visited, got %d: %v", len(visited), visited)
	}
	expected := []string{"default:inbox", "work:inbox", "test_acct:inbox"}
	for i, exp := range expected {
		if visited[i] != exp {
			t.Errorf("visited[%d] = %q, want %q", i, visited[i], exp)
		}
	}
}

// A reference that names no account covers the default account, unless the
// caller sweeps every account. `feral %` used to list every inbox.
func TestForEachAccountUnnamedSpan(t *testing.T) {
	visit := func(config *cfg_g.Config, spec string, span Span) []string {
		var visited []string
		if err := ForEachAccount(config, spec, span, func(acc cfg_acc.AccountConfig, _ string) (bool, error) {
			visited = append(visited, acc.Name)
			return false, nil
		}); err != nil {
			t.Fatal(err)
		}
		return visited
	}
	config := twoOutlookAccounts()
	if got := visit(config, "%inbox", DefaultSpan); len(got) != 1 || got[0] != "default" {
		t.Errorf("with no default set, an unnamed reference covers the first account, got %v", got)
	}
	config.Accounts[1].IsDefault = true
	if got := visit(config, "%inbox", DefaultSpan); len(got) != 1 || got[0] != "work" {
		t.Errorf("an unnamed reference must cover the default account, got %v", got)
	}
	if got := visit(config, "%inbox", EverySpan); len(got) != 3 {
		t.Errorf("a sweep must cover every account, got %v", got)
	}
	if got := visit(config, "%all:inbox", DefaultSpan); len(got) != 3 {
		t.Errorf("%%all: must cover every account, got %v", got)
	}
	config.SelectedAccount = "test_acct"
	if got := visit(config, "%inbox", EverySpan); len(got) != 1 || got[0] != "test_acct" {
		t.Errorf("a selection narrows a sweep to that account, got %v", got)
	}
}
