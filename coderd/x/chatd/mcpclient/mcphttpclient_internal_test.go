package mcpclient

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/safedial"
)

func TestHTTPClientWithHeadersRejectsCrossOriginRedirect(t *testing.T) {
	t.Parallel()

	var targetHits atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		targetHits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()

	base := safedial.NewHTTPClient(source.Client(), safedial.WithAllowedPrefixes(
		netip.MustParsePrefix("127.0.0.0/8"),
		netip.MustParsePrefix("::1/128"),
	))
	client := httpClientWithHeaders(base, map[string]string{"Authorization": "Bearer secret"})
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, source.URL, nil)
	require.NoError(t, err)
	resp, err := client.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	require.Error(t, err)
	require.Zero(t, targetHits.Load())
}

func TestHTTPClientWithHeadersGuardsClientWithoutTransport(t *testing.T) {
	t.Parallel()

	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := httpClientWithHeaders(&http.Client{}, nil)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, nil)
	require.NoError(t, err)
	resp, err := client.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	require.Error(t, err)
	require.Zero(t, hits.Load())
}

// TestMCPTransportTimeouts guards the transport hardening: MCP
// traffic must never ride a transport without dial and
// response-header bounds, or a black-holed server holds
// connections until the enclosing context expires.
func TestMCPTransportTimeouts(t *testing.T) {
	t.Parallel()

	client := NewHTTPClient(nil)
	tr, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	require.NotNil(t, tr.DialContext)
	require.Equal(t, responseHeaderTimeout, tr.ResponseHeaderTimeout)
	// The response-header bound must not undercut the tool-call
	// budget, or slow JSON-response tools within budget would be
	// killed at the HTTP layer.
	require.GreaterOrEqual(t, responseHeaderTimeout, toolCallTimeout)

	// Clients are built per call with private transports, so closed
	// test servers cannot leave stale pooled connections behind for
	// later clients that reuse the same address.
	other := NewHTTPClient(nil)
	require.NotSame(t, client.Transport, other.Transport)
}
