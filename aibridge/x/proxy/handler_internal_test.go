package proxy

import (
	"context"
	"fmt"
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
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/circuitbreaker"
	"github.com/coder/coder/v2/aibridge/client"
	"github.com/coder/coder/v2/aibridge/config"
	aibcontext "github.com/coder/coder/v2/aibridge/context"
	"github.com/coder/coder/v2/aibridge/credential"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/metrics"
	"github.com/coder/coder/v2/aibridge/provider"
	"github.com/coder/coder/v2/aibridge/recorder"
	"github.com/coder/coder/v2/aibridge/routing"
	"github.com/coder/coder/v2/aibridge/utils"
	codertestutil "github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// lifecycleRecorder records starts synchronously and ends from the handler's
// async recorder, which ServeHTTP joins before returning. Embedding a nil
// Recorder makes any unexpected content recording fail loudly.
type lifecycleRecorder struct {
	recorder.Recorder
	starts           []recorder.InterceptionRecord
	ends             []recorder.InterceptionRecordEnded
	startErr, endErr error
	onEnd            func(context.Context)
	// startReturned is set as RecordInterception returns, so forwarding can
	// assert that the start record completed before any upstream attempt.
	startReturned atomic.Bool
}

func (r *lifecycleRecorder) RecordInterception(_ context.Context, record *recorder.InterceptionRecord) error {
	r.starts = append(r.starts, *record)
	r.startReturned.Store(true)
	return r.startErr
}

func (r *lifecycleRecorder) RecordInterceptionEnded(ctx context.Context, record *recorder.InterceptionRecordEnded) error {
	r.ends = append(r.ends, *record)
	if r.onEnd != nil {
		r.onEnd(ctx)
	}
	return r.endErr
}

// serveForwardingRequest applies the router's body limit before serving.
func serveForwardingRequest(h *forwardingHandler, w http.ResponseWriter, r *http.Request, rec recorder.Recorder) {
	if r.Body != nil && r.Body != http.NoBody {
		r.Body = http.MaxBytesReader(w, r.Body, routing.MaxRequestBodyBytes)
		defer r.Body.Close()
	}
	handler := *h
	handler.recorder = rec
	handler.ServeHTTP(w, r)
}

func requestWithAuth(t *testing.T, body io.Reader) *http.Request {
	t.Helper()
	ctx := aibcontext.AsActor(t.Context(), aibcontext.Actor{
		ID:       uuid.New(),
		APIKeyID: uuid.NewString(),
		Username: t.Name(),
		Email:    "actor@example.test",
	})
	req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", body).WithContext(ctx)
	req.Pattern = "/openai/v1/chat/completions"
	req.Header.Set("Authorization", "Bearer user-secret-key")
	return req
}

func TestForwardingHandlerRejectsRequest(t *testing.T) {
	t.Parallel()
	resolutionErr := xerrors.New("credential store unavailable")
	for _, tc := range []struct {
		name           string
		prepare        func(*testing.T, *http.Request) *http.Request
		provider       func(*testing.T) provider.Provider
		noRecorder     bool
		status         int
		wantBody       string
		wantLogLevel   slog.Level
		wantLogMessage string
		wantLogError   error
	}{
		{
			name: "WebSocket",
			prepare: func(_ *testing.T, r *http.Request) *http.Request {
				r.Method = http.MethodGet
				r.Header.Set("Connection", "Upgrade")
				r.Header.Set("Upgrade", "websocket")
				return r
			},
			status:         http.StatusNotImplemented,
			wantBody:       "WebSocket transport is not supported, use HTTP\n",
			wantLogLevel:   slog.LevelDebug,
			wantLogMessage: "rejecting unsupported WebSocket upgrade",
		},
		{
			name: "MalformedFirewallSessionID",
			prepare: func(_ *testing.T, r *http.Request) *http.Request {
				r.Header.Set("X-Coder-Agent-Firewall-Session-Id", "not-a-uuid")
				r.Header.Set("X-Coder-Agent-Firewall-Sequence-Number", "1")
				return r
			},
			status:         http.StatusBadRequest,
			wantBody:       "invalid agent firewall headers\n",
			wantLogLevel:   slog.LevelWarn,
			wantLogMessage: "rejecting request with invalid agent firewall headers",
		},
		{
			name: "FirewallMissingSequence",
			prepare: func(_ *testing.T, r *http.Request) *http.Request {
				r.Header.Set("X-Coder-Agent-Firewall-Session-Id", "e5f6a7b8-1234-5678-9abc-def012345678")
				return r
			},
			status:         http.StatusBadRequest,
			wantBody:       "invalid agent firewall headers\n",
			wantLogLevel:   slog.LevelWarn,
			wantLogMessage: "rejecting request with invalid agent firewall headers",
		},
		{
			name: "DeclaredOversize",
			prepare: func(_ *testing.T, r *http.Request) *http.Request {
				r.ContentLength = routing.MaxRequestBodyBytes + 1
				return r
			},
			status:         http.StatusRequestEntityTooLarge,
			wantBody:       fmt.Sprintf("Request body too large. The maximum allowed request body size is %dMiB.\n", routing.MaxRequestBodyBytes>>20),
			wantLogLevel:   slog.LevelDebug,
			wantLogMessage: "rejecting oversized request body",
		},
		{
			name:           "MissingActor",
			prepare:        func(t *testing.T, r *http.Request) *http.Request { return r.WithContext(t.Context()) },
			status:         http.StatusBadRequest,
			wantBody:       "no actor found\n",
			wantLogLevel:   slog.LevelWarn,
			wantLogMessage: "rejecting request without an actor",
		},
		{
			name:           "MissingRecorder",
			noRecorder:     true,
			status:         http.StatusInternalServerError,
			wantBody:       "recorder unavailable\n",
			wantLogLevel:   slog.LevelWarn,
			wantLogMessage: "rejecting request without a recorder",
		},
		{
			name: "MissingCredential",
			prepare: func(_ *testing.T, r *http.Request) *http.Request {
				r.Header.Del("Authorization")
				return r
			},
			status:         http.StatusForbidden,
			wantBody:       "upstream authentication unavailable: no provider credentials supplied or configured\n",
			wantLogLevel:   slog.LevelWarn,
			wantLogMessage: "failed to resolve credential",
		},
		{
			name: "MissingCopilotCredential",
			prepare: func(_ *testing.T, r *http.Request) *http.Request {
				r.Header.Del("Authorization")
				return r
			},
			provider: func(*testing.T) provider.Provider {
				return provider.NewCopilot(config.Copilot{BaseURL: "https://upstream.example.test"})
			},
			status:         http.StatusForbidden,
			wantBody:       "upstream authentication unavailable: no provider credentials supplied or configured\n",
			wantLogLevel:   slog.LevelWarn,
			wantLogMessage: "failed to resolve credential",
		},
		{
			name: "CredentialResolutionError",
			provider: func(*testing.T) provider.Provider {
				return credentialErrorProvider{
					Provider: provider.NewOpenAI(config.OpenAI{BaseURL: "https://upstream.example.test"}),
					err:      resolutionErr,
				}
			},
			status:         http.StatusInternalServerError,
			wantBody:       "upstream authentication unavailable\n",
			wantLogLevel:   slog.LevelWarn,
			wantLogMessage: "failed to resolve credential",
			wantLogError:   resolutionErr,
		},
		{
			name: "UnsupportedSigning",
			prepare: func(_ *testing.T, r *http.Request) *http.Request {
				r.Header.Del("Authorization")
				return r
			},
			provider: func(t *testing.T) provider.Provider {
				prov, err := provider.NewAnthropic(t.Context(), config.Anthropic{BaseURL: "https://upstream.example.test"}, nil, &config.AWSClaudePlatform{Region: "us-west-2", WorkspaceID: "wrkspc_test"})
				require.NoError(t, err)
				return prov
			},
			status:         http.StatusNotImplemented,
			wantBody:       "upstream authentication is not supported in proxy mode\n",
			wantLogLevel:   slog.LevelWarn,
			wantLogMessage: "rejecting unsupported upstream credential",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var rec recorder.Recorder = &struct{ recorder.Recorder }{}
			if tc.noRecorder {
				rec = nil
			}
			body := &unreadBody{}
			req := requestWithAuth(t, body)
			if tc.prepare != nil {
				req = tc.prepare(t, req)
			}
			var prov provider.Provider = provider.NewOpenAI(config.OpenAI{BaseURL: "https://upstream.example.test"})
			if tc.provider != nil {
				prov = tc.provider(t)
			}
			sink := codertestutil.NewFakeSink(t)
			h := &forwardingHandler{provider: prov, logger: sink.Logger(), recorder: rec}
			response := httptest.NewRecorder()
			record, cred := h.checkRequest(response, req)
			require.Nil(t, record)
			require.Nil(t, cred)
			require.Equal(t, tc.status, response.Code)
			require.Equal(t, tc.wantBody, response.Body.String())
			require.False(t, body.read, "preflight must not read the request body")
			require.False(t, body.closed, "the transport owns request-body cleanup")
			entries := sink.Entries()
			require.Len(t, entries, 1)
			require.Equal(t, tc.wantLogLevel, entries[0].Level)
			require.Equal(t, tc.wantLogMessage, entries[0].Message)
			require.Contains(t, entries[0].Fields, slog.F("provider", prov.Name()))
			if tc.wantLogError != nil {
				require.Contains(t, entries[0].Fields, slog.Error(tc.wantLogError))
				require.Contains(t, entries[0].Fields, slog.F("path", req.URL.Path))
			}
			require.NotContains(t, fmt.Sprint(entries), "user-secret-key", "rejection logs must not include credentials")
		})
	}
}

