package agentmcp

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

type headerRecorder struct {
	mu   sync.Mutex
	seen map[string][]http.Header
}

func (r *headerRecorder) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	if r.seen == nil {
		r.seen = make(map[string][]http.Header)
	}
	r.seen[req.URL.Path] = append(r.seen[req.URL.Path], req.Header.Clone())
	r.mu.Unlock()
	w.WriteHeader(http.StatusOK)
}

func (r *headerRecorder) requests(path string) []http.Header {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]http.Header(nil), r.seen[path]...)
}

func TestHTTPClientWithHeaders(t *testing.T) {
	t.Parallel()

	const (
		headerName  = "X-Mcp-Test"
		headerValue = "configured"
	)

	otherRec := &headerRecorder{}
	otherSrv := httptest.NewServer(otherRec)
	t.Cleanup(otherSrv.Close)

	originRec := &headerRecorder{}
	originSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/redirect-other":
			http.Redirect(w, req, otherSrv.URL+"/landed", http.StatusFound)
			return
		case "/redirect-same":
			http.Redirect(w, req, "/landed-same", http.StatusFound)
			return
		}
		originRec.ServeHTTP(w, req)
	}))
	t.Cleanup(originSrv.Close)

	client, err := httpClientWithHeaders(originSrv.URL+"/mcp", map[string]string{headerName: headerValue})
	require.NoError(t, err)

	get := func(t *testing.T, rawURL string, header http.Header) {
		t.Helper()
		ctx := testutil.Context(t, testutil.WaitShort)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		require.NoError(t, err)
		for k, v := range header {
			req.Header[k] = v
		}
		resp, err := client.Do(req)
		require.NoError(t, err)
		_ = resp.Body.Close()
	}

	t.Run("SameOriginGetsHeader", func(t *testing.T) {
		t.Parallel()
		get(t, originSrv.URL+"/same", nil)

		seen := originRec.requests("/same")
		require.Len(t, seen, 1)
		assert.Equal(t, []string{headerValue}, seen[0].Values(headerName))
	})

	t.Run("ClientHeaderWins", func(t *testing.T) {
		t.Parallel()
		get(t, originSrv.URL+"/client", http.Header{headerName: {"from-client"}})

		seen := originRec.requests("/client")
		require.Len(t, seen, 1)
		assert.Equal(t, []string{"from-client"}, seen[0].Values(headerName))
	})

	t.Run("OtherOriginWithoutHeader", func(t *testing.T) {
		t.Parallel()
		get(t, otherSrv.URL+"/direct", nil)

		seen := otherRec.requests("/direct")
		require.Len(t, seen, 1)
		assert.Empty(t, seen[0].Values(headerName))
	})

	t.Run("CrossOriginRedirectFollowedWithoutHeader", func(t *testing.T) {
		t.Parallel()
		get(t, originSrv.URL+"/redirect-other", nil)

		seen := otherRec.requests("/landed")
		require.Len(t, seen, 1)
		assert.Empty(t, seen[0].Values(headerName))
	})

	t.Run("SameOriginRedirectKeepsHeader", func(t *testing.T) {
		t.Parallel()
		get(t, originSrv.URL+"/redirect-same", nil)

		seen := originRec.requests("/landed-same")
		require.Len(t, seen, 1)
		assert.Equal(t, []string{headerValue}, seen[0].Values(headerName))
	})
}

func TestOriginOf(t *testing.T) {
	t.Parallel()

	base := originOf(mustParseURL(t, "https://mcp.example.com/mcp"))
	tests := []struct {
		name   string
		target string
		same   bool
	}{
		{name: "SamePath", target: "https://mcp.example.com/other", same: true},
		{name: "HostCase", target: "https://MCP.example.com/x", same: true},
		{name: "ExplicitDefaultPort", target: "https://mcp.example.com:443/x", same: true},
		{name: "HTTPSToHTTP", target: "http://mcp.example.com/x"},
		{name: "OtherPort", target: "https://mcp.example.com:8443/x"},
		{name: "OtherHost", target: "https://evil.example.com/x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.same, originOf(mustParseURL(t, tt.target)) == base)
		})
	}
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	require.NoError(t, err)
	return u
}

func TestHTTPClientWithHeaders_InvalidURLOmitsURL(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		"http://user:pa55word@[::1/mcp?api_key=SECRETQ",
		"https://deploy:Zm9v/YmFy@mcp.example.com/mcp",
	} {
		_, err := httpClientWithHeaders(raw, nil)
		require.EqualError(t, err, "invalid server url", raw)
	}
}
