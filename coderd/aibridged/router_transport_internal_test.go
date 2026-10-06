package aibridged

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/testutil"
)

// A provider that fails forwarding construction after earlier providers built
// their transports leaves the previous router serving.
func TestReplaceProvidersMalformedBaseURLRetainsRouter(t *testing.T) {
	t.Parallel()

	server := newProxyTestServer(t, http.NotFoundHandler())
	ctx := testutil.Context(t, testutil.WaitShort)
	retained := server.backend.Load().proxyRouter
	require.NotNil(t, retained)

	err := server.ReplaceProviders(ctx, []aibridge.Provider{
		aibridge.NewOpenAIProvider(config.OpenAI{Name: "valid", BaseURL: "http://upstream.test"}),
		aibridge.NewOpenAIProvider(config.OpenAI{Name: "malformed", BaseURL: "http://upstream.test/%zz"}),
	})
	require.ErrorContains(t, err, `configure provider "malformed" base URL`)
	require.Same(t, retained, server.backend.Load().proxyRouter, "a failed snapshot must not replace the router")

	handler, err := server.GetRequestHandler(ctx, Request{})
	require.NoError(t, err)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/openai/v1/models", nil))
	require.Equal(t, http.StatusNotFound, rec.Code, "the retained router must keep proxying upstream")
}
