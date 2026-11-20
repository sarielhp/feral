package google

import (
	"errors"
	"net/http"
	"testing"

	"github.com/sarielhp/feral/cfg_g"
)

const readOnlyICS = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:ro-1@example.com\r\n" +
	"SUMMARY:Read-only probe\r\nDTSTART:20260901T140000Z\r\nDTEND:20260901T150000Z\r\n" +
	"END:VEVENT\r\nEND:VCALENDAR\r\n"

// --read-only and --dry-run refuse every calendar write before an OAuth client
// is built. They used to stop mail but not the calendar: `--read-only tui`
// could still add an invitation to Google Calendar, because calendar writes
// never pass through the ReadOnlyMailClient decorator.
func TestCalendarWritesRefusedReadOnlyAndDryRun(t *testing.T) {
	for _, mode := range []struct {
		name string
		cfg  cfg_g.Config
	}{
		{"read-only", cfg_g.Config{ReadOnly: true}},
		{"dry-run", cfg_g.Config{DryRun: true}},
	} {
		s := &Service{NewHTTPClient: func(*cfg_g.Config) (*http.Client, error) {
			t.Fatalf("%s: a calendar write reached the OAuth client", mode.name)
			return nil, nil
		}}
		writes := map[string]func() error{
			"AddICSDataToCalendar": func() error {
				_, _, err := s.AddICSDataToCalendar(&mode.cfg, []byte(readOnlyICS))
				return err
			},
			"AddICSDataAsNew": func() error {
				_, err := s.AddICSDataAsNew(&mode.cfg, []byte(readOnlyICS))
				return err
			},
			"AddICSDataWithVerification": func() error {
				_, _, _, _, err := s.AddICSDataWithVerification(&mode.cfg, []byte(readOnlyICS))
				return err
			},
			"PerformCalAddAll":   func() error { return s.PerformCalAddAll(&mode.cfg, nil) },
			"PerformCalendarAdd": func() error { return s.PerformCalendarAdd(&mode.cfg, nil, "", "") },
		}
		for name, write := range writes {
			if err := write(); !errors.Is(err, ErrCalendarWriteBlocked) {
				t.Errorf("%s %s: err = %v, want ErrCalendarWriteBlocked", mode.name, name, err)
			}
		}
	}
}
