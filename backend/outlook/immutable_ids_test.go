package outlook

import (
	"net/http"
	"testing"
	"time"

	"github.com/sarielhp/feral/cache/cachedir"
	"github.com/sarielhp/feral/cache/meta"
	"github.com/sarielhp/feral/cache/msg"
	"github.com/sarielhp/feral/cfg_acc"
	"github.com/sarielhp/feral/cfg_g"
)

// The transport asks Graph for immutable IDs exactly when told to.
func TestOAuthTransport_PreferImmutableIDs(t *testing.T) {
	tok := &Token{AccessToken: "t", RefreshToken: "r", Expiry: time.Now().Add(time.Hour)}
	for _, immutable := range []bool{false, true} {
		var got string
		tr := &oauthTransport{
			tokenSource:  &tokenSource{token: tok},
			immutableIDs: immutable,
			base: &mockRoundTripper{roundTripFunc: func(req *http.Request) (*http.Response, error) {
				got = req.Header.Get("Prefer")
				return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
			}},
		}
		req, err := http.NewRequest(http.MethodGet, "https://graph.microsoft.com/v1.0/me/messages", nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := tr.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if want := map[bool]string{false: "", true: preferImmutableIDs}[immutable]; got != want {
			t.Errorf("immutable=%v: Prefer %q, want %q", immutable, got, want)
		}
	}
}

// A new account starts with immutable IDs; one whose cache already holds
// default IDs keeps them until it is migrated, or every message would split
// in two; a migrated one uses immutable IDs.
func TestUsesImmutableIDs(t *testing.T) {
	newClient := func(acct cachedir.Account) *client {
		return &client{config: &cfg_g.Config{AccountCache: acct}, account: cfg_acc.AccountConfig{Name: "o", Type: "outlook"}}
	}

	fresh := cachedir.At(t.TempDir())
	if !newClient(fresh).usesImmutableIDs() {
		t.Error("an empty cache must start with immutable IDs")
	}

	legacy := cachedir.At(t.TempDir())
	if err := msg.Store(legacy, "AQMk-default", []byte("Subject: x\r\n\r\nx"), time.Now()); err != nil {
		t.Fatal(err)
	}
	if newClient(legacy).usesImmutableIDs() {
		t.Error("a cache holding default IDs must keep them until migrated")
	}
	if err := meta.Write(legacy, meta.Manifest{Version: meta.CurrentVersion, Backend: "outlook", IDScheme: meta.SchemeGraphDefault}); err != nil {
		t.Fatal(err)
	}
	if newClient(legacy).usesImmutableIDs() {
		t.Error("a manifest saying default IDs must be obeyed")
	}

	if err := meta.MarkImmutableIDs(legacy, "test"); err != nil {
		t.Fatal(err)
	}
	if !newClient(legacy).usesImmutableIDs() {
		t.Error("a migrated cache must use immutable IDs")
	}

	if newClient(cachedir.Account{}).usesImmutableIDs() {
		t.Error("no cache, no marker: default IDs")
	}
}
