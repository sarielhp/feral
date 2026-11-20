package cfghandler

import (
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/sarielhp/feral/app"
	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
)

// HandleConfigShow renders the resolved configuration to w: global settings,
// the local LLM section, and every account with the selected one marked. It
// errors only when the config cannot be resolved.
func HandleConfigShow(w io.Writer, config *cfg_g.Config) error {
	resolved, err := cfg_g.Resolve(config)
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "======================================================================\n")
	fmt.Fprintf(w, "%s\n", app.ColorBoldCyan.Sprint("                        FERAL CONFIGURATION"))
	fmt.Fprintf(w, "======================================================================\n")
	fmt.Fprintf(w, "  Download/Cache Dir:  %s\n", config.CacheRoot)
	fmt.Fprintf(w, "  Score Threshold:     %.2f\n", config.ScoreThreshold)
	fmt.Fprintf(w, "  Limit:               %d\n", config.Limit)
	fmt.Fprintf(w, "  Block Political:     %t\n", config.BlockPolitical)
	fmt.Fprintf(w, "  Auto Unsubscribe:    %t\n", config.AutoUnsubscribe)
	browserDisp := "system default"
	if config.Browser != "" {
		browserDisp = config.Browser
	}
	fmt.Fprintf(w, "  Browser Command:     %s\n", browserDisp)
	saveDirDisp := "system default (~/Downloads)"
	if config.AttachmentSaveDir != "" {
		saveDirDisp = config.AttachmentSaveDir
	}
	fmt.Fprintf(w, "  Attachment Save Dir: %s\n", saveDirDisp)
	fmt.Fprintf(w, "  Save Locations:      %s\n", strings.Join(config.SaveLocationsOrDefault(), ", "))
	fmt.Fprintf(w, "  My Emails:           %s\n", strings.Join(config.MyEmails, ", "))
	muttDisp := "[not configured]"
	if config.MuttAliases != "" {
		muttDisp = config.MuttAliases
	}
	fmt.Fprintf(w, "  Mutt Aliases File:   %s\n", muttDisp)
	sendAccDisp := "[not configured]"
	if config.DefaultSendAccount != "" {
		sendAccDisp = config.DefaultSendAccount
	}
	fmt.Fprintf(w, "  Default Send Account: %s\n", sendAccDisp)
	fmt.Fprintf(w, "\n")
	printLLMConfig(w, config)
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "%s\n", app.ColorBoldGreen.Sprint("  ACCOUNTS:"))
	for _, acc := range *resolved.File.Accounts {
		prefix := "  "
		if strings.EqualFold(acc.Name, resolved.Account.Name) {
			prefix = "* "
		}
		fmt.Fprintf(w, "  %sAccount: %s (%s)\n", prefix, acc.Name, acc.Type)
		fmt.Fprintf(w, "      Username:        %s\n", acc.Username)
		fmt.Fprintf(w, "      Spam Folder:     %s\n", acc.SpamFolder)
		fmt.Fprintf(w, "      Spam Learn:      %s\n", acc.SpamLearn)
		fmt.Fprintf(w, "      Unspam Learn:    %s\n", acc.UnspamLearn)
		if len(acc.Aliases) > 0 {
			fmt.Fprintf(w, "      Aliases:         %s\n", strings.Join(acc.Aliases, ", "))
		}
		fmt.Fprintf(w, "\n")
	}
	fmt.Fprintf(w, "======================================================================\n")
	return nil
}

