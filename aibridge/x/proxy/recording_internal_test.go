package proxy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge/clientmeta"
	"github.com/coder/coder/v2/aibridge/config"
	aibcontext "github.com/coder/coder/v2/aibridge/context"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/metrics"
	"github.com/coder/coder/v2/aibridge/provider"
	"github.com/coder/coder/v2/aibridge/recorder"
	"github.com/coder/coder/v2/aibridge/routing"
	codertestutil "github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

type recordingProvider struct {
	*testutil.MockProvider
	typ          string
	authHeader   string
	failover     keypool.KeyFailoverConfig
	breaker      *config.CircuitBreaker
	failoverCall func(slog.Logger) keypool.KeyFailoverConfig
}

func (p *recordingProvider) Type() string       { return p.typ }
func (p *recordingProvider) AuthHeader() string { return p.authHeader }
func (p *recordingProvider) KeyFailoverConfig(logger slog.Logger) keypool.KeyFailoverConfig {
	if p.failoverCall != nil {
		return p.failoverCall(logger)
	}
	return p.failover
}
func (p *recordingProvider) CircuitBreakerConfig() *config.CircuitBreaker { return p.breaker }

type lifecycleRecorder struct {
	testutil.MockRecorder

	mu sync.Mutex

	startErr error
	endErr   error
	started  chan struct{}
	release  chan struct{}

	starts []*recorder.InterceptionRecord
	ends   []*recorder.InterceptionRecordEnded
	endCtx []error
}

func (r *lifecycleRecorder) RecordInterception(_ context.Context, record *recorder.InterceptionRecord) error {
	if r.startErr != nil {
		return r.startErr
	}
	r.mu.Lock()
	copyRecord := *record
	r.starts = append(r.starts, &copyRecord)
	if r.started != nil {
		r.started <- struct{}{}
		r.started = nil
	}
	r.mu.Unlock()
	if r.release != nil {
		<-r.release
	}
	return nil
}

func (r *lifecycleRecorder) RecordInterceptionEnded(ctx context.Context, record *recorder.InterceptionRecordEnded) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	copyRecord := *record
	r.ends = append(r.ends, &copyRecord)
	r.endCtx = append(r.endCtx, ctx.Err())
	return r.endErr
}

func (r *lifecycleRecorder) records() ([]*recorder.InterceptionRecord, []*recorder.InterceptionRecordEnded, []error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]*recorder.InterceptionRecord(nil), r.starts...), append([]*recorder.InterceptionRecordEnded(nil), r.ends...), append([]error(nil), r.endCtx...)
}

