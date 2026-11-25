package proxy

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/circuitbreaker"
	"github.com/coder/coder/v2/aibridge/config"
	aibcontext "github.com/coder/coder/v2/aibridge/context"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/metrics"
	"github.com/coder/coder/v2/aibridge/provider"
	"github.com/coder/coder/v2/aibridge/routing"
	codertestutil "github.com/coder/coder/v2/testutil"
)

type readerFunc func([]byte) (int, error)

func (f readerFunc) Read(p []byte) (int, error) { return f(p) }

func TestForwardingTransportOutcomes(t *testing.T) {
	t.Parallel()
	chunk := func(context.CancelFunc) io.Reader { return strings.NewReader("chunk") }
	for _, tc := range []struct {
		name string
		// upstream returns the upstream response body. cancel cancels the
		// request context.
		upstream     func(cancel context.CancelFunc) io.Reader
		status       int
		transportErr error
		// underServer runs the request with an HTTP server context, the only
		// case in which ReverseProxy aborts a broken stream.
		underServer     bool
		wantStatus      int
		wantPanic       any
		wantResponseErr error
		wantClientBody  string
	}{
		{
			name:           "Success",
			upstream:       chunk,
			status:         http.StatusOK,
			wantStatus:     http.StatusOK,
			wantClientBody: "chunk",
		},
		{
			name: "BytesWithEOF",
			upstream: func(context.CancelFunc) io.Reader {
				return readerFunc(func(p []byte) (int, error) { return copy(p, "chunk"), io.EOF })
			},
			status:         http.StatusOK,
			wantStatus:     http.StatusOK,
			wantClientBody: "chunk",
		},
		{
			name:         "TransportFailure",
			transportErr: io.ErrClosedPipe,
			wantStatus:   http.StatusBadGateway,
		},
		{
			name:           "Upstream503",
			upstream:       chunk,
			status:         http.StatusServiceUnavailable,
			wantStatus:     http.StatusServiceUnavailable,
			wantClientBody: "chunk",
		},
		{
			name:       "Upgrade",
			upstream:   chunk,
			status:     http.StatusSwitchingProtocols,
			wantStatus: http.StatusBadGateway,
		},
		{
			name: "ReadFailure",
			upstream: func(context.CancelFunc) io.Reader {
				return io.MultiReader(strings.NewReader("chunk"), readerFunc(func([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }))
			},
			status:          http.StatusOK,
			underServer:     true,
			wantPanic:       http.ErrAbortHandler,
			wantResponseErr: io.ErrUnexpectedEOF,
		},
		{
			name: "Cancellation",
			upstream: func(cancel context.CancelFunc) io.Reader {
				return io.MultiReader(strings.NewReader("chunk"), readerFunc(func([]byte) (int, error) { cancel(); return 0, context.Canceled }))
			},
			status:          http.StatusOK,
			wantStatus:      http.StatusOK,
			wantResponseErr: context.Canceled,
			wantClientBody:  "chunk",
		},
		{
			name:         "PoolFailure",
			transportErr: &keypool.Error{Kind: keypool.ErrorKindRateLimited},
			wantStatus:   http.StatusBadGateway,
		},
		{
			name:         "CircuitOpen",
			transportErr: circuitbreaker.ErrCircuitOpen,
			wantStatus:   http.StatusBadGateway,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			reqBody := &trackedBody{Reader: strings.NewReader("not JSON")}
			req := requestWithAuth(t, reqBody)
			ctx, cancel := context.WithCancel(req.Context())
			defer cancel()
			if tc.underServer {
				ctx = context.WithValue(ctx, http.ServerContextKey, &http.Server{ReadHeaderTimeout: codertestutil.WaitShort})
			}
			respBody := &trackedBody{}
			if tc.upstream != nil {
				respBody.Reader = tc.upstream(cancel)
			}
			response := httptest.NewRecorder()
			h := newTestForwardingHandler(t, provider.NewOpenAI(config.OpenAI{BaseURL: "https://upstream.example.test"}), metrics.NewMetrics(prometheus.NewRegistry()))
			calls := 0
			h.transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				payload, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				require.Equal(t, "not JSON", string(payload))
				require.NotNil(t, r.GetBody)
				replay, err := r.GetBody()
				require.NoError(t, err)
				payload, err = io.ReadAll(replay)
				require.NoError(t, err)
				require.NoError(t, replay.Close())
				require.Equal(t, "not JSON", string(payload))
				if tc.transportErr != nil {
					return nil, tc.transportErr
				}
				return &http.Response{StatusCode: tc.status, Header: http.Header{}, Body: respBody}, nil
			})
			req = req.WithContext(ctx)
			call := &forwardedRequest{handler: h}
			if tc.wantPanic != nil {
				require.PanicsWithValue(t, tc.wantPanic, func() { call.serve(response, req) })
			} else {
				call.serve(response, req)
				require.Equal(t, tc.wantStatus, response.Code)
			}
			require.Equal(t, 1, calls)
			require.Zero(t, reqBody.closes.Load(), "the server closes the request body after the handler returns")
			require.EqualValues(t, 1, reqBody.eofs.Load())
			require.NotNil(t, call.requestBody, "forwarding must prepare the request body")
			replayed, err := call.requestBody.Wait(codertestutil.Context(t, codertestutil.WaitLong))
			require.NoError(t, err)
			require.Equal(t, "not JSON", string(replayed))
			require.EqualValues(t, 1, reqBody.eofs.Load(), "replay must not read completed input again")
			if tc.transportErr != nil {
				require.ErrorIs(t, call.err, tc.transportErr)
				require.Nil(t, call.responseBody)
				require.Zero(t, respBody.closes.Load())
				return
			}
			if tc.status == http.StatusSwitchingProtocols {
				require.ErrorContains(t, call.err, "upstream protocol upgrades are not supported")
				require.Nil(t, call.responseBody, "rejected upgrades must not be observed as responses")
				require.EqualValues(t, 1, respBody.closes.Load(), "rejected upgrades must close the upstream body")
				return
			}
			require.Equal(t, tc.status, call.status)
			require.NotNil(t, call.responseBody)
			require.EqualValues(t, 1, respBody.closes.Load(), "forwarding must close the upstream body")
			captured, responseErr := call.responseBody.Wait(codertestutil.Context(t, codertestutil.WaitShort))
			if tc.wantResponseErr != nil {
				require.ErrorIs(t, responseErr, tc.wantResponseErr)
			} else {
				require.NoError(t, call.err)
				require.NoError(t, responseErr)
				require.Equal(t, "chunk", string(captured))
			}
			if tc.wantClientBody != "" {
				require.Equal(t, tc.wantClientBody, response.Body.String(), "forwarding must deliver the bytes read before any failure")
			}
		})
	}
}

