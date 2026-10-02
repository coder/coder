package proxy

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/circuitbreaker"
	"github.com/coder/coder/v2/aibridge/config"
	aibcontext "github.com/coder/coder/v2/aibridge/context"
	"github.com/coder/coder/v2/aibridge/credential"
	"github.com/coder/coder/v2/aibridge/intercept/apidump"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/metrics"
	"github.com/coder/coder/v2/aibridge/provider"
	"github.com/coder/coder/v2/aibridge/recorder"
	"github.com/coder/coder/v2/aibridge/routing"
	"github.com/coder/coder/v2/aibridge/utils"
	codertestutil "github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type readerFunc func([]byte) (int, error)

func (f readerFunc) Read(p []byte) (int, error) { return f(p) }

type trackedBody struct {
	io.Reader
	closes, eofs int
}

func (b *trackedBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	if err == io.EOF {
		b.eofs++
	}
	return n, err
}
func (b *trackedBody) Close() error { b.closes++; return nil }

type failingWriter struct{ http.ResponseWriter }

func (*failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
func (w *failingWriter) Flush()                  { _ = http.NewResponseController(w.ResponseWriter).Flush() }
func (w *failingWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return http.NewResponseController(w.ResponseWriter).Hijack()
}

type failingFlushWriter struct{ http.ResponseWriter }

func (*failingFlushWriter) FlushError() error { return io.ErrClosedPipe }
func (w *failingFlushWriter) Flush()          { _ = w.FlushError() }
func (w *failingFlushWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return http.NewResponseController(w.ResponseWriter).Hijack()
}

func newTestForwardingHandler(t *testing.T, prov provider.Provider, m *metrics.Metrics) *forwardingHandler {
	t.Helper()
	gate := aibridge.NewInflightGate(codertestutil.NewFakeSink(t).Logger())
	h, transport, err := newForwardingHandler(prov, codertestutil.NewFakeSink(t).Logger(), m, noop.NewTracerProvider().Tracer(t.Name()), gate, &struct{ recorder.Recorder }{})
	require.NoError(t, err)
	require.NotNil(t, h.proxy)
	t.Cleanup(transport.CloseIdleConnections)
	t.Cleanup(func() { require.NoError(t, gate.Shutdown(context.Background())); gate.Close() })
	return h
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
	route := strings.TrimPrefix(c.request.Pattern, "/"+c.handler.provider.Name())
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
	for _, name := range []string{"Success", "BytesWithEOF", "TransportFailure", "Upstream503", "ReadFailure", "WriteFailure", "FlushFailure", "Panic", "Cancellation", "PoolFailure", "CircuitOpen"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			body := &trackedBody{Reader: strings.NewReader("chunk")}
			input := &trackedBody{Reader: strings.NewReader("not JSON")}
			req := recordedRequest(t, input)
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
			case "ReadFailure":
				body.Reader = io.MultiReader(body.Reader, readerFunc(func([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }))
				wantPanic = http.ErrAbortHandler
			case "WriteFailure":
				writer = &failingWriter{response}
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
			req := recordedRequest(t, nil)
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

func TestForwardingWireAndFailover(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"BYOKWithAPIDump", "Pool", "ExhaustedPool"} {
		pooled := name != "BYOKWithAPIDump"
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var compressed bytes.Buffer
			gz := gzip.NewWriter(&compressed)
			_, err := gz.Write([]byte("raw response"))
			require.NoError(t, err)
			require.NoError(t, gz.Close())
			keys := make(chan string, 2)
			var attempts atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				keys <- r.Header.Get("Authorization")
				attempt := attempts.Add(1)
				payload, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				assert.Equal(t, "  not JSON\x00\n", string(payload))
				assert.EqualValues(t, -1, r.ContentLength)
				assert.Equal(t, "/base/models/a%2Fb", r.URL.EscapedPath())
				assert.Equal(t, "configured=1&raw=a;b", r.URL.RawQuery)
				if pooled {
					assert.Equal(t, "claude-code/1.0", r.UserAgent())
					assert.Equal(t, "gzip", r.Header.Get("Accept-Encoding"))
				} else {
					assert.Empty(t, r.UserAgent())
					assert.Empty(t, r.Header.Get("Accept-Encoding"))
				}
				if pooled && (attempt == 1 || name == "ExhaustedPool") {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				w.Header().Set("Content-Encoding", "gzip")
				_, _ = w.Write(compressed.Bytes())
			}))
			t.Cleanup(upstream.Close)
			var pool *keypool.Pool
			if pooled {
				pool, err = keypool.New("openai", []string{"first-secret-key", "second-secret-key"}, quartz.NewMock(t), nil)
				require.NoError(t, err)
			}
			dumpDir := ""
			if !pooled {
				dumpDir = t.TempDir()
			}
			h := newTestForwardingHandler(t, provider.NewOpenAI(config.OpenAI{BaseURL: upstream.URL + "/base?configured=1", KeyPool: pool, APIDumpDir: dumpDir}), nil)
			body := &trackedBody{Reader: strings.NewReader("  not JSON\x00\n")}
			req := recordedRequest(t, body)
			req.URL.Path = "/openai/v1/models/a/b"
			req.URL.RawPath = "/openai/v1/models/a%2Fb"
			req.URL.RawQuery = "raw=a;b"
			if pooled {
				req.Header.Del("Authorization")
				req.Header.Set("User-Agent", "claude-code/1.0")
				req.Header.Set("Accept-Encoding", "gzip")
			}
			original := req.Header.Clone()
			response := httptest.NewRecorder()
			call := mustStartForwarding(t, h, response, req)
			call.forward(response)
			if name == "ExhaustedPool" {
				require.Equal(t, http.StatusBadGateway, response.Code)
			} else {
				require.Equal(t, http.StatusOK, response.Code)
				require.Equal(t, compressed.Bytes(), response.Body.Bytes())
				require.Equal(t, "gzip", response.Header().Get("Content-Encoding"))
			}
			require.Equal(t, original, req.Header)
			require.Equal(t, 1, body.eofs)
			require.Equal(t, 1, body.closes)
			replayed, err := call.replay(t)
			require.NoError(t, err)
			require.Equal(t, "  not JSON\x00\n", string(replayed))
			if name == "ExhaustedPool" {
				_, ok := errors.AsType[*keypool.Error](call.state.err)
				require.True(t, ok, "pool exhaustion must surface the key pool error")
			} else {
				require.NoError(t, call.state.err)
			}
			if dumpDir != "" {
				for _, suffix := range []string{apidump.SuffixRequest, apidump.SuffixResponse} {
					files, err := filepath.Glob(filepath.Join(dumpDir, "openai", "passthrough", "*"+suffix))
					require.NoError(t, err)
					require.Len(t, files, 1)
					data, err := os.ReadFile(files[0])
					require.NoError(t, err)
					require.NotContains(t, string(data), "user-secret-key")
					if suffix == apidump.SuffixRequest {
						require.Contains(t, string(data), "Authorization: "+utils.MaskSecret("Bearer user-secret-key"))
						require.Contains(t, string(data), "  not JSON\x00\n")
					} else {
						require.Contains(t, string(data), "HTTP/1.1 200 OK")
					}
				}
			}
			if pooled {
				require.Equal(t, "Bearer first-secret-key", codertestutil.RequireReceive(codertestutil.Context(t, codertestutil.WaitLong), t, keys))
				require.Equal(t, "Bearer second-secret-key", codertestutil.RequireReceive(codertestutil.Context(t, codertestutil.WaitLong), t, keys))
				require.Equal(t, utils.MaskSecret("second-secret-key"), call.state.credentialHint)
			} else {
				require.Equal(t, "Bearer user-secret-key", codertestutil.RequireReceive(codertestutil.Context(t, codertestutil.WaitLong), t, keys))
				require.Equal(t, utils.MaskSecret("user-secret-key"), call.state.credentialHint)
			}
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

func TestForwardingStreamedRetry(t *testing.T) {
	t.Parallel()
	ctx := codertestutil.Context(t, codertestutil.WaitLong)
	input, output := io.Pipe()
	defer input.Close()
	defer output.Close()
	const prefix = `{"metadata":{"user_id":"user_hash_account_id_session_`
	const suffix = `session-one"}}`
	pool, err := keypool.New("openai", []string{"first-key", "second-key"}, quartz.NewMock(t), nil)
	require.NoError(t, err)
	h := newTestForwardingHandler(t, provider.NewOpenAI(config.OpenAI{BaseURL: "https://upstream.example.test", KeyPool: pool}), nil)
	readPrefix := make(chan struct{}, 1)
	attempts := 0
	h.transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		defer r.Body.Close()
		attempts++
		if attempts == 1 {
			chunk := make([]byte, len(prefix))
			_, err := io.ReadFull(r.Body, chunk)
			assert.NoError(t, err)
			assert.Equal(t, prefix, string(chunk))
			readPrefix <- struct{}{}
			return &http.Response{StatusCode: http.StatusUnauthorized, Header: http.Header{}, Body: http.NoBody}, nil
		}
		payload, err := io.ReadAll(r.Body)
		if !assert.NoError(t, err) {
			return nil, err
		}
		assert.Equal(t, prefix+suffix, string(payload), "the retry must replay the prefix and read the remaining input")
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: http.NoBody}, nil
	})
	req := recordedRequest(t, input)
	req.Header.Del("Authorization")
	req.Header.Set("User-Agent", "claude-code/1.0")
	response := httptest.NewRecorder()
	call := mustStartForwarding(t, h, response, req)
	served := make(chan struct{})
	go func() {
		defer close(served)
		call.forward(response)
	}()
	_, err = io.WriteString(output, prefix)
	require.NoError(t, err)
	// The first attempt must forward the prefix before the input is complete.
	codertestutil.RequireReceive(ctx, t, readPrefix)
	_, err = io.WriteString(output, suffix)
	require.NoError(t, err)
	require.NoError(t, output.Close())
	codertestutil.TryReceive(ctx, t, served)
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, 2, attempts)
	require.NoError(t, call.state.err)
	require.Equal(t, utils.MaskSecret("second-key"), call.state.credentialHint)
	replayed, err := call.replay(t)
	require.NoError(t, err)
	require.Equal(t, prefix+suffix, string(replayed))
}

