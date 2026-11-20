package runner

import (
	"fmt"

	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/cache/msg"
	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/mailclient"
	"github.com/sarielhp/feral/mailops"
	"github.com/sarielhp/feral/report"
)

// RunMboxSave appends every message of the folders matching labelPrefix to the
// mbox file at path, creating it when it does not exist. The folders are
// resolved per account, exactly as `msg show` resolves them, so a label prefix
// can name a folder in more than one account.
//
// It appends rather than replaces: repeated runs accumulate into one corpus,
// and `mbox merge` is what deduplicates.
func RunMboxSave(config *cfg_g.Config, path, labelPrefix string, rep report.Reporter) error {
	total := 0
	err := mailops.ForEachAccount(config, labelPrefix, mailops.DefaultSpan, func(acc cfg_acc.AccountConfig, prefix string) (bool, error) {
		client, err := mailops.NewMailClient(acc, config, rep)
		if err != nil {
			return true, err
		}
		if validateErr := client.Validate(); validateErr != nil {
			return true, validateErr
		}
		n, err := appendClientFolderToMbox(config, client, prefix, path, rep)
		if err != nil {
			return true, err
		}
		total += n
		return false, nil
	})
	if err != nil {
		return err
	}

	switch {
	case total == 0:
		rep.Warnf("No messages matched %q; %s is unchanged.", labelPrefix, path)
	case config.DryRun:
		rep.Infof("Dry run: %d message(s) would be appended to %s", total, path)
	default:
		rep.Successf("Appended %d message(s) to %s", total, path)
	}
	return nil
}

// appendClientFolderToMbox resolves one folder on client, downloads it, and
// appends its messages to path, returning how many would be written. The raw
// bytes come from the cache the download filled, so nothing is fetched twice.
func appendClientFolderToMbox(config *cfg_g.Config, client mailclient.MailClient, prefix, path string, rep report.Reporter) (int, error) {
	// ResolveLabel normalizes "inbox" itself, so the prefix goes straight in.
	folder, err := mailclient.ResolveLabel(rep, client, prefix)
	if err != nil {
		return 0, err
	}

	ids, err := client.FetchAndDownloadEmails(folder, cfg_g.SanitizeLabelForCache(folder.String()))
	if err != nil {
		return 0, fmt.Errorf("downloading %s: %w", folder.String(), err)
	}
	raws := readCachedRaws(client.Config().AccountCache, client, ids)
	if config.DryRun {
		rep.Infof("%s: %d message(s)\n", folder.String(), len(raws))
		return len(raws), nil
	}

	n, err := AppendMboxEntries(path, raws)
	if err != nil {
		return n, err
	}
	rep.Infof("%s: %d message(s)\n", folder.String(), n)
	return n, nil
}

// readCachedRaws reads the raw message behind each id, from the cache first and
// from the server only for one the download did not store. An id whose message
// cannot be read at all is skipped: one unreadable message must not cost the
// rest of the folder.
func readCachedRaws(accountCache cachedir.Account, client mailclient.MailClient, ids []email.ID) [][]byte {
	raws := make([][]byte, 0, len(ids))
	for _, id := range ids {
		if data, err := msg.Read(accountCache, id); err == nil && len(data) > 0 {
			raws = append(raws, data)
			continue
		}
		if client == nil {
			continue
		}
		if data, err := client.GetRawMessage(id); err == nil && len(data) > 0 {
			raws = append(raws, data)
		}
	}
	return raws
}