func TestRecordedHandlerMetadataAndCredentialHints(t *testing.T) {
	t.Parallel()

	t.Run("BYOKMetadata", func(t *testing.T) {
		t.Parallel()
		upstreamRequest := make(chan string, 1)
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				upstreamRequest <- err.Error()
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			upstreamRequest <- r.URL.RequestURI() + ":" + string(body)
			w.WriteHeader(http.StatusCreated)
		}))
		t.Cleanup(upstream.Close)

		rec := &testutil.MockRecorder{}
		prov := &recordingProvider{
			MockProvider: &testutil.MockProvider{NameStr: "anthropic", URL: upstream.URL + "/base?configured=1", Bridged: []string{"/v1/messages"}},
			typ:          config.ProviderAnthropic, authHeader: "X-Api-Key",
			failover: keypool.KeyFailoverConfig{IsBYOK: func(r *http.Request) bool {
				return r.Header.Get("X-Api-Key") != "" || r.Header.Get("Authorization") != ""
			}},
		}
		router := newTestRouter(t, []provider.Provider{prov}, rec, slogtest.Make(t, nil), nil)
		payload := `{"metadata":{"user_id":"user_hash_account_id_session_session-123"}}`
		req := recordedRequest(http.MethodPost, "/anthropic/v1/messages?raw=a;b", strings.NewReader(payload))
		req = req.WithContext(aibcontext.AsActor(req.Context(), "actor-1", recorder.Metadata{"Username": "alice"}))
		req.Header.Set("User-Agent", "claude-cli/1.0")
		req.Header.Set("X-Api-Key", "anthropic-personal-api-key")
		req.Header.Set("X-Coder-Agent-Firewall-Session-Id", "e5f6a7b8-1234-5678-9abc-def012345678")
		req.Header.Set("X-Coder-Agent-Firewall-Sequence-Number", "42")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		require.Equal(t, http.StatusCreated, response.Code)
		require.Equal(t, "/base/v1/messages?configured=1&raw=a;b:"+payload, <-upstreamRequest)

		starts := rec.RecordedInterceptions()
		require.Len(t, starts, 1)
		start := starts[0]
		require.Equal(t, "actor-1", start.InitiatorID)
		require.Equal(t, recorder.Metadata{"Username": "alice"}, start.Metadata)
		require.Equal(t, config.ProviderAnthropic, start.Provider)
		require.Equal(t, "anthropic", start.ProviderName)
		require.Equal(t, unknownModel, start.Model)
		require.Equal(t, "Claude Code", start.Client)
		require.Equal(t, new("session-123"), start.ClientSessionID)
		require.Equal(t, recorder.CredentialKindBYOK, start.CredentialKind)
		require.Equal(t, "anth...-key", start.CredentialHint)
		require.Equal(t, new("e5f6a7b8-1234-5678-9abc-def012345678"), start.AgentFirewallSessionID)
		require.Equal(t, new(int32(42)), start.AgentFirewallSequenceNumber)
		end := rec.RecordedInterceptionEnd(start.ID)
		require.NotNil(t, end)
		require.Equal(t, start.CredentialHint, end.CredentialHint)
		require.Empty(t, end.ErrorType)
		require.Empty(t, end.ErrorMessage)
	})

	t.Run("AuthorizationSchemes", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			name          string
			authorization string
			wantHint      string
		}{
			{name: "Token", authorization: "Token opaque-provider-token", wantHint: "opaq...oken"},
			{name: "Basic", authorization: "Basic dXNlcjpwYXNz", wantHint: "dX...Nz"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
				t.Cleanup(upstream.Close)
				rec := &testutil.MockRecorder{}
				prov := &recordingProvider{
					MockProvider: &testutil.MockProvider{NameStr: "anthropic", URL: upstream.URL, Bridged: []string{"/v1/messages"}},
					typ:          config.ProviderAnthropic, authHeader: "X-Api-Key",
					failover: keypool.KeyFailoverConfig{IsBYOK: func(r *http.Request) bool {
						return r.Header.Get("Authorization") != ""
					}},
				}
				router := newTestRouter(t, []provider.Provider{prov}, rec, slogtest.Make(t, nil), nil)
				req := recordedRequest(http.MethodPost, "/anthropic/v1/messages", strings.NewReader(`{}`))
				req.Header.Set("Authorization", tc.authorization)
				router.ServeHTTP(httptest.NewRecorder(), req)
				starts := rec.RecordedInterceptions()
				require.Len(t, starts, 1)
				require.Equal(t, recorder.CredentialKindBYOK, starts[0].CredentialKind)
				require.Equal(t, tc.wantHint, starts[0].CredentialHint)
			})
		}
	})

	t.Run("CentralizedFinalKeyHint", func(t *testing.T) {
		t.Parallel()
		pool, err := keypool.New(config.ProviderOpenAI, []string{"first-centralized-provider-key", "second-centralized-provider-key"}, quartz.NewMock(t), nil)
		require.NoError(t, err)
		attempts := atomic.Int32{}
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.Copy(io.Discard, r.Body)
			if attempts.Add(1) == 1 {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		}))
		t.Cleanup(upstream.Close)
		rec := &testutil.MockRecorder{}
		dumpDir := t.TempDir()
		prov := provider.NewOpenAI(config.OpenAI{BaseURL: upstream.URL, KeyPool: pool, APIDumpDir: dumpDir})
		router := newTestRouter(t, []provider.Provider{prov}, rec, slogtest.Make(t, nil), nil)
		router.ServeHTTP(httptest.NewRecorder(), recordedRequest(http.MethodPost, "/openai/v1/chat/completions", strings.NewReader("payload")))
		starts := rec.RecordedInterceptions()
		require.Len(t, starts, 1)
		require.Equal(t, recorder.CredentialKindCentralized, starts[0].CredentialKind)
		require.Equal(t, recorder.CredentialHintFailoverKey, starts[0].CredentialHint)
		require.Equal(t, "seco...-key", rec.RecordedInterceptionEnd(starts[0].ID).CredentialHint)

		dumps, err := filepath.Glob(filepath.Join(dumpDir, config.ProviderOpenAI, "passthrough", "*.req.txt"))
		require.NoError(t, err)
		require.Len(t, dumps, 2)
		for _, dump := range dumps {
			content, err := os.ReadFile(dump)
			require.NoError(t, err)
			require.Contains(t, string(content), "payload")
			require.NotContains(t, string(content), "first-centralized-provider-key")
			require.NotContains(t, string(content), "second-centralized-provider-key")
		}
	})
}

