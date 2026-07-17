package aibridged_test

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/aibridged"
	"github.com/coder/coder/v2/testutil"
)

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
	ctx, cancel := context.WithCancel(parentCtx)
	ctx = aibridge.WithDelegatedAPIKeyID(ctx, "test-key-id")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://aibridge/stream", nil)
	require.NoError(t, err)

	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	cancel()
	_, err = io.ReadAll(resp.Body)
	require.Error(t, err)

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
}
