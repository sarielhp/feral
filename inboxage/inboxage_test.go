package inboxage

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/mailclient"
)

func TestParseMaxAge(t *testing.T) {
	cases := []struct {
		in      string
		want    time.Duration
		wantErr bool
	}{
		{"", 0, false},
		{"15m", 15 * time.Minute, false},
		{"1h30m", 90 * time.Minute, false},
		{"30s", 0, true},
		{"-5m", 0, true},
		{"fifteen", 0, true},
		{"15", 0, true},
	}
	for _, tc := range cases {
		got, err := ParseMaxAge(tc.in)
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Errorf("ParseMaxAge(%q) = %v, %v; want %v, err=%v", tc.in, got, err, tc.want, tc.wantErr)
		}
		if err != nil && !errors.Is(err, ErrBadMaxAge) {
			t.Errorf("ParseMaxAge(%q) error %v is not ErrBadMaxAge", tc.in, err)
		}
	}
}

func TestRelaysKeepsOnlyAccountsWithALimit(t *testing.T) {
	got := Relays([]cfg_acc.AccountConfig{
		{Name: "main"}, {Name: "relay", InboxMaxAge: "15m"}, {Name: "typo", InboxMaxAge: "soon"},
	})
	if len(got) != 2 || got[0].Name != "relay" || got[1].Name != "typo" {
		t.Errorf("Relays = %+v, want relay and typo (a bad value is reported, not dropped)", got)
	}
}

func TestCheck(t *testing.T) {
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	relay := cfg_acc.AccountConfig{Name: "relay", InboxMaxAge: "15m"}
	fresh := mailclient.InboxEntry{ID: "new", Received: now.Add(-5 * time.Minute), Subject: "just in"}
	old := mailclient.InboxEntry{ID: "old", Received: now.Add(-40 * time.Minute), Subject: "stuck"}

	t.Run("drained", func(t *testing.T) {
		st := Check(t.Context(), &mailclient.MockMailClient{InboxEntries: []mailclient.InboxEntry{fresh}}, relay, now)
		if !st.OK() || !strings.Contains(st.Summary(now), "drained") {
			t.Errorf("fresh mail only: got %+v (%s), want OK", st, st.Summary(now))
		}
	})
	t.Run("stale", func(t *testing.T) {
		st := Check(t.Context(), &mailclient.MockMailClient{InboxEntries: []mailclient.InboxEntry{fresh, old}}, relay, now)
		if !st.Stale || st.Entry.ID != "old" {
			t.Fatalf("40-minute-old mail: got %+v, want stale on old", st)
		}
		if s := st.Summary(now); !strings.Contains(s, "at least 40m") || !strings.Contains(s, "stuck") {
			t.Errorf("summary %q does not give the age and subject", s)
		}
	})
	t.Run("cannot check", func(t *testing.T) {
		boom := errors.New("token expired")
		st := Check(t.Context(), &mailclient.MockMailClient{FailOn: map[string]error{"InboxMessageBefore": boom}}, relay, now)
		if st.OK() || !errors.Is(st.Err, boom) || !strings.Contains(st.Summary(now), "cannot check") {
			t.Errorf("a failed check must not read as OK: %+v", st)
		}
	})
	t.Run("bad limit", func(t *testing.T) {
		st := Check(t.Context(), &mailclient.MockMailClient{}, cfg_acc.AccountConfig{Name: "x", InboxMaxAge: "soon"}, now)
		if !errors.Is(st.Err, ErrBadMaxAge) {
			t.Errorf("a bad inbox_max_age must be reported, got %+v", st)
		}
	})
}

func TestAge(t *testing.T) {
	for d, want := range map[time.Duration]string{
		15 * time.Minute:                "15m",
		59*time.Minute + 59*time.Second: "59m",
		2*time.Hour + 5*time.Minute:     "2h05m",
		26*time.Hour + 30*time.Second:   "26h00m",
	} {
		if got := age(d); got != want {
			t.Errorf("age(%v) = %q, want %q", d, got, want)
		}
	}
}
