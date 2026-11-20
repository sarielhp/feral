package cli

import (
	"bytes"
	"errors"
	"testing"

	"github.com/sarielhp/feral/cfg_g"
)

// utils keys prints what the TUI says its default keymap is, as it is.
func TestUtilsKeysPrintsTheDefaultKeymap(t *testing.T) {
	want := []byte("{\n  \"format\": 1\n}\n")
	var out bytes.Buffer
	session := &Session{
		Config: &cfg_g.Config{},
		Deps:   Deps{TUI: stubTUI{DefaultKeysFn: func() ([]byte, error) { return want, nil }}},
	}
	app := InitCLI(session)
	app.Stdout = &out
	if err := app.Execute([]string{"utils", "keys"}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out.Bytes(), want) {
		t.Errorf("printed %q, want %q", out.String(), want)
	}
	session.Deps.TUI = stubTUI{}
	if err := InitCLI(session).Execute([]string{"utils", "keys"}); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("without a TUI: %v", err)
	}
}
