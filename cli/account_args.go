package cli

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/sarielhp/clihelp"
	"github.com/spf13/pflag"
)

// The command line is walked before clihelp parses it twice over: main
// rewrites the numeric shortcut -N into --account-index=N, and `link add`
// takes the account out of a preset it is given. Both need what only the
// command tree knows -- which words are commands and how many words each flag
// takes -- so both use one walker over the real option declarations rather
// than another table of flag names and arities.

// argKind is what one step of the walk found.
type argKind int

const (
	argWord  argKind = iota // a positional or a command name
	argIndex                // the numeric shortcut -N
	argFlag                 // a flag and its value, one or two words
	argTail                 // the rest, verbatim: after -- or an unknown flag
)

// argUnit is one step of the walk.
type argUnit struct {
	kind  argKind
	words []string
	// names are a flag unit's long flag names, one per shorthand in a cluster.
	names []string
	// command is the command a word named, possibly by an abbreviation.
	command *clihelp.Command
}

// commandLineWalker holds what the walk knows at its current position: the
// commands a word may still name and the flags in scope.
type commandLineWalker struct {
	app       *clihelp.App
	cmds      []clihelp.Command
	inherited []clihelp.Option
	fs        *pflag.FlagSet
}

func newCommandLineWalker(a *clihelp.App) (*commandLineWalker, error) {
	w := &commandLineWalker{
		app:       a,
		cmds:      append(append([]clihelp.Command{}, a.Commands...), a.Shortcuts...),
		inherited: append(append([]clihelp.Option{}, a.PersistentOptions...), a.GlobalFlags...),
	}
	fs, err := accountNormalizationFlags(a, append(append([]clihelp.Option{}, w.inherited...), a.Options...))
	w.fs = fs
	return w, err
}

// next classifies the unit args starts with.
func (w *commandLineWalker) next(args []string) (argUnit, error) {
	word := args[0]
	switch {
	case word == "--":
		return argUnit{kind: argTail, words: args}, nil
	case numericAccountFlag(word):
		return argUnit{kind: argIndex, words: args[:1]}, nil
	case strings.HasPrefix(word, "-") && len(word) > 1:
		count, names, known := accountFlagWords(w.fs, args)
		if !known {
			// Leave malformed input to clihelp rather than guessing its arity.
			return argUnit{kind: argTail, words: args}, nil
		}
		return argUnit{kind: argFlag, words: args[:count], names: names}, nil
	}
	cmd := accountArgumentCommand(w.cmds, word, w.app.AbbrevCommands)
	if cmd == nil {
		w.cmds = nil // a positional ends the command path
		return argUnit{kind: argWord, words: args[:1]}, nil
	}
	w.inherited = append(w.inherited, cmd.PersistentOptions...)
	fs, err := accountNormalizationFlags(w.app, append(append([]clihelp.Option{}, w.inherited...), cmd.Options...))
	if err != nil {
		return argUnit{}, err
	}
	w.fs, w.cmds = fs, cmd.Subcommands
	return argUnit{kind: argWord, words: args[:1], command: cmd}, nil
}

// walkCommandLine walks args from the root, replacing each unit with what
// visit returns.
func walkCommandLine(a *clihelp.App, args []string, visit func(argUnit) ([]string, error)) ([]string, error) {
	w, err := newCommandLineWalker(a)
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(args))
	for len(args) > 0 {
		u, err := w.next(args)
		if err != nil {
			return nil, err
		}
		words, err := visit(u)
		if err != nil {
			return nil, err
		}
		result = append(result, words...)
		args = args[len(u.words):]
	}
	return result, nil
}

// NormalizeAccountArgs preserves the application's numeric convenience
// spelling as an ordinary declared option: -2 becomes --account-index=2, one
// word for one word. It never loads configuration or selects an account.
// Flag values and everything after -- remain data.
func NormalizeAccountArgs(a *clihelp.App, args []string) ([]string, error) {
	if len(args) == 0 || args[0] == "__clihelp" {
		return args, nil
	}
	var proto []string
	if args[0] == "__complete" || args[0] == "__explain" {
		proto, args = args[:1], args[1:]
	}
	out, err := walkCommandLine(a, args, func(u argUnit) ([]string, error) {
		if u.kind == argIndex {
			return []string{"--account-index=" + u.words[0][1:]}, nil
		}
		return u.words, nil
	})
	if err != nil {
		return nil, err
	}
	return append(slices.Clone(proto), out...), nil
}

// presetAccount is a preset with its account flags taken out.
type presetAccount struct {
	choice accountChoice // the last account flag's value, if any
	rest   []string      // the preset without them
	full   []string      // rest with every command spelled in full
}

// splitPresetAccount takes every -A/--account, --account-index and -N out of
// preset, keeping the last one's value, as running it would. A flag cluster
// that holds -A with other flags is refused rather than taken apart.
func splitPresetAccount(a *clihelp.App, preset []string) (presetAccount, error) {
	var p presetAccount
	rest, err := walkCommandLine(a, preset, func(u argUnit) ([]string, error) {
		words := u.words
		switch u.kind {
		case argIndex:
			p.choice = accountChoice{value: u.words[0][1:], byIndex: true}
			return nil, nil
		case argFlag:
			var err error
			if words, err = takeAccountFlag(u, &p.choice); err != nil {
				return nil, err
			}
		}
		p.full = append(p.full, words...)
		if u.command != nil && abbreviates(u.words[0], u.command) {
			p.full[len(p.full)-1] = u.command.Name
		}
		return words, nil
	})
	if err != nil {
		return p, err
	}
	p.rest = rest
	return p, nil
}

