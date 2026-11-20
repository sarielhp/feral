package calendar

import (
	"bytes"
	"net/mail"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCleanTZID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`"Eastern Standard Time"`, "Eastern Standard Time"},
		{`/America/New_York`, "America/New_York"},
		{`zoneinfo/America/Chicago`, "America/Chicago"},
		{`tzone://Microsoft/Custom`, "Custom"},
		{`(UTC-05:00) Eastern Time (US & Canada)`, "Eastern Time (US & Canada)"},
		{`(GMT+01:00) Amsterdam, Berlin, Bern, Rome, Stockholm, Vienna`, "Amsterdam, Berlin, Bern, Rome, Stockholm, Vienna"},
		{`   Europe/London   `, "Europe/London"},
	}

	for _, tc := range tests {
		got := CleanTZID(tc.input)
		if got != tc.want {
			t.Errorf("CleanTZID(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestResolveTimeZone_WindowsAndAliases(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Eastern Standard Time", "America/New_York"},
		{"Central Standard Time", "America/Chicago"},
		{"Mountain Standard Time", "America/Denver"},
		{"Pacific Standard Time", "America/Los_Angeles"},
		{"Hawaiian Standard Time", "Pacific/Honolulu"},
		{"Alaskan Standard Time", "America/Anchorage"},
		{"W. Europe Standard Time", "Europe/Berlin"},
		{"Romance Standard Time", "Europe/Paris"},
		{"GMT Standard Time", "Europe/London"},
		{"Israel Standard Time", "Asia/Jerusalem"},
		{"India Standard Time", "Asia/Kolkata"},
		{"Tokyo Standard Time", "Asia/Tokyo"},
		{"China Standard Time", "Asia/Shanghai"},
		{"America/New_York", "America/New_York"},
		{"America/Chicago", "America/Chicago"},
		{"UTC", "UTC"},
		{"EST", "America/New_York"},
		{"EDT", "America/New_York"},
		{"CST", "America/Chicago"},
		{"CDT", "America/Chicago"},
		{"PST", "America/Los_Angeles"},
		{"PDT", "America/Los_Angeles"},
		{"eastern time", "America/New_York"},
		{"(UTC-05:00) Eastern Time (US & Canada)", "America/New_York"},
		{"/mozilla.org/20070129_1/America/New_York", "America/New_York"},
		{"Customer/Invented-Zone", ""},
	}

	for _, tc := range tests {
		got := ResolveTimeZone(tc.input)
		if got != tc.want {
			t.Errorf("ResolveTimeZone(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestParseUTCOffset(t *testing.T) {
	tests := []struct {
		input   string
		wantSec int
		wantOk  bool
	}{
		{"-0500", -18000, true},
		{"+0200", 7200, true},
		{"-04:00", -14400, true},
		{"+05:30", 19800, true},
		{"UTC-05:00", -18000, true},
		{"GMT+0100", 3600, true},
		{"invalid", 0, false},
	}

	for _, tc := range tests {
		sec, ok := ParseUTCOffset(tc.input)
		if ok != tc.wantOk || sec != tc.wantSec {
			t.Errorf("ParseUTCOffset(%q) = (%d, %v), want (%d, %v)", tc.input, sec, ok, tc.wantSec, tc.wantOk)
		}
	}
}

func TestParseFirstICSEvent_WindowsEasternStandardTime(t *testing.T) {
	ics := `BEGIN:VCALENDAR
PRODID:Microsoft Exchange Server 2010
VERSION:2.0
BEGIN:VTIMEZONE
TZID:Eastern Standard Time
BEGIN:STANDARD
DTSTART:16010101T020000
TZOFFSETFROM:-0400
TZOFFSETTO:-0500
RRULE:FREQ=YEARLY;INTERVAL=1;BYDAY=1SU;BYMONTH=11
END:STANDARD
BEGIN:DAYLIGHT
DTSTART:16010101T020000
TZOFFSETFROM:-0500
TZOFFSETTO:-0400
RRULE:FREQ=YEARLY;INTERVAL=1;BYDAY=2SU;BYMONTH=3
END:DAYLIGHT
END:VTIMEZONE
BEGIN:VEVENT
UID:040000008200E00074C5B7101A82E008
SUMMARY:Grad School advice meeting
DTSTART;TZID=Eastern Standard Time:20261027T160000
DTEND;TZID=Eastern Standard Time:20261027T163000
DESCRIPTION:https://scheduler.zoom.us/t/_qoxfbpo/grad-school-advice-meeting
END:VEVENT
END:VCALENDAR`

	ev, err := ParseFirstICSEvent(ics)
	if err != nil {
		t.Fatalf("ParseFirstICSEvent failed: %v", err)
	}

	if ev.Summary != "Grad School advice meeting" {
		t.Errorf("got Summary %q, want %q", ev.Summary, "Grad School advice meeting")
	}
	if ev.StartTimeZone != "America/New_York" {
		t.Errorf("got StartTimeZone %q, want %q", ev.StartTimeZone, "America/New_York")
	}
	if ev.EndTimeZone != "America/New_York" {
		t.Errorf("got EndTimeZone %q, want %q", ev.EndTimeZone, "America/New_York")
	}
	if ev.ZoneUnresolved {
		t.Errorf("expected ZoneUnresolved to be false for Eastern Standard Time")
	}

	// 2026-10-27 is daylight saving time (EDT, UTC-4).
	// 16:00 EDT = 20:00 UTC.
	wantUTC := time.Date(2026, 10, 27, 20, 0, 0, 0, time.UTC)
	if !ev.StartParsed.Equal(wantUTC) {
		t.Errorf("StartParsed = %v, want UTC %v", ev.StartParsed, wantUTC)
	}
	if ev.StartParsed.Hour() != 16 || ev.StartParsed.Minute() != 0 {
		t.Errorf("StartParsed local wall time = %d:%02d, want 16:00", ev.StartParsed.Hour(), ev.StartParsed.Minute())
	}

	wantEndUTC := time.Date(2026, 10, 27, 20, 30, 0, 0, time.UTC)
	if !ev.EndParsed.Equal(wantEndUTC) {
		t.Errorf("EndParsed = %v, want UTC %v", ev.EndParsed, wantEndUTC)
	}
}

func TestParseRealEmailAttachmentIfPresent(t *testing.T) {
	emlPath := filepath.Join(os.Getenv("HOME"), ".cache/mail_cli/fastmail/messages/2026/10/26/StlsAfhzCVDo.eml")
	data, err := os.ReadFile(emlPath)
	if err != nil {
		t.Skip("real cache email not available, skipping")
	}
	msg, err := mail.ReadMessage(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("ReadMessage failed: %v", err)
	}
	icsData, err := ExtractICSAttachmentFromMsg(msg.Header, msg.Body)
	if err != nil {
		t.Fatalf("ExtractICSAttachmentFromMsg failed: %v", err)
	}
	if len(icsData) == 0 {
		t.Fatalf("no ICS data extracted from real email")
	}
	ev, err := ParseFirstICSEvent(string(icsData))
	if err != nil {
		t.Fatalf("ParseFirstICSEvent failed: %v", err)
	}
	if ev.Summary != "Grad School advice meeting" {
		t.Errorf("got summary %q, want %q", ev.Summary, "Grad School advice meeting")
	}
	if ev.StartTimeZone != "America/New_York" {
		t.Errorf("got StartTimeZone %q, want 'America/New_York'", ev.StartTimeZone)
	}
	if ev.ZoneUnresolved {
		t.Errorf("expected ZoneUnresolved to be false")
	}
	if ev.StartParsed.Hour() != 16 {
		t.Errorf("expected 16:00 wall time, got %d", ev.StartParsed.Hour())
	}
}
