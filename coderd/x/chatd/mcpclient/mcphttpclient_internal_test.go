package mcpclient

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
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

type staticRoundTripper struct {
	body          string
	contentLength int64
}

func (s *staticRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode:    http.StatusOK,
		Body:          io.NopCloser(strings.NewReader(s.body)),
		ContentLength: s.contentLength,
	}, nil
}

func TestMaxResponseBodyRoundTripper(t *testing.T) {
	t.Parallel()

	newReq := func(t *testing.T) *http.Request {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://example.invalid/", nil)
		require.NoError(t, err)
		return req
	}

	t.Run("ExactlyAtCapPasses", func(t *testing.T) {
		t.Parallel()
		body := strings.Repeat("a", 16)
		rt := &maxResponseBodyRoundTripper{
			base:     &staticRoundTripper{body: body, contentLength: -1},
			maxBytes: 16,
		}
		resp, err := rt.RoundTrip(newReq(t))
		require.NoError(t, err)
		defer resp.Body.Close()
		got, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, body, string(got))
	})

	t.Run("UnknownLengthOverCapFailsOnRead", func(t *testing.T) {
		t.Parallel()
		rt := &maxResponseBodyRoundTripper{
			base:     &staticRoundTripper{body: strings.Repeat("a", 17), contentLength: -1},
			maxBytes: 16,
		}
		resp, err := rt.RoundTrip(newReq(t))
		require.NoError(t, err)
		defer resp.Body.Close()
		got, err := io.ReadAll(resp.Body)
		require.ErrorIs(t, err, errChatAttachedResponseTooLarge)
		require.LessOrEqual(t, len(got), 16)
	})

	t.Run("ContentLengthOverCapFailsBeforeRead", func(t *testing.T) {
		t.Parallel()
		rt := &maxResponseBodyRoundTripper{
			base:     &staticRoundTripper{body: strings.Repeat("a", 17), contentLength: 17},
			maxBytes: 16,
		}
		resp, err := rt.RoundTrip(newReq(t)) //nolint:bodyclose // resp is nil on error
		require.ErrorIs(t, err, errChatAttachedResponseTooLarge)
		require.Nil(t, resp)
	})

	t.Run("ChatAttachedClientWrapsTransport", func(t *testing.T) {
		t.Parallel()
		base := NewHTTPClient(nil)
		client := chatAttachedHTTPClient(base)
		require.NotSame(t, base, client)
		wrapped, ok := client.Transport.(*maxResponseBodyRoundTripper)
		require.True(t, ok)
		require.Same(t, base.Transport, wrapped.base)
		require.EqualValues(t, maxChatAttachedHTTPResponseBytes, wrapped.maxBytes)
		require.Equal(t, base.Timeout, client.Timeout)

		fallback := chatAttachedHTTPClient(nil)
		_, ok = fallback.Transport.(*maxResponseBodyRoundTripper)
		require.True(t, ok)
	})
}
