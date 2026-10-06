package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/provider"
	codertestutil "github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// roundTripFunc adapts a function to http.RoundTripper so tests can observe
// each upstream attempt.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// newTestPool returns a key pool that offers keys in the given order.
func newTestPool(t *testing.T, keys ...string) *keypool.Pool {
	t.Helper()
	pool, err := keypool.New("test", keys, quartz.NewMock(t), nil)
	require.NoError(t, err)
	return pool
}

// Centralized requests move to the next key after a key failure, resending the
// same body bytes and client headers, and answer with the pool response once
// every key has failed. Rejected upstream responses never reach the client.
func TestForwardingFailoverPool(t *testing.T) {
	t.Parallel()
	const payload = "  not JSON\x00\n"
	for _, name := range []string{"Rotation", "Exhausted"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			type attempt struct {
				header http.Header
				body   string
			}
			// One spare slot keeps an unexpected extra attempt from blocking.
			attempts := make(chan attempt, 3)
			var count atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				attempts <- attempt{header: r.Header.Clone(), body: string(body)}
				if count.Add(1) == 1 || name == "Exhausted" {
					w.Header().Set("X-Rejected", "true")
					w.WriteHeader(http.StatusUnauthorized)
					_, _ = io.WriteString(w, "rejected key")
					return
				}
				_, _ = io.WriteString(w, "ok")
			}))
			t.Cleanup(upstream.Close)
			pool := newTestPool(t, "first-secret-key", "second-secret-key")
			h := newTestForwardingHandler(t, provider.NewOpenAI(config.OpenAI{BaseURL: upstream.URL, KeyPool: pool}), nil)
			input := &trackedBody{Reader: strings.NewReader(payload)}
			req := requestWithAuth(t, input)
			req.Header.Del("Authorization")
			req.Header.Set("User-Agent", "claude-code/1.0")
			req.Header.Set("Accept-Encoding", "gzip")
			original := req.Header.Clone()

			response := httptest.NewRecorder()
			prepareAndProxy(t, h, response, req)
			ctx := codertestutil.Context(t, codertestutil.WaitShort)
			for _, key := range []string{"first-secret-key", "second-secret-key"} {
				got := codertestutil.RequireReceive(ctx, t, attempts)
				require.Equal(t, "Bearer "+key, got.header.Get("Authorization"))
				require.Equal(t, payload, got.body, "every attempt must send the full request body")
				require.Equal(t, "claude-code/1.0", got.header.Get("User-Agent"))
				require.Equal(t, "gzip", got.header.Get("Accept-Encoding"))
			}
			require.Empty(t, attempts, "two keys allow exactly two attempts")
			require.Equal(t, original, req.Header, "failover must not mutate the inbound headers")
			require.Equal(t, 1, input.eofs, "retries must replay buffered input instead of reading it again")
			require.Zero(t, input.closes, "the server owns the inbound body")

			require.Empty(t, response.Header().Get("X-Rejected"), "rejected responses must not reach the client")
			require.NotContains(t, response.Body.String(), "rejected key")
			if name == "Exhausted" {
				require.Equal(t, http.StatusBadGateway, response.Code)
				require.Equal(t, "application/json", response.Header().Get("Content-Type"))
				require.NotContains(t, response.Body.String(), "secret-key")
				return
			}
			require.Equal(t, http.StatusOK, response.Code)
			require.Equal(t, "ok", response.Body.String())
		})
	}
}

