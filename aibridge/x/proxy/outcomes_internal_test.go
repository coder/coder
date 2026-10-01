package proxy

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
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

	"github.com/coder/coder/v2/aibridge/circuitbreaker"
	"github.com/coder/coder/v2/aibridge/config"
	aibcontext "github.com/coder/coder/v2/aibridge/context"
	"github.com/coder/coder/v2/aibridge/credential"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/metrics"
	"github.com/coder/coder/v2/aibridge/provider"
	"github.com/coder/coder/v2/aibridge/routing"
	"github.com/coder/coder/v2/aibridge/utils"
	codertestutil "github.com/coder/coder/v2/testutil"
)

type failingWriter struct{ http.ResponseWriter }

func (*failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
func (w *failingWriter) Flush()                  { _ = http.NewResponseController(w.ResponseWriter).Flush() }
func (w *failingWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return http.NewResponseController(w.ResponseWriter).Hijack()
}

// shortWriter delivers only part of each write without reporting an error.
type shortWriter struct{ failingWriter }

func (w *shortWriter) Write(p []byte) (int, error) { return w.ResponseWriter.Write(p[:len(p)/2]) }

type failingFlushWriter struct{ http.ResponseWriter }

func (*failingFlushWriter) FlushError() error { return io.ErrClosedPipe }
func (w *failingFlushWriter) Flush()          { _ = w.FlushError() }
func (w *failingFlushWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return http.NewResponseController(w.ResponseWriter).Hijack()
}

// forwardingCall is a request prepared for the forwarding seam.
type forwardingCall struct {
	handler  *forwardingHandler
	request  *http.Request
	body     *requestBuffer
	cred     credential.Credential
	state    *responseObservation
	cleanups []func()
}

// startForwarding performs the bridged handler's setup around the forwarding
// seam: server body limiting, inflight admission, request validation, and
// observation installation. It reports false after writing a rejection. The
// validation recorder fails loudly if anything tries to record through it.
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
	record, cred := h.checkRequest(w, r)
	if record == nil {
		call.finish()
		return nil, false
	}
	call.state = &responseObservation{credentialHint: record.CredentialHint, client: w}
	call.cred = cred
	call.request = r.WithContext(context.WithValue(ctx, observationContextKey{}, call.state))
	return call, true
}

// mustStartForwarding is startForwarding for the test goroutine.
func mustStartForwarding(t *testing.T, h *forwardingHandler, w http.ResponseWriter, r *http.Request) *forwardingCall {
	t.Helper()
	call, ok := startForwarding(h, w, r)
	require.True(t, ok, "request must pass validation")
	return call
}

// forward invokes the forwarding seam and then releases the request's
// admission and body, including when forwarding aborts.
func (c *forwardingCall) forward(w http.ResponseWriter) {
	defer c.finish()
	c.state.client = w
	route := strings.TrimPrefix(c.request.URL.Path, "/"+c.handler.provider.Name())
	c.state.err = c.handler.breaker.Execute(route, "", w, func(rw http.ResponseWriter) error {
		outbound, body := c.handler.prepareForwarding(c.request, c.cred)
		c.body = body
		return c.handler.forwardPrepared(rw, outbound, body, c.state)
	})
}

func (c *forwardingCall) finish() {
	for i := len(c.cleanups) - 1; i >= 0; i-- {
		c.cleanups[i]()
	}
	c.cleanups = nil
}

// replay reads the prepared request body from the start through its replay
// reader. After forwarding reaches the source's terminal state, replay reads
// only buffered input and the recorded terminal error.
func (c *forwardingCall) replay(t *testing.T) ([]byte, error) {
	t.Helper()
	require.NotNil(t, c.body, "forwarding must prepare the request body")
	reader, err := c.body.getBody()
	require.NoError(t, err)
	defer reader.Close()
	return io.ReadAll(reader)
}

