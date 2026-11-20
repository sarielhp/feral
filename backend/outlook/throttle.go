package outlook

import (
	"io"
	"log/slog"
	"net/http"
	"time"
)

// maxThrottleWait is the longest Retry-After a request waits out. A longer
// one is returned to the caller as the 429 it is: the client's own timeout
// would end the wait anyway, and a user is looking at the screen.
const maxThrottleWait = 10 * time.Second

// throttleTransport retries a read that Graph throttled. Graph answers 429
// (or 503) with Retry-After when an application sends too much, and the
// request was not acted on, so a GET can simply be sent again. Without this,
// one throttled request in a folder listing or a message list failed the
// whole operation -- or, in the folder walk, silently dropped a subtree.
// Only GET and HEAD are retried: they carry no body to replay and change
// nothing on the server.
type throttleTransport struct {
	base http.RoundTripper
}

func (t *throttleTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		return t.base.RoundTrip(req)
	}
	for attempt := 1; ; attempt++ {
		resp, err := t.base.RoundTrip(req)
		if err != nil {
			return resp, err
		}
		wait, throttled := graphRetryAfter(resp)
		if !throttled || attempt >= graphThrottleRetries || wait > maxThrottleWait {
			return resp, nil
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		slog.Info("throttled by Graph, retrying", slog.String("url", req.URL.Path),
			slog.Int("attempt", attempt), slog.Duration("wait", wait))
		select {
		case <-req.Context().Done():
			return nil, req.Context().Err()
		case <-time.After(wait):
		}
	}
}
