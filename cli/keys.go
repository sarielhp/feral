package cli

import (
	"fmt"

	"github.com/sarielhp/clihelp"
)

// KeysCmd prints the TUI's default keymap as a keys.json to start from.
func KeysCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "keys",
		Group:       "Other",
		Description: "Print the default TUI keys as keys.json.",
		LongDescription: "The TUI reads keys.json from the configuration directory, beside config.json, " +
			"when it starts. It maps action names to keys for two groups of screens: list (the message list) " +
			"and message (an open message). An action left out keeps its default; one that is named runs on " +
			"the keys given, a key or a list of keys, and on no other. The TUI refuses to start on an unknown " +
			"group or action, a key that is not a key, a key two actions of one group would share, or a key " +
			"the screens keep for themselves -- q, Esc, Tab, f, F, h, ?, Enter, the arrows, PgUp, PgDn, Home, End, " +
			"Shift+← and Shift+→, F1-F4, F8, F9, Ctrl+C, Ctrl+D, Ctrl+Q and every Alt chord -- and names the line. The help (F1) shows the keys " +
			"in effect. This command prints every action with its default keys and writes nothing.",
		UsageLine: "feral utils keys",
		Examples: []clihelp.Example{
			{Line: "feral utils keys > ~/.config/feral/keys.json", Description: "Start a keys file to edit"},
		},
		Args: clihelp.NoArgs,
		Run: func(_ *clihelp.Context) error {
			data, err := session.tui().DefaultKeys()
			if err != nil {
				return err
			}
			if _, err := session.out().Write(data); err != nil {
				return fmt.Errorf("print the keys: %w", err)
			}
			return nil
		},
	}
}
