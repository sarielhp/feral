package email

import (
	"net/mail"
	"reflect"
	"strings"
	"testing"
)

func TestFormatAddress(t *testing.T) {
	cases := []struct{ name, addr, want string }{
		{"Alice Adams", "alice@example.com", "Alice Adams <alice@example.com>"},
		{"Doe, Alice", "a@x.com", `"Doe, Alice" <a@x.com>`},
		{`Al "the pal" Adams`, "a@x.com", `"Al \"the pal\" Adams" <a@x.com>`},
		{"Harvest H.", "h@x.com", "Harvest H. <h@x.com>"},
		{"דנה דיאז", "d@x.com", "דנה דיאז <d@x.com>"},
		{"", "a@x.com", "a@x.com"},
		{"Nobody", "", ""},
	}
	for _, c := range cases {
		got := FormatAddress(c.name, c.addr)
		if got != c.want {
			t.Errorf("FormatAddress(%q, %q) = %q, want %q", c.name, c.addr, got, c.want)
		}
		if got == "" {
			continue
		}
		// What it produces must parse back to the same address and name.
		parsed, err := mail.ParseAddress(got)
		if err != nil || parsed.Address != c.addr || parsed.Name != c.name {
			t.Errorf("%q parses as %+v, %v", got, parsed, err)
		}
	}
}

func TestSplitAddressList(t *testing.T) {
	cases := []struct {
		list string
		want []string
	}{
		{"a@x.com, b@y.com", []string{"a@x.com", "b@y.com"}},
		{`"Doe, Alice" <a@x.com>, b@y.com`, []string{`"Doe, Alice" <a@x.com>`, "b@y.com"}},
		{`"Al \"x, y\" A" <a@x.com>,b@y.com`, []string{`"Al \"x, y\" A" <a@x.com>`, "b@y.com"}},
		{" , a@x.com ,, ", []string{"a@x.com"}},
		{"", nil},
	}
	for _, c := range cases {
		if got := SplitAddressList(c.list); !reflect.DeepEqual(got, c.want) {
			t.Errorf("SplitAddressList(%q) = %q, want %q", c.list, got, c.want)
		}
	}
	// A joined split is a list the standard parser accepts.
	list := `"Doe, Alice" <a@x.com>, Bob <b@y.com>`
	if _, err := mail.ParseAddressList(strings.Join(SplitAddressList(list), ", ")); err != nil {
		t.Errorf("round trip: %v", err)
	}
}

func TestLastAddressStart(t *testing.T) {
	cases := map[string]int{
		"":                           0,
		"ali":                        0,
		"a@x.com, bo":                8,
		`"Doe, Alice" <a@x.com>`:     0,
		`"Doe, Alice" <a@x.com>, ca`: 23,
	}
	for list, want := range cases {
		if got := LastAddressStart(list); got != want {
			t.Errorf("LastAddressStart(%q) = %d, want %d", list, got, want)
		}
	}
}

func TestFormatAddressListNames(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"   ", ""},
		{"Alice Smith <alice@example.com>", "Alice Smith"},
		{"bob@example.com", "bob@example.com"},
		{`"Smith, Alice" <alice@example.com>, bob@example.com, Charlie Brown <charlie@example.com>`, "Smith, Alice, bob@example.com, Charlie Brown"},
		{"=?UTF-8?B?SsO2cmcgTcO8bGxlcg==?= <jorg@example.com>", "Jörg Müller"},
	}
	for _, c := range cases {
		got := FormatAddressListNames(nil, c.input)
		if got != c.want {
			t.Errorf("FormatAddressListNames(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestFormatAddressListVerbose(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"   ", ""},
		{"Alice Smith <alice@example.com>", "Alice Smith <alice@example.com>"},
		{"bob@example.com", "bob@example.com"},
		{"Alice Smith <alice@example.com>, bob@example.com", "Alice Smith <alice@example.com>, bob@example.com"},
	}
	for _, c := range cases {
		got := FormatAddressListVerbose(nil, c.input)
		if got != c.want {
			t.Errorf("FormatAddressListVerbose(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}