// printLLMConfig renders the LOCAL LLM section of `config show`. It is a helper
// rather than another block in HandleConfigShow because that function was at its
// branch ceiling, and the section has its own shape: an endpoint that is not set
// short-circuits, then every field is a line.
func printLLMConfig(w io.Writer, config *cfg_g.Config) {
	fmt.Fprintf(w, "%s\n", app.ColorBoldGreen.Sprint("  LOCAL LLM:"))
	if config.LLM.Endpoint == "" {
		fmt.Fprintf(w, "      [Not configured] Run 'tools/setup_local_llm' or 'feral config set llm_endpoint <url>'\n")
		return
	}
	fmt.Fprintf(w, "      Endpoint:        %s\n", config.LLM.Endpoint)
	model := config.LLM.Model
	if model == "" {
		model = "llama3.1 (default)"
	}
	fmt.Fprintf(w, "      Model:           %s\n", model)
	fmt.Fprintf(w, "      API Key:         %s\n", maskAPIKey(config.LLM.APIKey))
	if config.LLM.Temperature != nil {
		fmt.Fprintf(w, "      Temperature:     %.2f\n", *config.LLM.Temperature)
	}
	if config.LLM.MaxTokens != nil {
		fmt.Fprintf(w, "      Max Tokens:      %d\n", *config.LLM.MaxTokens)
	}
	effortDisp := config.LLM.ReasoningEffort
	if effortDisp == "" {
		effortDisp = "server default"
	}
	fmt.Fprintf(w, "      Reasoning Effort: %s\n", effortDisp)
	autoSumDisp := "true (enabled by default)"
	if !config.LLM.IsAutoSummaryEnabled() {
		autoSumDisp = "false (disabled)"
	}
	fmt.Fprintf(w, "      Auto Summary:    %s\n", autoSumDisp)
	autoSubjDisp := "true (enabled by default)"
	if !config.LLM.IsAutoSubjectEnabled() {
		autoSubjDisp = "false (disabled)"
	}
	fmt.Fprintf(w, "      Auto Subject:    %s\n", autoSubjDisp)
}

// maskAPIKey shows enough of a key to recognize it without printing a secret.
func maskAPIKey(key string) string {
	if key == "" {
		return "[not configured]"
	}
	if len(key) > 8 {
		return key[:4] + "..." + key[len(key)-3:] + " [configured]"
	}
	return "[configured]"
}

func validateCacheDir(w io.Writer, dir string) bool {
	fmt.Fprintf(w, "[*] Checking Download/Cache directory: %s\n", dir)
	if dir == "" {
		fmt.Fprintf(w, "%s\n", app.ColorBoldRed.Sprint("  [FAIL] download_dir is empty"))
		return false
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		fmt.Fprintf(w, "%s", app.ColorBoldRed.Sprintf("  [FAIL] failed to create download_dir: %v\n", err))
		return false
	}
	fmt.Fprintf(w, "%s\n", app.ColorBoldGreen.Sprint("  [OK] Cache directory exists and is writable"))
	return true
}

func validateAccountHost(w io.Writer, acc cfg_acc.AccountConfig) bool {
	var host string
	if strings.EqualFold(acc.Type, "gmail") {
		host = acc.IMAPHost
	} else if strings.EqualFold(acc.Type, "jmap") {
		if u, err := url.Parse(acc.SessionURL); err == nil {
			host = u.Host
		}
	}
	if host == "" {
		return true
	}
	if strings.Contains(host, ":") {
		host, _, _ = net.SplitHostPort(host)
	}
	fmt.Fprintf(w, "    Checking reachability of host: %s...\n", host)
	addrs, err := net.LookupHost(host)
	if err != nil || len(addrs) == 0 {
		fmt.Fprintf(w, "%s", app.ColorBoldRed.Sprintf("    [FAIL] Could not resolve host name: %v\n", err))
		return false
	}
	fmt.Fprintf(w, "%s", app.ColorBoldGreen.Sprintf("    [OK] Resolved host to: %s\n", strings.Join(addrs, ", ")))
	return true
}

func validateAccounts(w io.Writer, accounts []cfg_acc.AccountConfig) bool {
	if len(accounts) == 0 {
		fmt.Fprintf(w, "%s\n", app.ColorBoldRed.Sprint("[FAIL] No accounts configured in config.json"))
		return false
	}
	hasErrors := false
	fmt.Fprintf(w, "[*] Checking %d account(s)...\n", len(accounts))
	for _, acc := range accounts {
		fmt.Fprintf(w, "  Account %q (%s):\n", acc.Name, acc.Type)
		if err := cfg_g.ValidateAccountParams(acc); err != nil {
			fmt.Fprintf(w, "%s", app.ColorBoldRed.Sprintf("    [FAIL] Parameters invalid: %v\n", err))
			hasErrors = true
		} else {
			fmt.Fprintf(w, "%s\n", app.ColorBoldGreen.Sprint("    [OK] Parameters are valid"))
		}
		if !validateAccountHost(w, acc) {
			hasErrors = true
		}
		// A warning, not a failure: the account works, and is fixed by
		// renaming it rather than by anything the check could repair.
		if err := cfg_g.ValidateAccountName(acc.Name); err != nil {
			fmt.Fprintf(w, "%s\n", app.ColorBoldYellow.Sprintf("    [WARN] %v; rename it: feral account rename %s <new_name>", err, acc.Name))
		}
	}
	return !hasErrors
}