func TestForwardingTransportOutcomes(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"Success", "BytesWithEOF", "TransportFailure", "Upstream503", "Upgrade", "ReadFailure", "WriteFailure", "ShortWrite", "FlushFailure", "Panic", "Cancellation", "PoolFailure", "CircuitOpen"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			body := &trackedBody{Reader: strings.NewReader("chunk")}
			input := &trackedBody{Reader: strings.NewReader("not JSON")}
			req := requestWithAuth(t, input)
			ctx, cancel := context.WithCancel(req.Context())
			defer cancel()
			req = req.WithContext(ctx)
			response := httptest.NewRecorder()
			var writer http.ResponseWriter = response
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
				wantPanic = http.ErrAbortHandler
			case "WriteFailure":
				writer = &failingWriter{response}
				wantPanic = http.ErrAbortHandler
			case "ShortWrite":
				writer = &shortWriter{failingWriter{response}}
				wantPanic = http.ErrAbortHandler
			case "FlushFailure":
				writer = &failingFlushWriter{response}
				wantPanic = http.ErrAbortHandler
			case "Panic":
				body.Reader = readerFunc(func([]byte) (int, error) { panic("copy panic") })
				wantPanic = "copy panic"
			case "Cancellation":
				body.Reader = io.MultiReader(body.Reader, readerFunc(func([]byte) (int, error) { cancel(); return 0, context.Canceled }))
				wantPanic = http.ErrAbortHandler
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
				require.Zero(t, input.eofs, "forwarding must not read the request body eagerly")
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
			call := mustStartForwarding(t, h, writer, req)
			if wantPanic != nil {
				require.PanicsWithValue(t, wantPanic, func() { call.forward(writer) })
			} else {
				call.forward(writer)
				require.Equal(t, wantStatus, response.Code)
			}
			require.Equal(t, 1, calls)
			require.Equal(t, 1, input.closes)
			require.Equal(t, 1, input.eofs)
			replayed, err := call.replay(t)
			require.NoError(t, err)
			require.Equal(t, "not JSON", string(replayed))
			require.Equal(t, 1, input.eofs, "replay must not read completed input again")
			state := call.state
			require.Equal(t, utils.MaskSecret("user-secret-key"), state.credentialHint)
			if transportErr != nil {
				require.ErrorIs(t, state.err, transportErr)
				require.Nil(t, state.body)
				require.Zero(t, body.closes)
				return
			}
			if name == "Upgrade" {
				require.ErrorContains(t, state.err, "upstream protocol upgrades are not supported")
				require.Nil(t, state.body, "rejected upgrades must not be observed as responses")
				require.Equal(t, 1, body.closes, "rejected upgrades must close the upstream body")
				return
			}
			require.Equal(t, status, state.status)
			require.NotNil(t, state.body)
			require.True(t, state.body.closed)
			require.Equal(t, 1, body.closes)
			switch name {
			case "Success", "BytesWithEOF", "Upstream503":
				require.NoError(t, state.err)
				require.True(t, state.body.eof)
				require.NoError(t, state.body.readErr)
				require.Equal(t, "chunk", response.Body.String())
			case "ReadFailure":
				require.False(t, state.body.eof)
				require.ErrorIs(t, state.body.readErr, io.ErrUnexpectedEOF)
			case "WriteFailure", "FlushFailure":
				require.ErrorIs(t, state.err, io.ErrClosedPipe)
			case "ShortWrite":
				require.ErrorIs(t, state.err, io.ErrShortWrite)
				require.Equal(t, "ch", response.Body.String())
			case "Panic":
				require.False(t, state.body.eof)
			case "Cancellation":
				require.False(t, state.body.eof)
				require.ErrorIs(t, state.body.readErr, context.Canceled)
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
				require.Zero(t, read.Load(), "forwarding must not read the request body eagerly")
				_, err := io.Copy(io.Discard, r.Body)
				return nil, err
			})
			response := httptest.NewRecorder()
			call := mustStartForwarding(t, h, response, req)
			call.forward(response)
			require.Equal(t, status, response.Code)
			require.Equal(t, 1, calls)
			require.Nil(t, call.state.body)
			readBefore := read.Load()
			replay, err := call.body.getBody()
			require.NoError(t, err)
			_, replayErr := io.Copy(io.Discard, replay)
			require.Equal(t, readBefore, read.Load(), "replay must not read failed input again")
			if name == "UnknownLengthOversize" {
				_, ok := errors.AsType[*http.MaxBytesError](call.state.err)
				require.True(t, ok, "oversize failures must surface the body limit")
				_, ok = errors.AsType[*http.MaxBytesError](replayErr)
				require.True(t, ok, "replay must surface the body limit")
				require.LessOrEqual(t, read.Load(), int64(routing.MaxRequestBodyBytes+1), "the body limit must stop reading input")
			} else {
				require.ErrorIs(t, call.state.err, io.ErrUnexpectedEOF)
				require.ErrorIs(t, replayErr, io.ErrUnexpectedEOF)
				require.EqualValues(t, 1, read.Load())
			}
		})
	}
}