func TestRecordedHandlerLifecycle(t *testing.T) {
	t.Parallel()

	t.Run("StartBlocksDispatch", func(t *testing.T) {
		t.Parallel()
		upstreamCalled := make(chan struct{}, 1)
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			upstreamCalled <- struct{}{}
			w.WriteHeader(http.StatusNoContent)
		}))
		t.Cleanup(upstream.Close)
		started := make(chan struct{}, 1)
		release := make(chan struct{})
		t.Cleanup(func() {
			select {
			case <-release:
			default:
				close(release)
			}
		})
		rec := &lifecycleRecorder{started: started, release: release}
		prov := recordedProvider("openai", config.ProviderOpenAI, upstream.URL, "/v1/chat")
		router := newTestRouter(t, []provider.Provider{prov}, rec, slogtest.Make(t, nil), nil)
		done := make(chan struct{}, 1)
		go func() {
			router.ServeHTTP(httptest.NewRecorder(), recordedRequest(http.MethodPost, "/openai/v1/chat", nil))
			done <- struct{}{}
		}()
		ctx := codertestutil.Context(t, codertestutil.WaitShort)
		codertestutil.RequireReceive(ctx, t, started)
		select {
		case <-upstreamCalled:
			t.Fatal("upstream dispatched before start recording completed")
		default:
		}
		close(release)
		codertestutil.RequireReceive(ctx, t, done)
		codertestutil.RequireReceive(ctx, t, upstreamCalled)
		starts, ends, _ := rec.records()
		require.Len(t, starts, 1)
		require.Len(t, ends, 1)
	})

	t.Run("MissingActorDoesNotStart", func(t *testing.T) {
		t.Parallel()
		upstreamCalls := atomic.Int32{}
		upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { upstreamCalls.Add(1) }))
		t.Cleanup(upstream.Close)
		rec := &lifecycleRecorder{}
		prov := recordedProvider("openai", config.ProviderOpenAI, upstream.URL, "/v1/chat")
		router := newTestRouter(t, []provider.Provider{prov}, rec, slogtest.Make(t, nil), nil)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/openai/v1/chat", nil))
		require.Equal(t, http.StatusBadRequest, response.Code)
		require.Zero(t, upstreamCalls.Load())
		starts, ends, _ := rec.records()
		require.Empty(t, starts)
		require.Empty(t, ends)
	})

	t.Run("StartFailureDoesNotDispatchOrEnd", func(t *testing.T) {
		t.Parallel()
		upstreamCalls := atomic.Int32{}
		upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { upstreamCalls.Add(1) }))
		t.Cleanup(upstream.Close)
		rec := &lifecycleRecorder{startErr: xerrors.New("recording unavailable")}
		prov := recordedProvider("openai", config.ProviderOpenAI, upstream.URL, "/v1/chat")
		sink := codertestutil.NewFakeSink(t)
		router := newTestRouter(t, []provider.Provider{prov}, rec, sink.Logger(), nil)
		response := httptest.NewRecorder()
		req := recordedRequest(http.MethodPost, "/openai/v1/chat", strings.NewReader("body-secret"))
		req.Header.Set("Authorization", "Bearer header-secret")
		router.ServeHTTP(response, req)
		require.Equal(t, http.StatusInternalServerError, response.Code)
		require.Zero(t, upstreamCalls.Load())
		entries := fmt.Sprint(sink.Entries())
		require.Contains(t, entries, "failed to record interception")
		require.NotContains(t, entries, "header-secret")
		require.NotContains(t, entries, "body-secret")
		starts, ends, _ := rec.records()
		require.Empty(t, starts)
		require.Empty(t, ends)
	})

	t.Run("PanicAfterStartEndsOnceAndPropagates", func(t *testing.T) {
		t.Parallel()
		rec := &lifecycleRecorder{}
		prov := recordedProvider("openai", config.ProviderOpenAI, "http://127.0.0.1:1", "/v1/chat")
		prov.failoverCall = func(slog.Logger) keypool.KeyFailoverConfig {
			return keypool.KeyFailoverConfig{
				Pool:                 testutil.SingleKeyPool(config.ProviderOpenAI, "panic-key"),
				IsBYOK:               func(*http.Request) bool { return false },
				InjectAuthKey:        func(*http.Header, string) { panic("after start") },
				BuildKeyPoolResponse: func(*keypool.Error) *http.Response { return nil },
			}
		}
		router := newTestRouter(t, []provider.Provider{prov}, rec, slogtest.Make(t, nil), nil)
		require.PanicsWithValue(t, "after start", func() {
			router.ServeHTTP(httptest.NewRecorder(), recordedRequest(http.MethodPost, "/openai/v1/chat", nil))
		})
		starts, ends, _ := rec.records()
		require.Len(t, starts, 1)
		require.Len(t, ends, 1)
	})

	t.Run("EndFailureDoesNotMutateTraffic", func(t *testing.T) {
		t.Parallel()
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
		t.Cleanup(upstream.Close)
		sink := codertestutil.NewFakeSink(t)
		rec := &lifecycleRecorder{endErr: xerrors.New("end unavailable")}
		prov := recordedProvider("openai", config.ProviderOpenAI, upstream.URL, "/v1/chat")
		router := newTestRouter(t, []provider.Provider{prov}, rec, sink.Logger(), nil)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, recordedRequest(http.MethodPost, "/openai/v1/chat", nil))
		require.Equal(t, http.StatusNoContent, response.Code)
		_, ends, endCtx := rec.records()
		require.Len(t, ends, 1)
		require.NoError(t, endCtx[0])
		require.Contains(t, fmt.Sprint(sink.Entries()), "failed to record interception end")
	})

	t.Run("ConcurrentStateIsSeparate", func(t *testing.T) {
		t.Parallel()
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.Copy(io.Discard, r.Body)
			w.WriteHeader(http.StatusNoContent)
		}))
		t.Cleanup(upstream.Close)
		rec := &lifecycleRecorder{}
		prov := recordedProvider("openai", config.ProviderOpenAI, upstream.URL, "/v1/chat")
		router := newTestRouter(t, []provider.Provider{prov}, rec, slogtest.Make(t, nil), nil)
		const count = 8
		var wg sync.WaitGroup
		for i := range count {
			wg.Go(func() {
				actorID := fmt.Sprintf("actor-%d", i)
				req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat", strings.NewReader(actorID))
				req = req.WithContext(aibcontext.AsActor(req.Context(), actorID, nil))
				router.ServeHTTP(httptest.NewRecorder(), req)
			})
		}
		wg.Wait()
		starts, ends, _ := rec.records()
		require.Len(t, starts, count)
		require.Len(t, ends, count)
		startIDs := make(map[string]int, count)
		for _, start := range starts {
			startIDs[start.ID]++
		}
		endIDs := make(map[string]int, count)
		for _, end := range ends {
			endIDs[end.ID]++
		}
		require.Equal(t, startIDs, endIDs)
		for _, occurrences := range startIDs {
			require.Equal(t, 1, occurrences)
		}
	})

	t.Run("EndUsesDetachedContext", func(t *testing.T) {
		t.Parallel()
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
		t.Cleanup(upstream.Close)
		rec := &lifecycleRecorder{endErr: xerrors.New("end unavailable")}
		prov := recordedProvider("openai", config.ProviderOpenAI, upstream.URL, "/v1/chat")
		router := newTestRouter(t, []provider.Provider{prov}, rec, slogtest.Make(t, nil), nil)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		req := recordedRequest(http.MethodPost, "/openai/v1/chat", nil).WithContext(ctx)
		req = req.WithContext(aibcontext.AsActor(req.Context(), "actor", nil))
		require.PanicsWithValue(t, http.ErrAbortHandler, func() {
			router.ServeHTTP(httptest.NewRecorder(), req)
		})
		_, ends, endCtx := rec.records()
		require.Len(t, ends, 1)
		require.NoError(t, endCtx[0])
	})
}