type credentialErrorProvider struct {
	provider.Provider
	err error
}

func (p credentialErrorProvider) ResolveCredential(*http.Request) (credential.Credential, error) {
	return nil, p.err
}

func TestForwardingHandlerInterceptionRecord(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		useKeyPool bool
		wantKind   credential.Kind
		wantHint   string
	}{
		{
			name:     "BYOK",
			wantKind: credential.KindBYOK,
			wantHint: utils.MaskSecret("user-secret-key"),
		},
		{
			name:       "KeyPool",
			useKeyPool: true,
			wantKind:   credential.KindCentralized,
			wantHint:   credential.HintFailoverKey,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			body := &unreadBody{}
			req := requestWithAuth(t, body)
			req.Header.Set("User-Agent", "claude-code/1.0")
			req.Header.Set("X-Coder-Agent-Firewall-Session-Id", "e5f6a7b8-1234-5678-9abc-def012345678")
			req.Header.Set("X-Coder-Agent-Firewall-Sequence-Number", "42")
			var pool *keypool.Pool
			if tc.useKeyPool {
				var err error
				pool, err = keypool.New("openai", []string{"pool-key"}, quartz.NewMock(t), nil)
				require.NoError(t, err)
				req.Header.Del("Authorization")
			}
			h := &forwardingHandler{
				provider: provider.NewOpenAI(config.OpenAI{BaseURL: "https://upstream.example.test", KeyPool: pool}),
				recorder: &struct{ recorder.Recorder }{},
			}
			response := httptest.NewRecorder()
			before := time.Now()
			record, cred := h.checkRequest(response, req)
			require.NotNil(t, record)
			require.NotNil(t, cred)
			id, err := uuid.Parse(record.ID)
			require.NoError(t, err)
			require.NotEqual(t, uuid.Nil, id)
			require.WithinRange(t, record.StartedAt, before, time.Now())
			require.Equal(t, time.UTC, record.StartedAt.Location())
			require.Equal(t, tc.wantKind, cred.Kind())
			require.Equal(t, tc.wantKind, record.CredentialKind)
			require.Equal(t, tc.wantHint, record.CredentialHint)
			require.Equal(t, aibcontext.ActorIDFromContext(req.Context()), record.InitiatorID)
			require.Equal(t, recorder.Metadata{"Username": t.Name()}, record.Metadata)
			require.Equal(t, "openai", record.ProviderName)
			require.Equal(t, config.ProviderOpenAI, record.Provider)
			require.Equal(t, string(client.ClaudeCode), record.Client)
			require.Equal(t, req.UserAgent(), record.UserAgent)
			require.Equal(t, new("e5f6a7b8-1234-5678-9abc-def012345678"), record.AgentFirewallSessionID)
			require.Equal(t, new(int32(42)), record.AgentFirewallSequenceNumber)
			require.False(t, body.read)
			require.False(t, body.closed)
		})
	}
}

