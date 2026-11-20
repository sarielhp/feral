package tui

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/filebrowser"
)

// The save dialog opens where feral was started, and its jumps are that
// directory, the configured attachment directory, Downloads and home, with
// duplicates dropped; the directory of the last save joins them.
func TestSaveDialog_OpensInStartDirWithJumps(t *testing.T) {
	saveDir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()

	m := newTuiModel(nil, "INBOX", nil)
	m.cfg = &cfg_g.Config{}
	m.cfg.AttachmentSaveDir = saveDir
	m.width, m.height = 100, 40

	m.openAttachmentSaveDialog(email.Attachment{Filename: "a.pdf"})
	if !m.fileBrowserOpen {
		t.Fatal("the dialog did not open")
	}
	if m.fileBrowser.CurrentDir != cwd {
		t.Errorf("the dialog opened in %q, want the start directory %q", m.fileBrowser.CurrentDir, cwd)
	}
	wantPaths := []string{cwd, saveDir, filepath.Join(home, "Downloads"), home}
	if got := locationPaths(m.fileBrowser.Locations); !slices.Equal(got, wantPaths) {
		t.Errorf("jumps are %v, want %v", got, wantPaths)
	}

	// A save into another directory offers it on the next dialog.
	chosen := t.TempDir()
	fb := m.fileBrowser
	fb.Done, fb.SelectedDir = true, chosen
	m.afterFileBrowser(fb, nil)
	m.openAttachmentSaveDialog(email.Attachment{Filename: "b.pdf"})
	wantPaths = []string{cwd, saveDir, chosen, filepath.Join(home, "Downloads"), home}
	if got := locationPaths(m.fileBrowser.Locations); !slices.Equal(got, wantPaths) {
		t.Errorf("after a save to %s the jumps are %v, want %v", chosen, got, wantPaths)
	}
}

// With no configured directory the jumps are just the start directory,
// Downloads and home.
func TestSaveDialog_NoConfiguredDirectory(t *testing.T) {
	cwd, _ := os.Getwd()
	home, _ := os.UserHomeDir()
	m := newTuiModel(nil, "INBOX", nil)
	m.width, m.height = 100, 40
	m.openAttachmentSaveDialog(email.Attachment{Filename: "a.pdf"})
	want := []string{cwd, filepath.Join(home, "Downloads"), home}
	if got := locationPaths(m.fileBrowser.Locations); !slices.Equal(got, want) {
		t.Errorf("jumps are %v, want %v", got, want)
	}
}

func locationPaths(locs []filebrowser.Location) []string {
	paths := make([]string, 0, len(locs))
	for _, l := range locs {
		paths = append(paths, l.Path)
	}
	return paths
}

// Configured save_locations replace the default pair and are expanded like
// attachment_save_dir, tildes included.
func TestSaveDialog_ConfiguredLocations(t *testing.T) {
	cwd, _ := os.Getwd()
	home, _ := os.UserHomeDir()
	m := newTuiModel(nil, "INBOX", nil)
	m.cfg = &cfg_g.Config{}
	m.cfg.SaveLocations = []string{"~/info/misc", "~/papers"}
	m.width, m.height = 100, 40
	m.openAttachmentSaveDialog(email.Attachment{Filename: "a.pdf"})
	want := []string{cwd, filepath.Join(home, "info", "misc"), filepath.Join(home, "papers")}
	if got := locationPaths(m.fileBrowser.Locations); !slices.Equal(got, want) {
		t.Errorf("jumps are %v, want %v", got, want)
	}
}