func validateBogofilter(w io.Writer) bool {
	fmt.Fprintf(w, "[*] Checking local Bogofilter installation...\n")
	bogoPath, err := exec.LookPath("bogofilter")
	if err != nil {
		fmt.Fprintf(w, "%s\n", app.ColorBoldRed.Sprint("  [FAIL] Bogofilter executable not found in PATH."))
		fmt.Fprintf(w, "%s\n", app.ColorBoldRed.Sprint("         Please install bogofilter on your system."))
		fmt.Fprintf(w, "%s\n", app.ColorBoldRed.Sprint("         On Debian/Ubuntu, run: sudo apt-get install bogofilter"))
		return false
	}
	cmd := exec.Command(bogoPath, "-V")
	output, errRun := cmd.CombinedOutput()
	if errRun != nil {
		fmt.Fprintf(w, "%s", app.ColorBoldRed.Sprintf("  [FAIL] Bogofilter version check failed: %v\n", errRun))
		return false
	}
	versionLine := ""
	lines := strings.Split(string(output), "\n")
	if len(lines) > 0 {
		versionLine = strings.TrimSpace(lines[0])
	}
	fmt.Fprintf(w, "%s", app.ColorBoldGreen.Sprintf("  [OK] Bogofilter is available: %s (%s)\n", bogoPath, versionLine))
	return true
}

func validateLLMEndpoint(w io.Writer, llm *cfg_g.LLMConfig) bool {
	if llm == nil || llm.Endpoint == "" {
		return true
	}
	fmt.Fprintf(w, "[*] Checking LLM Endpoint: %s\n", llm.Endpoint)
	u, err := url.Parse(llm.Endpoint)
	if err != nil || u.Host == "" {
		fmt.Fprintf(w, "%s", app.ColorBoldRed.Sprintf("  [FAIL] Invalid LLM endpoint URL: %v\n", err))
		return false
	}
	host := u.Host
	if strings.Contains(host, ":") {
		host, _, _ = net.SplitHostPort(host)
	}
	fmt.Fprintf(w, "    Checking reachability of LLM host %s...\n", host)
	addrs, errLookup := net.LookupHost(host)
	if errLookup != nil || len(addrs) == 0 {
		fmt.Fprintf(w, "%s", app.ColorBoldYellow.Sprintf("    [WARN] Could not resolve LLM host name %q (check if Tailscale/VPN/LAN is reachable)\n", host))
	} else {
		fmt.Fprintf(w, "%s", app.ColorBoldGreen.Sprintf("    [OK] Resolved LLM host to: %s\n", strings.Join(addrs, ", ")))
	}
	return true
}

