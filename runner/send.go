package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"mime"
	"net/mail"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/sarielhp/feral/cache"
	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/cache/meta"
	"github.com/sarielhp/feral/cache/msg"
	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/cli"
	"github.com/sarielhp/feral/contacts"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/label"
	"github.com/sarielhp/feral/mailclient"
	"github.com/sarielhp/feral/mailops"
	"github.com/sarielhp/feral/organize"
	"github.com/sarielhp/feral/outbox"
	"github.com/sarielhp/feral/pgp"
	"github.com/sarielhp/feral/report"
)

// SendService implements cli.SendOps to deliver outbound emails.
type SendService struct {
	Rep report.Reporter
	Out io.Writer
	TUI cli.TUILauncher
}

func (s SendService) rep() report.Reporter { return report.Or(s.Rep) }

func (s SendService) out() io.Writer {
	if s.Out != nil {
		return s.Out
	}
	return os.Stdout
}

func (s SendService) tui() cli.TUILauncher {
	if s.TUI != nil {
		return s.TUI
	}
	return TUIService{}
}

// Send resolves the sender account, builds RFC822 MIME message, and transmits it.
func (s SendService) Send(config *cfg_g.Config, opts cli.SendOptions) error {
	if opts.Raw {
		return s.sendRaw(config, opts)
	}

	client, acc, err := mailops.GetClientForSending(config, opts.Account, s.rep())
	if err != nil {
		return err
	}

	from := accountFromHeader(*acc)

	toRecipients := expandRecipients(config, opts.To)
	ccRecipients := expandRecipients(config, opts.Cc)
	bccRecipients := expandRecipients(config, opts.Bcc)

	attachments, err := loadAttachments(opts.Attachments)
	if err != nil {
		return err
	}

	comp := &email.ComposeMessage{
		From:        from,
		To:          strings.Join(toRecipients, ", "),
		Cc:          strings.Join(ccRecipients, ", "),
		Bcc:         strings.Join(bccRecipients, ", "),
		Subject:     opts.Subject,
		Body:        email.SignBody(opts.Body, s.signatureUnless(opts.NoSignature, config, *acc), ""),
		Attachments: attachments,
		// A message sent later is dated when it is due, not when it was
		// written; zero (sent now) dates it at build time.
		Date: opts.SendAfter,
	}

	recipients := pgp.Recipients{
		Visible: append(append([]string{}, toRecipients...), ccRecipients...),
		Hidden:  bccRecipients,
	}
	rawBytes, err := s.buildOutbound(comp, opts, *acc, recipients)
	if err != nil {
		return err
	}

	if opts.DryRun {
		_, _ = s.out().Write(rawBytes)
		fmt.Fprintln(s.out())
		s.rep().Infof("[DRY-RUN] RFC822 message constructed (%d bytes) for delivery via %s (%s)", len(rawBytes), acc.Name, acc.Username)
		s.noteDryRunSchedule(opts)
		return nil
	}

	cacheRoot := ""
	if config != nil {
		cacheRoot = config.CacheRoot
	}

	if opts.Queue || !opts.SendAfter.IsZero() {
		return s.enqueue(config, outbox.Entry{Account: acc.Name, To: toRecipients, Subject: opts.Subject, SendAfter: opts.SendAfter}, rawBytes)
	}

	s.rep().Infof("Sending email via %s (%s)...", acc.Name, acc.Username)
	if err := client.SendEmail(rawBytes); err != nil {
		s.spoolOffline(outbox.Dir(cacheRoot), outbox.Entry{Account: acc.Name, To: toRecipients, Subject: opts.Subject}, rawBytes, err)
		return fmt.Errorf("failed to send email via %s: %w", acc.Name, err)
	}

	contacts.RecordSentRecipients(config, rawBytes)
	s.rep().Successf("Email sent to %s via %s (%s)", strings.Join(toRecipients, ", "), acc.Name, acc.Username)
	return nil
}

