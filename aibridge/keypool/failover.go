package keypool

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/coder/coder/v2/aibridge/utils"
)

// KeyFailoverConfig is the per-provider configuration consumed by
// NewKeyFailoverTransport.
type KeyFailoverConfig struct {
	// Pool is the key pool to walk. Nil disables key failover.
	Pool *Pool

	// IsBYOK returns true when the request already carries
	// user-supplied auth. BYOK requests skip key failover.
	IsBYOK func(*http.Request) bool

	// InjectAuthKey writes the key value into the outbound headers
	// in the format the provider expects.
	InjectAuthKey func(*http.Header, string)

	// MarkKey marks the key based on the upstream response.
	// Returns true when the response is a key-specific error,
	// causing the walker to advance and retry with the next key.
	MarkKey func(ctx context.Context, key *Key, resp *http.Response) bool

	// BuildExhaustedResponse returns the response sent to the
	// client when the walker has no more keys to try.
	BuildExhaustedResponse func(err error) *http.Response
}

// keyFailoverTransport retries inner across the key pool on
// key-specific failures.
type keyFailoverTransport struct {
	inner  http.RoundTripper
	config KeyFailoverConfig
}

// NewKeyFailoverTransport returns an http.RoundTripper backed by
// keyFailoverTransport. If config.Pool is nil, inner is returned
// unchanged.
func NewKeyFailoverTransport(inner http.RoundTripper, config KeyFailoverConfig) http.RoundTripper {
	if config.Pool == nil {
		return inner
	}
	return &keyFailoverTransport{
		inner:  inner,
		config: config,
	}
}

// RoundTrip is invoked by the proxy once per outer client request,
// after Rewrite has applied proxy headers.
//
// For centralized requests it walks the key pool, retrying on
// key-specific failures until one key succeeds or the pool is
// exhausted. BYOK requests skip the failover loop.
func (t *keyFailoverTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.config.IsBYOK(req) {
		return t.inner.RoundTrip(req)
	}

	// Buffer once so retries can replay the body.
	body, err := bufferBody(req)
	if err != nil {
		return nil, err
	}

	// Fresh walker per request, independent of other inflight requests.
	walker := t.config.Pool.Walker()
	for {
		key, err := walker.Next()
		if err != nil {
			resp := t.config.BuildExhaustedResponse(err)
			if resp == nil {
				// Fallback if BuildExhaustedResponse returns nil.
				body := []byte(fmt.Sprintf(`{"error":"key pool exhausted: %s"}`, err))
				resp = utils.NewJSONErrorResponse(http.StatusBadGateway, 0, body)
			}
			return resp, nil
		}

		// Clone per attempt so the original request isn't mutated.
		outReq := req.Clone(req.Context())
		if body != nil {
			outReq.Body = io.NopCloser(bytes.NewReader(body))
		}
		t.config.InjectAuthKey(&outReq.Header, key.Value())

		resp, rtErr := t.inner.RoundTrip(outReq)
		if rtErr != nil {
			// Transport-level error, not a key issue.
			return resp, rtErr
		}
		// MarkKey returns true on key-specific failures (e.g. 401/403/429).
		if t.config.MarkKey(req.Context(), key, resp) {
			// Drain and retry with the next key.
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			continue
		}
		// Success or non-key error, forward as-is.
		return resp, nil
	}
}

// bufferBody reads the request body fully so it can be replayed
// across key-failover retries. Returns nil for a nil body.
func bufferBody(req *http.Request) ([]byte, error) {
	if req.Body == nil {
		return nil, nil
	}
	defer req.Body.Close()
	return io.ReadAll(req.Body)
}