// PerformConfigValidation checks the resolved config's cache directory,
// accounts, bogofilter and LLM endpoint, writing a report to w, and returns an
// error when any of them fails. It reports every problem rather than stopping at
// the first.
func PerformConfigValidation(w io.Writer, config *cfg_g.Config) error {
	resolved, err := cfg_g.Resolve(config)
	if err != nil {
		return fmt.Errorf("failed to read/parse config: %w", err)
	}

	fmt.Fprintf(w, "%s\n", app.ColorBoldCyan.Sprint("\n========================================================"))
	fmt.Fprintf(w, "%s\n", app.ColorBoldCyan.Sprint("              FERAL CONFIG VALIDATION"))
	fmt.Fprintf(w, "%s\n", app.ColorBoldCyan.Sprint("========================================================"))
	fmt.Fprintf(w, "Config file path: %s\n\n", resolved.Path)

	hasErrors := false
	if !validateCacheDir(w, resolved.File.DownloadDir) {
		hasErrors = true
	}
	fmt.Fprintf(w, "\n")

	var accs []cfg_acc.AccountConfig
	if resolved.File.Accounts != nil {
		accs = *resolved.File.Accounts
	}
	if cfg_g.DanglingDefault(accs, resolved.File.DefaultAccount) {
		fmt.Fprintf(w, "%s\n", app.ColorBoldYellow.Sprintf("[WARN] default_account names %q, which is not configured; the first account is used", resolved.File.DefaultAccount))
	}
	if !validateAccounts(w, accs) {
		hasErrors = true
	}
	fmt.Fprintf(w, "\n")

	if !validateBogofilter(w) {
		hasErrors = true
	}
	fmt.Fprintf(w, "\n")

	if !validateLLMEndpoint(w, resolved.File.LLM) {
		hasErrors = true
	}

	fmt.Fprintf(w, "%s\n", app.ColorBoldCyan.Sprint("\n========================================================"))
	if hasErrors {
		fmt.Fprintf(w, "%s\n", app.ColorBoldRed.Sprint("  VALIDATION FAILED: Please fix configuration errors above."))
	} else {
		fmt.Fprintf(w, "%s\n", app.ColorBoldGreen.Sprint("  VALIDATION SUCCESSFUL: Configuration is healthy!"))
	}
	fmt.Fprintf(w, "%s\n", app.ColorBoldCyan.Sprint("========================================================"))
	fmt.Fprintf(w, "\n")

	if hasErrors {
		return fmt.Errorf("configuration validation failed")
	}
	return nil
}

// parseConfigBool reads a switch the way the llm_* keys always have: anything
// but an explicit no is on.
func parseConfigBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "false", "0", "no", "off":
		return false
	}
	return true
}

func handleConfigSetLLM(w io.Writer, config *cfg_g.Config, fc *cfg_g.FileConfig, configPath, key, value string) (bool, error) {
	if fc.LLM == nil {
		fc.LLM = &cfg_g.LLMConfig{}
	}
	switch key {
	case "llm_endpoint", "llm.endpoint", "llm_url":
		fc.LLM.Endpoint = value
		config.LLM.Endpoint = value
		fprintfPrefixed(w, app.PrefixSuccess, "Successfully updated LLM endpoint to %q.\n", value)
	case "llm_api_key", "llm.api_key", "llm_key":
		fc.LLM.APIKey = value
		config.LLM.APIKey = value
		fprintfPrefixed(w, app.PrefixSuccess, "Successfully updated LLM API key.\n")
	case "llm_model", "llm.model":
		fc.LLM.Model = value
		config.LLM.Model = value
		fprintfPrefixed(w, app.PrefixSuccess, "Successfully updated LLM model to %q.\n", value)
	case "llm_temperature", "llm.temperature":
		temp, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return true, fmt.Errorf("invalid temperature value %q (must be a float between 0.0 and 2.0): %w", value, err)
		}
		fc.LLM.Temperature = &temp
		config.LLM.Temperature = &temp
		fprintfPrefixed(w, app.PrefixSuccess, "Successfully updated LLM temperature to %.2f.\n", temp)
	case "llm_max_tokens", "llm.max_tokens":
		tokens, err := strconv.Atoi(value)
		if err != nil || tokens <= 0 {
			return true, fmt.Errorf("invalid max_tokens value %q (must be a positive integer): %w", value, err)
		}
		fc.LLM.MaxTokens = &tokens
		config.LLM.MaxTokens = &tokens
		fprintfPrefixed(w, app.PrefixSuccess, "Successfully updated LLM max tokens to %d.\n", tokens)
	case "llm_system_prompt", "llm.system_prompt":
		fc.LLM.SystemPrompt = value
		config.LLM.SystemPrompt = value
		fprintfPrefixed(w, app.PrefixSuccess, "Successfully updated LLM system prompt.\n")
	case "llm_reasoning_effort", "llm.reasoning_effort":
		effort := strings.ToLower(strings.TrimSpace(value))
		switch effort {
		case "none", "low", "medium", "high":
		default:
			return true, fmt.Errorf("invalid reasoning effort %q (expected none, low, medium, or high)", value)
		}
		fc.LLM.ReasoningEffort = effort
		config.LLM.ReasoningEffort = effort
		fprintfPrefixed(w, app.PrefixSuccess, "Successfully updated LLM reasoning effort to %q.\n", effort)
	case "llm_auto_summary", "llm.auto_summary", "llm_background_summary", "llm.background_summary":
		val := parseConfigBool(value)
		fc.LLM.AutoSummary = &val
		config.LLM.AutoSummary = &val
		fprintfPrefixed(w, app.PrefixSuccess, "Successfully updated LLM background auto-summary to %t.\n", val)
	case "llm_auto_subject", "llm.auto_subject":
		val := parseConfigBool(value)
		fc.LLM.AutoSubject = &val
		config.LLM.AutoSubject = &val
		fprintfPrefixed(w, app.PrefixSuccess, "Successfully updated LLM auto-subject to %t.\n", val)
	default:
		return false, nil
	}
	return true, cfg_g.SaveConfigFile(configPath, fc)
}