// buildOutbound builds the message and applies the PGP protection the caller
// asked for.
//
// Protection happens before the message is printed, queued or spooled, so what
// the user reviews, and what sits in the outbox, is what will be sent.
// Encrypting after spooling would leave a plaintext copy of the message in the
// cache.
func (s SendService) buildOutbound(comp *email.ComposeMessage, opts cli.SendOptions, acc cfg_acc.AccountConfig, recipients pgp.Recipients) ([]byte, error) {
	built, err := comp.Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build RFC822 email: %w", err)
	}
	if !opts.Sign && !opts.Encrypt {
		return built.Bytes(), nil
	}

	keyID := opts.PgpKeyID
	if keyID == "" {
		keyID = acc.PgpKeyID
	}

	ctx := context.Background()
	targets := recipients.Visible
	if opts.Encrypt {
		targets = s.encryptTargets(ctx, recipients.Visible, acc.GetSendAddress())
	}

	rawBytes, err := pgp.Protect(ctx, built, pgp.OutboundOptions{
		Sign:             opts.Sign,
		Encrypt:          opts.Encrypt,
		KeyID:            keyID,
		Recipients:       targets,
		HiddenRecipients: recipients.Hidden,
	})
	if err != nil {
		return nil, err
	}
	return rawBytes, nil
}

// encryptTargets is everyone the message has to be readable by: its recipients,
// plus the sender when the keyring holds a key for them, so the copy in Sent
// can be read back.
//
// A sender with no key is dropped rather than fatal -- they may be sending from
// a machine that has never held their own key -- but it is said out loud,
// because the alternative is a sent folder the sender cannot read and no
// explanation of why.
func (s SendService) encryptTargets(ctx context.Context, recipients []string, sender string) []string {
	targets := append([]string{}, recipients...)
	if sender == "" {
		return targets
	}
	if pgp.HasPublicKey(ctx, sender) {
		return append(targets, sender)
	}
	s.rep().Warnf("No public key for %s; the copy saved to Sent will not be readable here", sender)
	return targets
}

func loadAttachments(paths []string) ([]email.Attachment, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	attachments := make([]email.Attachment, 0, len(paths))
	for _, p := range paths {
		cleaned := strings.TrimSpace(p)
		if cleaned == "" {
			continue
		}
		data, err := os.ReadFile(cleaned)
		if err != nil {
			return nil, fmt.Errorf("reading attachment %q: %w", cleaned, err)
		}
		cType := mime.TypeByExtension(filepath.Ext(cleaned))
		if cType == "" {
			cType = "application/octet-stream"
		}
		attachments = append(attachments, email.Attachment{
			Filename:    filepath.Base(cleaned),
			ContentType: cType,
			Data:        data,
		})
	}
	return attachments, nil
}

func expandRecipients(config *cfg_g.Config, raw []string) []string {
	if len(raw) == 0 {
		return nil
	}
	bookPath := contacts.DefaultPath(config)
	book, _ := contacts.LoadBook(bookPath)

	var expanded []string
	for _, item := range raw {
		for _, part := range email.SplitAddressList(item) {
			expanded = append(expanded, resolveSingleRecipient(book, part)...)
		}
	}
	return expanded
}

// resolveSingleRecipient is the addresses item stands for: a contact's name
// or alias becomes "Name <addr>", a group alias every one of its members,
// and anything else stays as typed.
func resolveSingleRecipient(book *contacts.Book, item string) []string {
	if strings.Contains(item, "@") || book == nil {
		return []string{item}
	}
	for _, c := range book.Contacts {
		if matched, ok := matchContactRecipient(c, item); ok {
			return matched
		}
	}
	return []string{item}
}