func TestForwardingHandlerOpenCircuit(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultCircuitBreaker()
	cfg.FailureThreshold = 1
	cfg.Timeout = time.Hour
	prov := provider.NewOpenAI(config.OpenAI{CircuitBreaker: &cfg})
	logger := slogtest.Make(t, nil)
	gate := aibridge.NewInflightGate(logger)
	t.Cleanup(gate.Close)
	handler, err := newForwardingHandler(prov, logger, nil, noop.NewTracerProvider().Tracer(t.Name()), gate, &struct{ recorder.Recorder }{}, http.DefaultTransport)
	require.NoError(t, err)
	const route = "/v1/responses"
	require.NoError(t, handler.breaker.Execute(route, "", httptest.NewRecorder(), func(w http.ResponseWriter) error {
		w.WriteHeader(http.StatusServiceUnavailable)
		return nil
	}))

	for _, tc := range []struct {
		name         string
		websocket    bool
		noCredential bool
		status       int
	}{
		{
			name:   "OpenCircuit",
			status: http.StatusServiceUnavailable,
		},
		{
			name:      "ValidationBeforeCircuit",
			websocket: true,
			status:    http.StatusNotImplemented,
		},
		{
			name:         "CredentialBeforeCircuit",
			noCredential: true,
			status:       http.StatusForbidden,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			body := &unreadBody{}
			req := httptest.NewRequest(http.MethodPost, prov.RoutePrefix()+"/responses", body)
			req = req.WithContext(aibridge.AsActor(t.Context(), aibridge.Actor{ID: uuid.New(), APIKeyID: uuid.NewString()}))
			req.Header.Set("Authorization", "Bearer user-key")
			if tc.noCredential {
				req.Header.Del("Authorization")
			}
			if tc.websocket {
				req.Method = http.MethodGet
				req.Header.Set("Connection", "Upgrade")
				req.Header.Set("Upgrade", "websocket")
			}
			captured := &lifecycleRecorder{}
			response := httptest.NewRecorder()
			requestHandler := *handler
			requestHandler.recorder = captured
			requestHandler.ServeHTTP(response, req)
			require.Equal(t, tc.status, response.Code)
			require.False(t, body.read)
			require.False(t, body.closed)
			if tc.status != http.StatusServiceUnavailable {
				require.Empty(t, captured.starts)
				require.Empty(t, captured.ends)
				return
			}
			require.Contains(t, response.Body.String(), circuitbreaker.ErrCircuitOpen.Error())
			require.Len(t, captured.starts, 1)
			require.Len(t, captured.ends, 1)
			require.Equal(t, captured.starts[0].ID, captured.ends[0].ID)
			require.Equal(t, recorder.ErrorTypeServerError, captured.ends[0].ErrorType)
			require.Equal(t, circuitbreaker.ErrCircuitOpen.Error(), captured.ends[0].ErrorMessage)
		})
	}
}

