// Package when reads a point in time as a person types it on the command
// line -- "2026-11-20 08:00", "+2h", "tomorrow 8am" -- in the local time
// zone. Sending later and snoozing both ask "when?", and both read the answer
// here, so the two accept the same forms.
//
// Nothing here reads the clock: every function takes now, so a caller can
// test what a form means at any moment, and so a form is read once, against
// one instant.
package when

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Forms lists the accepted forms, for help text and error messages.
const Forms = `"2026-11-20 08:00", "+2h", "+30m", "+1d", "today 17:00" or "tomorrow 8am"`

// ErrPast is a time that is not after now.
var ErrPast = errors.New("that time is not in the future")

// ErrUnknownForm is text that is none of the accepted forms.
var ErrUnknownForm = errors.New("unrecognized time")

// absoluteLayouts are the absolute forms, read in now's location.
var absoluteLayouts = []string{
	"2006-01-02 15:04",
	"2006-01-02 15:04:05",
	"2006-01-02t15:04", // the text is lowercased before it is read
	"2006-01-02t15:04:05",
}

var (
	relativeTerm = regexp.MustCompile(`^(\d+)([mhdw])`)
	clockTime    = regexp.MustCompile(`^(\d{1,2})(?::(\d{2}))?\s*(am|pm)?$`)
)

// Parse reads s as a point in time relative to now, in now's location. It
// does not care whether the result is in the past; Future does.
func Parse(s string, now time.Time) (time.Time, error) {
	text := strings.ToLower(strings.Join(strings.Fields(s), " "))
	if text == "" {
		return time.Time{}, unknown(s)
	}
	if rest, ok := strings.CutPrefix(text, "+"); ok {
		return parseRelative(s, rest, now)
	}
	if day, clock, ok := strings.Cut(text, " "); ok && (day == "today" || day == "tomorrow") {
		return parseDayClock(s, day, clock, now)
	}
	for _, layout := range absoluteLayouts {
		if t, err := time.ParseInLocation(layout, text, now.Location()); err == nil {
			return t, nil
		}
	}
	return time.Time{}, unknown(s)
}

// Future is Parse, refusing a time that is not after now.
func Future(s string, now time.Time) (time.Time, error) {
	t, err := Parse(s, now)
	if err != nil {
		return time.Time{}, err
	}
	if !t.After(now) {
		return time.Time{}, fmt.Errorf("%q is %s: %w", s, t.Format("2006-01-02 15:04 MST"), ErrPast)
	}
	return t, nil
}

func unknown(s string) error {
	return fmt.Errorf("%w %q: use %s", ErrUnknownForm, s, Forms)
}

// parseRelative reads "2h", "30m", "1d", "1w" or a run of them ("1d2h"). Days
// and weeks are calendar days, so "+1d" keeps the clock time across a
// daylight-saving change.
func parseRelative(orig, rest string, now time.Time) (time.Time, error) {
	if rest == "" {
		return time.Time{}, unknown(orig)
	}
	days := 0
	var span time.Duration
	for rest != "" {
		m := relativeTerm.FindStringSubmatch(rest)
		if m == nil {
			return time.Time{}, unknown(orig)
		}
		n, err := strconv.Atoi(m[1])
		if err != nil || n > 100000 {
			return time.Time{}, unknown(orig)
		}
		switch m[2] {
		case "m":
			span += time.Duration(n) * time.Minute
		case "h":
			span += time.Duration(n) * time.Hour
		case "d":
			days += n
		case "w":
			days += 7 * n
		}
		rest = rest[len(m[0]):]
	}
	return now.AddDate(0, 0, days).Add(span), nil
}

// parseDayClock reads "today 17:00" or "tomorrow 8am".
func parseDayClock(orig, day, clock string, now time.Time) (time.Time, error) {
	hour, minute, ok := parseClock(clock)
	if !ok {
		return time.Time{}, unknown(orig)
	}
	offset := 0
	if day == "tomorrow" {
		offset = 1
	}
	y, mo, d := now.Date()
	return time.Date(y, mo, d+offset, hour, minute, 0, 0, now.Location()), nil
}

// parseClock reads a time of day: "8am", "8:30 pm", "08:00", "20:15". With am
// or pm the hour runs 1 to 12 (12am is midnight, 12pm is noon); without, 0 to
// 23, and a bare hour needs its minutes ("8" alone is refused).
func parseClock(s string) (hour, minute int, ok bool) {
	m := clockTime.FindStringSubmatch(s)
	if m == nil {
		return 0, 0, false
	}
	hour, _ = strconv.Atoi(m[1])
	if m[2] != "" {
		minute, _ = strconv.Atoi(m[2])
	}
	if minute > 59 {
		return 0, 0, false
	}
	switch m[3] {
	case "":
		return hour, minute, m[2] != "" && hour <= 23
	case "pm":
		if hour < 1 || hour > 12 {
			return 0, 0, false
		}
		return hour%12 + 12, minute, true
	default: // am
		if hour < 1 || hour > 12 {
			return 0, 0, false
		}
		return hour % 12, minute, true
	}
}