func matchContactRecipient(c contacts.Contact, item string) ([]string, bool) {
	if c.PrimaryEmail() == "" || !namesContact(c, item) {
		return nil, false
	}
	// A group's name ("Group: 273ta") is a label, not a person: its members
	// go out under their own addresses.
	if c.IsGroup() {
		return slices.Clone(c.Emails), true
	}
	return []string{formatRecipient(c.Name, c.PrimaryEmail())}, true
}

// namesContact reports whether item is c's name or one of its aliases.
func namesContact(c contacts.Contact, item string) bool {
	if strings.EqualFold(c.Name, item) {
		return true
	}
	for _, nick := range c.AllAliases() {
		if strings.EqualFold(nick, item) {
			return true
		}
	}
	return false
}

func formatRecipient(name, addr string) string {
	return email.FormatAddress(name, addr)
}

func (s SendService) sendRaw(config *cfg_g.Config, opts cli.SendOptions) error {
	msg, err := mail.ReadMessage(bytes.NewReader(opts.RawBytes))
	if err != nil {
		return fmt.Errorf("parsing raw RFC822 message: %w", err)
	}

	targetAccount := opts.Account
	if targetAccount == "" {
		targetAccount = inferAccountFromFromHeader(config, msg.Header.Get("From"))
	}

	client, acc, err := mailops.GetClientForSending(config, targetAccount, s.rep())
	if err != nil {
		return err
	}

	if opts.DryRun {
		_, _ = s.out().Write(opts.RawBytes)
		fmt.Fprintln(s.out())
		s.rep().Infof("[DRY-RUN] Raw RFC822 message (%d bytes) for delivery via %s (%s)", len(opts.RawBytes), acc.Name, acc.Username)
		s.noteDryRunSchedule(opts)
		return nil
	}

	cacheRoot := ""
	if config != nil {
		cacheRoot = config.CacheRoot
	}
	to := msg.Header.Get("To")
	if to == "" {
		to = strings.Join(opts.To, ", ")
	}
	subj := msg.Header.Get("Subject")

	if opts.Queue || !opts.SendAfter.IsZero() {
		return s.enqueue(config, outbox.Entry{Account: acc.Name, To: []string{to}, Subject: subj, SendAfter: opts.SendAfter}, opts.RawBytes)
	}

	s.rep().Infof("Sending raw email via %s (%s)...", acc.Name, acc.Username)
	if err := client.SendEmail(opts.RawBytes); err != nil {
		s.spoolOffline(outbox.Dir(cacheRoot), outbox.Entry{Account: acc.Name, To: []string{to}, Subject: subj}, opts.RawBytes, err)
		return fmt.Errorf("failed to send raw email via %s: %w", acc.Name, err)
	}

	contacts.RecordSentRecipients(config, opts.RawBytes)
	s.rep().Successf("Raw email sent to %s via %s (%s)", to, acc.Name, acc.Username)
	return nil
}

func inferAccountFromFromHeader(config *cfg_g.Config, fromHeader string) string {
	if config == nil || fromHeader == "" {
		return ""
	}
	addr, err := mail.ParseAddress(fromHeader)
	if err != nil {
		return ""
	}
	for _, acc := range config.Accounts {
		if strings.EqualFold(acc.Username, addr.Address) ||
			(acc.UserEmail != "" && strings.EqualFold(acc.UserEmail, addr.Address)) {
			return acc.Name
		}
	}
	return ""
}

type replyPlan struct {
	Account       cfg_acc.AccountConfig
	Client        mailclient.MailClient
	ResolvedMsgID email.ID
	Details       email.ReplyDetails
	Subject       string
	Cc            []string
	QuotedBody    string
	Opts          cli.ReplyOptions
}

// replyDetails is the reply's headers and quote: to the list alone for
// --list, which is refused rather than sent to the sender when the message
// names no list address or the list takes no posts; otherwise to the sender,
// or to everyone for --group.
func replyDetails(origMsg *mail.Message, own map[string]bool, opts cli.ReplyOptions, authored bool) (email.ReplyDetails, error) {
	if !opts.List {
		return email.BuildReplyDetails(origMsg, own, opts.Group, authored), nil
	}
	details, err := email.BuildListReplyDetails(origMsg)
	if err != nil {
		return email.ReplyDetails{}, fmt.Errorf("reply to the list: %w", err)
	}
	return details, nil
}