// Request body read failures reject the request before recording or forwarding
// it.
func TestForwardingBodyFailures(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		// source returns the request body. read counts what it produces.
		source     func(read *atomic.Int64) io.Reader
		wantStatus int
		maxRead    int64
	}{
		{
			name: "UnknownLengthOversize",
			source: func(read *atomic.Int64) io.Reader {
				return readerFunc(func(p []byte) (int, error) { clear(p); read.Add(int64(len(p))); return len(p), nil })
			},
			wantStatus: http.StatusRequestEntityTooLarge,
			maxRead:    routing.MaxRequestBodyBytes + 1,
		},
		{
			name: "BodyReadFailure",
			source: func(read *atomic.Int64) io.Reader {
				return readerFunc(func([]byte) (int, error) { read.Add(1); return 0, io.ErrUnexpectedEOF })
			},
			wantStatus: http.StatusBadRequest,
			maxRead:    1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var read atomic.Int64
			response := httptest.NewRecorder()
			req := requestWithAuth(t, nil)
			req.Body = http.MaxBytesReader(response, io.NopCloser(tc.source(&read)), routing.MaxRequestBodyBytes)
			req.ContentLength = -1
			h := newTestForwardingHandler(t, provider.NewOpenAI(config.OpenAI{BaseURL: "https://upstream.example.test"}), nil)
			rec := &testutil.MockRecorder{}
			h.recorder = rec
			h.transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				t.Error("rejected requests must not call upstream")
				return nil, io.ErrUnexpectedEOF
			})
			call := &forwardedRequest{handler: h}
			call.serve(response, req)
			require.NoError(t, h.inflight.Shutdown(codertestutil.Context(t, codertestutil.WaitLong)))

			require.Equal(t, tc.wantStatus, response.Code)
			require.Empty(t, rec.RecordedInterceptions(), "rejected requests must not record a start")
			require.Nil(t, call.requestBody, "rejected requests must not keep the capture")
			require.LessOrEqual(t, read.Load(), tc.maxRead, "failure must stop reading input")
		})
	}
}

