package testenv

import "testing"

// recorder is a testing.TB that records a Fatalf instead of stopping.
type recorder struct {
	testing.TB
	failed string
}

func (r *recorder) Helper() {}

func (r *recorder) Fatalf(format string, _ ...any) {
	r.failed = format
}

func TestGetenv(t *testing.T) {
	cases := []struct {
		name, newValue, oldValue, want string
		fails                          bool
	}{
		{name: "the new name is read", newValue: "1", want: "1"},
		{name: "the new name wins over the old", newValue: "1", oldValue: "2", want: "1"},
		{name: "the old name alone fails", oldValue: "1", fails: true},
		{name: "neither is empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("FERAL_ZZ_PROBE", tc.newValue)
			t.Setenv("MAIL_CLI_ZZ_PROBE", tc.oldValue)
			r := &recorder{TB: t}
			if got := Getenv(r, "FERAL_ZZ_PROBE"); got != tc.want {
				t.Errorf("Getenv = %q, want %q", got, tc.want)
			}
			if failed := r.failed != ""; failed != tc.fails {
				t.Errorf("failed = %v (%q), want %v", failed, r.failed, tc.fails)
			}
		})
	}
}
