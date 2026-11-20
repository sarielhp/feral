package outlook

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
)

func statusResponse(code int, retryAfter, body string) *http.Response {
	h := make(http.Header)
	if retryAfter != "" {
		h.Set("Retry-After", retryAfter)
	}
	return &http.Response{StatusCode: code, Header: h, Body: io.NopCloser(bytes.NewReader([]byte(body)))}
}

// countingTransport answers with the given responses in turn and counts calls.
func countingTransport(calls *int, responses ...*http.Response) http.RoundTripper {
	return mockTransport(func(_ *http.Request) *http.Response {
		r := responses[min(*calls, len(responses)-1)]
		*calls++
		return r
	})
}

func TestThrottleTransport(t *testing.T) {
	for _, tc := range []struct {
		name      string
		method    string
		responses []*http.Response
		wantCalls int
		wantCode  int
	}{
		{"a throttled read is retried", http.MethodGet,
			[]*http.Response{statusResponse(429, "0", ""), statusResponse(200, "", "ok")}, 2, 200},
		{"a write is never retried", http.MethodPost,
			[]*http.Response{statusResponse(429, "0", ""), statusResponse(200, "", "ok")}, 1, 429},
		{"a long Retry-After is the caller's", http.MethodGet,
			[]*http.Response{statusResponse(429, "60", ""), statusResponse(200, "", "ok")}, 1, 429},
		{"retries are bounded", http.MethodGet,
			[]*http.Response{statusResponse(503, "0", "")}, graphThrottleRetries, 503},
		{"other errors pass through", http.MethodGet,
			[]*http.Response{statusResponse(404, "", ""), statusResponse(200, "", "ok")}, 1, 404},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			tr := &throttleTransport{base: countingTransport(&calls, tc.responses...)}
			req, err := http.NewRequest(tc.method, "https://graph.example/x", nil)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := tr.RoundTrip(req)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			if calls != tc.wantCalls || resp.StatusCode != tc.wantCode {
				t.Errorf("calls = %d, status = %d; want %d, %d", calls, resp.StatusCode, tc.wantCalls, tc.wantCode)
			}
		})
	}
}

// A subtree that cannot be listed fails the listing. It used to be skipped,
// and the partial tree was cached as the whole: 81 folders of 131 under
// throttling, and the missing ones vanished from the TUI and from label rm.
func TestGetLabelItems_FailedSubtreeFailsTheListing(t *testing.T) {
	c := &client{
		config:  &cfg_g.Config{},
		account: cfg_acc.AccountConfig{Name: "test"},
		client: &http.Client{Transport: mockTransport(func(req *http.Request) *http.Response {
			if strings.Contains(req.URL.Path, "childFolders") {
				return statusResponse(http.StatusInternalServerError, "", `{"error":"boom"}`)
			}
			return statusResponse(http.StatusOK, "",
				`{"value":[{"id":"p","displayName":"Parent","childFolderCount":1},{"id":"q","displayName":"Flat"}]}`)
		})},
	}
	items, err := c.GetLabelItems()
	if err == nil {
		t.Fatalf("GetLabelItems returned %d items and no error for a failed subtree", len(items))
	}
	if !strings.Contains(err.Error(), "Parent") {
		t.Errorf("error does not name the folder: %v", err)
	}
}
