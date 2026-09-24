package aibridged_test

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	aibridgecore "github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/recorder"
	"github.com/coder/coder/v2/aibridge/x/proxy"
	"github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/aibridged"
	"github.com/coder/coder/v2/testutil"
)

type transportRecorder struct {
	mu     sync.Mutex
	starts []*recorder.InterceptionRecord
	ends   map[string]*recorder.InterceptionRecordEnded
}

func (r *transportRecorder) RecordInterception(_ context.Context, record *recorder.InterceptionRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.starts = append(r.starts, record)
	return nil
}

func (r *transportRecorder) RecordInterceptionEnded(_ context.Context, record *recorder.InterceptionRecordEnded) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ends == nil {
		r.ends = map[string]*recorder.InterceptionRecordEnded{}
	}
	r.ends[record.ID] = record
	return nil
}

func (*transportRecorder) RecordTokenUsage(context.Context, *recorder.TokenUsageRecord) error {
	return nil
}

func (*transportRecorder) RecordPromptUsage(context.Context, *recorder.PromptUsageRecord) error {
	return nil
}

func (*transportRecorder) RecordToolUsage(context.Context, *recorder.ToolUsageRecord) error {
	return nil
}

func (*transportRecorder) RecordModelThought(context.Context, *recorder.ModelThoughtRecord) error {
	return nil
}

func (r *transportRecorder) records() ([]*recorder.InterceptionRecord, map[string]*recorder.InterceptionRecordEnded) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]*recorder.InterceptionRecord(nil), r.starts...), maps.Clone(r.ends)
}

func TestTransportFactory_TransportFor(t *testing.T) {
	t.Parallel()

	t.Run("ReturnsTransport", func(t *testing.T) {
		t.Parallel()
		f := aibridged.NewTransportFactory(http.NotFoundHandler())
		rt, err := f.TransportFor("openai", aibridge.SourceAgents)
		require.NoError(t, err)
		require.NotNil(t, rt)
	})

	t.Run("NilHandlerErrors", func(t *testing.T) {
		t.Parallel()
		f := aibridged.NewTransportFactory(nil)
		_, err := f.TransportFor("openai", aibridge.SourceAgents)
		require.Error(t, err)
	})

	t.Run("EmptyProviderErrors", func(t *testing.T) {
		t.Parallel()
		f := aibridged.NewTransportFactory(http.NotFoundHandler())
		_, err := f.TransportFor("", aibridge.SourceAgents)
		require.Error(t, err)
	})

	t.Run("RewritesURLToAibridgeMount", func(t *testing.T) {
		t.Parallel()

		// The round-tripper must adapt an upstream-shaped URL.Path
		// ("/v1/messages") to the ai-gateway mount layout
		// ("/api/v2/ai-gateway/<provider>/v1/messages") so callers don't
		// have to encode the daemon's routing key into their requests.
		got := make(chan string, 1)
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got <- r.URL.Path
			w.WriteHeader(http.StatusOK)
		})

		rt, err := aibridged.NewTransportFactory(handler).TransportFor("my-anthropic", aibridge.SourceAgents)
		require.NoError(t, err)

		ctx := aibridge.WithDelegatedAPIKeyID(testutil.Context(t, testutil.WaitShort), "test-key-id")
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://upstream/v1/messages", nil)
		require.NoError(t, err)

		// The caller's req.URL.Path is the upstream shape. Capture it so
		// we can prove the transport mutates a clone, not the caller's
		// request, after RoundTrip returns.
		origPath := req.URL.Path

		resp, err := rt.RoundTrip(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, "/api/v2/ai-gateway/my-anthropic/v1/messages", <-got)
		require.Equal(t, origPath, req.URL.Path,
			"caller's request URL must not be mutated by RoundTrip")
	})

	t.Run("AttachesSourceToContext", func(t *testing.T) {
		t.Parallel()

		got := make(chan aibridge.Source, 1)
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got <- aibridge.SourceFromContext(r.Context())
			w.WriteHeader(http.StatusOK)
		})

		rt, err := aibridged.NewTransportFactory(handler).TransportFor("openai", aibridge.SourceAgents)
		require.NoError(t, err)

		ctx := aibridge.WithDelegatedAPIKeyID(testutil.Context(t, testutil.WaitShort), "test-key-id")
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://aibridge/v1/test", nil)
		require.NoError(t, err)

		resp, err := rt.RoundTrip(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, aibridge.SourceAgents, <-got)
	})
}

