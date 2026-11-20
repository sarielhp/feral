package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/sarielhp/feral/app"
	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/mailclient"

	"github.com/sarielhp/clihelp"
)

func runAccountList(session *Session) error {
	_, configPath, err := cfg_g.ResolveConfigPath(session.Config)
	if err != nil {
		return err
	}
	fc, err := cfg_g.LoadConfigFile(configPath)
	if err != nil {
		return err
	}

	if wantJSON() {
		return session.emitJSON(accountListing(session, *fc.Accounts))
	}

	if len(*fc.Accounts) == 0 {
		fmt.Fprintln(session.out(), "No accounts configured.")
		return nil
	}
	fmt.Fprintf(session.out(), "Configured accounts (%d):\n", len(*fc.Accounts))
	for i, acc := range *fc.Accounts {
		selected := ""
		if session.Config.IsSelected(acc) {
			selected = " *"
		}
		accType := acc.AccountType
		if accType == "" {
			accType = "regular"
		}
		if acc.IsDefault {
			selected += " (default)"
		}
		displayName := acc.GetDisplayName()
		if displayName != acc.Name {
			fmt.Fprintf(session.out(), "%d. %s [%s] (%s, type: %s)%s\n", i+1, displayName, acc.Name, acc.Type, accType, selected)
		} else {
			fmt.Fprintf(session.out(), "%d. %s (%s, type: %s)%s\n", i+1, acc.Name, acc.Type, accType, selected)
		}
	}
	warnAccountNames(session, *fc.Accounts, fc.DefaultAccount)
	return nil
}

func runAccountAssociate(session *Session, accountName, progName string) error {
	_, configPath, err := cfg_g.ResolveConfigPath(session.Config)
	if err != nil {
		return err
	}
	fc, err := cfg_g.LoadConfigFile(configPath)
	if err != nil {
		return err
	}

	for i := range *fc.Accounts {
		if strings.EqualFold((*fc.Accounts)[i].Name, accountName) {
			acc := &(*fc.Accounts)[i]
			if acc.Aliases == nil {
				acc.Aliases = []string{}
			}
			acc.Aliases = append(acc.Aliases, progName)
			if err := cfg_g.SaveConfigFile(configPath, fc); err != nil {
				return err
			}
			fmt.Fprintf(session.errOut(), "%s %q is now an alias of account %q: -A %s selects it.\n",
				app.PrefixSuccess, progName, acc.Name, progName)
			// Starting feral under an alias selected its account until links
			// replaced that; say how to get it now.
			if fc.Links != nil && cfg_g.ValidateLinkName(progName) == nil {
				fmt.Fprintf(session.errOut(), "%s To run feral as %q on it, add a link: feral link add %s -- -A %s\n",
					app.PrefixInfo, progName, progName, acc.Name)
			}
			return nil
		}
	}
	return ErrAccountNotFound(accountName)
}

func runAccountTest(session *Session, args []string) error {
	if len(args) > 0 {
		name := args[0]
		matched := ""
		for _, a := range session.Config.Accounts {
			if strings.EqualFold(a.Name, name) || strings.EqualFold(a.GetDisplayName(), name) {
				matched = string(cfg_g.NameOf(a))
				break
			}
		}
		if matched == "" {
			return ErrAccountNotFoundAlt(name)
		}
		session.Config.SelectedAccount = cfg_g.AccountName(matched)
	}
	// The client is resolved and validated for its side effect: reaching the
	// server is what "verified" means here.
	if _, err := GetValidatedClient(session); err != nil {
		return err
	}
	fmt.Fprintf(session.errOut(), "%s Access successfully verified for account %q!\n", app.PrefixSuccess, session.Config.SelectedAccount)
	return nil
}

