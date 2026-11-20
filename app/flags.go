package app

import (
	"errors"

	"github.com/sarielhp/clihelp"
)

// The global flag values the CLI parses into: the booleans select modes such
// as --verbose and --dry-run, and CmdExecutionStarted records that a command
// has begun. The account a command targets is the CLI's own (cli.flagAccount).
var (
	FlagVerbose   bool
	FlagVersion   bool
	FlagLogAppend bool
	FlagFixConfig bool
	FlagReadOnly  bool
	FlagDryRun    bool
	FlagJSON      bool

	CmdExecutionStarted bool
)

// ErrUsage marks an error as a command-line mistake, which main turns into exit
// code 2 rather than 1.
var ErrUsage = clihelp.ErrUsage

// ErrCancelled is a command the user backed out of -- Esc in `contacts
// pick` -- which main turns into exit code 130 and no message, so a calling
// program can tell it from a failure.
var ErrCancelled = errors.New("cancelled")