func TestRecordedHandlerTerminalOutcomes(t *testing.T) {
	t.Parallel()

	t.Run("CircuitOpenStillEnds", func(t *testing.T) {
		t.Parallel()
		upstreamCalls := atomic.Int32{}
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			upstreamCalls.Add(1)
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		t.Cleanup(upstream.Close)
		rec := &lifecycleRecorder{}
		prov := recordedProvider("openai", config.ProviderOpenAI, upstream.URL, "/v1/chat")
		prov.breaker = &config.CircuitBreaker{FailureThreshold: 1, Timeout: time.Minute}
		router := newTestRouter(t, []provider.Provider{prov}, rec, slogtest.Make(t, nil), nil)
		for range 2 {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, recordedRequest(http.MethodPost, "/openai/v1/chat", nil))
			require.Equal(t, http.StatusServiceUnavailable, response.Code)
		}
		require.EqualValues(t, 1, upstreamCalls.Load())
		starts, ends, _ := rec.records()
		require.Len(t, starts, 2)
		require.Len(t, ends, 2)
	})

	t.Run("WriteFailureAbortsAndEnds", func(t *testing.T) {
		t.Parallel()
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("response"))
		}))
		t.Cleanup(upstream.Close)
		rec := &lifecycleRecorder{}
		prov := recordedProvider("openai", config.ProviderOpenAI, upstream.URL, "/v1/chat")
		router := newTestRouter(t, []provider.Provider{prov}, rec, slogtest.Make(t, nil), nil)
		writer := &failingResponseWriter{header: http.Header{}, err: xerrors.New("client disconnected")}
		require.PanicsWithValue(t, http.ErrAbortHandler, func() {
			router.ServeHTTP(writer, recordedRequest(http.MethodPost, "/openai/v1/chat", nil))
		})
		starts, ends, _ := rec.records()
		require.Len(t, starts, 1)
		require.Len(t, ends, 1)
		require.Equal(t, recorder.ErrorTypeUnknown, ends[0].ErrorType)
	})

	t.Run("PoolErrorWinsOverWriteAbort", func(t *testing.T) {
		t.Parallel()
		pool, err := keypool.New(config.ProviderOpenAI, []string{"only-key"}, quartz.NewMock(t), nil)
		require.NoError(t, err)
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) }))
		t.Cleanup(upstream.Close)
		rec := &lifecycleRecorder{}
		prov := provider.NewOpenAI(config.OpenAI{BaseURL: upstream.URL, KeyPool: pool})
		router := newTestRouter(t, []provider.Provider{prov}, rec, slogtest.Make(t, nil), nil)
		writer := &failingResponseWriter{header: http.Header{}, err: xerrors.New("client disconnected")}
		require.PanicsWithValue(t, http.ErrAbortHandler, func() {
			router.ServeHTTP(writer, recordedRequest(http.MethodPost, "/openai/v1/chat/completions", strings.NewReader(`{}`)))
		})
		_, ends, _ := rec.records()
		require.Len(t, ends, 1)
		require.Equal(t, recorder.ErrorTypeUnauthorized, ends[0].ErrorType)
	})

	t.Run("TransportFailureEnds", func(t *testing.T) {
		t.Parallel()
		rec := &lifecycleRecorder{}
		prov := recordedProvider("openai", config.ProviderOpenAI, "http://127.0.0.1:1", "/v1/chat")
		router := newTestRouter(t, []provider.Provider{prov}, rec, slogtest.Make(t, nil), nil)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, recordedRequest(http.MethodPost, "/openai/v1/chat", nil))
		require.Equal(t, http.StatusBadGateway, response.Code)
		_, ends, _ := rec.records()
		require.Len(t, ends, 1)
		require.Equal(t, recorder.ErrorTypeUnknown, ends[0].ErrorType)
		require.NotEmpty(t, ends[0].ErrorMessage)
	})

	t.Run("TimeoutEndsAndAborts", func(t *testing.T) {
		t.Parallel()
		rec := &lifecycleRecorder{}
		prov := recordedProvider("openai", config.ProviderOpenAI, "http://127.0.0.1:1", "/v1/chat")
		sink := codertestutil.NewFakeSink(t)
		router := newTestRouter(t, []provider.Provider{prov}, rec, sink.Logger(), nil)
		ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
		defer cancel()
		req := recordedRequest(http.MethodPost, "/openai/v1/chat", nil).WithContext(ctx)
		req = req.WithContext(aibcontext.AsActor(req.Context(), "actor", nil))
		require.PanicsWithValue(t, http.ErrAbortHandler, func() {
			router.ServeHTTP(httptest.NewRecorder(), req)
		})
		_, ends, _ := rec.records()
		require.Len(t, ends, 1)
		require.Equal(t, recorder.ErrorTypeTimeout, ends[0].ErrorType)
		entries := sink.Entries(func(entry slog.SinkEntry) bool { return entry.Message == "interception failed" })
		require.Len(t, entries, 1)
		require.Equal(t, slog.LevelDebug, entries[0].Level)
	})

	t.Run("RecordedUpstreamUpgrade", func(t *testing.T) {
		t.Parallel()
		body := &closeTrackingBody{Reader: strings.NewReader("upgrade")}
		rec := &lifecycleRecorder{}
		prov := recordedProvider("openai", config.ProviderOpenAI, "https://openai.example.test", "/v1/chat")
		handler := &forwardingHandler{
			provider: prov,
			baseURL:  mustParseURL(t, prov.BaseURL()),
			transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusSwitchingProtocols, Header: http.Header{"Connection": {"Upgrade"}, "Upgrade": {"h2c"}}, Body: body}, nil
			}),
			recorder:    rec,
			logger:      slogtest.Make(t, nil),
			tracer:      noop.NewTracerProvider().Tracer(t.Name()),
			record:      true,
			metricRoute: "/v1/chat",
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, recordedRequest(http.MethodPost, "/openai/v1/chat", nil))
		require.Equal(t, http.StatusBadGateway, response.Code)
		require.True(t, body.closed)
		_, ends, _ := rec.records()
		require.Len(t, ends, 1)
		require.Contains(t, ends[0].ErrorMessage, "upstream protocol upgrades are not supported")
		require.NotContains(t, ends[0].ErrorMessage, "before EOF")
	})

	t.Run("ProviderStatuses", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			name     string
			status   int
			newProv  func(*testing.T, string) provider.Provider
			path     string
			wantType recorder.ErrorType
			wantMsg  string
		}{
			{
				name:   "RateLimited",
				status: http.StatusTooManyRequests,
				newProv: func(_ *testing.T, baseURL string) provider.Provider {
					return recordedProvider("copilot", config.ProviderCopilot, baseURL, "/v1/chat")
				},
				path:     "/copilot/v1/chat",
				wantType: recorder.ErrorTypeRateLimited,
				wantMsg:  http.StatusText(http.StatusTooManyRequests),
			},
			{
				name:   "OpenAIOverloaded",
				status: http.StatusServiceUnavailable,
				newProv: func(_ *testing.T, baseURL string) provider.Provider {
					return provider.NewOpenAI(config.OpenAI{BaseURL: baseURL})
				},
				path:     "/openai/v1/chat/completions",
				wantType: recorder.ErrorTypeOverloaded,
				wantMsg:  http.StatusText(http.StatusServiceUnavailable),
			},
			{
				name:   "AnthropicOverloaded",
				status: 529,
				newProv: func(t *testing.T, baseURL string) provider.Provider {
					prov, err := provider.NewAnthropic(t.Context(), config.Anthropic{BaseURL: baseURL}, nil)
					require.NoError(t, err)
					return prov
				},
				path:     "/anthropic/v1/messages",
				wantType: recorder.ErrorTypeOverloaded,
				wantMsg:  "HTTP status 529",
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(tc.status) }))
				t.Cleanup(upstream.Close)
				rec := &lifecycleRecorder{}
				sink := codertestutil.NewFakeSink(t)
				prov := tc.newProv(t, upstream.URL)
				router := newTestRouter(t, []provider.Provider{prov}, rec, sink.Logger(), nil)
				response := httptest.NewRecorder()
				router.ServeHTTP(response, recordedRequest(http.MethodPost, tc.path, nil))
				require.Equal(t, tc.status, response.Code)
				_, ends, _ := rec.records()
				require.Len(t, ends, 1)
				require.Equal(t, tc.wantType, ends[0].ErrorType)
				require.Equal(t, tc.wantMsg, ends[0].ErrorMessage)
				entries := sink.Entries(func(entry slog.SinkEntry) bool { return entry.Message == "interception failed" })
				require.Len(t, entries, 1)
				require.Equal(t, slog.LevelWarn, entries[0].Level)
				fields := fmt.Sprint(entries[0].Fields)
				require.Contains(t, fields, ends[0].ID)
				require.Contains(t, fields, string(tc.wantType))
			})
		}
	})
}