func TestRequestBufferReaders(t *testing.T) {
	t.Parallel()
	for _, terminal := range []error{io.EOF, io.ErrClosedPipe} {
		t.Run(terminal.Error(), func(t *testing.T) {
			t.Parallel()
			ctx := codertestutil.Context(t, codertestutil.WaitLong)
			reading, release := make(chan struct{}), make(chan struct{})
			defer close(release)
			calls := 0
			b := &requestBuffer{source: readerFunc(func(p []byte) (int, error) {
				calls++
				if calls == 1 {
					return copy(p, "prefix"), nil
				}
				close(reading)
				<-release
				return copy(p, "suffix"), terminal
			})}
			first, err := b.getBody()
			require.NoError(t, err)
			defer first.Close()
			prefix := make([]byte, len("prefix"))
			_, err = io.ReadFull(first, prefix)
			require.NoError(t, err)
			assertComplete := func(t assert.TestingT, payload []byte, err error) {
				assert.Equal(t, "prefixsuffix", string(payload))
				if errors.Is(terminal, io.EOF) {
					assert.NoError(t, err)
				} else {
					assert.ErrorIs(t, err, terminal)
				}
			}
			blocked, err := b.getBody()
			require.NoError(t, err)
			defer blocked.Close()
			done := make(chan struct{}, 2)
			go func() {
				payload, err := io.ReadAll(blocked)
				assertComplete(t, payload, err)
				done <- struct{}{}
			}()
			codertestutil.TryReceive(ctx, t, reading)
			second, err := b.getBody()
			require.NoError(t, err)
			defer second.Close()
			go func() {
				_, err := io.ReadFull(second, prefix)
				assert.NoError(t, err)
				assert.Equal(t, "prefix", string(prefix))
				done <- struct{}{}
			}()
			// A cached prefix must remain readable while another reader is
			// blocked waiting for input at the buffered end.
			codertestutil.RequireReceive(ctx, t, done)
			release <- struct{}{}
			payload, err := io.ReadAll(second)
			require.Equal(t, "suffix", string(payload))
			if errors.Is(terminal, io.EOF) {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, terminal)
			}
			codertestutil.RequireReceive(ctx, t, done)
			replay, err := b.getBody()
			require.NoError(t, err)
			defer replay.Close()
			payload, err = io.ReadAll(replay)
			assertComplete(t, payload, err)
			require.Equal(t, 2, calls, "input bytes must be read from the source only once")
		})
	}
}

