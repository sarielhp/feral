package when

import (
	"errors"
	"testing"
	"time"
)

// A zone with daylight saving, so a test that passed only in UTC would fail.
func testZone(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Skipf("no zone database: %v", err)
	}
	return loc
}

func TestParse(t *testing.T) {
	loc := testZone(t)
	now := time.Date(2026, 11, 20, 14, 30, 0, 0, loc)
	at := func(mo time.Month, d, h, mi int) time.Time { return time.Date(2026, mo, d, h, mi, 0, 0, loc) }
	cases := []struct {
		in   string
		want time.Time
	}{
		{"2026-11-20 08:00", at(11, 20, 8, 0)},
		{"2026-11-21 8:05", at(11, 21, 8, 5)},
		{"2026-12-01T09:15", at(12, 1, 9, 15)},
		{"2026-12-01 09:15:30", time.Date(2026, 12, 1, 9, 15, 30, 0, loc)},
		{"  2026-11-20   08:00 ", at(11, 20, 8, 0)},
		{"+2h", now.Add(2 * time.Hour)},
		{"+30m", now.Add(30 * time.Minute)},
		{"+1d", at(11, 21, 14, 30)},
		{"+1w", at(11, 27, 14, 30)},
		{"+1d2h30m", at(11, 21, 17, 0)},
		{"+2H", now.Add(2 * time.Hour)},
		{"tomorrow 8am", at(11, 21, 8, 0)},
		{"Tomorrow 8 AM", at(11, 21, 8, 0)},
		{"tomorrow 08:00", at(11, 21, 8, 0)},
		{"tomorrow 8:30pm", at(11, 21, 20, 30)},
		{"tomorrow 12am", at(11, 21, 0, 0)},
		{"tomorrow 12pm", at(11, 21, 12, 0)},
		{"tomorrow 12:15am", at(11, 21, 0, 15)},
		{"tomorrow 23:59", at(11, 21, 23, 59)},
		{"today 17:00", at(11, 20, 17, 0)},
		{"today 9am", at(11, 20, 9, 0)},
	}
	for _, c := range cases {
		got, err := Parse(c.in, now)
		if err != nil {
			t.Errorf("Parse(%q): %v", c.in, err)
			continue
		}
		if !got.Equal(c.want) {
			t.Errorf("Parse(%q) = %v, want %v", c.in, got, c.want)
		}
		if got.Location() != loc {
			t.Errorf("Parse(%q) is in %v, want the local zone %v", c.in, got.Location(), loc)
		}
	}
}

func TestParseRefusesWhatItCannotRead(t *testing.T) {
	now := time.Date(2026, 11, 20, 14, 30, 0, 0, testZone(t))
	for _, in := range []string{
		"", "   ", "+", "+2", "+h", "+2x", "+2h+", "-2h", "2h", "tomorrow", "tomorrow 8",
		"tomorrow 13pm", "tomorrow 0am", "tomorrow 24:00", "tomorrow 8:60", "yesterday 8am",
		"next week", "2026-11-20", "2026-13-01 08:00", "20/11/2026 08:00", "+999999d",
	} {
		if got, err := Parse(in, now); !errors.Is(err, ErrUnknownForm) {
			t.Errorf("Parse(%q) = %v, %v; want ErrUnknownForm", in, got, err)
		}
	}
}

func TestFutureRefusesThePast(t *testing.T) {
	loc := testZone(t)
	now := time.Date(2026, 11, 20, 14, 30, 0, 0, loc)
	for _, in := range []string{"2026-11-20 08:00", "today 9am", "today 14:30", "+0m", "2025-01-01 00:00"} {
		if _, err := Future(in, now); !errors.Is(err, ErrPast) {
			t.Errorf("Future(%q): want ErrPast, got %v", in, err)
		}
	}
	got, err := Future("today 14:31", now)
	if err != nil || !got.Equal(now.Add(time.Minute)) {
		t.Errorf("Future(today 14:31) = %v, %v", got, err)
	}
	if _, err := Future("soon", now); !errors.Is(err, ErrUnknownForm) {
		t.Errorf("Future(soon): want ErrUnknownForm, got %v", err)
	}
}

// "+1d" is a calendar day: across the autumn change it keeps the clock time,
// which is 25 hours later, not 24.
func TestRelativeDaysKeepTheClockAcrossDaylightSaving(t *testing.T) {
	loc := testZone(t)
	now := time.Date(2026, 10, 31, 9, 0, 0, 0, loc) // DST ends 2026-11-01 in Chicago
	got, err := Parse("+1d", now)
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 11, 1, 9, 0, 0, 0, loc); !got.Equal(want) {
		t.Errorf("+1d = %v, want %v", got, want)
	}
	if got.Sub(now) != 25*time.Hour {
		t.Errorf("+1d across the change spans %v, want 25h", got.Sub(now))
	}
}