// signalReadBody closes read once the first bytes arrive from the client.
type signalReadBody struct {
	io.ReadCloser
	read chan struct{}
	once sync.Once
}

func (b *signalReadBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		b.once.Do(func() { close(b.read) })
	}
	return n, err
}

// Closing the inflight gate unblocks a request whose body stalls mid-read, so
// shutdown does not wait for the client to finish sending.
func TestForwardingStalledBodyShutdown(t *testing.T) {
	t.Parallel()
	h := newTestForwardingHandler(t, provider.NewOpenAI(config.OpenAI{BaseURL: "https://upstream.example.test"}), nil)
	rec := &testutil.MockRecorder{}
	h.recorder = rec
	h.transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Error("stalled requests must not call upstream")
		return nil, io.ErrUnexpectedEOF
	})
	reading, served := make(chan struct{}), make(chan struct{})
	// The handler response is recorded here, the stalled client cannot read it.
	response := httptest.NewRecorder()
	gateway := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		defer close(served)
		r.Body = &signalReadBody{ReadCloser: r.Body, read: reading}
		ctx := aibcontext.AsActor(r.Context(), aibcontext.Actor{ID: uuid.New(), APIKeyID: uuid.NewString()})
		h.ServeHTTP(response, r.WithContext(ctx))
	}))
	t.Cleanup(gateway.Close)
	// The client sends one byte, then stalls until cleanup ends the body.
	body, stall := io.Pipe()
	t.Cleanup(func() { _ = stall.Close() })
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, gateway.URL+"/openai/v1/chat/completions", body)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+userSecretKey)
	go func() {
		resp, err := gateway.Client().Do(req)
		if err == nil {
			_ = resp.Body.Close()
		}
	}()
	_, err = stall.Write([]byte("{"))
	require.NoError(t, err)

	ctx := codertestutil.Context(t, codertestutil.WaitShort)
	codertestutil.TryReceive(ctx, t, reading)
	h.inflight.Close()
	require.NoError(t, h.inflight.Shutdown(ctx), "a stalled body must not hold admission after close")
	codertestutil.TryReceive(ctx, t, served)
	require.Equal(t, http.StatusServiceUnavailable, response.Code, "cancellation is not a client error")
	require.Empty(t, rec.RecordedInterceptions(), "canceled requests must not record a start")
}

// Key failover retries with the next pooled key, and an exhausted pool returns
// its error response.
func TestForwardingKeyOutcomes(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		statuses   []int
		wantStatus int
		// wantErr is the key pool error forwarding reports. Only Kind is compared.
		wantErr *keypool.Error
	}{
		{
			name:       "Rotation",
			statuses:   []int{http.StatusUnauthorized, http.StatusOK},
			wantStatus: http.StatusOK,
		},
		{
			name:       "Exhausted",
			statuses:   []int{http.StatusUnauthorized, http.StatusUnauthorized},
			wantStatus: http.StatusBadGateway,
			wantErr:    &keypool.Error{Kind: keypool.ErrorKindUnauthorized},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newTestForwardingHandler(t, provider.NewOpenAI(config.OpenAI{BaseURL: "https://upstream.example.test", KeyPool: newTestPool(t, "first-key", "second-key")}), nil)
			attempts := 0
			h.transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				require.Less(t, attempts, len(tc.statuses), "unexpected upstream attempt")
				status := tc.statuses[attempts]
				attempts++
				_, err := io.Copy(io.Discard, r.Body)
				require.NoError(t, err)
				return &http.Response{StatusCode: status, Header: http.Header{}, Body: http.NoBody}, nil
			})
			req := requestWithAuth(t, strings.NewReader("body"))
			req.Header.Del("Authorization")
			response := httptest.NewRecorder()
			call := &forwardedRequest{handler: h}
			call.serve(response, req)
			require.Equal(t, len(tc.statuses), attempts)
			require.Equal(t, tc.wantStatus, response.Code)
			require.Equal(t, tc.wantStatus, call.status)
			if tc.wantErr == nil {
				require.NoError(t, call.err)
			} else {
				keyErr, ok := errors.AsType[*keypool.Error](call.err)
				require.True(t, ok, "unexpected forwarding error: %v", call.err)
				require.Equal(t, tc.wantErr.Kind, keyErr.Kind)
			}
			require.NotNil(t, call.responseBody)
			_, err := call.responseBody.Wait(codertestutil.Context(t, codertestutil.WaitShort))
			require.NoError(t, err)
		})
	}
}

