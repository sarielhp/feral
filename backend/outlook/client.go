package outlook

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/sarielhp/feral/cache/meta"
	"github.com/sarielhp/feral/label"

	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/mailclient"
	"github.com/sarielhp/feral/report"
)

const maxErrorBodyBytes = 4096

func readErrorBody(r io.Reader) string {
	b, _ := io.ReadAll(io.LimitReader(r, maxErrorBodyBytes))
	return string(b)
}

type client struct {
	config  *cfg_g.Config
	account cfg_acc.AccountConfig
	client  *http.Client
	rep     report.Reporter
	// immutableIDs is set by init when the account's cache is keyed by
	// Graph's immutable IDs. See usesImmutableIDs.
	immutableIDs bool
}

// NewClient builds a client that reports progress through rep. A nil rep
// discards, which suits a caller that has nowhere to show it.
func NewClient(acc cfg_acc.AccountConfig, config *cfg_g.Config, rep report.Reporter) mailclient.MailClient {
	return &client{
		config:  config,
		account: acc,
		rep:     report.Or(rep),
	}
}

func (c *client) init() error {
	if c.client != nil {
		return nil
	}
	c.immutableIDs = c.usesImmutableIDs()
	c.client = getOutlookClient(c.config, c.account.Name, c.rep, c.immutableIDs)
	return nil
}

// usesImmutableIDs reports whether this account's cache is keyed by Graph's
// immutable IDs, as its manifest says. A cache keyed by the default IDs keeps
// them until `utils cache outlook-ids` migrates it: mixing the two would split
// every message in two. A client built without the factory, which writes the
// manifest, finds none: an empty cache then starts with immutable IDs.
func (c *client) usesImmutableIDs() bool {
	acct := c.config.AccountCache
	if acct.IsZero() {
		return false
	}
	m, ok, err := meta.Read(acct)
	if err != nil {
		return false
	}
	if ok {
		return m.IDScheme == meta.SchemeGraphImmutable
	}
	return !meta.HoldsMessages(acct)
}

func (c *client) Validate() error {
	if err := cfg_g.ValidateAccountParams(c.account); err != nil {
		return err
	}
	if err := c.init(); err != nil {
		return err
	}

	resp, err := c.client.Get("https://graph.microsoft.com/v1.0/me/mailFolders")
	if err != nil {
		return fmt.Errorf("outlook validation request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("outlook API returned status code %d", resp.StatusCode)
	}

	return nil
}

func (c *client) Config() *cfg_g.Config {
	return c.config
}

func (c *client) inboxName() string {
	return "Inbox"
}

func (c *client) RewriteRuleLabelCasing(label string) string {
	if strings.EqualFold(label, "inbox") {
		return "Inbox"
	}
	return label
}

func (c *client) BackendType() string { return "outlook" }

// InboxFolder is the account's inbox, as the server names it.
func (c *client) InboxFolder() label.Folder {
	return label.FromServer(c.inboxName())
}