// The observation reports the pooled key that served the final response, and
// pool exhaustion surfaces the key pool error.
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
			require.Equal(t, utils.MaskSecret("second-key"), call.state.credentialHint, "the hint must name the last key used")
			require.NotNil(t, call.state.body)
			require.True(t, call.state.body.eof)
			if name == "Exhausted" {
				require.Equal(t, http.StatusBadGateway, response.Code)
				require.Equal(t, http.StatusBadGateway, call.state.status)
				_, ok := errors.AsType[*keypool.Error](call.state.err)
				require.True(t, ok, "pool exhaustion must surface the key pool error")
				return
			}
			require.Equal(t, http.StatusOK, response.Code)
			require.Equal(t, http.StatusOK, call.state.status)
			require.NoError(t, call.state.err)
		})
	}
}

// A BYOK upstream rejection is the final outcome, observed with the client
// credential's hint even when the provider has a configured pool.
func TestForwardingBYOKOutcomes(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"OpenAI", "AnthropicAPIKey", "Copilot"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pool := newTestPool(t, "pool-secret-key")
			req := requestWithAuth(t, strings.NewReader("body"))
			var prov provider.Provider = provider.NewOpenAI(config.OpenAI{BaseURL: "https://upstream.example.test", KeyPool: pool})
			wantHint := utils.MaskSecret("user-secret-key")
			switch name {
			case "AnthropicAPIKey":
				var err error
				prov, err = provider.NewAnthropic(t.Context(), config.Anthropic{BaseURL: "https://upstream.example.test", KeyPool: pool}, nil, nil)
				require.NoError(t, err)
				req.Header.Del("Authorization")
				req.Header.Set("X-Api-Key", "anthropic-secret-key")
				wantHint = utils.MaskSecret("anthropic-secret-key")
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
			require.Equal(t, http.StatusUnauthorized, call.state.status)
			require.NoError(t, call.state.err)
			require.Equal(t, wantHint, call.state.credentialHint)
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
	req.Header.Set("Authorization", "Bearer user-secret-key")
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
	require.Equal(t, http.StatusOK, call.state.status)
	require.NotNil(t, call.state.body)
	require.False(t, call.state.body.eof, "a canceled stream must not look complete")
	require.True(t, call.state.body.closed)
}

func TestForwardingCircuitBreaker(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusEarlyHints)
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
			// ResponseRecorder reports the first informational status as Code.
			require.Equal(t, http.StatusServiceUnavailable, call.state.status)
			require.NoError(t, call.state.err)
			replay, err := call.body.getBody()
			require.NoError(t, err)
			require.Equal(t, http.NoBody, replay, "known empty bodies must not become chunked requests")
		} else {
			require.Equal(t, http.StatusServiceUnavailable, response.Code)
			require.ErrorIs(t, call.state.err, circuitbreaker.ErrCircuitOpen)
			require.Nil(t, call.state.body)
			require.Nil(t, call.body, "open circuit must skip request preparation")
		}
	}
	require.EqualValues(t, 1, calls.Load(), "the final 503 must trip the breaker despite informational responses")
}
