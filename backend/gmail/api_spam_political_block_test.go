package gmail

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/sarielhp/feral/cfg_g"
	"github.com/sarielhp/feral/report"

	gmailapi "google.golang.org/api/gmail/v1"
)

// fakeFilterServer answers the label and filter calls blockSenderOnGmail
// makes, holding the given filters, and records each filter it is asked to
// create and every request made.
type fakeFilterServer struct {
	mu       sync.Mutex
	existing []*gmailapi.Filter
	created  []*gmailapi.Filter
	requests []string
}

func (f *fakeFilterServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r.Method+" "+r.URL.Path)
	w.Header().Set("Content-Type", "application/json")
	switch {
	case strings.HasSuffix(r.URL.Path, "/settings/filters") && r.Method == http.MethodGet:
		_ = json.NewEncoder(w).Encode(map[string]any{"filter": f.existing})
	case strings.HasSuffix(r.URL.Path, "/settings/filters") && r.Method == http.MethodPost:
		var filter gmailapi.Filter
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &filter)
		f.created = append(f.created, &filter)
		_, _ = w.Write(body)
	case strings.HasSuffix(r.URL.Path, "/labels"):
		_, _ = w.Write([]byte(`{"labels":[{"id":"INBOX","name":"INBOX"},{"id":"Label_7","name":"LearnSpam"}]}`))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func TestBlockSenderOnGmail(t *testing.T) {
	existing := []*gmailapi.Filter{{Id: "f1", Criteria: &gmailapi.FilterCriteria{From: "kept@pac.example"},
		Action: &gmailapi.FilterAction{AddLabelIds: []string{"STARRED"}}}}
	cases := []struct {
		name       string
		sender     string
		config     *Config
		wantCreate bool
		wantCalls  bool
	}{
		{"new sender is filtered out of the inbox", "donate@pac.example", &cfg_g.Config{}, true, true},
		{"a filter the user has is left alone", "kept@pac.example", &cfg_g.Config{}, false, true},
		{"whitelisted sender is never blocked", "friend@pac.example",
			&cfg_g.Config{Policy: cfg_g.Policy{Whitelist: []string{"friend@pac.example"}}}, false, false},
		{"read-only changes nothing", "donate@pac.example", &cfg_g.Config{ReadOnly: true}, false, false},
		{"dry-run changes nothing", "donate@pac.example", &cfg_g.Config{DryRun: true}, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeFilterServer{existing: existing}
			ts := httptest.NewServer(fake)
			defer ts.Close()
			if err := blockSendersOnGmail(report.Discard(), newGmailTestService(t, ts), tc.config, []string{tc.sender}); err != nil {
				t.Fatal(err)
			}
			if got := len(fake.requests) > 0; got != tc.wantCalls {
				t.Fatalf("made requests %v, want any: %v", fake.requests, tc.wantCalls)
			}
			if !tc.wantCreate {
				if len(fake.created) != 0 {
					t.Fatalf("created %+v", fake.created[0])
				}
				return
			}
			if len(fake.created) != 1 {
				t.Fatalf("created %d filters, want 1", len(fake.created))
			}
			f := fake.created[0]
			if f.Criteria == nil || f.Criteria.From != tc.sender {
				t.Errorf("filter criteria = %+v, want from:%s", f.Criteria, tc.sender)
			}
			if f.Action == nil || len(f.Action.AddLabelIds) != 1 || f.Action.AddLabelIds[0] != "Label_7" ||
				len(f.Action.RemoveLabelIds) != 1 || f.Action.RemoveLabelIds[0] != "INBOX" {
				t.Errorf("filter action = %+v, want LearnSpam added and the inbox skipped", f.Action)
			}
		})
	}
}

// A sweep pushes every blacklisted political sender at once: one filter list
// and one label lookup, and a filter only for those without one.
func TestBlockSendersOnGmailPushesOnlyWhatIsMissing(t *testing.T) {
	fake := &fakeFilterServer{existing: []*gmailapi.Filter{{Id: "f1",
		Criteria: &gmailapi.FilterCriteria{From: "old@pac.example"},
		Action:   &gmailapi.FilterAction{AddLabelIds: []string{"Label_7"}, RemoveLabelIds: []string{"INBOX"}}}}}
	ts := httptest.NewServer(fake)
	defer ts.Close()
	senders := []string{"old@pac.example", "a@pac.example", "b@pac.example"}
	if err := blockSendersOnGmail(report.Discard(), newGmailTestService(t, ts), &cfg_g.Config{}, senders); err != nil {
		t.Fatal(err)
	}
	if len(fake.created) != 2 || fake.created[0].Criteria.From != "a@pac.example" || fake.created[1].Criteria.From != "b@pac.example" {
		t.Fatalf("created %d filters, want a@ and b@ only", len(fake.created))
	}
	lists := 0
	for _, r := range fake.requests {
		if strings.HasPrefix(r, "GET ") {
			lists++
		}
	}
	if lists != 2 {
		t.Errorf("made %d list requests, want one for filters and one for labels: %v", lists, fake.requests)
	}
}

// The senders a sweep blocks are those the political auto-blacklist wrote --
// internal rules -- and those it added this run; a rule the user wrote is
// pushed by rule export, not here.
func TestPoliticalSendersAreTheInternalRulesAndTheNewOnes(t *testing.T) {
	rules := []rule{
		{Sender: "old@pac.example", Label: "LearnSpam", Internal: true},
		{Sender: "boss@work.example", Label: "Work"},
		{Subject: "donate", Label: "LearnSpam", Internal: true},
	}
	got := politicalSenders(rules, []string{"new@pac.example", "OLD@pac.example"})
	if len(got) != 2 || got[0] != "old@pac.example" || got[1] != "new@pac.example" {
		t.Fatalf("senders = %v, want [old@pac.example new@pac.example]", got)
	}
}
