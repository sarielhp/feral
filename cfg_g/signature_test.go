package cfg_g

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSignatureStorage(t *testing.T) {
	dir := t.TempDir()
	if sig, set, err := ReadSignature(dir, "work"); sig != "" || set || err != nil {
		t.Fatalf("no file: %q, %v, %v", sig, set, err)
	}
	if err := WriteSignature(dir, "work", "Bob\n"); err != nil {
		t.Fatal(err)
	}
	if got := SignaturePath(dir, "work"); got != filepath.Join(AccountDir(dir, "work"), "signature.txt") {
		t.Errorf("SignaturePath = %s", got)
	}
	info, err := os.Stat(SignaturePath(dir, "work"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("signature.txt: %v, %v", info, err)
	}
	if sig, set, err := ReadSignature(dir, "work"); sig != "Bob\n" || !set || err != nil {
		t.Errorf("after a write: %q, %v, %v", sig, set, err)
	}
	if err := WriteSignature(dir, "work", ""); err != nil {
		t.Fatal(err)
	}
	if sig, set, _ := ReadSignature(dir, "work"); sig != "" || !set {
		t.Errorf("an empty signature is a file set to nothing: %q, %v", sig, set)
	}
}

// A config with no directory -- one a test built by hand -- has no signature,
// and never reaches for the user's real configuration.
func TestSignatureWithoutAConfigDirectory(t *testing.T) {
	if SignaturePath("", "work") != "" {
		t.Error("a path was made up without a configuration directory")
	}
	if sig, set, err := ReadSignature("", "work"); sig != "" || set || err != nil {
		t.Errorf("ReadSignature = %q, %v, %v", sig, set, err)
	}
	if err := WriteSignature("", "work", "Bob\n"); err == nil {
		t.Error("a signature was written without a configuration directory")
	}
}

func TestWriteSignatureRefusedUnderDryRun(t *testing.T) {
	ForbidConfigWrites(true)
	t.Cleanup(func() { ForbidConfigWrites(false) })
	dir := t.TempDir()
	if err := WriteSignature(dir, "work", "Bob\n"); !errors.Is(err, ErrConfigWriteForbidden) {
		t.Errorf("err = %v, want ErrConfigWriteForbidden", err)
	}
	if _, set, _ := ReadSignature(dir, "work"); set {
		t.Error("the signature was written")
	}
}

func TestEditorCommand(t *testing.T) {
	t.Setenv("EDITOR", "vi")
	tests := []struct {
		name     string
		cfg      *Config
		wantName string
		wantArgs []string
	}{
		{"configured", &Config{Prefs: Prefs{Editor: "nano", EditorArgs: []string{"-w"}}}, "nano", []string{"-w", "f"}},
		{"environment", &Config{}, "vi", []string{"f"}},
		{"nil config", nil, "vi", []string{"f"}},
	}
	for _, tt := range tests {
		name, args := EditorCommand(tt.cfg, "f")
		if name != tt.wantName || !reflect.DeepEqual(args, tt.wantArgs) {
			t.Errorf("%s: %s %v, want %s %v", tt.name, name, args, tt.wantName, tt.wantArgs)
		}
	}
	t.Setenv("EDITOR", "")
	if name, args := EditorCommand(nil, "f"); name != "emacs" || !reflect.DeepEqual(args, []string{"-nw", "f"}) {
		t.Errorf("default: %s %v", name, args)
	}
}