// A retry replays the prefix that the rejected attempt already consumed and
// then continues reading the streamed request body. The rejected response is
// drained and closed without reaching the client.
func TestForwardingFailoverStreamedRetry(t *testing.T) {
	t.Parallel()
	ctx := codertestutil.Context(t, codertestutil.WaitLong)
	source, sink := io.Pipe()
	defer sink.Close()
	input := &trackedBody{Reader: source}
	const prefix = `{"metadata":{"user_id":"user_hash_account_id_session_`
	const suffix = `session-one"}}`
	h := newTestForwardingHandler(t, provider.NewOpenAI(config.OpenAI{BaseURL: "https://upstream.example.test", KeyPool: newTestPool(t, "first-key", "second-key")}), nil)
	rejected := &trackedBody{Reader: strings.NewReader("rejected key")}
	readPrefix := make(chan struct{}, 1)
	var auth []string
	h.transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		defer r.Body.Close()
		auth = append(auth, r.Header.Get("Authorization"))
		if len(auth) == 1 {
			chunk := make([]byte, len(prefix))
			_, err := io.ReadFull(r.Body, chunk)
			assert.NoError(t, err)
			assert.Equal(t, prefix, string(chunk))
			readPrefix <- struct{}{}
			return &http.Response{StatusCode: http.StatusUnauthorized, Header: http.Header{"X-Rejected": {"true"}}, Body: rejected}, nil
		}
		payload, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		assert.Equal(t, prefix+suffix, string(payload), "the retry must replay the prefix and read the remaining input")
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("ok"))}, nil
	})
	req := requestWithAuth(t, input)
	req.Header.Del("Authorization")
	response := httptest.NewRecorder()
	record, cred := h.checkRequest(response, req)
	require.NotNil(t, record, "request must pass validation")
	outbound, body := h.prepareForwarding(req, cred)

	served := make(chan struct{})
	go func() {
		defer close(served)
		h.proxy.ServeHTTP(response, outbound)
	}()
	_, err := io.WriteString(sink, prefix)
	require.NoError(t, err)
	// The first attempt must forward the prefix before the input is complete.
	codertestutil.RequireReceive(ctx, t, readPrefix)
	_, err = io.WriteString(sink, suffix)
	require.NoError(t, err)
	require.NoError(t, sink.Close())
	codertestutil.TryReceive(ctx, t, served)

	require.Equal(t, []string{"Bearer first-key", "Bearer second-key"}, auth)
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "ok", response.Body.String())
	require.Empty(t, response.Header().Get("X-Rejected"), "rejected responses must not reach the client")
	require.Equal(t, 1, rejected.eofs, "rejected responses must be drained")
	require.Equal(t, 1, rejected.closes, "rejected responses must be closed")
	require.Equal(t, 1, input.eofs)
	require.Zero(t, input.closes, "the server owns the inbound body")

	replay, err := body.getBody()
	require.NoError(t, err)
	replayed, err := io.ReadAll(replay)
	require.NoError(t, err)
	require.Equal(t, prefix+suffix, string(replayed))
	require.Equal(t, 1, input.eofs, "replay must not read completed input again")
}

// BYOK requests use only the client credential even when the provider has a
// configured pool, so an upstream 401 reaches the client without a retry.
func TestForwardingFailoverBYOK401(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"OpenAI", "Anthropic"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pool := newTestPool(t, "pool-secret-key")
			req := requestWithAuth(t, strings.NewReader("body"))
			var prov provider.Provider = provider.NewOpenAI(config.OpenAI{BaseURL: "https://upstream.example.test", KeyPool: pool})
			want := http.Header{"Authorization": {"Bearer user-secret-key"}}
			if name == "Anthropic" {
				var err error
				prov, err = provider.NewAnthropic(t.Context(), config.Anthropic{BaseURL: "https://upstream.example.test", KeyPool: pool}, nil, nil)
				require.NoError(t, err)
				req.Header.Del("Authorization")
				req.Header.Set("X-Api-Key", "anthropic-secret-key")
				want = http.Header{"X-Api-Key": {"anthropic-secret-key"}}
			}
			req.URL.Path = prov.RoutePrefix() + "/messages"
			h := newTestForwardingHandler(t, prov, nil)
			var sent []http.Header
			h.transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				defer r.Body.Close()
				sent = append(sent, r.Header.Clone())
				payload, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				assert.Equal(t, "body", string(payload))
				return &http.Response{StatusCode: http.StatusUnauthorized, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("invalid key"))}, nil
			})

			response := httptest.NewRecorder()
			prepareAndProxy(t, h, response, req)
			require.Equal(t, http.StatusUnauthorized, response.Code)
			require.Equal(t, "invalid key", response.Body.String())
			require.Len(t, sent, 1, "BYOK failures must not fall back to pooled credentials")
			for _, header := range []string{"Authorization", "X-Api-Key"} {
				require.Equal(t, want.Values(header), sent[0].Values(header), header)
			}
		})
	}
}