func handleConfigSetAccount(w io.Writer, config *cfg_g.Config, fc *cfg_g.FileConfig, targetAcc *cfg_acc.AccountConfig, configPath, key, value string) error {
	var accToModify *cfg_acc.AccountConfig
	for i := range *fc.Accounts {
		if strings.EqualFold((*fc.Accounts)[i].Name, targetAcc.Name) {
			accToModify = &(*fc.Accounts)[i]
			break
		}
	}
	if accToModify == nil {
		return fmt.Errorf("could not find account %q in config file", targetAcc.Name)
	}

	switch key {
	case "spam_learn":
		accToModify.SpamLearn = value
	case "unspam_learn":
		accToModify.UnspamLearn = value
	case "badge":
		accToModify.Badge = value
	case "username":
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("username cannot be empty")
		}
		accToModify.Username = strings.TrimSpace(value)
	default:
		return fmt.Errorf("unsupported config key %q. Supported keys: spam_learn, unspam_learn, badge, username, browser, llm_endpoint, llm_api_key, llm_model, llm_temperature, llm_max_tokens, llm_system_prompt, llm_reasoning_effort, llm_auto_summary, llm_auto_subject", key)
	}

	if err := cfg_g.SaveConfigFile(configPath, fc); err != nil {
		return err
	}
	if config.IsSelected(*accToModify) {
		if key == "spam_learn" {
			config.SpamLearn = value
		} else if key == "unspam_learn" {
			config.UnspamLearn = value
		}
	}
	fprintfPrefixed(w, app.PrefixSuccess, "Successfully updated %s to %q for account %s.\n", key, value, accToModify.Name)
	return nil
}

// HandleConfigSet writes one configuration key.
//
// Which file it lands in is decided by the key, not by the caller: the keys
// handleConfigSetGlobal knows are global, the llm_* keys have their own handler,
// and everything else belongs to the account -A selected. There used to be an
// accountSpecific bool threaded here through three layers, set from an
// undocumented third positional argument, and read nowhere in this function --
// so `config set key value anything` promised an account override and wrote
// exactly what it would have written anyway.
func HandleConfigSet(w io.Writer, config *cfg_g.Config, key, value string) error {
	resolved, err := cfg_g.Resolve(config)
	if err != nil {
		return err
	}

	if handled, globalErr := handleConfigSetGlobal(w, config, resolved, key, value); handled {
		return globalErr
	}

	handled, err := handleConfigSetLLM(w, config, resolved.File, resolved.Path, key, value)
	if handled {
		return err
	}

	return handleConfigSetAccount(w, config, resolved.File, resolved.Account, resolved.Path, key, value)
}

