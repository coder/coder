package proxy

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/circuitbreaker"
	"github.com/coder/coder/v2/aibridge/config"
	aibcontext "github.com/coder/coder/v2/aibridge/context"
	"github.com/coder/coder/v2/aibridge/credential"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/metrics"
	"github.com/coder/coder/v2/aibridge/provider"
	"github.com/coder/coder/v2/aibridge/routing"
	"github.com/coder/coder/v2/aibridge/x/proxy/replaybody"
	codertestutil "github.com/coder/coder/v2/testutil"
)

type readerFunc func([]byte) (int, error)

func (f readerFunc) Read(p []byte) (int, error) { return f(p) }

// forwardingCall is a request prepared for the forwarding seam.
type forwardingCall struct {
	handler  *forwardingHandler
	request  *http.Request
	body     *replaybody.TeeBuffer
	logger   slog.Logger
	cred     credential.Credential
	result   forwardResult
	err      error
	cleanups []func()
}

// startForwarding performs the bridged handler's setup around the forwarding
// seam: server body limiting, inflight admission, and request validation. It
// reports false after writing a rejection. The validation recorder fails
// loudly if anything tries to record through it.
func startForwarding(h *forwardingHandler, w http.ResponseWriter, r *http.Request) (*forwardingCall, bool) {
	call := &forwardingCall{handler: h}
	if r.Body != nil && r.Body != http.NoBody {
		body := http.MaxBytesReader(w, r.Body, routing.MaxRequestBodyBytes)
		r.Body = body
		call.cleanups = append(call.cleanups, func() { _ = body.Close() })
	}
	release, ok := h.inflight.Admit()
	if !ok {
		call.finish()
		http.Error(w, "AI Gateway is shutting down", http.StatusServiceUnavailable)
		return nil, false
	}
	ctx, cleanup := h.inflight.RequestContext(r.Context())
	call.cleanups = append(call.cleanups, release, cleanup)
	r = r.WithContext(ctx)
	logger, record, cred := h.checkRequest(w, r)
	call.logger = logger
	if record == nil {
		call.finish()
		return nil, false
	}
	call.cred = cred
	call.request = r
	return call, true
}

// mustStartForwarding is startForwarding for the test goroutine.
func mustStartForwarding(t *testing.T, h *forwardingHandler, w http.ResponseWriter, r *http.Request) *forwardingCall {
	t.Helper()
	call, ok := startForwarding(h, w, r)
	require.True(t, ok, "request must pass validation")
	return call
}

// forward runs the handler's breaker and forwarding steps, then releases the
// request's admission and body.
func (c *forwardingCall) forward(w http.ResponseWriter) {
	defer c.finish()
	route := strings.TrimPrefix(c.request.URL.Path, "/"+c.handler.provider.Name())
	c.err = c.handler.breaker.Execute(route, "", w, func(rw http.ResponseWriter) error {
		outbound, body := replaybody.NewRequest(c.request)
		c.request, c.body = outbound, body
		c.cleanups = append(c.cleanups, func() { _ = body.Close() })
		return c.handler.forward(rw, outbound, c.cred, &c.result, c.logger)
	})
}

func (c *forwardingCall) finish() {
	for i := len(c.cleanups) - 1; i >= 0; i-- {
		c.cleanups[i]()
	}
	c.cleanups = nil
}

// replay reads the retained request bytes and capture error after forwarding.
func (c *forwardingCall) replay(t *testing.T) ([]byte, error) {
	t.Helper()
	require.NotNil(t, c.body, "forwarding must prepare the request body")
	return c.body.Wait(codertestutil.Context(t, codertestutil.WaitLong))
}

