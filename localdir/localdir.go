// Package localdir resolves the on-disk directory a subsystem keeps its own
// files in: <downloadDir>/<name> when a download directory is configured, and
// ~/.cache/feral/<name> otherwise, so a subsystem still has somewhere to
// write when no account -- and therefore no download directory -- is selected.
package localdir

import (
	"os"
	"path/filepath"

	"github.com/sarielhp/feral/app"
	"github.com/sarielhp/feral/cfg_g"
)

// Path returns the directory named name below downloadDir. With no download
// directory it falls back to ~/.cache/feral/name, or a temp directory when
// the home directory cannot be determined.
func Path(downloadDir, name string) string {
	if downloadDir != "" {
		return filepath.Join(downloadDir, name)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		if root, rootErr := app.TempRoot(); rootErr == nil {
			return filepath.Join(root, name)
		}
		return filepath.Join(os.TempDir(), name)
	}
	return filepath.Join(cfg_g.CacheDirIn(filepath.Join(home, ".cache")), name)
}