func TestForwardingProviderCredentials(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"OpenAIConnectionAuth", "AnthropicAPIKey", "CopilotBearer"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pool, err := keypool.New("test", []string{"pool-secret-key"}, quartz.NewMock(t), nil)
			require.NoError(t, err)
			var prov provider.Provider = provider.NewOpenAI(config.OpenAI{BaseURL: "https://upstream.example.test", KeyPool: pool})
			req := recordedRequest(t, strings.NewReader("body"))
			wantAuth := http.Header{"Authorization": {"Bearer user-secret-key"}}
			wantHint := utils.MaskSecret("user-secret-key")
			switch name {
			case "OpenAIConnectionAuth":
				req.Header.Set("Connection", "Authorization")
			case "AnthropicAPIKey":
				prov, err = provider.NewAnthropic(t.Context(), config.Anthropic{BaseURL: "https://upstream.example.test", KeyPool: pool}, nil, nil)
				require.NoError(t, err)
				req.Header.Set("X-Api-Key", "anthropic-secret-key")
				wantAuth = http.Header{"X-Api-Key": {"anthropic-secret-key"}}
				wantHint = utils.MaskSecret("anthropic-secret-key")
			case "CopilotBearer":
				prov = provider.NewCopilot(config.Copilot{BaseURL: "https://upstream.example.test"})
			}
			h := newTestForwardingHandler(t, prov, nil)
			calls := 0
			h.transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				for _, header := range []string{"Authorization", "X-Api-Key"} {
					require.Equal(t, wantAuth.Values(header), r.Header.Values(header))
				}
				return &http.Response{StatusCode: http.StatusUnauthorized, Header: http.Header{}, Body: http.NoBody}, nil
			})
			response := httptest.NewRecorder()
			call := mustStartForwarding(t, h, response, req)
			call.forward(response)
			require.Equal(t, http.StatusUnauthorized, response.Code)
			require.Equal(t, 1, calls, "BYOK failures must not fall back to pooled credentials")
			require.Equal(t, http.StatusUnauthorized, call.state.status)
			require.NoError(t, call.state.err)
			require.Equal(t, wantHint, call.state.credentialHint)
		})
	}
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
		call := mustStartForwarding(t, h, response, recordedRequest(t, nil))
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