func TestRecordedHandlerBodyCaptureAndMetrics(t *testing.T) {
	t.Parallel()

	t.Run("ReadFailureDoesNotStart", func(t *testing.T) {
		t.Parallel()
		upstreamCalls := atomic.Int32{}
		upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { upstreamCalls.Add(1) }))
		t.Cleanup(upstream.Close)
		rec := &lifecycleRecorder{}
		prov := recordedProvider("openai", config.ProviderOpenAI, upstream.URL, "/v1/chat")
		sink := codertestutil.NewFakeSink(t)
		router := newTestRouter(t, []provider.Provider{prov}, rec, sink.Logger(), nil)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, recordedRequest(http.MethodPost, "/openai/v1/chat", &failingBody{payload: []byte("body-secret"), err: xerrors.New("read failed")}))
		require.Equal(t, http.StatusBadRequest, response.Code)
		require.Zero(t, upstreamCalls.Load())
		entries := fmt.Sprint(sink.Entries())
		require.Contains(t, entries, "read failed")
		require.NotContains(t, entries, "body-secret")
		starts, ends, _ := rec.records()
		require.Empty(t, starts)
		require.Empty(t, ends)
	})

	t.Run("CaptureClosesBodyAndEnforcesLimit", func(t *testing.T) {
		t.Parallel()
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.Copy(io.Discard, r.Body)
			w.WriteHeader(http.StatusNoContent)
		}))
		t.Cleanup(upstream.Close)
		rec := &testutil.MockRecorder{}
		prov := recordedProvider("openai", config.ProviderOpenAI, upstream.URL, "/v1/chat")
		router := newTestRouter(t, []provider.Provider{prov}, rec, slogtest.Make(t, nil), nil)
		body := &closeTrackingBody{Reader: strings.NewReader("payload")}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, recordedRequest(http.MethodPost, "/openai/v1/chat", body))
		require.Equal(t, http.StatusNoContent, response.Code)
		require.True(t, body.closed)
		rec.VerifyAllInterceptionsEnded(t)

		exact := &closeTrackingBody{Reader: bytes.NewReader(make([]byte, routing.MaxRequestBodyBytes))}
		response = httptest.NewRecorder()
		router.ServeHTTP(response, recordedRequest(http.MethodPost, "/openai/v1/chat", exact))
		require.Equal(t, http.StatusNoContent, response.Code)
		require.True(t, exact.closed)

		declared := &closeTrackingBody{Reader: strings.NewReader("small")}
		req := recordedRequest(http.MethodPost, "/openai/v1/chat", declared)
		req.ContentLength = routing.MaxRequestBodyBytes + 1
		response = httptest.NewRecorder()
		router.ServeHTTP(response, req)
		require.Equal(t, http.StatusRequestEntityTooLarge, response.Code)
		require.True(t, declared.closed)

		oversize := &closeTrackingBody{Reader: io.LimitReader(zeroReader{}, routing.MaxRequestBodyBytes+1)}
		req = recordedRequest(http.MethodPost, "/openai/v1/chat", oversize)
		req.ContentLength = -1
		response = httptest.NewRecorder()
		router.ServeHTTP(response, req)
		require.Equal(t, http.StatusRequestEntityTooLarge, response.Code)
		require.True(t, oversize.closed)
		require.Len(t, rec.RecordedInterceptions(), 2)
	})

	t.Run("Metrics", func(t *testing.T) {
		t.Parallel()
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Test-Failure") != "" {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		}))
		t.Cleanup(upstream.Close)
		registry := prometheus.NewRegistry()
		m := metrics.NewMetrics(registry)
		rec := &testutil.MockRecorder{}
		prov := recordedProvider("openai", config.ProviderOpenAI, upstream.URL, "/v1/chat")
		router := newTestRouter(t, []provider.Provider{prov}, rec, slogtest.Make(t, nil), m)
		for _, failure := range []bool{false, true} {
			req := recordedRequest(http.MethodPost, "/openai/v1/chat", nil)
			if failure {
				req.Header.Set("X-Test-Failure", "true")
			}
			router.ServeHTTP(httptest.NewRecorder(), req)
		}
		require.Zero(t, promtest.ToFloat64(m.InterceptionsInflight.WithLabelValues("openai", unknownModel, "/v1/chat")))
		require.Equal(t, 1.0, promtest.ToFloat64(m.InterceptionCount.WithLabelValues("openai", unknownModel, metrics.InterceptionCountStatusCompleted, "/v1/chat", http.MethodPost, "actor", string(clientmeta.ClientUnknown))))
		require.Equal(t, 1.0, promtest.ToFloat64(m.InterceptionCount.WithLabelValues("openai", unknownModel, metrics.InterceptionCountStatusFailed, "/v1/chat", http.MethodPost, "actor", string(clientmeta.ClientUnknown))))
	})
}

