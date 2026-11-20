package tui

import "github.com/sarielhp/feral/app"

// testTempRoot keeps the package's scratch directories under
// /tmp/<user>/feral rather than in /tmp itself.
func testTempRoot() string {
	root, err := app.TempRoot()
	if err != nil {
		panic(err)
	}
	return root
}