func runAccountCalendar(session *Session, args []string) error {
	_, configPath, err := cfg_g.ResolveConfigPath(session.Config)
	if err != nil {
		return err
	}
	fc, err := cfg_g.LoadConfigFile(configPath)
	if err != nil {
		return err
	}

	if len(args) == 0 {
		calName := ""
		for _, acc := range *fc.Accounts {
			if acc.CalendarManager {
				calName = acc.Name
				break
			}
		}
		if calName == "" {
			fmt.Fprintln(session.out(), "No calendar manager account designated.")
			return nil
		}
		fmt.Fprintf(session.out(), "Calendar manager account: %s\n", calName)
		return nil
	}

	accountName := args[0]
	for i := range *fc.Accounts {
		if strings.EqualFold((*fc.Accounts)[i].Name, accountName) {
			(*fc.Accounts)[i].CalendarManager = true
			if err := cfg_g.SaveConfigFile(configPath, fc); err != nil {
				return err
			}
			fmt.Fprintf(session.errOut(), "%s Successfully designated account %q as calendar manager.\n", app.PrefixSuccess, accountName)
			return nil
		}
	}
	return ErrAccountNotFound(accountName)
}

func cleanupAccountToken(session *Session, configPath string, targetAcc *cfg_acc.AccountConfig) {
	tokenPath, err := session.accounts().TokenPath(filepath.Dir(configPath), *targetAcc)
	if err != nil || tokenPath == "" {
		return
	}

	if err := os.Remove(tokenPath); err != nil {
		if !os.IsNotExist(err) {
			fmt.Fprintf(session.errOut(), "%s Failed to delete old token file %s: %v\n", app.PrefixWarn, tokenPath, err)
		}
		return
	}
	fmt.Fprintf(session.errOut(), "%s Deleted old token file: %s\n", app.PrefixInfo, tokenPath)
}

// setAsideSuffix names a token moved out of the way while a login runs.
const setAsideSuffix = ".prev"

// setAsideAccountToken moves the account's token aside so the login after it
// signs in afresh, and returns what to do once the login has ended: drop the
// old token if a new one was written, put it back otherwise.
//
// Login used to delete the token first. Abandoning the sign-in -- Esc,
// Ctrl-C, closing the window, or a stray "l" in the TUI's account list --
// then left a working account with no credentials at all. A process killed
// mid-login cannot run the returned function; restoreSetAsideTokens, which
// every command runs at start, puts the token back then.
func setAsideAccountToken(session *Session, configPath string, acc *cfg_acc.AccountConfig) func() {
	tokenPath, err := session.accounts().TokenPath(filepath.Dir(configPath), *acc)
	if err != nil || tokenPath == "" {
		return func() {}
	}
	prev := tokenPath + setAsideSuffix
	if err := os.Rename(tokenPath, prev); err != nil {
		if !os.IsNotExist(err) {
			fmt.Fprintf(session.errOut(), "%s Could not set the old token aside: %v\n", app.PrefixWarn, err)
		}
		return func() {}
	}
	return func() { settleSetAsideToken(tokenPath) }
}

// settleSetAsideToken resolves a set-aside token: a new token at tokenPath
// wins and the old one goes; with none, the old one comes back.
func settleSetAsideToken(tokenPath string) {
	prev := tokenPath + setAsideSuffix
	if _, err := os.Stat(prev); err != nil {
		return
	}
	if _, err := os.Stat(tokenPath); err == nil {
		_ = os.Remove(prev)
		return
	}
	_ = os.Rename(prev, tokenPath)
}

// restoreSetAsideTokens settles any token a killed login left aside.
func restoreSetAsideTokens(session *Session) {
	if session.Config == nil || session.Config.ConfigDir == "" {
		return
	}
	for _, acc := range session.Config.Accounts {
		tokenPath, err := session.accounts().TokenPath(session.Config.ConfigDir, acc)
		if err != nil || tokenPath == "" {
			continue
		}
		settleSetAsideToken(tokenPath)
	}
}