func TestForwardingHandlerLifecycle(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name         string
		startErr     error
		endErr       error
		upstream     func(cancel context.CancelFunc) io.Reader
		status       int
		transportErr error
		wantCode     int
		wantPanic    any
		wantErrType  recorder.ErrorType
		wantFailed   bool
	}{
		{
			name:     "Success",
			wantCode: http.StatusOK,
		},
		{
			name:     "StartFailure",
			startErr: xerrors.New("start unavailable"),
			wantCode: http.StatusInternalServerError,
		},
		{
			name:     "EndFailure",
			endErr:   xerrors.New("end unavailable"),
			wantCode: http.StatusOK,
		},
		{
			name:        "Upstream503",
			status:      http.StatusServiceUnavailable,
			wantCode:    http.StatusServiceUnavailable,
			wantErrType: recorder.ErrorTypeServerError,
			wantFailed:  true,
		},
		{
			name:         "TransportFailure",
			transportErr: io.ErrClosedPipe,
			wantCode:     http.StatusBadGateway,
			wantFailed:   true,
		},
		{
			name: "TruncatedStream",
			upstream: func(context.CancelFunc) io.Reader {
				return io.MultiReader(strings.NewReader("chunk"), readerFunc(func([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }))
			},
			wantPanic:  http.ErrAbortHandler,
			wantFailed: true,
		},
		{
			name: "ClientCancellation",
			upstream: func(cancel context.CancelFunc) io.Reader {
				return readerFunc(func([]byte) (int, error) { cancel(); return 0, context.Canceled })
			},
			wantPanic:  http.ErrAbortHandler,
			wantFailed: true,
		},
		{
			name: "Panic",
			upstream: func(context.CancelFunc) io.Reader {
				return readerFunc(func([]byte) (int, error) { panic("copy panic") })
			},
			wantPanic:  "copy panic",
			wantFailed: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			reads := 0
			source := strings.NewReader("request")
			input := &trackedBody{Reader: readerFunc(func(p []byte) (int, error) { reads++; return source.Read(p) })}
			req := requestWithAuth(t, input)
			ctx, cancel := context.WithCancel(req.Context())
			defer cancel()
			req = req.WithContext(ctx)
			m := metrics.NewMetrics(prometheus.NewRegistry())
			h := newTestForwardingHandler(t, provider.NewOpenAI(config.OpenAI{BaseURL: "https://upstream.example.test"}), m)
			rec := &lifecycleRecorder{startErr: tc.startErr, endErr: tc.endErr}
			calls := 0
			h.transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				require.True(t, rec.startReturned.Load(), "the start record must be written before forwarding")
				_, err := io.Copy(io.Discard, r.Body)
				require.NoError(t, err)
				if tc.transportErr != nil {
					return nil, tc.transportErr
				}
				var upstream io.Reader = strings.NewReader("chunk")
				if tc.upstream != nil {
					upstream = tc.upstream(cancel)
				}
				status := http.StatusOK
				if tc.status != 0 {
					status = tc.status
				}
				return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(upstream)}, nil
			})
			response := httptest.NewRecorder()
			serve := func() { serveForwardingRequest(h, response, req, rec) }
			if tc.wantPanic != nil {
				require.PanicsWithValue(t, tc.wantPanic, serve)
			} else {
				serve()
				require.Equal(t, tc.wantCode, response.Code)
			}

			require.Len(t, rec.starts, 1)
			start := rec.starts[0]
			require.Equal(t, aibcontext.ActorIDFromContext(req.Context()), start.InitiatorID)
			require.Equal(t, credential.KindBYOK, start.CredentialKind)
			require.Equal(t, utils.MaskSecret("user-secret-key"), start.CredentialHint)
			// Body-derived metadata is not extracted yet.
			require.Empty(t, start.Model)
			require.Nil(t, start.ClientSessionID)
			require.Nil(t, start.CorrelatingToolCallID)
			if tc.startErr != nil {
				require.Equal(t, "failed to record interception\n", response.Body.String())
				require.True(t, rec.startReturned.Load())
				require.Zero(t, calls, "a failed start must not reach upstream")
				require.Zero(t, reads, "a failed start must not read the request body")
				require.Empty(t, rec.ends)
				require.Zero(t, promtest.CollectAndCount(m.InterceptionCount))
				return
			}

			require.Equal(t, 1, calls)
			require.Len(t, rec.ends, 1)
			end := rec.ends[0]
			require.Equal(t, start.ID, end.ID)
			require.False(t, end.EndedAt.Before(start.StartedAt))
			metricStatus := metrics.InterceptionCountStatusCompleted
			if tc.wantFailed {
				metricStatus = metrics.InterceptionCountStatusFailed
				require.NotEmpty(t, end.ErrorType)
				require.NotEmpty(t, end.ErrorMessage)
				if tc.wantErrType != "" {
					require.Equal(t, tc.wantErrType, end.ErrorType)
				}
			} else {
				require.Empty(t, end.ErrorType)
				require.Empty(t, end.ErrorMessage)
				require.Equal(t, "chunk", response.Body.String())
			}
			require.Equal(t, 1.0, promtest.ToFloat64(m.InterceptionCount.WithLabelValues("openai", "", metricStatus, "/v1/chat/completions", http.MethodPost, start.InitiatorID, string(client.Unknown))))
			require.Zero(t, promtest.CollectAndCount(m.PassthroughCount))
		})
	}
}