// HandleConfigReset clears the named key back to its default, in both the
// loaded config and the file on disk, and errors on an unsupported key.
func HandleConfigReset(w io.Writer, config *cfg_g.Config, key string) error {
	resolved, err := cfg_g.Resolve(config)
	if err != nil {
		return err
	}

	switch key {
	case "browser":
		resolved.File.Browser = ""
		config.Browser = ""
	case "attachment_save_dir", "save_dir":
		resolved.File.AttachmentSaveDir = ""
		config.AttachmentSaveDir = ""
	case "save_locations":
		resolved.File.SaveLocations = nil
		config.SaveLocations = nil
	case "my_emails":
		resolved.File.MyEmails = nil
		config.MyEmails = nil
	case "mutt_aliases", "mutt_alias_file", "mutt_aliases_file", "mutt_file", "paths.mutt_aliases", "paths.mutt_alias_file":
		resolved.File.MuttAliases = ""
		config.MuttAliases = ""
	case "default_send_account", "default_send", "send_account":
		resolved.File.DefaultSendAccount = ""
		config.DefaultSendAccount = ""
	case "default_account":
		resolved.File.DefaultAccount = ""
		config.DefaultAccount = ""
	case "llm":
		resolved.File.LLM = nil
		config.LLM = cfg_g.LLMConfig{}
	case "llm_endpoint", "llm.endpoint":
		if resolved.File.LLM != nil {
			resolved.File.LLM.Endpoint = ""
		}
		config.LLM.Endpoint = ""
	case "llm_api_key", "llm.api_key":
		if resolved.File.LLM != nil {
			resolved.File.LLM.APIKey = ""
		}
		config.LLM.APIKey = ""
	case "llm_model", "llm.model":
		if resolved.File.LLM != nil {
			resolved.File.LLM.Model = ""
		}
		config.LLM.Model = ""
	case "llm_temperature", "llm.temperature":
		if resolved.File.LLM != nil {
			resolved.File.LLM.Temperature = nil
		}
		config.LLM.Temperature = nil
	case "llm_max_tokens", "llm.max_tokens":
		if resolved.File.LLM != nil {
			resolved.File.LLM.MaxTokens = nil
		}
		config.LLM.MaxTokens = nil
	case "llm_system_prompt", "llm.system_prompt":
		if resolved.File.LLM != nil {
			resolved.File.LLM.SystemPrompt = ""
		}
		config.LLM.SystemPrompt = ""
	case "llm_reasoning_effort", "llm.reasoning_effort":
		if resolved.File.LLM != nil {
			resolved.File.LLM.ReasoningEffort = ""
		}
		config.LLM.ReasoningEffort = ""
	case "llm_auto_summary", "llm.auto_summary", "llm_background_summary":
		if resolved.File.LLM != nil {
			resolved.File.LLM.AutoSummary = nil
		}
		config.LLM.AutoSummary = nil
	case "llm_auto_subject", "llm.auto_subject":
		if resolved.File.LLM != nil {
			resolved.File.LLM.AutoSubject = nil
		}
		config.LLM.AutoSubject = nil
	default:
		return fmt.Errorf("unsupported config reset key %q. Supported keys: browser, attachment_save_dir, save_locations, my_emails, default_send_account, mutt_alias_file, mutt_aliases, llm, llm_endpoint, llm_api_key, llm_model, llm_temperature, llm_max_tokens, llm_system_prompt, llm_reasoning_effort, llm_auto_summary, llm_auto_subject", key)
	}

	if err := cfg_g.SaveConfigFile(resolved.Path, resolved.File); err != nil {
		return err
	}
	fprintfPrefixed(w, app.PrefixSuccess, "Successfully reset %s.\n", key)
	return nil
}