func runAccountLogin(session *Session, args []string) error {
	_, configPath, err := cfg_g.ResolveConfigPath(session.Config)
	if err != nil {
		return err
	}
	fc, err := cfg_g.LoadConfigFile(configPath)
	if err != nil {
		return err
	}

	name := string(session.Config.SelectedAccount)
	if len(args) > 0 {
		name = args[0]
	}
	var targetAcc *cfg_acc.AccountConfig
	for i := range *fc.Accounts {
		if strings.EqualFold((*fc.Accounts)[i].Name, name) {
			targetAcc = &(*fc.Accounts)[i]
			break
		}
	}
	if targetAcc == nil {
		return ErrAccountNotFoundAlt(name)
	}

	// Check the account's own settings before the old token is deleted. The
	// post-login check rejects an account with, say, no username before it ever
	// contacts the server, so this used to delete a working token, never sign
	// in, and then report "signed in ... the credential has been saved".
	if invalid := cfg_g.ValidateAccountParams(*targetAcc); invalid != nil {
		return accountSettingsError(targetAcc.Name, invalid)
	}
	if targetAcc.IsMbox() {
		return fmt.Errorf("account %s is a local mbox file: it has no credentials to sign in with", targetAcc.Name)
	}

	session.Config.SelectedAccount = cfg_g.NameOf(*targetAcc)
	settle := setAsideAccountToken(session, configPath, targetAcc)
	defer settle()

	client, err := session.clients().ForSelected(session.Config)
	if err != nil {
		return err
	}
	if err := validateAfterLogin(session.out(), client); err != nil {
		return err
	}
	fmt.Fprintf(session.errOut(), "%s Successfully authenticated and logged in account %q!\n", app.PrefixSuccess, targetAcc.Name)
	return nil
}

// accountSettingsError explains an account whose configuration is incomplete,
// and for the field a user most often lacks -- the username, which older
// versions of `account new` let through empty -- says how to set it.
func accountSettingsError(name string, err error) error {
	hint := fmt.Sprintf("fix the account's settings in its account.json under the config directory, then run `%s account login %s` again", app.AppName, name)
	if strings.Contains(err.Error(), "username") {
		hint = fmt.Sprintf("set its email address with `%s -A %s config set username you@example.com`, then run `%s account login %s` again",
			app.AppName, name, app.AppName, name)
	}
	return fmt.Errorf("account %q cannot sign in: %w; %s", name, err, hint)
}

// validateAfterLogin checks the credential just obtained, retrying once.
//
// The check runs immediately after a device-code exchange, and a server can be
// slow to honour a token it has only just issued -- Microsoft Graph answered
// the first request after login with a timeout. Reporting that as a failure is
// wrong twice over: the credential was obtained and written to disk, so the
// login did work, and the user is left believing they must do the whole
// browser dance again.
// loginRetryDelay gives the server a moment before the retry. A variable so a
// test does not have to wait it out.
var loginRetryDelay = 3 * time.Second

func validateAfterLogin(w io.Writer, client mailclient.Reader) error {
	err := client.Validate()
	if err == nil {
		return nil
	}

	fmt.Fprintf(w, "%s First check after sign-in did not answer (%v); retrying...\n", app.PrefixWarn, err)
	time.Sleep(loginRetryDelay)

	if retryErr := client.Validate(); retryErr != nil {
		return fmt.Errorf("signed in, but the account could not be reached: %w. "+
			"The credential has been saved, so try `%s account test` before signing in again",
			retryErr, app.AppName)
	}
	return nil
}

// updateAccountConfig applies mutate to the account named by accountName and
// saves the config, returning the account's canonical name.
func updateAccountConfig(session *Session, accountName string, mutate func(*cfg_acc.AccountConfig)) (string, error) {
	_, configPath, err := cfg_g.ResolveConfigPath(session.Config)
	if err != nil {
		return "", err
	}
	fc, err := cfg_g.LoadConfigFile(configPath)
	if err != nil {
		return "", err
	}

	accIdx := -1
	for i, acc := range *fc.Accounts {
		if strings.EqualFold(acc.Name, accountName) || (acc.DisplayName != "" && strings.EqualFold(acc.DisplayName, accountName)) {
			accIdx = i
			break
		}
	}
	if accIdx == -1 {
		return "", ErrAccountNotFound(accountName)
	}

	mutate(&(*fc.Accounts)[accIdx])
	if err := cfg_g.SaveConfigFile(configPath, fc); err != nil {
		return "", err
	}
	return (*fc.Accounts)[accIdx].Name, nil
}