// Reply resolves the target message, formats reply headers and quote, and delivers or launches compose.
func (s SendService) Reply(config *cfg_g.Config, opts cli.ReplyOptions) error {
	client, acc, resolvedMsgID, rawBytes, folder, err := findMessageAcrossAccounts(config, opts.Account, opts.MessageID, s.rep())
	if err != nil {
		return err
	}

	origMsg, err := mail.ReadMessage(bytes.NewReader(rawBytes))
	if err != nil {
		return fmt.Errorf("parsing original message: %w", err)
	}

	// Replying to the user's own mail -- a message the cache holds in the
	// account's Sent folder -- addresses the people it was sent to. The folder
	// is what decides it, not the From header, which spam forges.
	authored := folder != "" && organize.IsAuthoredFolder(client, label.FromServer(folder))
	// A group reply leaves out every address the user owns -- my_emails and
	// every account's -- not only this account's, so no copy comes back.
	own := config.OwnAddresses()
	maps.Copy(own, email.OwnSet(acc.GetSendAddress()))
	details, err := replyDetails(origMsg, own, opts, authored)
	if err != nil {
		return err
	}
	subject := details.Subject
	if opts.Subject != "" {
		subject = opts.Subject
	}

	plan := replyPlan{
		Account:       acc,
		Client:        client,
		ResolvedMsgID: resolvedMsgID,
		Details:       details,
		Subject:       subject,
		Cc:            append(details.Cc, opts.Cc...),
		QuotedBody:    details.QuotedBody,
		Opts:          opts,
	}
	if opts.NoQuote {
		plan.QuotedBody = ""
	}

	if !opts.Headless {
		return s.replyInteractive(config, plan)
	}
	return s.replyHeadless(config, plan)
}

func (s SendService) replyInteractive(config *cfg_g.Config, plan replyPlan) error {
	// The compose screen places the account's signature between the two.
	composeOpts := cli.ComposeOptions{
		To:          []string{plan.Details.To},
		Cc:          plan.Cc,
		Bcc:         plan.Opts.Bcc,
		Subject:     plan.Subject,
		Body:        plan.Opts.Body,
		Quote:       plan.QuotedBody,
		Attachments: plan.Opts.Attachments,
		Account:     plan.Account.Name,
		InReplyTo:   plan.Details.InReplyTo,
		References:  plan.Details.References,
		OriginalID:  plan.ResolvedMsgID,
	}

	return s.tui().Compose(config, composeOpts)
}