func TestInMemoryRoundTripper_PassesHeadersAndStatus(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom", "yes")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	rt, err := aibridged.NewTransportFactory(handler).TransportFor("openai", aibridge.SourceAgents)
	require.NoError(t, err)

	ctx := aibridge.WithDelegatedAPIKeyID(testutil.Context(t, testutil.WaitShort), "test-key-id")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://aibridge/v1/test", nil)
	require.NoError(t, err)

	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusTeapot, resp.StatusCode)
	require.Equal(t, "418 I'm a teapot", resp.Status)
	require.Equal(t, "yes", resp.Header.Get("X-Custom"))
	require.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, `{"ok":true}`, string(body))
}

func TestInMemoryRoundTripper_EmptyWrites(t *testing.T) {
	t.Parallel()

	for _, payload := range [][]byte{nil, {}} {
		t.Run(fmt.Sprintf("Nil=%t", payload == nil), func(t *testing.T) {
			t.Parallel()

			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				for _, chunk := range []string{"data: first\n\n", "data: second\n\n", ""} {
					for range 128 {
						if _, err := w.Write(payload); err != nil {
							return
						}
					}
					if _, err := io.WriteString(w, chunk); err != nil {
						return
					}
				}
			})

			rt, err := aibridged.NewTransportFactory(handler).TransportFor("openai", aibridge.SourceAgents)
			require.NoError(t, err)

			ctx := aibridge.WithDelegatedAPIKeyID(testutil.Context(t, testutil.WaitShort), "test-key-id")
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://aibridge/v1/responses", nil)
			require.NoError(t, err)

			resp, err := rt.RoundTrip(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)
			require.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

			reader := bufio.NewReader(resp.Body)
			for _, want := range []string{"data: first\n", "\n", "data: second\n", "\n"} {
				line, err := reader.ReadString('\n')
				require.NoError(t, err)
				require.Equal(t, want, line)
			}
			line, err := reader.ReadString('\n')
			require.ErrorIs(t, err, io.EOF)
			require.Empty(t, line)
		})
	}
}

// Verify that response chunks become readable on the client side before the
// handler has finished writing. This is the property SSE/NDJSON streaming
// depends on.
func TestInMemoryRoundTripper_Streams(t *testing.T) {
	t.Parallel()

	const chunks = 4
	released := make([]chan struct{}, chunks)
	for i := range released {
		released[i] = make(chan struct{})
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if !assert.True(t, ok, "ResponseWriter must implement http.Flusher") {
			return
		}
		for i := range chunks {
			<-released[i]
			_, err := fmt.Fprintf(w, "data: chunk-%d\n\n", i)
			if !assert.NoError(t, err) {
				return
			}
			flusher.Flush()
		}
	})

	rt, err := aibridged.NewTransportFactory(handler).TransportFor("openai", aibridge.SourceAgents)
	require.NoError(t, err)

	ctx := aibridge.WithDelegatedAPIKeyID(testutil.Context(t, testutil.WaitShort), "test-key-id")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://aibridge/stream", nil)
	require.NoError(t, err)

	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	br := bufio.NewReader(resp.Body)
	for i := range chunks {
		close(released[i])
		dataLine, err := br.ReadString('\n')
		require.NoError(t, err)
		require.Equal(t, fmt.Sprintf("data: chunk-%d\n", i), dataLine)
		// Consume blank-line separator.
		_, err = br.ReadString('\n')
		require.NoError(t, err)
	}
}

