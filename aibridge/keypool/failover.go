package keypool

import (
	"bytes"
	"context"
	"io"
	"net/http"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/utils"
)

// KeyFailoverConfig is the per-provider configuration consumed by
// NewKeyFailoverTransport.
type KeyFailoverConfig struct {
	// Pool is the key pool to walk. Nil disables key failover.
	Pool *Pool

	ProviderName string
	Logger       slog.Logger

	// IsBYOK returns true when the request already carries
	// user-supplied auth. BYOK requests skip key failover.
	IsBYOK func(*http.Request) bool

	// InjectAuthKey writes the key value into the outbound headers
	// in the format the provider expects.
	InjectAuthKey func(*http.Header, string)

	// BuildKeyPoolResponse renders the response sent to the client
	// when the walker has no more keys to try.
	BuildKeyPoolResponse func(*Error) *http.Response
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

	// result carries both return values of one attempt so the failover loop
	// hands back a success or a transport error as a single atomic payload.
	type result struct {
		resp *http.Response
		err  error
	}
	res, keyPoolErr := Failover(req.Context(), t.config.Pool, t.config.Logger, t.config.ProviderName,
		func(ctx context.Context, key *Key) (result, *Failure) {
			// Clone per attempt so the original request isn't mutated.
			outReq := req.Clone(ctx)
			if body != nil {
				outReq.Body = io.NopCloser(bytes.NewReader(body))
			}
			t.config.InjectAuthKey(&outReq.Header, key.Value())

			resp, rtErr := t.inner.RoundTrip(outReq)
			if rtErr != nil {
				// Transport-level error, not a key issue: stop and return.
				return result{resp, rtErr}, nil
			}
			failure := Classify(resp)
			if failure != nil {
				// Drain the discarded response before retrying with the next key.
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
			}
			return result{resp, nil}, failure
		})
	if keyPoolErr != nil {
		resp := t.config.BuildKeyPoolResponse(keyPoolErr)
		if resp == nil {
			// Fallback if BuildKeyPoolResponse returns nil.
			body := []byte(`{"error":"key pool unavailable"}`)
			resp = utils.NewJSONErrorResponse(http.StatusBadGateway, 0, body)
		}
		return resp, nil
	}
	// Success or non-key transport error, forward as-is.
	return res.resp, res.err
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
