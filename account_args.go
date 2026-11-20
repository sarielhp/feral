package main

import (
	"fmt"
	"slices"

	"github.com/sarielhp/clihelp"

	"github.com/sarielhp/feral/cli"
)

// normalizeAccountArgs rewrites the numeric shortcut -N into --account-index=N
// over the words as typed (cli.NormalizeAccountArgs).
var normalizeAccountArgs = cli.NormalizeAccountArgs

// normalizeInvokedArgs is normalizeAccountArgs for the line as it will run.
// Started as a link, clihelp puts the link's preset in front of the typed
// words, so the typed words are read from where the preset leaves them: a
// preset ending in a flag that takes a value makes a typed -2 that value, not
// an account. The preset words are normalized with the rest and then dropped
// again -- clihelp adds the preset itself -- which is sound because the
// rewrite is one word for one word and a stored preset holds no -N
// (linkInvocations rewrites one written by hand).
//
// __explain carries the whole typed line as one word, so there is nothing to
// walk; it, like __clihelp, is left to normalizeAccountArgs as before.
func normalizeInvokedArgs(a *clihelp.App, args []string) ([]string, error) {
	inv, ok := a.Invocation()
	if !ok || len(inv.Args) == 0 || (len(args) > 0 && (args[0] == "__clihelp" || args[0] == "__explain")) {
		return normalizeAccountArgs(a, args)
	}
	var proto []string
	if len(args) > 0 && args[0] == "__complete" {
		proto, args = args[:1], args[1:]
	}
	full := slices.Concat(proto, inv.Args, args)
	out, err := normalizeAccountArgs(a, full)
	if err != nil {
		return nil, err
	}
	skip := len(proto) + len(inv.Args)
	if len(out) != len(full) || !slices.Equal(out[len(proto):skip], inv.Args) {
		return nil, fmt.Errorf("the preset of %q did not survive normalization: %q", inv.Name, out)
	}
	return slices.Concat(proto, out[skip:]), nil
}
