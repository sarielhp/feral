package cli

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/mailclient"

	"github.com/sarielhp/clihelp"
)

func mboxViewCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "view",
		Description: "Browse a local mbox or .eml file in the TUI.",
		UsageLine:   "feral mbox view <file>",
		Parameters: []clihelp.Param{
			{Name: "<file>", Description: "A local mbox file, or a single message ending in .eml"},
		},
		Examples: []clihelp.Example{
			{Line: "feral mbox view archive.mbox"},
			{Line: "feral mbox view ~/mail/backup.mbox"},
			{Line: "feral mbox view message.eml", Description: "Open one saved message"},
		},
		Args: clihelp.ExactArgs(1),
		Run: func(ctx *clihelp.Context) error {
			return runMboxView(session, ctx.Args[0])
		},
	}
}

func runMboxView(session *Session, mboxPath string) error {
	localCfg, client, err := openMboxFile(session, mboxPath)
	if err != nil {
		return err
	}
	return session.tui().RunWithClient(localCfg, client, "INBOX")
}

// openMboxFile opens a local mbox or .eml file as a read-only pseudo-account,
// with its own cache under the root's mbox_views. It returns the config
// scoped to that pseudo-account and the client over the file.
func openMboxFile(session *Session, mboxPath string) (*cfg_g.Config, mailclient.MailClient, error) {
	absPath, err := filepath.Abs(expandHome(mboxPath))
	if err != nil {
		return nil, nil, fmt.Errorf("invalid mbox path: %w", err)
	}
	fi, err := os.Stat(absPath)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot access mbox %q: %w", absPath, err)
	}
	if fi.IsDir() {
		return nil, nil, fmt.Errorf("%q is a directory, not a mail file", absPath)
	}

	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(absPath)))[:12]
	base := cfg_g.SanitizeLabelForCache(filepath.Base(absPath))
	cacheDir := filepath.Join(session.Config.CacheRoot, "mbox_views", fmt.Sprintf("%s_%s", base, hash))

	// A pseudo-account: the mbox is not in accounts[], so there is no entry to
	// take a canonical name from.
	accName := cfg_g.AccountName("mbox")
	displayName := filepath.Base(absPath)
	localCfg := *session.Config
	localCfg.SelectedAccount = accName
	localCfg.ReadOnly = true
	// The view's cache is outside the per-account layout, so it is named
	// explicitly rather than derived from an account name.
	localCfg.AccountCache = cachedir.At(cacheDir)
	localCfg.Accounts = []cfg_acc.AccountConfig{
		{
			Name:        string(accName),
			DisplayName: displayName,
			Type:        "mbox",
			ReadOnly:    true,
		},
	}

	client, err := session.accounts().OpenMbox(&localCfg, absPath, localCfg.AccountCache)
	if err != nil {
		return nil, nil, err
	}
	return &localCfg, client, nil
}

// isMboxFileArg reports whether a command argument names a local mail file
// rather than a folder: it must look like a path -- absolute, ./ or ../ or
// ~/, or ending in .mbox or .eml -- and be a regular file. A bare word is
// always a folder, even if a file of that name happens to be in the current
// directory.
func isMboxFileArg(arg string) bool {
	if strings.HasPrefix(arg, cfg_g.LabelSigil) {
		return false
	}
	looksLikePath := filepath.IsAbs(arg) || strings.HasPrefix(arg, "./") || strings.HasPrefix(arg, "../") ||
		strings.HasPrefix(arg, "~/")
	ext := strings.ToLower(filepath.Ext(arg))
	if !looksLikePath && ext != ".mbox" && ext != ".eml" {
		return false
	}
	fi, err := os.Stat(expandHome(arg))
	return err == nil && fi.Mode().IsRegular()
}