func TestForwardingHandlerRecordsKeyFailover(t *testing.T) {
	t.Parallel()
	pool, err := keypool.New("openai", []string{"first-key", "second-key"}, quartz.NewMock(t), nil)
	require.NoError(t, err)
	h := newTestForwardingHandler(t, provider.NewOpenAI(config.OpenAI{BaseURL: "https://upstream.example.test", KeyPool: pool}), nil)
	attempts := 0
	h.transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		attempts++
		status := http.StatusUnauthorized
		if attempts == 2 {
			status = http.StatusOK
		}
		return &http.Response{StatusCode: status, Header: http.Header{}, Body: http.NoBody}, nil
	})
	req := requestWithAuth(t, nil)
	req.Header.Del("Authorization")
	rec := &lifecycleRecorder{}
	response := httptest.NewRecorder()
	serveForwardingRequest(h, response, req, rec)

	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, 2, attempts)
	require.Len(t, rec.starts, 1)
	require.Len(t, rec.ends, 1)
	require.Equal(t, credential.KindCentralized, rec.starts[0].CredentialKind)
	require.Equal(t, credential.HintFailoverKey, rec.starts[0].CredentialHint)
	require.Equal(t, utils.MaskSecret("second-key"), rec.ends[0].CredentialHint, "the end record must report the key that served the final attempt")
	require.Empty(t, rec.ends[0].ErrorType)
}

