package keypool

import (
	"bytes"
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

	Logger slog.Logger

	// IsBYOK returns true when the request already carries user-supplied auth.
	// It is required whenever Pool is non-nil. BYOK requests skip key failover.
	IsBYOK func(*http.Request) bool

	// InjectAuthKey writes the key value into the outbound headers in the format
	// the provider expects. It is required whenever Pool is non-nil.
	InjectAuthKey func(*http.Header, string)

	// BuildKeyPoolResponse renders the response sent to the client when the walker
	// has no more keys to try. It is required whenever Pool is non-nil.
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

	newBody, err := bufferBody(req)
	if err != nil {
		return nil, err
	}

	// Fresh walker per request, independent of other inflight requests.
	walker := t.config.Pool.Walker()
	attempts := 0
	defer func() { t.config.Pool.RecordAttempts(attempts) }()
	for {
		key, keyPoolErr := walker.Next()
		if keyPoolErr != nil {
			resp := t.config.BuildKeyPoolResponse(keyPoolErr)
			if resp == nil {
				// Fallback if BuildKeyPoolResponse returns nil.
				body := []byte(`{"error":"key pool unavailable"}`)
				resp = utils.NewJSONErrorResponse(http.StatusBadGateway, 0, body)
			}
			return resp, nil
		}

		// Clone per attempt so the original request isn't mutated.
		outReq := req.Clone(req.Context())
		attemptBody, err := newBody()
		if err != nil {
			if attemptBody != nil {
				_ = attemptBody.Close()
			}
			return nil, err
		}
		outReq.Body = attemptBody
		t.config.InjectAuthKey(&outReq.Header, key.Value())

		attempts++
		resp, rtErr := t.inner.RoundTrip(outReq)
		if rtErr != nil {
			// Transport-level error, not a key issue.
			return resp, rtErr
		}
		// MarkKeyOnStatus returns true on key-specific failures (e.g. 401/429).
		if t.config.Pool.MarkKeyOnStatus(req.Context(), key, resp, t.config.Logger) {
			// Drain and retry with the next key.
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			continue
		}
		// Success or non-key error, forward as-is.
		return resp, nil
	}
}

// bufferBody returns a factory for replaying the request body. It uses GetBody
// when available; otherwise it buffers the body once. The original body is closed.
func bufferBody(req *http.Request) (func() (io.ReadCloser, error), error) {
	if req.Body != nil {
		defer req.Body.Close()
	}
	if req.GetBody != nil {
		return req.GetBody, nil
	}
	if req.Body == nil {
		return func() (io.ReadCloser, error) {
			return nil, nil //nolint:nilnil // A nil request body represents no body.
		}, nil
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	return func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}, nil
}