// Canceling the request context must surface as a body-read error, matching
// real-network behavior, and the handler must observe the cancellation
// through its own request context.
func TestInMemoryRoundTripper_CancelCloses(t *testing.T) {
	t.Parallel()

	handlerCtxObserved := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-r.Context().Done()
		close(handlerCtxObserved)
	})

	rt, err := aibridged.NewTransportFactory(handler).TransportFor("openai", aibridge.SourceAgents)
	require.NoError(t, err)

	parentCtx := testutil.Context(t, testutil.WaitShort)
	cancelCause := xerrors.New("caller canceled")
	ctx, cancel := context.WithCancelCause(parentCtx)
	ctx = aibridge.WithDelegatedAPIKeyID(ctx, "test-key-id")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://aibridge/stream", nil)
	require.NoError(t, err)

	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	cancel(cancelCause)
	_, err = io.ReadAll(resp.Body)
	require.ErrorIs(t, err, cancelCause)

	select {
	case <-handlerCtxObserved:
	case <-parentCtx.Done():
		t.Fatal("handler did not observe context cancellation")
	}
}

// Many independent in-flight requests on a shared handler must not interfere.
func TestInMemoryRoundTripper_ConcurrentRequests(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	})

	rt, err := aibridged.NewTransportFactory(handler).TransportFor("openai", aibridge.SourceAgents)
	require.NoError(t, err)

	const n = 16
	errs := make(chan error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			payload := fmt.Sprintf("payload-%d", i)
			ctx := aibridge.WithDelegatedAPIKeyID(testutil.Context(t, testutil.WaitShort), "test-key-id")
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://aibridge/echo", strings.NewReader(payload))
			if err != nil {
				errs <- err
				return
			}
			resp, err := rt.RoundTrip(req)
			if err != nil {
				errs <- err
				return
			}
			defer resp.Body.Close()
			got, err := io.ReadAll(resp.Body)
			if err != nil {
				errs <- err
				return
			}
			if string(got) != payload {
				errs <- xerrors.Errorf("payload mismatch: want %q got %q", payload, string(got))
				return
			}
			errs <- nil
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
}

// A panicking handler must not crash the process; it should produce a 500
// response with an error on the body read, mirroring net/http.Server behavior.
func TestInMemoryRoundTripper_HandlerPanic(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("unexpected nil pointer")
	})

	rt, err := aibridged.NewTransportFactory(handler).TransportFor("openai", aibridge.SourceAgents)
	require.NoError(t, err)

	ctx := aibridge.WithDelegatedAPIKeyID(testutil.Context(t, testutil.WaitShort), "test-key-id")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://aibridge/panic", nil)
	require.NoError(t, err)

	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	_, err = io.ReadAll(resp.Body)
	require.Error(t, err)
	require.Contains(t, err.Error(), "handler panicked")
}

func TestInMemoryRoundTripper_HandlerAbortBeforeHeaders(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	})
	rt, err := aibridged.NewTransportFactory(handler).TransportFor("openai", aibridge.SourceAgents)
	require.NoError(t, err)

	ctx := aibridge.WithDelegatedAPIKeyID(testutil.Context(t, testutil.WaitShort), "test-key-id")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://aibridge/abort", nil)
	require.NoError(t, err)
	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotNil(t, resp.Header)
	resp.Header.Set("X-Test", "writable")
	require.Equal(t, "writable", resp.Header.Get("X-Test"))
	_, err = io.ReadAll(resp.Body)
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
}

// The in-memory transport must reject any RoundTrip whose context does not
// carry a delegated API key ID. The handler relies on this invariant to know
// the request has a delegated identity attached.
func TestInMemoryRoundTripper_RequiresDelegatedAPIKeyID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		withCtx func(context.Context) context.Context
		wantErr bool
	}{
		{
			name:    "missing delegated key ID",
			withCtx: func(ctx context.Context) context.Context { return ctx },
			wantErr: true,
		},
		{
			name: "empty delegated key ID",
			withCtx: func(ctx context.Context) context.Context {
				return aibridge.WithDelegatedAPIKeyID(ctx, "")
			},
			wantErr: true,
		},
		{
			name: "valid delegated key ID",
			withCtx: func(ctx context.Context) context.Context {
				return aibridge.WithDelegatedAPIKeyID(ctx, "test-key-id")
			},
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			handlerCalled := make(chan struct{}, 1)
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handlerCalled <- struct{}{}
				w.WriteHeader(http.StatusOK)
			})

			rt, err := aibridged.NewTransportFactory(handler).TransportFor("openai", aibridge.SourceAgents)
			require.NoError(t, err)

			ctx := tc.withCtx(testutil.Context(t, testutil.WaitShort))
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://aibridge/v1/test", nil)
			require.NoError(t, err)

			resp, err := rt.RoundTrip(req)
			if tc.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), "WithDelegatedAPIKeyID")
				// Handler must not have been invoked.
				select {
				case <-handlerCalled:
					t.Fatal("handler invoked despite transport rejecting the request")
				default:
				}
				return
			}
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)
		})
	}
}