// HandleConfigGet prints the current value of a configuration key.
func HandleConfigGet(w io.Writer, config *cfg_g.Config, key string) error {
	resolved, err := cfg_g.Resolve(config)
	if err != nil {
		return err
	}

	cleanKey := strings.ToLower(strings.TrimSpace(key))
	switch cleanKey {
	case "mutt_aliases", "mutt_alias_file", "mutt_aliases_file", "mutt_file", "paths.mutt_aliases", "paths.mutt_alias_file":
		fmt.Fprintln(w, config.MuttAliases)
		return nil
	case "default_send_account", "default_send", "send_account":
		fmt.Fprintln(w, config.DefaultSendAccount)
		return nil
	case "browser":
		fmt.Fprintln(w, config.Browser)
		return nil
	case "attachment_save_dir", "save_dir":
		fmt.Fprintln(w, config.AttachmentSaveDir)
		return nil
	case "save_locations":
		fmt.Fprintln(w, strings.Join(config.SaveLocationsOrDefault(), ", "))
		return nil
	case "my_emails":
		for _, addr := range config.MyEmails {
			fmt.Fprintln(w, addr)
		}
		return nil
	case "download_dir", "cache_dir":
		fmt.Fprintln(w, config.CacheRoot)
		return nil
	case "score_threshold":
		fmt.Fprintf(w, "%.2f\n", config.ScoreThreshold)
		return nil
	case "limit":
		fmt.Fprintf(w, "%d\n", config.Limit)
		return nil
	case "block_political":
		fmt.Fprintf(w, "%t\n", config.BlockPolitical)
		return nil
	case "auto_unsubscribe":
		fmt.Fprintf(w, "%t\n", config.AutoUnsubscribe)
		return nil
	case "llm_endpoint", "llm.endpoint":
		fmt.Fprintln(w, config.LLM.Endpoint)
		return nil
	case "llm_model", "llm.model":
		fmt.Fprintln(w, config.LLM.Model)
		return nil
	case "llm_api_key", "llm.api_key":
		fmt.Fprintln(w, config.LLM.APIKey)
		return nil
	case "llm_reasoning_effort", "llm.reasoning_effort":
		fmt.Fprintln(w, config.LLM.ReasoningEffort)
		return nil
	case "spam_folder":
		if resolved.Account != nil {
			fmt.Fprintln(w, resolved.Account.SpamFolder)
		}
		return nil
	case "spam_learn":
		if resolved.Account != nil {
			fmt.Fprintln(w, resolved.Account.SpamLearn)
		}
		return nil
	case "unspam_learn":
		if resolved.Account != nil {
			fmt.Fprintln(w, resolved.Account.UnspamLearn)
		}
		return nil
	default:
		return fmt.Errorf("unknown configuration key %q", key)
	}
}

// handleConfigSetGlobal writes a key that lives at the top of config.json
// rather than in an account -- browser, attachment_save_dir, save_locations,
// my_emails, mutt_aliases and default_send_account, with their spellings.
// handled is false for any other key.
func handleConfigSetGlobal(w io.Writer, config *cfg_g.Config, resolved cfg_g.Resolved, key, value string) (bool, error) {
	fc := resolved.File
	shown := value
	switch key {
	case "browser":
		fc.Browser, config.Browser = value, value
	case "attachment_save_dir", "save_dir":
		fc.AttachmentSaveDir, config.AttachmentSaveDir = value, value
	case "save_locations":
		locs := cfg_g.ParseSaveLocations(value)
		fc.SaveLocations, config.SaveLocations = locs, locs
		shown = strings.Join(locs, ", ")
	case "my_emails":
		addrs, err := cfg_g.ParseMyEmails(value)
		if err != nil {
			return true, err
		}
		fc.MyEmails, config.MyEmails = addrs, addrs
		shown = strings.Join(addrs, ", ")
	case "mutt_aliases", "mutt_alias_file", "mutt_aliases_file", "mutt_file", "paths.mutt_aliases", "paths.mutt_alias_file":
		fc.MuttAliases, config.MuttAliases = value, value
	case "default_send_account", "default_send", "send_account":
		if fc.Accounts != nil && len(*fc.Accounts) > 0 {
			if _, err := cfg_g.Lookup(*fc.Accounts, value); err != nil {
				return true, fmt.Errorf("cannot set %s: %w", key, err)
			}
		}
		fc.DefaultSendAccount, config.DefaultSendAccount = value, value
	default:
		return false, nil
	}
	if err := cfg_g.SaveConfigFile(resolved.Path, fc); err != nil {
		return true, err
	}
	fprintfPrefixed(w, app.PrefixSuccess, "Successfully updated %s to %q.\n", key, shown)
	return true, nil
}
