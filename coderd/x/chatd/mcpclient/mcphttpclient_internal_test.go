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
	client := httpClientWithHeaders(base, map[string]string{"Authorization": "Bearer secret"}, "")
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

	client := httpClientWithHeaders(&http.Client{}, nil, "")
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

func TestMCPSignatureVectors(t *testing.T) {
	t.Parallel()

	const signingSecret = "0123456789abcdef0123456789abcdef"
	tests := []struct {
		name      string
		timestamp string
		method    string
		url       string
		body      string
		headers   http.Header
		canonical string
		signature string
	}{
		{
			name:      "post with body",
			timestamp: "1700000000",
			method:    http.MethodPost,
			url:       "https://mcp.example.com/api/mcp",
			body:      `{"jsonrpc":"2.0","id":1,"method":"tools/call"}`,
			headers: http.Header{
				headerCoderOwnerID:     {"owner-1"},
				headerCoderChatID:      {"chat-1"},
				headerCoderSubchatID:   {"subchat-1"},
				headerCoderWorkspaceID: {"workspace-1"},
			},
			canonical: "v1\n1700000000\nPOST\n/api/mcp\n7423b5e8269f7d1be6b13214cb7ac414e4afa95ce9d4bf0590fa0e69f6978976\nowner=owner-1\nchat=chat-1\nsubchat=subchat-1\nworkspace=workspace-1",
			signature: "v1=50d37d544c31b98f7b3e5bdd25bb9742bb5c6f18a52c459c0228df20699bb8a1",
		},
		{
			name:      "get without body",
			timestamp: "1700000001",
			method:    http.MethodGet,
			url:       "https://mcp.example.com/api/mcp",
			headers: http.Header{
				headerCoderOwnerID:     {"owner-2"},
				headerCoderChatID:      {"chat-2"},
				headerCoderSubchatID:   {"subchat-2"},
				headerCoderWorkspaceID: {"workspace-2"},
			},
			canonical: "v1\n1700000001\nGET\n/api/mcp\ne3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855\nowner=owner-2\nchat=chat-2\nsubchat=subchat-2\nworkspace=workspace-2",
			signature: "v1=8dd946883ceb5c1f1aec023b37b38a978e0b386ecbdff337dc30daa426037ff3",
		},
		{
			name:      "absent optional headers",
			timestamp: "1700000002",
			method:    http.MethodPost,
			url:       "https://mcp.example.com/api/mcp",
			body:      `{}`,
			headers: http.Header{
				headerCoderOwnerID: {"owner-3"},
				headerCoderChatID:  {"chat-3"},
			},
			canonical: "v1\n1700000002\nPOST\n/api/mcp\n44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a\nowner=owner-3\nchat=chat-3\nsubchat=\nworkspace=",
			signature: "v1=5c2267a0875601f14ef5b6f6f9ce0cabdf5718af80b7ab950a16f71553332c58",
		},
		{
			name:      "query string",
			timestamp: "1700000003",
			method:    http.MethodGet,
			url:       "https://mcp.example.com/api/mcp?x=1&y=two",
			headers: http.Header{
				headerCoderOwnerID:     {"owner-4"},
				headerCoderChatID:      {"chat-4"},
				headerCoderWorkspaceID: {"workspace-4"},
			},
			canonical: "v1\n1700000003\nGET\n/api/mcp?x=1&y=two\ne3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855\nowner=owner-4\nchat=chat-4\nsubchat=\nworkspace=workspace-4",
			signature: "v1=8a59223017cfaebd56bc1606ee74104f141a381323605c70144b975edbe42a65",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var body io.Reader
			if tt.body != "" {
				body = strings.NewReader(tt.body)
			}
			req, err := http.NewRequestWithContext(t.Context(), tt.method, tt.url, body)
			require.NoError(t, err)
			req.Header = tt.headers.Clone()

			canonical := mcpSignatureCanonical(req, tt.timestamp, []byte(tt.body))
			require.Equal(t, tt.canonical, canonical)
			require.Equal(t, tt.signature, signMCPRequest(signingSecret, canonical))
		})
	}
}

func TestBufferRequestBodyRestoresBody(t *testing.T) {
	t.Parallel()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://mcp.example.com", nil)
	require.NoError(t, err)
	req.Body = io.NopCloser(strings.NewReader("request body"))
	req.GetBody = nil
	clone := req.Clone(req.Context())

	body, err := bufferRequestBody(req, clone)
	require.NoError(t, err)
	require.Equal(t, []byte("request body"), body)
	require.NotNil(t, req.GetBody)
	require.NotNil(t, clone.GetBody)

	cloneBody, err := io.ReadAll(clone.Body)
	require.NoError(t, err)
	require.Equal(t, []byte("request body"), cloneBody)

	retryBody, err := req.GetBody()
	require.NoError(t, err)
	defer retryBody.Close()
	retryBytes, err := io.ReadAll(retryBody)
	require.NoError(t, err)
	require.Equal(t, []byte("request body"), retryBytes)
}

func TestBufferRequestBodyRefreshesCloneBody(t *testing.T) {
	t.Parallel()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://mcp.example.com", strings.NewReader("request body"))
	require.NoError(t, err)
	require.NotNil(t, req.GetBody)

	for range 2 {
		clone := req.Clone(req.Context())
		body, err := bufferRequestBody(req, clone)
		require.NoError(t, err)
		require.Equal(t, []byte("request body"), body)

		cloneBody, err := io.ReadAll(clone.Body)
		require.NoError(t, err)
		require.Equal(t, []byte("request body"), cloneBody)
	}
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