func TestInMemoryRoundTripper_CloseCancelsServedRequestOnly(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	canceled := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte("chunk"))
		if !assert.NoError(t, err) {
			return
		}
		close(started)
		<-r.Context().Done()
		close(canceled)
	})

	rt, err := aibridged.NewTransportFactory(handler).TransportFor("openai", aibridge.SourceAgents)
	require.NoError(t, err)

	parentCtx := testutil.Context(t, testutil.WaitShort)
	ctx := aibridge.WithDelegatedAPIKeyID(parentCtx, "test-key-id")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://aibridge/stream", nil)
	require.NoError(t, err)

	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	buf := make([]byte, len("chunk"))
	_, err = io.ReadFull(resp.Body, buf)
	require.NoError(t, err)
	require.Equal(t, "chunk", string(buf))
	testutil.TryReceive(parentCtx, t, started)

	require.NoError(t, resp.Body.Close())
	testutil.TryReceive(parentCtx, t, canceled)
	require.NoError(t, parentCtx.Err(), "closing a response must not cancel the caller context")
}

func TestInMemoryRoundTripper_ProxyCloseCancelsStalledUpstream(t *testing.T) {
	t.Parallel()

	upstreamCanceled := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte("chunk"))
		if !assert.NoError(t, err) {
			return
		}
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-r.Context().Done()
		close(upstreamCanceled)
	}))
	t.Cleanup(upstream.Close)

	rec := &transportRecorder{}
	router, err := proxy.NewRouter([]aibridgecore.Provider{
		aibridgecore.NewOpenAIProvider(config.OpenAI{BaseURL: upstream.URL}),
	}, rec, slogtest.Make(t, nil), nil, nil)
	require.NoError(t, err)
	t.Cleanup(router.CloseIdleConnections)

	rt, err := aibridged.NewTransportFactory(http.StripPrefix(aibridge.AIGatewayRootPath, router)).TransportFor("openai", aibridge.SourceAgents)
	require.NoError(t, err)
	parentCtx := testutil.Context(t, testutil.WaitShort)
	ctx := aibridge.WithDelegatedAPIKeyID(parentCtx, "key-id")
	ctx = aibridgecore.AsActor(ctx, "actor-id", nil)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://upstream/v1/chat/completions", strings.NewReader(`{}`))
	require.NoError(t, err)

	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	buf := make([]byte, len("chunk"))
	_, err = io.ReadFull(resp.Body, buf)
	require.NoError(t, err)
	require.Equal(t, "chunk", string(buf))
	require.NoError(t, resp.Body.Close())

	testutil.TryReceive(parentCtx, t, upstreamCanceled)
	require.NoError(t, parentCtx.Err())
	var end *recorder.InterceptionRecordEnded
	require.Eventually(t, func() bool {
		starts, ends := rec.records()
		if len(starts) != 1 {
			return false
		}
		end = ends[starts[0].ID]
		return end != nil
	}, testutil.WaitShort, testutil.IntervalFast)
	require.Equal(t, recorder.ErrorTypeUnknown, end.ErrorType)
	require.Equal(t, context.Canceled.Error(), end.ErrorMessage)
}