func TestRecordedHandlerRealHTTPTruncation(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("chunk"))
	}))
	t.Cleanup(upstream.Close)
	rec := &testutil.MockRecorder{}
	prov := provider.NewOpenAI(config.OpenAI{BaseURL: upstream.URL})
	router := newTestRouter(t, []provider.Provider{prov}, rec, slogtest.Make(t, nil), nil)
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(aibcontext.AsActor(r.Context(), "actor", nil))
		router.ServeHTTP(w, r)
	}))
	t.Cleanup(gateway.Close)

	ctx := codertestutil.Context(t, codertestutil.WaitShort)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, gateway.URL+"/openai/v1/chat/completions", strings.NewReader(`{}`))
	require.NoError(t, err)
	resp, err := gateway.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, readErr := io.ReadAll(resp.Body)
	require.ErrorIs(t, readErr, io.ErrUnexpectedEOF)
	require.Equal(t, "chunk", string(body))
	starts := rec.RecordedInterceptions()
	require.Len(t, starts, 1)
	end := rec.RecordedInterceptionEnd(starts[0].ID)
	require.NotNil(t, end)
	require.Equal(t, recorder.ErrorTypeUnknown, end.ErrorType)
	require.Equal(t, io.ErrUnexpectedEOF.Error(), end.ErrorMessage)
}

func recordedProvider(name, typ, baseURL, route string) *recordingProvider {
	return &recordingProvider{
		MockProvider: &testutil.MockProvider{NameStr: name, URL: baseURL, Bridged: []string{route}},
		typ:          typ, authHeader: "Authorization",
		failover: keypool.KeyFailoverConfig{IsBYOK: func(r *http.Request) bool {
			return r.Header.Get("Authorization") != ""
		}},
	}
}

func recordedRequest(method, target string, body io.Reader) *http.Request {
	req := httptest.NewRequest(method, target, body)
	return req.WithContext(aibcontext.AsActor(req.Context(), "actor", nil))
}

type failingBody struct {
	payload []byte
	err     error
}

func (b *failingBody) Read(p []byte) (int, error) {
	if len(b.payload) == 0 {
		return 0, b.err
	}
	n := copy(p, b.payload)
	b.payload = b.payload[n:]
	return n, nil
}

func (*failingBody) Close() error { return nil }
