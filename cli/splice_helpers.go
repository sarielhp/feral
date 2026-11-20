package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/sarielhp/feral/app"

	"github.com/sarielhp/clihelp"
)

// ValidateSpliceArgs validates folder naming and message count constraints for splice.
func ValidateSpliceArgs(folder string, numMessages int, allowKeep bool) error {
	if !allowKeep && (strings.EqualFold(folder, "keep") || strings.HasPrefix(strings.ToLower(folder), "keep/")) {
		return fmt.Errorf("source folder %q must not be \"keep\" or start with \"keep/\" unless --allow is specified", folder)
	}
	if numMessages <= 0 {
		return fmt.Errorf("flag -n must be at least 1")
	}
	return nil
}

type spliceExampleItem struct {
	title       string
	command     string
	explanation string
	srcLabel    string
	destLabel   string
	mode        string
}

// SpliceExamplesCmd returns the clihelp.Command for "feral splice examples".
func SpliceExamplesCmd() clihelp.Command {
	return clihelp.Command{
		Name:            "examples",
		Title:           "splice examples",
		Description:     "Show archive mapping patterns and examples.",
		LongDescription: "Display structured examples with detailed explanations showing concrete source to destination label mappings.",
		UsageLine:       "feral splice examples",
		Args:            clihelp.NoArgs,
		Run: func(ctx *clihelp.Context) error {
			printSpliceExamples(ctxStdout(ctx))
			return nil
		},
	}
}

func formatExampleLabel(label string) string {
	parts := strings.SplitN(label, ":", 2)
	if len(parts) == 2 {
		return parts[0] + ":" + app.ColorBlue.Sprint(parts[1])
	}
	return app.ColorBlue.Sprint(label)
}

func printSpliceExamples(w io.Writer) {
	fmt.Fprintf(w, "%s Structured Splice Examples & Mapping Patterns:\n\n", app.PrefixInfo)
	examples := getSpliceExampleList()
	for i, ex := range examples {
		fmt.Fprintf(w, "%d. %s\n", i+1, app.ColorBold.Sprint(ex.title))
		fmt.Fprintf(w, "   Command:     %s\n", app.ColorGreen.Sprint(ex.command))
		fmt.Fprintf(w, "   Explanation: %s\n", ex.explanation)
		fmt.Fprintf(w, "   Mapping:     %s -> %s\n", formatExampleLabel(ex.srcLabel), formatExampleLabel(ex.destLabel))
		if ex.mode != "" {
			fmt.Fprintf(w, "   Mode:        %s\n", ex.mode)
		}
		fmt.Fprintln(w)
	}
}

func getSpliceExampleList() []spliceExampleItem {
	return []spliceExampleItem{
		{
			title:       "Basic Date Splice (Dry Run)",
			command:     "feral splice %GMail:receipts",
			explanation: "Preview organizing messages from folder 'receipts' into monthly archive folders based on email dates without modifying messages.",
			srcLabel:    "GMail:receipts",
			destLabel:   "GMail:keep/2026/09/receipts",
			mode:        "Dry run (default; no changes made)",
		},
		{
			title:       "Live Message Move (--move)",
			command:     "feral splice %GMail:receipts --move",
			explanation: "Actually moves messages into destination archive folders; verifies each message on the destination server before removing from source.",
			srcLabel:    "GMail:receipts",
			destLabel:   "GMail:keep/2026/09/receipts",
			mode:        "Live move (copies to destination, confirms, removes from source)",
		},
		{
			title:       "Custom Destination Folder Name (-f, --folder)",
			command:     "feral splice %GMail:inbox -f Archive --move",
			explanation: "Moves messages into a custom named folder under keep/YYYY/MM/ instead of using the source folder name.",
			srcLabel:    "GMail:inbox",
			destLabel:   "GMail:keep/2026/09/Archive",
		},
		{
			title:       "Folder with Year-Month Suffix (-F, --folder-suffix)",
			command:     "feral splice %GMail:services -F services --move",
			explanation: "Appends the message date suffix to the folder name under keep/YYYY/MM/<name>-YYYY-MM.",
			srcLabel:    "GMail:services",
			destLabel:   "GMail:keep/2026/09/services-2026-09",
		},
		{
			title:       "Year-Only Directory Hierarchy (-Y, --folder-year)",
			command:     "feral splice %GMail:services -Y services --move",
			explanation: "Places messages directly under keep/YYYY/ with year-month suffix attached (omitting the MM subdirectory).",
			srcLabel:    "GMail:services",
			destLabel:   "GMail:keep/2026/services-2026-09",
		},
		{
			title:       "Cross-Account Splicing (GMail to FastMail)",
			command:     "feral splice %GMail:received -Y %FastMail:services --move",
			explanation: "Migrates messages from account 'GMail' to 'FastMail' under keep/YYYY/services-YYYY-MM and trashes from source once verified.",
			srcLabel:    "GMail:received",
			destLabel:   "FastMail:keep/2026/services-2026-09",
			mode:        "Cross-account transfer (uploads to target, verifies, trashes source)",
		},
		{
			title:       "Copying Messages Without Deletion (--copy)",
			command:     "feral splice %GMail:receipts --copy -n 50",
			explanation: "Copies up to 50 messages to destination archive folders without deleting them from the source folder.",
			srcLabel:    "GMail:receipts",
			destLabel:   "GMail:keep/2026/09/receipts",
			mode:        "Copy (duplicates into destination archive)",
		},
		{
			title:       "Subject Pattern Filtering (-p, --pattern)",
			command:     `feral splice %GMail:inbox -p "*order*" -f Orders --move`,
			explanation: "Processes only messages whose subject matches the wildcard pattern '*order*' (* and ? wildcards supported).",
			srcLabel:    "GMail:inbox",
			destLabel:   "GMail:keep/2026/09/Orders",
		},
	}
}
