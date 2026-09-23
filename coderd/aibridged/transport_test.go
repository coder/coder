package aibridged_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

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