// A BYOK upstream rejection is the final outcome, even when the provider has a
// configured pool.
func TestForwardingBYOKOutcomes(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		newProvider func(t *testing.T, pool *keypool.Pool) provider.Provider
		// auth replaces the request's credential headers.
		auth http.Header
	}{
		{
			name: "OpenAI",
			newProvider: func(_ *testing.T, pool *keypool.Pool) provider.Provider {
				return provider.NewOpenAI(config.OpenAI{BaseURL: "https://upstream.example.test", KeyPool: pool})
			},
			auth: http.Header{"Authorization": {"Bearer " + userSecretKey}},
		},
		{
			name: "AnthropicAPIKey",
			newProvider: func(t *testing.T, pool *keypool.Pool) provider.Provider {
				prov, err := provider.NewAnthropic(t.Context(), config.Anthropic{BaseURL: "https://upstream.example.test", KeyPool: pool}, nil, nil)
				require.NoError(t, err)
				return prov
			},
			auth: http.Header{"X-Api-Key": {"anthropic-secret-key"}},
		},
		{
			name: "Copilot",
			newProvider: func(*testing.T, *keypool.Pool) provider.Provider {
				return provider.NewCopilot(config.Copilot{BaseURL: "https://upstream.example.test"})
			},
			auth: http.Header{"Authorization": {"Bearer " + userSecretKey}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			prov := tc.newProvider(t, newTestPool(t, "pool-secret-key"))
			req := requestWithAuth(t, strings.NewReader("body"))
			req.URL.Path = prov.RoutePrefix() + "/messages"
			req.Header.Del("Authorization")
			for name, values := range tc.auth {
				req.Header[name] = values
			}
			h := newTestForwardingHandler(t, prov, nil)
			attempts := 0
			h.transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				attempts++
				return &http.Response{StatusCode: http.StatusUnauthorized, Header: http.Header{}, Body: http.NoBody}, nil
			})
			response := httptest.NewRecorder()
			call := &forwardedRequest{handler: h}
			call.serve(response, req)
			require.Equal(t, http.StatusUnauthorized, response.Code)
			require.Equal(t, 1, attempts, "BYOK failures must not fall back to pooled credentials")
			require.Equal(t, http.StatusUnauthorized, call.status)
			require.NoError(t, call.err)
		})
	}
}

func TestForwardingStreamsAndCancels(t *testing.T) {
	t.Parallel()
	canceled, forwarded := make(chan struct{}), make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: first\n\n")
		assert.NoError(t, http.NewResponseController(w).Flush())
		select {
		case <-r.Context().Done():
			close(canceled)
		case <-release:
		}
	}))
	t.Cleanup(upstream.Close)
	h := newTestForwardingHandler(t, provider.NewOpenAI(config.OpenAI{BaseURL: upstream.URL}), nil)
	calls := make(chan *forwardedRequest, 1)
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(forwarded)
		ctx := aibcontext.AsActor(r.Context(), aibcontext.Actor{ID: uuid.New(), APIKeyID: uuid.NewString()})
		call := &forwardedRequest{handler: h}
		// A canceled stream aborts serve, so publish the call first.
		calls <- call
		call.serve(w, r.WithContext(ctx))
	}))
	t.Cleanup(gateway.Close)
	ctx, cancel := context.WithCancel(codertestutil.Context(t, codertestutil.WaitLong))
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, gateway.URL+"/openai/v1/chat/completions", strings.NewReader("request"))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+userSecretKey)
	resp, err := gateway.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	first := make([]byte, len("data: first\n\n"))
	_, err = io.ReadFull(resp.Body, first)
	require.NoError(t, err)
	require.Equal(t, "data: first\n\n", string(first))
	select {
	case <-forwarded:
		t.Fatal("forwarding returned before response delivery completed")
	default:
	}
	cancel()
	waitCtx := codertestutil.Context(t, codertestutil.WaitLong)
	codertestutil.TryReceive(waitCtx, t, canceled)
	codertestutil.TryReceive(waitCtx, t, forwarded)
	call := codertestutil.RequireReceive(waitCtx, t, calls)
	require.Equal(t, http.StatusOK, call.status)
	require.NotNil(t, call.responseBody)
	_, err = call.responseBody.Wait(waitCtx)
	require.Error(t, err, "a canceled stream must not look complete")
}