func (s SendService) replyHeadless(config *cfg_g.Config, plan replyPlan) error {
	// Reply has already dropped the quote under --no-quote.
	finalBody := email.SignBody(plan.Opts.Body, s.signatureUnless(plan.Opts.NoSignature, config, plan.Account), plan.QuotedBody)

	attachments, err := loadAttachments(plan.Opts.Attachments)
	if err != nil {
		return err
	}

	sendAddr := plan.Account.GetSendAddress()
	from := sendAddr
	if name := plan.Account.GetSenderName(); name != "" && sendAddr != "" {
		from = fmt.Sprintf("%s <%s>", name, sendAddr)
	}

	expandedTo := expandRecipients(config, []string{plan.Details.To})
	expandedCc := expandRecipients(config, plan.Cc)
	expandedBcc := expandRecipients(config, plan.Opts.Bcc)

	comp := &email.ComposeMessage{
		From:        from,
		To:          strings.Join(expandedTo, ", "),
		Cc:          strings.Join(expandedCc, ", "),
		Bcc:         strings.Join(expandedBcc, ", "),
		Subject:     plan.Subject,
		InReplyTo:   plan.Details.InReplyTo,
		References:  plan.Details.References,
		Body:        finalBody,
		Attachments: attachments,
	}

	rawRFC822, err := comp.BuildRFC822()
	if err != nil {
		return fmt.Errorf("failed to build RFC822 reply: %w", err)
	}

	if plan.Opts.DryRun {
		_, _ = s.out().Write(rawRFC822)
		fmt.Fprintln(s.out())
		s.rep().Infof("[DRY-RUN] RFC822 reply message constructed (%d bytes) for delivery via %s (%s)", len(rawRFC822), plan.Account.Name, plan.Account.Username)
		return nil
	}

	s.rep().Infof("Sending reply via %s (%s)...", plan.Account.Name, plan.Account.Username)
	if err := plan.Client.SendEmail(rawRFC822); err != nil {
		return fmt.Errorf("failed to send reply via %s: %w", plan.Account.Name, err)
	}

	contacts.RecordSentRecipients(config, rawRFC822)
	if plan.ResolvedMsgID != "" && plan.Client != nil && plan.Client.Config() != nil && !plan.Client.Config().AccountCache.IsZero() {
		_ = cache.MarkIDsReplied(plan.Client.Config().AccountCache, []email.ID{plan.ResolvedMsgID}, nil)
	}
	s.rep().Successf("Reply sent to %s via %s (%s)", strings.Join(expandedTo, ", "), plan.Account.Name, plan.Account.Username)
	return nil
}

func findCandidateAccounts(config *cfg_g.Config, overrideAccount string) ([]cfg_acc.AccountConfig, error) {
	if config == nil || len(config.Accounts) == 0 {
		return nil, fmt.Errorf("no accounts configured")
	}
	if overrideAccount != "" {
		for _, acc := range config.Accounts {
			if strings.EqualFold(acc.Name, overrideAccount) {
				return []cfg_acc.AccountConfig{acc}, nil
			}
		}
		return nil, fmt.Errorf("account %q not found", overrideAccount)
	}

	primary := string(config.SelectedAccount)
	if primary == "" {
		primary = config.DefaultSendAccount
	}

	var ordered []cfg_acc.AccountConfig
	for _, acc := range config.Accounts {
		if primary != "" && strings.EqualFold(acc.Name, primary) {
			ordered = append([]cfg_acc.AccountConfig{acc}, ordered...)
		} else {
			ordered = append(ordered, acc)
		}
	}
	return ordered, nil
}

// accountCacheDir is the account's cache, or the zero value -- skipped by the
// callers -- for a cache this build must not read (see cache/meta.Check).
func accountCacheDir(config *cfg_g.Config, acc cfg_acc.AccountConfig) cachedir.Account {
	acct := config.AccountCacheFor(acc)
	if err := meta.Check(acct, acc.Type); err != nil {
		slog.Warn("skipping an account cache this build does not understand", slog.String("account", acc.Name), slog.Any("error", err))
		return cachedir.Account{}
	}
	return acct
}

