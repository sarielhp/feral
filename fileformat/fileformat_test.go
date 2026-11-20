package fileformat

import (
	"errors"
	"strings"
	"testing"
)

func TestCheck(t *testing.T) {
	for format, ok := range map[int]bool{0: true, 1: true, 2: false} {
		err := Check("f.json", format)
		var tooNew *TooNewError
		if ok != (err == nil) || (!ok && !errors.As(err, &tooNew)) {
			t.Errorf("format %d: %v", format, err)
		}
	}
}

// Every shape a bare file has today passes; only a wrapped one is refused.
func TestRefuseWrapped(t *testing.T) {
	pass := map[string]string{
		"ledger list":           `["a","b"]`,
		"ledger map":            `{"a":true,"b":false}`,
		"read state":            `{"AQMk1":true}`,
		"pending reads":         `{"x":{"read":true,"at":"2026-11-19T00:00:00Z"}}`,
		"an ID named format":    `{"format":true}`,
		"empty":                 ``,
		"damaged":               `{not json`,
		"white list":            "[\n  \"a@example.com\"\n]",
		"rules":                 `[{"sender":"x","label":"y"}]`,
		"unsubscription log":    `[{"sender":"x","success":true}]`,
		"object map of objects": `{"format":{"read":true}}`,
	}
	for name, data := range pass {
		if err := RefuseWrapped("f.json", []byte(data)); err != nil {
			t.Errorf("%s: refused: %v", name, err)
		}
	}
	for _, data := range []string{`{"format":2,"ids":["a"]}`, ` {"format": 7, "entries": {}}`, `{"format":1,"list":[]}`} {
		var tooNew *TooNewError
		if err := RefuseWrapped("f.json", []byte(data)); !errors.As(err, &tooNew) {
			t.Errorf("%s: not refused: %v", data, err)
		}
	}
}

// A file whose own format moved past Current is checked against its own limit,
// and the refusal names that limit.
func TestCheckAt(t *testing.T) {
	if err := CheckAt("j.json", 2, 2); err != nil {
		t.Errorf("format 2 refused by a reader of format 2: %v", err)
	}
	err := CheckAt("j.json", 3, 2)
	var tooNew *TooNewError
	if !errors.As(err, &tooNew) || !strings.Contains(err.Error(), "reads format 2") {
		t.Errorf("CheckAt(3, 2) = %v; want a refusal naming format 2", err)
	}
}