func TestForwardingTransportOutcomes(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"Success", "BytesWithEOF", "TransportFailure", "Upstream503", "Upgrade", "ReadFailure", "Cancellation", "PoolFailure", "CircuitOpen"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			body := &trackedBody{Reader: strings.NewReader("chunk")}
			input := &trackedBody{Reader: strings.NewReader("not JSON")}
			req := requestWithAuth(t, input)
			ctx, cancel := context.WithCancel(req.Context())
			defer cancel()
			response := httptest.NewRecorder()
			status, wantStatus := http.StatusOK, http.StatusOK
			var transportErr error
			var wantPanic any
			switch name {
			case "BytesWithEOF":
				body.Reader = readerFunc(func(p []byte) (int, error) { return copy(p, "chunk"), io.EOF })
			case "TransportFailure":
				transportErr = io.ErrClosedPipe
				wantStatus = http.StatusBadGateway
			case "Upstream503":
				status = http.StatusServiceUnavailable
				wantStatus = status
			case "Upgrade":
				status = http.StatusSwitchingProtocols
				wantStatus = http.StatusBadGateway
			case "ReadFailure":
				body.Reader = io.MultiReader(body.Reader, readerFunc(func([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }))
				// ReverseProxy aborts a broken stream only under an HTTP server.
				ctx = context.WithValue(ctx, http.ServerContextKey, &http.Server{ReadHeaderTimeout: codertestutil.WaitShort})
				wantPanic = http.ErrAbortHandler
			case "Cancellation":
				body.Reader = io.MultiReader(body.Reader, readerFunc(func([]byte) (int, error) { cancel(); return 0, context.Canceled }))
			case "PoolFailure":
				transportErr = &keypool.Error{Kind: keypool.ErrorKindRateLimited}
				wantStatus = http.StatusBadGateway
			case "CircuitOpen":
				transportErr = circuitbreaker.ErrCircuitOpen
				wantStatus = http.StatusBadGateway
			}
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
				if transportErr != nil {
					return nil, transportErr
				}
				return &http.Response{StatusCode: status, Header: http.Header{}, Body: body}, nil
			})
			call := mustStartForwarding(t, h, response, req.WithContext(ctx))
			if wantPanic != nil {
				require.PanicsWithValue(t, wantPanic, func() { call.forward(response) })
			} else {
				call.forward(response)
				require.Equal(t, wantStatus, response.Code)
			}
			require.Equal(t, 1, calls)
			require.EqualValues(t, 1, input.closes.Load())
			require.EqualValues(t, 1, input.eofs.Load())
			replayed, err := call.replay(t)
			require.NoError(t, err)
			require.Equal(t, "not JSON", string(replayed))
			require.EqualValues(t, 1, input.eofs.Load(), "replay must not read completed input again")
			if transportErr != nil {
				require.ErrorIs(t, call.err, transportErr)
				require.Nil(t, call.result.response)
				require.Zero(t, body.closes.Load())
				return
			}
			if name == "Upgrade" {
				require.ErrorContains(t, call.err, "upstream protocol upgrades are not supported")
				require.Nil(t, call.result.response, "rejected upgrades must not be observed as responses")
				require.EqualValues(t, 1, body.closes.Load(), "rejected upgrades must close the upstream body")
				return
			}
			require.Equal(t, status, call.result.status)
			require.NotNil(t, call.result.response)
			require.EqualValues(t, 1, body.closes.Load(), "forwarding must close the upstream body")
			captured, responseErr := call.result.response.Wait(codertestutil.Context(t, codertestutil.WaitShort))
			switch name {
			case "Success", "BytesWithEOF", "Upstream503":
				require.NoError(t, call.err)
				require.NoError(t, responseErr)
				require.Equal(t, "chunk", string(captured))
				require.Equal(t, "chunk", response.Body.String())
			case "ReadFailure":
				require.ErrorIs(t, responseErr, io.ErrUnexpectedEOF)
			case "Cancellation":
				require.ErrorIs(t, responseErr, context.Canceled)
				require.Equal(t, "chunk", response.Body.String(), "forwarding must deliver the bytes read before the failure")
			}
		})
	}
}

func TestForwardingBodyFailures(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"UnknownLengthOversize", "BodyReadFailure"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var read atomic.Int64
			var source io.Reader
			status := http.StatusBadGateway
			switch name {
			case "UnknownLengthOversize":
				source = readerFunc(func(p []byte) (int, error) { clear(p); read.Add(int64(len(p))); return len(p), nil })
				status = http.StatusRequestEntityTooLarge
			case "BodyReadFailure":
				source = readerFunc(func([]byte) (int, error) { read.Add(1); return 0, io.ErrUnexpectedEOF })
			}
			req := requestWithAuth(t, nil)
			req.Body = io.NopCloser(source)
			req.ContentLength = -1
			h := newTestForwardingHandler(t, provider.NewOpenAI(config.OpenAI{BaseURL: "https://upstream.example.test"}), nil)
			calls := 0
			h.transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				_, err := io.Copy(io.Discard, r.Body)
				return nil, err
			})
			response := httptest.NewRecorder()
			call := mustStartForwarding(t, h, response, req)
			call.forward(response)
			require.Equal(t, status, response.Code)
			require.Equal(t, 1, calls)
			require.Nil(t, call.result.response)
			readBefore := read.Load()
			_, replayErr := call.body.Wait(codertestutil.Context(t, codertestutil.WaitLong))
			require.Equal(t, readBefore, read.Load(), "replay must not read failed input again")
			if name == "UnknownLengthOversize" {
				_, ok := errors.AsType[*http.MaxBytesError](call.err)
				require.True(t, ok, "oversize failures must surface the body limit")
				_, ok = errors.AsType[*http.MaxBytesError](replayErr)
				require.True(t, ok, "replay must surface the body limit")
				require.LessOrEqual(t, read.Load(), int64(routing.MaxRequestBodyBytes+1), "the body limit must stop reading input")
			} else {
				require.ErrorIs(t, call.err, io.ErrUnexpectedEOF)
				require.ErrorIs(t, replayErr, io.ErrUnexpectedEOF)
				require.EqualValues(t, 1, read.Load())
			}
		})
	}
}

