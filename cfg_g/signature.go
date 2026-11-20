package cfg_g

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// SignatureFile is the name of an account's signature inside its directory
// (AccountDir). It is plain text, and so carries no format field: the
// configuration directory's manifest (config_meta.json) covers it. Because it
// lives in the account's directory, `account rename` moves it with the rest.
const SignatureFile = "signature.txt"

// SignaturePath is where the account named name keeps its signature, or ""
// when there is no configuration directory to look in. It never falls back
// to the user's home: a config built without one -- every test that builds a
// Config by hand -- must not read the real configuration.
func SignaturePath(configDir, name string) string {
	if configDir == "" || name == "" {
		return ""
	}
	return filepath.Join(AccountDir(configDir, name), SignatureFile)
}

// ReadSignature is the account's stored signature as written, and whether it
// has a signature file at all; an empty file is a signature set to nothing.
// A missing file is not an error.
func ReadSignature(configDir, name string) (string, bool, error) {
	path := SignaturePath(configDir, name)
	if path == "" {
		return "", false, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("reading the signature of account %q: %w", name, err)
	}
	return string(data), true, nil
}

// WriteSignature stores sig as the account's signature, replacing the file
// atomically and creating the account's directory if it is missing. The
// caller normalizes sig; an empty one is stored as an empty file. Under
// --dry-run it writes nothing and returns ErrConfigWriteForbidden, as
// SaveConfigFile does.
func WriteSignature(configDir, name, sig string) error {
	if configWritesForbidden.Load() {
		return ErrConfigWriteForbidden
	}
	path := SignaturePath(configDir, name)
	if path == "" {
		return fmt.Errorf("account %q has no configuration directory to keep a signature in", name)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("creating the directory of account %q: %w", name, err)
	}
	return atomicWriteFile(path, []byte(sig), 0600)
}

// EditorCommand is the program and arguments that edit path: the configured
// editor and its arguments, else $EDITOR, else "emacs -nw". A nil config is
// the environment's choice alone.
func EditorCommand(cfg *Config, path string) (string, []string) {
	editor := ""
	if cfg != nil {
		editor = cfg.Editor
	}
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "emacs"
	}
	var args []string
	if cfg != nil && len(cfg.EditorArgs) > 0 {
		args = append(args, cfg.EditorArgs...)
	} else if editor == "emacs" {
		args = append(args, "-nw")
	}
	return editor, append(args, path)
}
