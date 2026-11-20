// Package testenv reads the environment variables that opt a test run into
// live, interactive or long-running suites.
//
// They were MAIL_CLI_* until the program became feral and are FERAL_* now. A
// suite that is skipped unless its variable is set would skip silently for
// anyone still typing the old name, and a run that was asked for must never
// pass by testing nothing -- so a run that sets only the old name fails here,
// naming the new one.
package testenv

import (
	"os"
	"strings"
	"testing"
)

// Getenv is os.Getenv(name) for a FERAL_* test variable, failing t when the
// variable is unset but its MAIL_CLI_* predecessor is set.
func Getenv(t testing.TB, name string) string {
	t.Helper()
	value := os.Getenv(name)
	if value != "" {
		return value
	}
	old := "MAIL_CLI_" + strings.TrimPrefix(name, "FERAL_")
	if os.Getenv(old) != "" {
		t.Fatalf("%s was renamed %s; set that instead", old, name)
	}
	return ""
}
