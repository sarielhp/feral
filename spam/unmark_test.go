package spam

import (
	"testing"

	"github.com/sarielhp/feral/bogofiltertest"
	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/email"
	"github.com/sarielhp/feral/report"
	"github.com/sarielhp/feral/train"
)

// UnmarkTraining takes back exactly what the mark gave: one unregistration
// per registration, and the ledger record only when the mark wrote it -- a
// record from an earlier training stays, since that training stays too.
func TestUnmarkTraining(t *testing.T) {
	const raw = "Subject: offer\r\n\r\nbuy now\r\n"
	cases := []struct {
		name     string
		training Training
		wantS    int
		wantKept bool
	}{
		{"registered twice and recorded", Training{Registered: 2, Recorded: true}, 2, false},
		{"recorded by an earlier training", Training{Registered: 1}, 1, true},
		{"already classified, nothing trained", Training{}, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			filter := bogofiltertest.InstallLearning(t)
			dir := t.TempDir()
			ledger := train.OpenOrEmpty(train.MessageIDPath(dir))
			if err := ledger.AddAll([]string{"m1", "other"}); err != nil {
				t.Fatal(err)
			}

			if err := UnmarkTraining(dir, email.ID("m1"), []byte(raw), tc.training); err != nil {
				t.Fatalf("UnmarkTraining: %v", err)
			}

			if got := filter.Count(t, "-S"); got != tc.wantS {
				t.Errorf("unregistrations = %d, want %d", got, tc.wantS)
			}
			if got := filter.Count(t, "-n"); got != 0 {
				t.Errorf("trained as ham %d times; undoing a mark is not a ham verdict", got)
			}
			after, err := train.Open(train.MessageIDPath(dir))
			if err != nil {
				t.Fatal(err)
			}
			if after.Has("m1") != tc.wantKept {
				t.Errorf("ledger has m1 = %v, want %v", after.Has("m1"), tc.wantKept)
			}
			if !after.Has("other") {
				t.Error("the ledger lost another message's record")
			}
		})
	}
}

// A mark trains until bogofilter agrees, and reports how many registrations
// that took, which is what UnmarkTraining is given back.
func TestTrainSpamUntilMarkedCountsRegistrations(t *testing.T) {
	filter := bogofiltertest.InstallLearning(t)
	_, n, err := trainSpamUntilMarked(report.Discard(), []byte("Subject: x\r\n\r\ny\r\n"), "m1", &cfg_g.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 || filter.Count(t, "-s") != 1 {
		t.Errorf("registered %d (bogofilter saw %d), want 1", n, filter.Count(t, "-s"))
	}
}
