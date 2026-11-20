package outlook

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/sarielhp/feral/email"
)

// The watcher's poll is one Graph request for the inbox's newest IDs, and
// nothing else: no folder walk, no message bodies.
func TestLatestInboxIDsIsOneInboxRequest(t *testing.T) {
	var urls []string
	c := outlookTestClient(mockTransport(func(req *http.Request) *http.Response {
		urls = append(urls, req.URL.String())
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{},
			Body: io.NopCloser(strings.NewReader(`{"value":[{"id":"n2"},{"id":"n1"}]}`))}
	}))
	ids, err := c.LatestInboxIDs(context.Background(), 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 || ids[0] != email.ID("n2") || ids[1] != email.ID("n1") {
		t.Fatalf("ids = %v, want [n2 n1] in the server's order", ids)
	}
	if len(urls) != 1 {
		t.Fatalf("made %d requests, want 1: %v", len(urls), urls)
	}
	u := urls[0]
	for _, want := range []string{"/me/mailFolders/inbox/messages", "$top=50", "$select=id", "receivedDateTime%20desc"} {
		if !strings.Contains(u, want) {
			t.Errorf("request %s lacks %s", u, want)
		}
	}
}

func TestLatestInboxIDsReportsARejectedRequest(t *testing.T) {
	c := outlookTestClient(mockTransport(func(*http.Request) *http.Response {
		return &http.Response{StatusCode: http.StatusUnauthorized, Header: http.Header{},
			Body: io.NopCloser(strings.NewReader(`{"error":{"code":"InvalidAuthenticationToken"}}`))}
	}))
	if _, err := c.LatestInboxIDs(context.Background(), 50); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("err = %v, want the 401 named", err)
	}
}