// runAccountUserName sets the personal name used in outgoing mail, leaving the
// account's display name (its label) untouched.
func runAccountUserName(session *Session, accountName, userName string) error {
	if strings.TrimSpace(userName) == "" {
		return fmt.Errorf("user name cannot be empty")
	}
	name, err := updateAccountConfig(session, accountName, func(acc *cfg_acc.AccountConfig) {
		acc.UserName = userName
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(session.errOut(), "%s Set sender name for account %q to %q.\n", app.PrefixSuccess, name, userName)
	return nil
}

// runAccountUserEmail sets the address outgoing mail is sent from, which may
// differ from the account's login username (a Fastmail alias, for example).
func runAccountUserEmail(session *Session, accountName, userEmail string) error {
	userEmail = strings.TrimSpace(userEmail)
	if !strings.Contains(userEmail, "@") {
		return fmt.Errorf("user email %q is not a valid address", userEmail)
	}
	name, err := updateAccountConfig(session, accountName, func(acc *cfg_acc.AccountConfig) {
		acc.UserEmail = userEmail
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(session.errOut(), "%s Set send address for account %q to %q.\n", app.PrefixSuccess, name, userEmail)
	return nil
}

func runAccountDelete(session *Session, name string) error {
	_, configPath, err := cfg_g.ResolveConfigPath(session.Config)
	if err != nil {
		return err
	}
	fc, err := cfg_g.LoadConfigFile(configPath)
	if err != nil {
		return err
	}

	foundIdx := -1
	for i, acc := range *fc.Accounts {
		if strings.EqualFold(acc.Name, name) || (acc.DisplayName != "" && strings.EqualFold(acc.DisplayName, name)) {
			foundIdx = i
			break
		}
	}

	if foundIdx == -1 {
		return ErrAccountNotFound(name)
	}

	targetAcc := (*fc.Accounts)[foundIdx]
	*fc.Accounts = append((*fc.Accounts)[:foundIdx], (*fc.Accounts)[foundIdx+1:]...)
	// Neither may go on naming an account that is gone.
	for _, ref := range []*string{&fc.DefaultAccount, &fc.DefaultSendAccount} {
		if *ref != "" && (strings.EqualFold(*ref, targetAcc.Name) || strings.EqualFold(*ref, targetAcc.DisplayName)) {
			*ref = ""
		}
	}

	if err := cfg_g.SaveConfigFile(configPath, fc); err != nil {
		return err
	}

	cleanupAccountToken(session, configPath, &targetAcc)

	fmt.Fprintf(session.errOut(), "%s Successfully deleted account %q and cleaned up associated token files.\n", app.PrefixSuccess, targetAcc.Name)
	// Its links are kept -- an account given the name again makes them work
	// again -- so the user hears about them now, not when one fails.
	if fc.Links != nil {
		if names := cfg_g.LinksNaming(*fc.Links, targetAcc.Name); len(names) > 0 {
			fmt.Fprintf(session.errOut(), "%s Links still naming it: %s; remove them with `feral link rm`.\n",
				app.PrefixWarn, strings.Join(names, ", "))
		}
	}
	return nil
}

func newAccountListCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "list",
		Description: "List configured accounts.",
		UsageLine:   "feral account list",
		Args:        clihelp.NoArgs,
		Run: func(_ *clihelp.Context) error {
			return runAccountList(session)
		},
		Examples: []clihelp.Example{
			{Line: "feral account list"},
		},
	}
}

func newAccountAssociateCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "link",
		Aliases:     []string{"associate"},
		Title:       "account link <name> <program>",
		Description: "Add a name -A accepts for an account.",
		LongDescription: "Add an alias to an account: a name -A accepts for it, besides its name and display name. " +
			"An alias used to select its account when feral was started under that name; that is now a link's " +
			"job (`feral link add <name> -- -A <account>`), and the first run that may write turns every alias " +
			"into a link once.",
		UsageLine: "feral account link <account_name> <program_name>",
		Parameters: []clihelp.Param{
			{Name: "<account_name>", Description: "The existing account name to associate with.", Complete: CompleteAccountNames},
			{Name: "<program_name>", Description: "The alias: another name for the account."},
		},
		Args: clihelp.ExactArgs(2),
		Run: func(ctx *clihelp.Context) error {
			return runAccountAssociate(session, ctx.Args[0], ctx.Args[1])
		},
		Examples: []clihelp.Example{
			{Line: "feral account link work-account work_mail"},
		},
	}
}

func newAccountTestCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "test",
		Title:       "account test [account_name]",
		Description: "Check an account's config and connection.",
		UsageLine:   "feral account test [account_name]",
		Parameters: []clihelp.Param{
			{Name: "[account_name]", Description: "Account name to test (defaults to currently selected account).", Complete: CompleteAccountNames},
		},
		Args: clihelp.MaximumNArgs(1),
		Run: func(ctx *clihelp.Context) error {
			return runAccountTest(session, ctx.Args)
		},
		Examples: []clihelp.Example{
			{Line: "feral account test"},
			{Line: "feral account test work-fastmail"},
		},
	}
}

func newAccountCalendarCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "calendar",
		Aliases:     []string{"cal"},
		Title:       "account calendar [account_name]",
		Description: "Show or set the calendar manager account.",
		UsageLine:   "feral account calendar [account_name]",
		Parameters: []clihelp.Param{
			{Name: "[account_name]", Description: "Account name to configure calendar for.", Complete: CompleteAccountNames},
		},
		Args: clihelp.MaximumNArgs(1),
		Run: func(ctx *clihelp.Context) error {
			return runAccountCalendar(session, ctx.Args)
		},
		Examples: []clihelp.Example{
			{Line: "feral account calendar"},
			{Line: "feral account calendar personal-gmail"},
		},
	}
}

func newAccountLoginCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "login",
		Title:           "account login [account_name]",
		Description:     "OAuth login for Gmail or Outlook.",
		LongDescription: "Perform interactive OAuth login for a Gmail or Outlook account.",
		UsageLine:       "feral account login [account_name]",
		Parameters: []clihelp.Param{
			{Name: "[account_name]", Description: "Account name to re-authenticate (defaults to active account).", Complete: CompleteAccountNames},
		},
		Args: clihelp.MaximumNArgs(1),
		Run: func(ctx *clihelp.Context) error {
			return runAccountLogin(session, ctx.Args)
		},
		Examples: []clihelp.Example{
			{Line: "feral account login"},
			{Line: "feral account login personal-gmail"},
		},
	}
}

// accountUserFieldSpec describes an `account user-<field>` subcommand, which
// sets one field of the identity used for outgoing mail from an account.
type accountUserFieldSpec struct {
	name        string
	title       string
	description string
	longDesc    string
	valueName   string
	valueDesc   string
	accountDesc string
	example     string
	set         func(*Session, string, string) error
}

func newAccountUserFieldCmd(session *Session, spec accountUserFieldSpec) clihelp.Command {
	return clihelp.Command{
		Name:            spec.name,
		Title:           spec.title,
		Description:     spec.description,
		LongDescription: spec.longDesc,
		UsageLine:       "feral account " + spec.name + " <account_name> " + spec.valueName,
		Parameters: []clihelp.Param{
			{Name: "<account_name>", Description: spec.accountDesc, Complete: CompleteAccountNames},
			{Name: spec.valueName, Description: spec.valueDesc},
		},
		Args: clihelp.ExactArgs(2),
		Run: func(ctx *clihelp.Context) error {
			return spec.set(session, ctx.Args[0], ctx.Args[1])
		},
		Examples: []clihelp.Example{
			{Line: spec.example},
		},
	}
}

func newAccountUserNameCmd(session *Session) clihelp.Command {
	return newAccountUserFieldCmd(session, accountUserFieldSpec{
		name:        "user-name",
		title:       "account user-name <account> <name>",
		description: "Set the name shown in outgoing mail.",
		longDesc:    "Set the personal name placed in the From header of mail you send from an account. It is separate from the account's display name, which only labels the account in listings and the TUI; the display name is never used as the sender.",
		valueName:   "<user_name>",
		valueDesc:   "Name to show in the From header, e.g. \"Sariel Har-Peled\".",
		accountDesc: "The account whose outgoing name to set.",
		example:     "feral account user-name fastmail \"Sariel Har-Peled\"",
		set:         runAccountUserName,
	})
}

