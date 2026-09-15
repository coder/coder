package agentmcp

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

// headerRecorder is an HTTP handler that records the headers of every
// request it receives, keyed by request path.
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

// requests returns the recorded headers for path.
func (r *headerRecorder) requests(path string) []http.Header {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]http.Header(nil), r.seen[path]...)
}

// all returns every recorded header set.
func (r *headerRecorder) all() []http.Header {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []http.Header
	for _, hs := range r.seen {
		out = append(out, hs...)
	}
	return out
}

func TestHTTPClientWithHeaders(t *testing.T) {
	t.Parallel()

	const (
		headerName  = "X-Mcp-Test"
		headerValue = "configured"
	)

	// other is a second origin. origin redirects /redirect there.
	otherRec := &headerRecorder{}
	other := httptest.NewServer(otherRec)
	t.Cleanup(other.Close)

	originRec := &headerRecorder{}
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/redirect" {
			http.Redirect(w, req, other.URL+"/landed", http.StatusFound)
			return
		}
		originRec.ServeHTTP(w, req)
	}))
	t.Cleanup(origin.Close)

	client, err := httpClientWithHeaders(origin.URL+"/mcp", map[string]string{headerName: headerValue})
	require.NoError(t, err)

	t.Run("SameOriginGetsHeader", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, origin.URL+"/same", nil)
		require.NoError(t, err)
		resp, err := client.Do(req)
		require.NoError(t, err)
		_ = resp.Body.Close()

		seen := originRec.requests("/same")
		require.Len(t, seen, 1)
		assert.Equal(t, []string{headerValue}, seen[0].Values(headerName))
	})

	t.Run("ClientHeaderWins", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, origin.URL+"/client", nil)
		require.NoError(t, err)
		req.Header.Set(headerName, "from-client")
		resp, err := client.Do(req)
		require.NoError(t, err)
		_ = resp.Body.Close()

		seen := originRec.requests("/client")
		require.Len(t, seen, 1)
		assert.Equal(t, []string{"from-client"}, seen[0].Values(headerName))
	})

	t.Run("OtherOriginNeverGetsHeader", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, other.URL+"/direct", nil)
		require.NoError(t, err)
		resp, err := client.Do(req)
		require.NoError(t, err)
		_ = resp.Body.Close()

		seen := otherRec.requests("/direct")
		require.Len(t, seen, 1)
		assert.Empty(t, seen[0].Values(headerName), "configured header leaked to another origin")
	})

	t.Run("CrossOriginRedirectRefused", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, origin.URL+"/redirect", nil)
		require.NoError(t, err)
		resp, err := client.Do(req)
		if resp != nil {
			_ = resp.Body.Close()
		}
		require.ErrorContains(t, err, "refusing redirect")

		assert.Empty(t, otherRec.requests("/landed"), "redirect target must not be requested")
		for _, h := range otherRec.all() {
			assert.Empty(t, h.Values(headerName), "configured header forwarded to another origin")
		}
	})
}

func TestSameOriginRedirectPolicy(t *testing.T) {
	t.Parallel()

	origin := requestOrigin{scheme: "https", host: "mcp.example.com"}
	policy := sameOriginRedirectPolicy(origin)

	newReq := func(t *testing.T, rawURL string) *http.Request {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, rawURL, nil)
		require.NoError(t, err)
		return req
	}

	tests := []struct {
		name    string
		target  string
		wantErr bool
	}{
		{name: "SameOrigin", target: "https://mcp.example.com/other/path"},
		{name: "SameOriginDifferentCase", target: "https://MCP.example.com/x"},
		{name: "HTTPSToHTTP", target: "http://mcp.example.com/x", wantErr: true},
		{name: "OtherHost", target: "https://evil.example.com/x", wantErr: true},
		{name: "OtherPort", target: "https://mcp.example.com:8443/x", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := policy(newReq(t, tt.target), []*http.Request{newReq(t, "https://mcp.example.com/mcp")})
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}

	t.Run("TooManyRedirects", func(t *testing.T) {
		t.Parallel()
		via := make([]*http.Request, maxRedirects)
		for i := range via {
			via[i] = newReq(t, "https://mcp.example.com/mcp")
		}
		assert.ErrorContains(t, policy(newReq(t, "https://mcp.example.com/x"), via), "stopped after")
	})
}

func TestHTTPClientWithHeaders_InvalidURL(t *testing.T) {
	t.Parallel()

	_, err := httpClientWithHeaders("http://[::1", nil)
	require.Error(t, err)
}
