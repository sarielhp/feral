package gmail

import (
	"fmt"
	"strings"

	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/report"

	gmailapi "google.golang.org/api/gmail/v1"
)

// politicalSenders are the senders the political sweep has blacklisted:
// the account's internal rules, which only that auto-blacklist writes, and
// the senders this run added, which are not in config.Rules yet because
// AutoBlacklistInternal writes them to the config file.
func politicalSenders(rules []rule, added []string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		key := strings.ToLower(strings.TrimSpace(s))
		if key != "" && !seen[key] {
			seen[key] = true
			out = append(out, strings.TrimSpace(s))
		}
	}
	for _, r := range rules {
		if r.Internal && r.Subject == "" {
			add(r.Sender)
		}
	}
	for _, s := range added {
		add(s)
	}
	return out
}

// blockSendersOnGmail makes Gmail itself keep each political sender's mail
// out of the inbox: a server filter, from the sender, that files their mail
// under the spam-learn label and skips the inbox -- the filter `msg block
// --push` makes. The local blacklist acts only when feral scans; the filter
// acts on delivery, so the mail never reaches the inbox, a phone or a
// new-mail notification. A sweep pushes every blacklisted political sender,
// so the ones blacklisted before this existed are blocked too.
//
// A sender that some filter already names is left alone, whatever that filter
// does: one the user wrote is theirs, and one an earlier sweep wrote is done.
// Whitelisted senders are skipped, and under --read-only or --dry-run it only
// says what it would do.
func blockSendersOnGmail(rep report.Reporter, srv *gmailapi.Service, config *Config, senders []string) error {
	var todo []string
	for _, s := range senders {
		if !cfg_g.IsWhitelisted(s, config.Whitelist) {
			todo = append(todo, s)
		}
	}
	if len(todo) == 0 {
		return nil
	}
	if config.ReadOnly || config.DryRun {
		rep.Infof("[DRY-RUN] Would make sure %d political sender(s) are blocked by Gmail filters.\n", len(todo))
		return nil
	}
	filters, err := srv.Users.Settings.Filters.List("me").Do()
	if err != nil {
		return fmt.Errorf("list Gmail filters: %w", err)
	}
	labelID := ""
	created := 0
	for _, sender := range todo {
		if findExistingRuleFilter(filters.Filter, rule{Sender: sender}) != nil {
			continue
		}
		if labelID == "" {
			if labelID, err = spamLearnLabelID(rep, srv, config); err != nil {
				return err
			}
		}
		if _, err := srv.Users.Settings.Filters.Create("me", buildRuleFilter(rule{Sender: sender}, labelID)).Do(); err != nil {
			return fmt.Errorf("create Gmail filter for %s: %w", sender, err)
		}
		created++
		rep.Successf("Blocked %s on Gmail: their mail is filed under %s, skipping the inbox.\n",
			sender, spamLearnLabel(config))
	}
	if created == 0 {
		rep.Infof("All %d blacklisted political sender(s) are already blocked on Gmail.\n", len(todo))
	}
	return nil
}

// spamLearnLabel is the label blocked mail is filed under.
func spamLearnLabel(config *Config) string {
	if config.SpamLearn != "" {
		return config.SpamLearn
	}
	return "LearnSpam"
}

// spamLearnLabelID resolves the spam-learn label, creating it if need be.
func spamLearnLabelID(rep report.Reporter, srv *gmailapi.Service, config *Config) (string, error) {
	labels, err := srv.Users.Labels.List("me").Do()
	if err != nil {
		return "", fmt.Errorf("list Gmail labels: %w", err)
	}
	nameToID := make(map[string]string, len(labels.Labels))
	idToName := make(map[string]string, len(labels.Labels))
	for _, l := range labels.Labels {
		nameToID[strings.ToLower(l.Name)] = l.Id
		idToName[l.Id] = l.Name
	}
	return ensureLabelHierarchyREST(rep, srv, spamLearnLabel(config), nameToID, idToName)
}