func newAccountUserEmailCmd(session *Session) clihelp.Command {
	return newAccountUserFieldCmd(session, accountUserFieldSpec{
		name:        "user-email",
		title:       "account user-email <account> <email>",
		description: "Set the address outgoing mail is sent from.",
		longDesc:    "Set the From address used when sending from an account, which may differ from the login username -- a Fastmail alias or a verified send-as identity, for example. The login credential is unchanged; only the From header and the submission envelope use this address.",
		valueName:   "<email>",
		valueDesc:   "Address to send from, e.g. an alias of the account.",
		accountDesc: "The account whose send address to set.",
		example:     "feral account user-email fastmail sariel@sarielhp.org",
		set:         runAccountUserEmail,
	})
}

func newAccountDeleteCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "rm",
		NoAbbrev:    true,
		Aliases:     []string{"delete", "del"},
		Title:       "account rm <account_name>",
		Description: "Delete an account and its credentials.",
		UsageLine:   "feral account rm <account_name>",
		Parameters: []clihelp.Param{
			{Name: "<account_name>", Description: "The name of the account to delete.", Complete: CompleteAccountNames},
		},
		Args: clihelp.ExactArgs(1),
		Options: []clihelp.Option{
			clihelp.Bool(&flagAccountDeleteYes, "-y, --yes", false, "Proceed without the confirmation prompt."),
		},
		Run: func(ctx *clihelp.Context) error {
			if err := session.Confirm(fmt.Sprintf("delete account %q and its credentials", ctx.Args[0]),
				flagAccountDeleteYes); err != nil {
				return declined(session, err)
			}
			return runAccountDelete(session, ctx.Args[0])
		},
		Examples: []clihelp.Example{
			{Line: "feral account rm temp-account"},
		},
	}
}

// accountIdentityCmds are the commands that set who an account is to the
// people it writes to and to the user: its name and the label it is shown
// under, its signature, whether it is the default, and the sender name and
// address its mail carries.
func accountIdentityCmds(session *Session) []clihelp.Command {
	return []clihelp.Command{
		newAccountRenameCmd(session),
		newAccountDisplayNameCmd(session),
		newAccountSignatureCmd(session),
		newAccountDefaultCmd(session),
		newAccountUserNameCmd(session),
		newAccountUserEmailCmd(session),
	}
}

// AccountsCmd returns the clihelp.Command for the account command.
func AccountsCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "account",
		Group:       "Setup",
		Description: "Manage and list configured mail accounts.",
		UsageLine:   "feral account <subcommand> [args...]",
		Subcommands: slices.Concat([]clihelp.Command{
			newAccountListCmd(session),
			accountNewCmd(session),
			newAccountAssociateCmd(session),
			newAccountTestCmd(session),
			newAccountCalendarCmd(session),
			newAccountLoginCmd(session),
		}, accountIdentityCmds(session), []clihelp.Command{
			newAccountStaleCmd(session),
			newAccountInboxMaxAgeCmd(session),
			newAccountDeleteCmd(session),
		}),
		Examples: []clihelp.Example{
			{Line: "feral account list"},
			{Line: "feral account new personal-gmail"},
			{Line: "feral account test"},
			{Line: "feral account rename old-gmail new-gmail"},
			{Line: "feral account rm temp-account"},
		},
	}
}

// accountJSON is one account as a script sees it.
type accountJSON struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Type        string `json:"type"`
	AccountType string `json:"account_type"`
	Selected    bool   `json:"selected"`
	Default     bool   `json:"default"`
}

// accountListing renders the accounts for --json.
func accountListing(session *Session, accounts []cfg_acc.AccountConfig) []accountJSON {
	out := make([]accountJSON, 0, len(accounts))
	for _, acc := range accounts {
		accType := acc.AccountType
		if accType == "" {
			accType = "regular"
		}
		out = append(out, accountJSON{
			Name:        acc.Name,
			DisplayName: acc.GetDisplayName(),
			Type:        acc.Type,
			AccountType: accType,
			Selected:    session.Config.IsSelected(acc),
			Default:     acc.IsDefault,
		})
	}
	return out
}