func TestForwardingCircuitBreaker(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(upstream.Close)
	cfg := config.DefaultCircuitBreaker()
	cfg.FailureThreshold = 1
	cfg.Timeout = time.Hour
	h := newTestForwardingHandler(t, provider.NewOpenAI(config.OpenAI{BaseURL: upstream.URL, CircuitBreaker: &cfg}), nil)

	tripped := &forwardedRequest{handler: h}
	tripped.serve(httptest.NewRecorder(), requestWithAuth(t, nil))
	require.Equal(t, http.StatusServiceUnavailable, tripped.status)
	require.NoError(t, tripped.err)
	require.NotNil(t, tripped.requestBody)
	replay, err := tripped.outbound.GetBody()
	require.NoError(t, err)
	require.Equal(t, http.NoBody, replay, "known empty bodies must not become chunked requests")

	response := httptest.NewRecorder()
	rejected := &forwardedRequest{handler: h}
	rejected.serve(response, requestWithAuth(t, nil))
	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	require.ErrorIs(t, rejected.err, circuitbreaker.ErrCircuitOpen)
	require.Nil(t, rejected.responseBody)

	require.EqualValues(t, 1, calls.Load(), "the 503 must trip the breaker")
}

// A client that aborts a healthy stream must not count as an upstream failure.
func TestForwardingAbortDoesNotTripBreaker(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: chunk\n\n")
		if calls.Add(1) == 1 {
			assert.NoError(t, http.NewResponseController(w).Flush())
			<-r.Context().Done()
		}
	}))
	t.Cleanup(upstream.Close)
	cfg := config.DefaultCircuitBreaker()
	cfg.FailureThreshold = 1
	cfg.Timeout = time.Hour
	h := newTestForwardingHandler(t, provider.NewOpenAI(config.OpenAI{BaseURL: upstream.URL, CircuitBreaker: &cfg}), nil)
	served := make(chan struct{}, 2)
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { served <- struct{}{} }()
		ctx := aibcontext.AsActor(r.Context(), aibcontext.Actor{ID: uuid.New(), APIKeyID: uuid.NewString()})
		h.ServeHTTP(w, r.WithContext(ctx))
	}))
	t.Cleanup(gateway.Close)
	send := func(ctx context.Context) *http.Response {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, gateway.URL+"/openai/v1/chat/completions", strings.NewReader("{}"))
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+userSecretKey)
		resp, err := gateway.Client().Do(req)
		require.NoError(t, err)
		return resp
	}

	ctx, cancel := context.WithCancel(codertestutil.Context(t, codertestutil.WaitLong))
	first := send(ctx)
	chunk := make([]byte, len("data: chunk\n\n"))
	_, err := io.ReadFull(first.Body, chunk)
	require.NoError(t, err)
	cancel()
	_ = first.Body.Close()
	codertestutil.TryReceive(codertestutil.Context(t, codertestutil.WaitLong), t, served)

	second := send(codertestutil.Context(t, codertestutil.WaitLong))
	defer second.Body.Close()
	body, err := io.ReadAll(second.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, second.StatusCode, "body: %s", body)
	require.Equal(t, "data: chunk\n\n", string(body))
	require.EqualValues(t, 2, calls.Load())
}