// abbreviates reports whether word names cmd by a prefix rather than by its
// name or one of its aliases.
func abbreviates(word string, cmd *clihelp.Command) bool {
	return word != cmd.Name && !slices.Contains(cmd.Aliases, word)
}

// takeAccountFlag drops an account flag unit and records its value; any other
// flag passes through.
func takeAccountFlag(u argUnit, choice *accountChoice) ([]string, error) {
	isAccount := func(n string) bool { return n == "account" || n == "account-index" }
	if !slices.ContainsFunc(u.names, isAccount) {
		return u.words, nil
	}
	if len(u.names) > 1 {
		return nil, fmt.Errorf("%w: write -A apart from other flags in a link's arguments (%q)", clihelp.ErrUsage, u.words[0])
	}
	value, ok := flagUnitValue(u)
	if !ok {
		return nil, fmt.Errorf("%w: %s needs a value", clihelp.ErrUsage, u.words[0])
	}
	*choice = accountChoice{value: value, byIndex: u.names[0] == "account-index"}
	return nil, nil
}

// flagUnitValue is the value of a one-flag unit: the next word, or what is
// attached to the flag (--account=x, -Ax, -A=x).
func flagUnitValue(u argUnit) (string, bool) {
	if len(u.words) == 2 {
		return u.words[1], true
	}
	word := u.words[0]
	if strings.HasPrefix(word, "--") {
		_, value, ok := strings.Cut(word, "=")
		return value, ok
	}
	if len(word) <= 2 {
		return "", false
	}
	return strings.TrimPrefix(word[2:], "="), true
}

func numericAccountFlag(word string) bool {
	if len(word) < 2 || word[0] != '-' {
		return false
	}
	for _, digit := range word[1:] {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

// accountNormalizationFlags binds opts on a flag set that is never parsed:
// the walk only looks flags up. Binding is not free of effects: an option's
// Binder writes its declared default through the option's variable, as
// clihelp does when it parses. Before Execute that is nothing; inside a Run
// handler (link add) it resets the variables of every option the walk passes
// -- so a handler reads its own flags before it walks, and reads the global
// modes from session.Config, which BeforeRun has already set. It uses the actual option declarations,
// including optional values and aliases, instead of maintaining another
// flag-name/arity table.
func accountNormalizationFlags(a *clihelp.App, opts []clihelp.Option) (*pflag.FlagSet, error) {
	fs := pflag.NewFlagSet(a.Name, pflag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolP("help-concise", "h", false, "")
	fs.Bool("help", false, "")
	if a.ExtendedHelpFlag {
		fs.BoolP("help-extended", "H", false, "")
	}
	if a.EnableExamplesFlag {
		fs.BoolP("examples", "E", false, "")
	}
	for _, opt := range opts {
		if opt.Binder == nil {
			continue
		}
		if err := opt.Binder(fs); err != nil {
			return nil, fmt.Errorf("normalize account flags: %w", err)
		}
	}
	return fs, nil
}

// accountFlagWords is how many words the flag args starts with takes, and the
// long names of the flags in it; known is false for a flag the set lacks.
func accountFlagWords(fs *pflag.FlagSet, args []string) (count int, names []string, known bool) {
	word := args[0]
	if strings.HasPrefix(word, "--") {
		name, _, attached := strings.Cut(word[2:], "=")
		flag := fs.Lookup(name)
		if flag == nil {
			return 0, nil, false
		}
		if attached || flag.NoOptDefVal != "" || len(args) == 1 {
			return 1, []string{flag.Name}, true
		}
		return 2, []string{flag.Name}, true
	}
	for rest := word[1:]; rest != ""; rest = rest[1:] {
		flag := fs.ShorthandLookup(rest[:1])
		if flag == nil {
			return 0, nil, false
		}
		names = append(names, flag.Name)
		if len(rest) > 1 && rest[1] == '=' {
			return 1, names, true
		}
		if flag.NoOptDefVal != "" {
			continue
		}
		if len(rest) > 1 || len(args) == 1 {
			return 1, names, true
		}
		return 2, names, true
	}
	return 1, names, true
}

func accountArgumentCommand(cmds []clihelp.Command, word string, abbrev bool) *clihelp.Command {
	for i := range cmds {
		for _, name := range append([]string{cmds[i].Name}, cmds[i].Aliases...) {
			if word == name {
				return &cmds[i]
			}
		}
	}
	if !abbrev {
		return nil
	}
	var matched *clihelp.Command
	for i := range cmds {
		if cmds[i].NoAbbrev {
			continue
		}
		for _, name := range append([]string{cmds[i].Name}, cmds[i].Aliases...) {
			if !strings.HasPrefix(name, word) {
				continue
			}
			if matched != nil && matched != &cmds[i] {
				return nil
			}
			matched = &cmds[i]
		}
	}
	return matched
}
