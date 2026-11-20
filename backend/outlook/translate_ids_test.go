package outlook

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/sarielhp/feral/email"
)

// graphStub answers translateExchangeIds by prefixing each input with "imm-",
// except those named in untranslatable, and $search with legacy IDs.
type graphStub struct {
	untranslatable map[string]bool
	searchIDs      []string
	translateCalls int
	batchSizes     []int
	status         int
}

func (g *graphStub) client() *http.Client {
	return &http.Client{Transport: &mockRoundTripper{roundTripFunc: g.roundTrip}}
}

func (g *graphStub) roundTrip(req *http.Request) (*http.Response, error) {
	respond := func(status int, v any) (*http.Response, error) {
		b, _ := json.Marshal(v)
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(string(b)))}, nil
	}
	if strings.Contains(req.URL.String(), "translateExchangeIds") {
		g.translateCalls++
		if g.status != 0 {
			return respond(g.status, map[string]string{"error": "forbidden"})
		}
		var in struct {
			InputIDs []string `json:"inputIds"`
		}
		_ = json.NewDecoder(req.Body).Decode(&in)
		g.batchSizes = append(g.batchSizes, len(in.InputIDs))
		var out []map[string]any
		for _, id := range in.InputIDs {
			if g.untranslatable[id] {
				out = append(out, map[string]any{"sourceId": id, "errorDetails": map[string]string{"code": "bad"}})
				continue
			}
			out = append(out, map[string]any{"sourceId": id, "targetId": "imm-" + id})
		}
		return respond(http.StatusOK, map[string]any{"value": out})
	}
	var value []map[string]string
	for _, id := range g.searchIDs {
		value = append(value, map[string]string{"id": id})
	}
	return respond(http.StatusOK, map[string]any{"value": value})
}

// Translation goes in batches of at most a thousand, and an ID Graph cannot
// translate is left out.
func TestIDTranslator_BatchesAndSkipsFailures(t *testing.T) {
	g := &graphStub{untranslatable: map[string]bool{"x7": true}}
	ids := make([]email.ID, 1500)
	for i := range ids {
		ids[i] = email.ID("x" + string(rune('0'+i%10)) + strings.Repeat("y", i/10))
	}
	ids[7] = "x7"
	got, err := (&IDTranslator{client: g.client()}).Translate(t.Context(), ids)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.batchSizes) != 2 || g.batchSizes[0] != translateBatch || g.batchSizes[1] != 500 {
		t.Errorf("batches %v, want [1000 500]", g.batchSizes)
	}
	if _, ok := got["x7"]; ok {
		t.Error("an untranslatable ID was given a translation")
	}
	if got[ids[0]] != "imm-"+ids[0] {
		t.Errorf("translation of %s = %q", ids[0], got[ids[0]])
	}
}

// A failed request is an error, never a partial or empty result taken as
// "nothing translated".
func TestIDTranslator_RequestFailureIsAnError(t *testing.T) {
	g := &graphStub{status: http.StatusForbidden}
	if got, err := (&IDTranslator{client: g.client()}).Translate(t.Context(), []email.ID{"a"}); err == nil {
		t.Errorf("expected an error, got %v", got)
	}
}

// $search answers with default IDs whatever was asked for, so under
// immutable IDs its results are translated; an ID that cannot be is kept.
func TestSearchEmails_TranslatesUnderImmutableIDs(t *testing.T) {
	for _, immutable := range []bool{false, true} {
		g := &graphStub{searchIDs: []string{"AQMk-1", "AQMk-2"}, untranslatable: map[string]bool{"AQMk-2": true}}
		c := &client{client: g.client(), immutableIDs: immutable}
		got, err := c.SearchEmails("quill", 10)
		if err != nil {
			t.Fatal(err)
		}
		want := []email.ID{"AQMk-1", "AQMk-2"}
		if immutable {
			want = []email.ID{"imm-AQMk-1", "AQMk-2"}
		}
		if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
			t.Errorf("immutable=%v: search gave %v, want %v", immutable, got, want)
		}
		if !immutable && g.translateCalls != 0 {
			t.Error("default IDs need no translation")
		}
	}
}

// listingStub answers the sample listings with plain or immutable IDs
// according to the Prefer header, and translateExchangeIds through graphStub.
func listingStub(g *graphStub, plain, immutable []string) *http.Client {
	return &http.Client{Transport: &mockRoundTripper{roundTripFunc: func(req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.String(), "translateExchangeIds") {
			return g.roundTrip(req)
		}
		ids := plain
		if req.Header.Get("Prefer") == preferImmutableIDs {
			ids = immutable
		}
		var value []map[string]string
		for _, id := range ids {
			value = append(value, map[string]string{"id": id})
		}
		b, _ := json.Marshal(map[string]any{"value": value})
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(b)))}, nil
	}}}
}

// Verify passes when translation gives exactly what Graph lists, fails on any
// difference, and passes an account with nothing to compare.
func TestIDTranslator_Verify(t *testing.T) {
	cases := []struct {
		name      string
		plain     []string
		immutable []string
		ok        bool
	}{
		{"agrees", []string{"a", "b"}, []string{"imm-a", "imm-b"}, true},
		{"differs", []string{"a", "b"}, []string{"imm-a", "IMM-b"}, false},
		{"lengths differ", []string{"a", "b"}, []string{"imm-a"}, false},
		{"empty mailbox", nil, nil, true},
	}
	for _, c := range cases {
		tr := &IDTranslator{client: listingStub(&graphStub{}, c.plain, c.immutable)}
		if err := tr.Verify(t.Context()); (err == nil) != c.ok {
			t.Errorf("%s: Verify = %v, want ok=%v", c.name, err, c.ok)
		}
	}
}