// Key failover retries with the next pooled key, and an exhausted pool returns
// its error response.
func TestForwardingKeyOutcomes(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"Rotation", "Exhausted"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newTestForwardingHandler(t, provider.NewOpenAI(config.OpenAI{BaseURL: "https://upstream.example.test", KeyPool: newTestPool(t, "first-key", "second-key")}), nil)
			attempts := 0
			h.transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				attempts++
				_, err := io.Copy(io.Discard, r.Body)
				require.NoError(t, err)
				status := http.StatusOK
				if attempts == 1 || name == "Exhausted" {
					status = http.StatusUnauthorized
				}
				return &http.Response{StatusCode: status, Header: http.Header{}, Body: http.NoBody}, nil
			})
			req := requestWithAuth(t, strings.NewReader("body"))
			req.Header.Del("Authorization")
			response := httptest.NewRecorder()
			call := mustStartForwarding(t, h, response, req)
			call.forward(response)
			require.Equal(t, 2, attempts)
			require.NotNil(t, call.result.response)
			_, err := call.result.response.Wait(codertestutil.Context(t, codertestutil.WaitShort))
			require.NoError(t, err)
			if name == "Exhausted" {
				require.Equal(t, http.StatusBadGateway, response.Code)
				require.Equal(t, http.StatusBadGateway, call.result.status)
				return
			}
			require.Equal(t, http.StatusOK, response.Code)
			require.Equal(t, http.StatusOK, call.result.status)
			require.NoError(t, call.err)
		})
	}
}

// A BYOK upstream rejection is the final outcome, even when the provider has a
// configured pool.
func TestForwardingBYOKOutcomes(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"OpenAI", "AnthropicAPIKey", "Copilot"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pool := newTestPool(t, "pool-secret-key")
			req := requestWithAuth(t, strings.NewReader("body"))
			var prov provider.Provider = provider.NewOpenAI(config.OpenAI{BaseURL: "https://upstream.example.test", KeyPool: pool})
			switch name {
			case "AnthropicAPIKey":
				var err error
				prov, err = provider.NewAnthropic(t.Context(), config.Anthropic{BaseURL: "https://upstream.example.test", KeyPool: pool}, nil, nil)
				require.NoError(t, err)
				req.Header.Del("Authorization")
				req.Header.Set("X-Api-Key", "anthropic-secret-key")
			case "Copilot":
				prov = provider.NewCopilot(config.Copilot{BaseURL: "https://upstream.example.test"})
			}
			req.URL.Path = prov.RoutePrefix() + "/messages"
			h := newTestForwardingHandler(t, prov, nil)
			attempts := 0
			h.transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				attempts++
				return &http.Response{StatusCode: http.StatusUnauthorized, Header: http.Header{}, Body: http.NoBody}, nil
			})
			response := httptest.NewRecorder()
			call := mustStartForwarding(t, h, response, req)
			call.forward(response)
			require.Equal(t, http.StatusUnauthorized, response.Code)
			require.Equal(t, 1, attempts, "BYOK failures must not fall back to pooled credentials")
			require.Equal(t, http.StatusUnauthorized, call.result.status)
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
	calls := make(chan *forwardingCall, 1)
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := aibcontext.AsActor(r.Context(), aibcontext.Actor{ID: uuid.New(), APIKeyID: uuid.NewString()})
		call, ok := startForwarding(h, w, r.WithContext(ctx))
		if !assert.True(t, ok, "request must pass validation") {
			return
		}
		calls <- call
		defer close(forwarded)
		call.forward(w)
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
	require.Equal(t, http.StatusOK, call.result.status)
	require.NotNil(t, call.result.response)
	_, err = call.result.response.Wait(waitCtx)
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
	for attempt := range 2 {
		response := httptest.NewRecorder()
		call := mustStartForwarding(t, h, response, requestWithAuth(t, nil))
		call.forward(response)
		if attempt == 0 {
			require.Equal(t, http.StatusServiceUnavailable, call.result.status)
			require.NoError(t, call.err)
			replay, err := call.request.GetBody()
			require.NoError(t, err)
			require.NotNil(t, call.body)
			require.Equal(t, http.NoBody, replay, "known empty bodies must not become chunked requests")
		} else {
			require.Equal(t, http.StatusServiceUnavailable, response.Code)
			require.ErrorIs(t, call.err, circuitbreaker.ErrCircuitOpen)
			require.Nil(t, call.result.response)
			require.Nil(t, call.body, "open circuit must skip request preparation")
		}
	}
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
