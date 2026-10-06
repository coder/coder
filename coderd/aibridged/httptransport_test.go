package aibridged_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/aibridged"
	"github.com/coder/coder/v2/testutil"
)

func httpGatewayTransport(t *testing.T, target, key string) http.RoundTripper {
	t.Helper()
	u, err := url.Parse(target)
	require.NoError(t, err)
	rt, err := aibridged.NewHTTPTransportFactory(u, key).TransportFor("openai", aibridge.SourceAgents)
	require.NoError(t, err)
	return rt
}

func TestHTTPTransport_Delegation(t *testing.T) {
	t.Parallel()
	workspace := uuid.New()
	gateway := httptest.NewServer(aibridged.DelegationMiddleware("shared-key")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/prefix/api/v2/ai-gateway/openai/v1/a%2Fb", r.URL.EscapedPath())
		assert.Equal(t, "stream=true", r.URL.RawQuery)
		id, ok := aibridge.DelegatedAPIKeyIDFromContext(r.Context())
		assert.True(t, ok)
		assert.Equal(t, "synthetic-key", id)
		attr, ok := aibridge.DelegatedAttributionFromContext(r.Context())
		assert.True(t, ok)
		assert.Equal(t, workspace, attr.WorkspaceID)
		assert.Equal(t, aibridge.SourceAgents, aibridge.SourceFromContext(r.Context()))
		assert.Equal(t, "Bearer byok", r.Header.Get("Authorization"))
		for _, name := range []string{aibridge.HeaderGatewayKey, aibridge.HeaderDelegatedAPIKeyID, aibridge.HeaderDelegatedWorkspace, aibridge.HeaderDelegatedSource} {
			assert.Empty(t, r.Header.Get(name))
		}
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		assert.Equal(t, "request", string(body))
		_, _ = io.WriteString(w, "response")
	})))
	t.Cleanup(gateway.Close)
	ctx := aibridge.WithDelegatedAPIKeyID(testutil.Context(t, testutil.WaitShort), "synthetic-key")
	ctx = aibridge.WithDelegatedAttribution(ctx, aibridge.Attribution{WorkspaceID: workspace})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://upstream/v1/a%2Fb?stream=true", strings.NewReader("request"))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer byok")
	req.Header.Set(aibridge.HeaderDelegatedWorkspace, uuid.NewString())
	originalHeaders := req.Header.Clone()
	resp, err := httpGatewayTransport(t, gateway.URL+"/prefix", "shared-key").RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, "response", string(body))
	require.Equal(t, "http://upstream/v1/a%2Fb?stream=true", req.URL.String())
	require.Equal(t, originalHeaders, req.Header)
}

func TestDelegationMiddleware_RejectsInvalidMetadata(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		modify func(http.Header)
		status int
	}{
		{"missing key", func(h http.Header) { h.Del(aibridge.HeaderGatewayKey) }, 401},
		{"wrong key", func(h http.Header) { h.Set(aibridge.HeaderGatewayKey, "wrong") }, 401},
		{"duplicate key", func(h http.Header) { h.Add(aibridge.HeaderGatewayKey, "shared-key") }, 401},
		{"missing identity", func(h http.Header) { h.Del(aibridge.HeaderDelegatedAPIKeyID) }, 400},
		{"duplicate identity", func(h http.Header) { h.Add(aibridge.HeaderDelegatedAPIKeyID, "other") }, 400},
		{"missing source", func(h http.Header) { h.Del(aibridge.HeaderDelegatedSource) }, 400},
		{"invalid workspace", func(h http.Header) { h.Set(aibridge.HeaderDelegatedWorkspace, "invalid") }, 400},
		{"duplicate workspace", func(h http.Header) { h.Add(aibridge.HeaderDelegatedWorkspace, uuid.NewString()) }, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			handler := aibridged.DelegationMiddleware("shared-key")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Error("invalid delegation reached provider dispatch")
			}))
			req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", nil)
			req.Header.Set(aibridge.HeaderGatewayKey, "shared-key")
			req.Header.Set(aibridge.HeaderDelegatedAPIKeyID, "synthetic-key")
			req.Header.Set(aibridge.HeaderDelegatedSource, "agents")
			req.Header.Set(aibridge.HeaderDelegatedWorkspace, uuid.NewString())
			// Invalid delegation must not fall through to ordinary client auth.
			req.Header.Set("Authorization", "Bearer valid-user-token")
			tc.modify(req.Header)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			require.Equal(t, tc.status, rec.Code)
			if tc.status == 401 {
				require.Equal(t, aibridge.GatewayKeyMismatchCode, rec.Header().Get(aibridge.HeaderGatewayError))
			}
		})
	}
}