// findMessageAcrossAccounts resolves ref, a message ID as the user typed it,
// to the message and the account holding it. The folder it returned is the
// cached one, empty when the message was found only on a server.
func findMessageAcrossAccounts(config *cfg_g.Config, accountName, ref string, rep report.Reporter) (mailclient.MailClient, cfg_acc.AccountConfig, email.ID, []byte, string, error) {
	candidates, err := findCandidateAccounts(config, accountName)
	if err != nil {
		return nil, cfg_acc.AccountConfig{}, "", nil, "", err
	}

	for _, acc := range candidates {
		dlDir := accountCacheDir(config, acc)
		if dlDir.IsZero() {
			continue
		}
		foundID, folderName, errFind := cache.FindCachedEmailByID(dlDir, ref)
		if errFind != nil || foundID == "" {
			continue
		}
		raw, errRaw := msg.Read(dlDir, foundID)
		if errRaw != nil || len(raw) == 0 {
			continue
		}
		client, accCfg, errCl := mailops.GetClientForSending(config, acc.Name, rep)
		if errCl != nil {
			continue
		}
		return client, *accCfg, foundID, raw, folderName, nil
	}

	// Only a full ID can be asked of a server; a short one is a hash of it.
	if short, ok := email.ParseShortID(ref); ok {
		return nil, cfg_acc.AccountConfig{}, "", nil, "", errShortIDNotCached(short)
	}

	// Nothing local knows ref and it is not a short ID, so it can only be a
	// full server ID.
	id := email.ID(ref)
	for _, acc := range candidates {
		client, accCfg, errCl := mailops.GetClientForSending(config, acc.Name, rep)
		if errCl != nil {
			continue
		}
		raw, errRaw := client.GetRawMessage(id)
		if errRaw == nil && len(raw) > 0 {
			return client, *accCfg, id, raw, "", nil
		}
	}

	return nil, cfg_acc.AccountConfig{}, "", nil, "", fmt.Errorf("message %q not found in configured accounts", ref)
}

// ErrQueueReadOnly refuses --queue under --read-only. Queueing is not sending,
// but the next run without the flag flushes the outbox: the message used to
// be queued anyway and went out later.
var ErrQueueReadOnly = errors.New("queueing refused under --read-only: the next run without it would send the message")

func refuseQueueReadOnly(config *cfg_g.Config) error {
	if config != nil && config.ReadOnly {
		return ErrQueueReadOnly
	}
	return nil
}

// noteDryRunSchedule says, under --dry-run, when send --at would have sent.
func (s SendService) noteDryRunSchedule(opts cli.SendOptions) {
	if !opts.SendAfter.IsZero() {
		s.rep().Infof("[DRY-RUN] Would be queued to send at %s", opts.SendAfter.Local().Format("Mon 2006-01-02 15:04 MST"))
	}
}

// enqueue puts a built message in the outbox: to be sent by the next flush,
// or, with a due time (send --at), by the first flush at or after it. Both are
// refused under --read-only, since the next run without it would send it.
func (s SendService) enqueue(config *cfg_g.Config, entry outbox.Entry, raw []byte) error {
	if err := refuseQueueReadOnly(config); err != nil {
		return err
	}
	cacheRoot := ""
	if config != nil {
		cacheRoot = config.CacheRoot
	}
	queued, err := outbox.EnqueueEntry(outbox.Dir(cacheRoot), entry, raw)
	if err != nil {
		return fmt.Errorf("queueing email to outbox: %w", err)
	}
	if queued.SendAfter.IsZero() {
		s.rep().Successf("Email queued to outbox (ID: %s, Account: %s)", queued.ID, queued.Account)
		return nil
	}
	s.rep().Successf("Email scheduled for %s (ID: %s, Account: %s); feral watch or msg outbox flush sends it once due",
		queued.SendAfter.Local().Format("Mon 2006-01-02 15:04 MST"), queued.ID, queued.Account)
	return nil
}

// signatureUnless is acc's signature, or "" when the caller asked for none
// (--no-signature): a script piping mail through msg send opts out here.
func (s SendService) signatureUnless(none bool, config *cfg_g.Config, acc cfg_acc.AccountConfig) string {
	if none {
		return ""
	}
	return s.signature(config, acc)
}

// signature is the signature acc puts under what it sends, from its
// signature.txt (`account signature`); "" without one. A signature that cannot
// be read is reported and left off: the message is still the user's.
func (s SendService) signature(config *cfg_g.Config, acc cfg_acc.AccountConfig) string {
	if config == nil {
		return ""
	}
	sig, _, err := cfg_g.ReadSignature(config.ConfigDir, acc.Name)
	if err != nil {
		s.rep().Warnf("Sending without a signature: %v", err)
		return ""
	}
	return sig
}