func TestInMemoryRoundTripper_ProxyForwardsNilBody(t *testing.T) {
	t.Parallel()

	upstreamCalled := make(chan struct{}, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.Empty(t, body)
		upstreamCalled <- struct{}{}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(upstream.Close)

	router, err := proxy.NewRouter([]aibridgecore.Provider{
		aibridgecore.NewCopilotProvider(config.Copilot{BaseURL: upstream.URL}),
	}, &transportRecorder{}, slogtest.Make(t, nil), nil, nil)
	require.NoError(t, err)
	t.Cleanup(router.CloseIdleConnections)

	rt, err := aibridged.NewTransportFactory(http.StripPrefix(aibridge.AIGatewayRootPath, router)).TransportFor("copilot", aibridge.SourceAgents)
	require.NoError(t, err)
	ctx := aibridge.WithDelegatedAPIKeyID(testutil.Context(t, testutil.WaitShort), "key-id")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://upstream/other", nil)
	require.NoError(t, err)
	require.Nil(t, req.Body)

	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	testutil.TryReceive(ctx, t, upstreamCalled)
}

func TestInMemoryRoundTripper_PassthroughTruncatedStreamReturnsReadError(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("chunk"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	t.Cleanup(upstream.Close)

	router, err := proxy.NewRouter([]aibridgecore.Provider{
		aibridgecore.NewCopilotProvider(config.Copilot{BaseURL: upstream.URL}),
	}, &transportRecorder{}, slogtest.Make(t, nil), nil, nil)
	require.NoError(t, err)
	t.Cleanup(router.CloseIdleConnections)

	rt, err := aibridged.NewTransportFactory(http.StripPrefix(aibridge.AIGatewayRootPath, router)).TransportFor("copilot", aibridge.SourceAgents)
	require.NoError(t, err)
	ctx := aibridge.WithDelegatedAPIKeyID(testutil.Context(t, testutil.WaitShort), "key-id")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://upstream/other", nil)
	require.NoError(t, err)

	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	_, readErr := io.ReadAll(resp.Body)
	require.ErrorIs(t, readErr, io.ErrUnexpectedEOF)
	require.NoError(t, resp.Body.Close())
}

func TestInMemoryRoundTripper_ProxyTruncatedStreamReturnsReadErrorAndEnds(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("chunk"))
	}))
	t.Cleanup(upstream.Close)

	rec := &transportRecorder{}
	router, err := proxy.NewRouter([]aibridgecore.Provider{
		aibridgecore.NewOpenAIProvider(config.OpenAI{BaseURL: upstream.URL}),
	}, rec, slogtest.Make(t, nil), nil, nil)
	require.NoError(t, err)
	t.Cleanup(router.CloseIdleConnections)

	rt, err := aibridged.NewTransportFactory(http.StripPrefix(aibridge.AIGatewayRootPath, router)).TransportFor("openai", aibridge.SourceAgents)
	require.NoError(t, err)
	ctx := aibridge.WithDelegatedAPIKeyID(testutil.Context(t, testutil.WaitShort), "key-id")
	ctx = aibridgecore.AsActor(ctx, "actor-id", nil)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://upstream/v1/chat/completions", strings.NewReader(`{}`))
	require.NoError(t, err)

	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	body, readErr := io.ReadAll(resp.Body)
	require.ErrorIs(t, readErr, io.ErrUnexpectedEOF)
	require.Equal(t, "chunk", string(body))
	require.NoError(t, resp.Body.Close())

	var end *recorder.InterceptionRecordEnded
	require.Eventually(t, func() bool {
		starts, ends := rec.records()
		if len(starts) != 1 {
			return false
		}
		end = ends[starts[0].ID]
		return end != nil
	}, testutil.WaitShort, testutil.IntervalFast)
	require.Equal(t, recorder.ErrorTypeUnknown, end.ErrorType)
	require.Equal(t, io.ErrUnexpectedEOF.Error(), end.ErrorMessage)
}

// A handler that returns without writing must not block RoundTrip; the caller
// gets a zero-length 200 OK.
func TestInMemoryRoundTripper_HandlerReturnsWithoutWriting(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	rt, err := aibridged.NewTransportFactory(handler).TransportFor("openai", aibridge.SourceAgents)
	require.NoError(t, err)

	ctx := aibridge.WithDelegatedAPIKeyID(testutil.Context(t, testutil.WaitShort), "test-key-id")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://aibridge/noop", nil)
	require.NoError(t, err)

	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Empty(t, body)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotNil(t, resp.Header)
	resp.Header.Set("X-Test", "writable")
	require.Equal(t, "writable", resp.Header.Get("X-Test"))
}