func TestDelegationMiddleware_OrdinaryClient(t *testing.T) {
	t.Parallel()
	called := false
	handler := aibridged.DelegationMiddleware("shared-key")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		_, delegated := aibridge.DelegatedAPIKeyIDFromContext(r.Context())
		require.False(t, delegated)
		require.Equal(t, "Bearer user-token", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer user-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	require.True(t, called)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestHTTPTransport_AuthenticationErrors(t *testing.T) {
	t.Parallel()
	for _, mismatch := range []bool{false, true} {
		t.Run(map[bool]string{false: "provider401", true: "gatewayMismatch"}[mismatch], func(t *testing.T) {
			t.Parallel()
			var calls atomic.Int32
			gateway := httptest.NewServer(aibridged.DelegationMiddleware("shared-key")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				// Even a provider mimicking the infrastructure error marker
				// must remain an ordinary provider authentication failure.
				w.Header().Set(aibridge.HeaderGatewayError, aibridge.GatewayKeyMismatchCode)
				http.Error(w, "provider authentication failed", http.StatusUnauthorized)
			})))
			t.Cleanup(gateway.Close)
			key := "shared-key"
			if mismatch {
				key = "wrong"
			}
			ctx := aibridge.WithDelegatedAPIKeyID(testutil.Context(t, testutil.WaitShort), "synthetic-key")
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://upstream/v1/chat/completions", strings.NewReader("{}"))
			require.NoError(t, err)
			resp, err := httpGatewayTransport(t, gateway.URL, key).RoundTrip(req)
			if mismatch {
				require.ErrorIs(t, err, aibridge.ErrGatewayKeyMismatch)
				require.Nil(t, resp)
				require.Zero(t, calls.Load())
			} else {
				require.NoError(t, err)
				defer resp.Body.Close()
				require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
				require.Empty(t, resp.Header.Get(aibridge.HeaderGatewayError))
				require.EqualValues(t, 1, calls.Load())
			}
		})
	}
}

func TestHTTPTransport_StreamingCancellation(t *testing.T) {
	t.Parallel()
	canceled := make(chan struct{})
	gateway := httptest.NewServer(aibridged.DelegationMiddleware("shared-key")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: first\n\n")
		assert.NoError(t, http.NewResponseController(w).Flush())
		<-r.Context().Done()
		close(canceled)
	})))
	t.Cleanup(gateway.Close)
	ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitShort))
	defer cancel()
	ctx = aibridge.WithDelegatedAPIKeyID(ctx, "synthetic-key")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://upstream/v1/chat/completions", strings.NewReader("{}"))
	require.NoError(t, err)
	resp, err := httpGatewayTransport(t, gateway.URL, "shared-key").RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	first := make([]byte, len("data: first\n\n"))
	_, err = io.ReadFull(resp.Body, first)
	require.NoError(t, err)
	require.Equal(t, "data: first\n\n", string(first))
	cancel()
	select {
	case <-canceled:
	case <-testutil.Context(t, testutil.WaitShort).Done():
		t.Fatal("Gateway did not observe cancellation")
	}
}

func TestHTTPTransport_Replicas(t *testing.T) {
	t.Parallel()
	var hits [2]atomic.Int32
	var targets []*url.URL
	for i := range hits {
		gateway := httptest.NewServer(aibridged.DelegationMiddleware("shared-key")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			hits[i].Add(1)
			w.WriteHeader(http.StatusOK)
		})))
		t.Cleanup(gateway.Close)
		u, err := url.Parse(gateway.URL)
		require.NoError(t, err)
		targets = append(targets, u)
	}
	var next atomic.Uint32
	lb := httptest.NewServer(&httputil.ReverseProxy{Rewrite: func(r *httputil.ProxyRequest) {
		r.SetURL(targets[(next.Add(1)-1)%2])
	}})
	t.Cleanup(lb.Close)
	for range 2 {
		// Independent coderd factories share only the URL and configured key.
		rt := httpGatewayTransport(t, lb.URL, "shared-key")
		for range 2 {
			ctx := aibridge.WithDelegatedAPIKeyID(testutil.Context(t, testutil.WaitShort), uuid.NewString())
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://upstream/v1/chat/completions", strings.NewReader("{}"))
			require.NoError(t, err)
			resp, err := rt.RoundTrip(req)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, resp.StatusCode)
			require.NoError(t, resp.Body.Close())
		}
	}
	for i := range hits {
		require.EqualValues(t, 2, hits[i].Load())
	}
}

func TestHTTPTransport_NoRedirectReplay(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Redirect(w, r, "/other", http.StatusTemporaryRedirect)
	}))
	t.Cleanup(gateway.Close)
	ctx := aibridge.WithDelegatedAPIKeyID(testutil.Context(t, testutil.WaitShort), "synthetic-key")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://upstream/v1/chat/completions", strings.NewReader("{}"))
	require.NoError(t, err)
	client := &http.Client{Transport: httpGatewayTransport(t, gateway.URL, "shared-key")}
	resp, err := client.Do(req)
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.ErrorContains(t, err, "must not redirect")
	require.EqualValues(t, 1, calls.Load())
}

func TestHTTPTransport_RequiresTrustedDelegation(t *testing.T) {
	t.Parallel()
	gateway := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("request without trusted identity reached the network")
	}))
	t.Cleanup(gateway.Close)
	req, err := http.NewRequestWithContext(testutil.Context(t, testutil.WaitShort), http.MethodPost, "http://upstream/v1/chat/completions", nil)
	require.NoError(t, err)
	req.Header.Set(aibridge.HeaderDelegatedAPIKeyID, "forged-identity")
	resp, err := httpGatewayTransport(t, gateway.URL, "shared-key").RoundTrip(req)
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.ErrorContains(t, err, "WithDelegatedAPIKeyID")
}