func TestRouterBridgedTransportReuse(t *testing.T) {
	t.Parallel()
	remotes := make(chan string, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.Copy(io.Discard, r.Body)
		assert.NoError(t, err)
		remotes <- r.RemoteAddr
		_, _ = io.WriteString(w, "ok")
	}))
	t.Cleanup(upstream.Close)
	logger := codertestutil.NewFakeSink(t).Logger()
	gate := aibridge.NewInflightGate(logger)
	router, err := NewRouter([]provider.Provider{provider.NewOpenAI(config.OpenAI{BaseURL: upstream.URL})}, logger, nil, noop.NewTracerProvider().Tracer(t.Name()), gate, &struct{ recorder.Recorder }{})
	require.NoError(t, err)
	t.Cleanup(router.CloseIdleConnections)
	t.Cleanup(func() { require.NoError(t, gate.Shutdown(context.Background())); gate.Close() })

	ctx := codertestutil.Context(t, codertestutil.WaitLong)
	var shared *forwardingHandler
	// The public bridged handler is dormant, so drive the routed handler's
	// forwarding seam directly.
	send := func(path string) string {
		t.Helper()
		req := recordedRequest(t, strings.NewReader("body"))
		req.URL.Path = path
		handler, pattern := router.mux.Handler(req)
		req.Pattern = pattern
		h, ok := handler.(*forwardingHandler)
		require.True(t, ok, "%s must route to bridged forwarding", path)
		if shared == nil {
			shared = h
		}
		require.Same(t, shared, h, "bridged routes must share the provider's forwarding handler")
		response := httptest.NewRecorder()
		mustStartForwarding(t, h, response, req).forward(response)
		require.Equal(t, http.StatusOK, response.Code)
		require.Equal(t, "ok", response.Body.String())
		return codertestutil.RequireReceive(ctx, t, remotes)
	}
	first := send("/openai/v1/chat/completions")
	require.Equal(t, first, send("/openai/v1/responses"), "bridged routes must reuse the provider connection")
	router.CloseIdleConnections()
	require.NotEqual(t, first, send("/openai/v1/chat/completions"), "closing idle connections must force a new connection")
}