// TestForwardingHandlerWaitsForEnd asserts that the handler stays admitted
// until the end record is written, and that recording the end is detached
// from request cancellation but keeps the request's actor.
func TestForwardingHandlerWaitsForEnd(t *testing.T) {
	t.Parallel()
	waitCtx := codertestutil.Context(t, codertestutil.WaitLong)
	h := newTestForwardingHandler(t, provider.NewOpenAI(config.OpenAI{BaseURL: "https://upstream.example.test"}), nil)
	h.transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("response"))}, nil
	})
	entered := make(chan context.Context, 1)
	release := make(chan struct{})
	releaseEnd := sync.OnceFunc(func() { close(release) })
	defer releaseEnd()
	rec := &lifecycleRecorder{onEnd: func(ctx context.Context) {
		entered <- ctx
		<-release
	}}
	req := requestWithAuth(t, nil)
	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()
	req = req.WithContext(ctx)
	response := httptest.NewRecorder()
	served := make(chan struct{})
	go func() {
		defer close(served)
		serveForwardingRequest(h, response, req, rec)
	}()

	endCtx := codertestutil.RequireReceive(waitCtx, t, entered)
	select {
	case <-served:
		t.Fatal("the handler returned before the end record was written")
	default:
	}
	cancel()
	require.NoError(t, endCtx.Err(), "recording the end must not follow request cancellation")
	_, ok := endCtx.Deadline()
	require.True(t, ok, "recording the end must be bounded")
	require.Equal(t, aibcontext.ActorFromContext(req.Context()), aibcontext.ActorFromContext(endCtx))
	shutdownCtx, cancelShutdown := context.WithCancel(waitCtx)
	cancelShutdown()
	require.ErrorIs(t, h.inflight.Shutdown(shutdownCtx), context.Canceled, "the request must stay admitted while the end is recorded")

	releaseEnd()
	codertestutil.TryReceive(waitCtx, t, served)
	require.NoError(t, h.inflight.Shutdown(waitCtx))
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "response", response.Body.String())
	require.Len(t, rec.ends, 1)
}

type unreadBody struct {
	read   bool
	closed bool
}

func (b *unreadBody) Read([]byte) (int, error) {
	b.read = true
	return 0, io.ErrUnexpectedEOF
}

func (b *unreadBody) Close() error {
	b.closed = true
	return nil
}
