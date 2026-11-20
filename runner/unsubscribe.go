package runner

import (
	"fmt"
	"io"
	"os"

	"github.com/sarielhp/feral/browser"
	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/cli"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/mailclient"
	"github.com/sarielhp/feral/report"
)

// UnsubscribeService implements cli.UnsubscribeOps: `msg unsub`.
//
// PostOneClick and OpenBrowser default to the SSRF-guarded POST and the
// user's browser; tests set them, so no test reaches the network or starts a
// program.
type UnsubscribeService struct {
	Rep          report.Reporter
	Out          io.Writer
	PostOneClick func(url string) error
	OpenBrowser  func(config *cfg_g.Config, url string) error
}

func (s UnsubscribeService) rep() report.Reporter { return report.Or(s.Rep) }

func (s UnsubscribeService) out() io.Writer {
	if s.Out != nil {
		return s.Out
	}
	return os.Stdout
}

// Unsubscribe finds the message in its own account, shows what leaving its
// list would do, and does it once confirmed. Under --read-only or --dry-run
// -- or for an account that refuses changes -- it stops after showing.
//
// The mailto is sent straight through the account's client, not through
// SendService.Send: an unsubscribe message that fails is reported, not
// spooled to the outbox for a later run, and its address is not recorded
// in the contacts history.
func (s UnsubscribeService) Unsubscribe(config *cfg_g.Config, opts cli.UnsubscribeOptions) error {
	client, acc, _, raw, _, err := findMessageAcrossAccounts(config, opts.Account, opts.MessageID, s.rep())
	if err != nil {
		return err
	}
	unsub, err := email.PrepareUnsubscribe(raw)
	if err != nil {
		return fmt.Errorf("message %s: %w", opts.MessageID, err)
	}
	unsub.From = accountFromHeader(acc)

	fmt.Fprint(s.out(), unsub.Preview())
	if config.ReadOnly || config.DryRun || mailclient.Refuses(client) {
		mode := "[READ-ONLY]"
		if config.DryRun {
			mode = "[DRY-RUN]"
		}
		s.rep().Infof("%s Would %s via %s; nothing was sent.", mode, unsub.Summary(), acc.Name)
		return nil
	}
	if opts.Confirm == nil {
		return fmt.Errorf("unsubscribing needs a confirmation, and none was available")
	}
	if err := opts.Confirm("Unsubscribe from " + unsub.Sender); err != nil {
		return err
	}

	if err := unsub.CarryOut(s.effects(config, client), config.ConfigDir); err != nil {
		return fmt.Errorf("unsubscribing from %s failed: %w", unsub.Sender, err)
	}
	if unsub.Plan.Method == email.UnsubscribeBrowser {
		s.rep().Successf("Opened the unsubscribe page in the browser; finish there.")
		return nil
	}
	s.rep().Successf("Unsubscribed from %s (%s).", unsub.Sender, unsub.Plan.Method)
	return nil
}

func (s UnsubscribeService) effects(config *cfg_g.Config, client mailclient.MailClient) email.UnsubscribeEffects {
	post := s.PostOneClick
	if post == nil {
		post = email.ExecuteOneClickUnsubscribe
	}
	open := s.OpenBrowser
	if open == nil {
		open = openInBrowser
	}
	return email.UnsubscribeEffects{
		PostOneClick: post,
		OpenBrowser:  func(url string) error { return open(config, url) },
		SendMail:     client.SendEmail,
	}
}

// openInBrowser opens url in the configured browser, else the platform's.
func openInBrowser(config *cfg_g.Config, url string) error {
	if config != nil && config.Browser != "" {
		if err := browser.OpenWith(config.Browser, url); err == nil {
			return nil
		}
	}
	return browser.Open(url)
}

// accountFromHeader is the From header the account sends as.
func accountFromHeader(acc cfg_acc.AccountConfig) string {
	sendAddr := acc.GetSendAddress()
	if name := acc.GetSenderName(); name != "" && sendAddr != "" {
		return fmt.Sprintf("%s <%s>", name, sendAddr)
	}
	return sendAddr
}
