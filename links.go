package main

import (
	"fmt"
	"os"

	"github.com/sarielhp/clihelp"

	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/cli"
)

// linkInvocations is App.Invocations: the links in config.json, as clihelp
// runs them. It is set in buildCLI and not in cli.InitCLI, because every unit
// test of cli runs as cli.test -- not feral -- and would otherwise read the
// developer's own config.json.
//
// It is called on every Tab, so it reads only config.json's "links" key, from
// the configuration directory the loader would use (mail_cli's, in place,
// while that is the one there), and never writes. An error refuses a run under
// a link name -- clihelp's rule -- because running it unexpanded would run its
// command against the default account.
func linkInvocations(a *clihelp.App) func() ([]clihelp.Invocation, error) {
	return func() ([]clihelp.Invocation, error) {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("links: %w", err)
		}
		links, _, err := cfg_g.ReadLinks(cfg_g.ConfigDirIn(home))
		if err != nil {
			return nil, err
		}
		invs := make([]clihelp.Invocation, 0, len(links))
		for _, name := range cfg_g.LinkNames(links) {
			l := links[name]
			// A -N written into config.json by hand becomes --account-index=N
			// here, so that normalizeInvokedArgs finds the preset as stored.
			args, err := cli.NormalizeAccountArgs(a, l.Preset())
			if err != nil {
				return nil, fmt.Errorf("link %q: %w", name, err)
			}
			invs = append(invs, clihelp.Invocation{Name: name, Args: args, Description: l.Description})
		}
		return invs, nil
	}
}
