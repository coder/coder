package agentmcp

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
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

func (r *headerRecorder) headers(path string) []http.Header {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]http.Header(nil), r.seen[path]...)
}

// entrySink records log entries at warn level and above.
type entrySink struct {
	mu      sync.Mutex
	entries []slog.SinkEntry
}

func (s *entrySink) LogEntry(_ context.Context, e slog.SinkEntry) {
	if e.Level < slog.LevelWarn {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, e)
}

func (*entrySink) Sync() {}

func (s *entrySink) warnings() []slog.SinkEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]slog.SinkEntry(nil), s.entries...)
}

func fieldValue(e slog.SinkEntry, name string) any {
	for _, f := range e.Fields {
		if f.Name == name {
			return f.Value
		}
	}
	return nil
}

// headerTestServers starts an origin server and another-origin server.
// The origin server redirects /redirect-other to the other server's
// /landed and /redirect-same to its own /landed-same.
func headerTestServers(t *testing.T) (originSrv *httptest.Server, originRec *headerRecorder, otherSrv *httptest.Server, otherRec *headerRecorder) {
	t.Helper()
	otherRec = &headerRecorder{}
	otherSrv = httptest.NewServer(otherRec)
	t.Cleanup(otherSrv.Close)

	originRec = &headerRecorder{}
	originSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
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
	return originSrv, originRec, otherSrv, otherRec
}

func doGet(t *testing.T, client *http.Client, rawURL string, header http.Header) {
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

// TestHTTPClientWithHeaders pins the legacy .mcp.json behavior:
// configured headers go to every origin and override client headers.
func TestHTTPClientWithHeaders(t *testing.T) {
	t.Parallel()

	const (
		headerName  = "X-Mcp-Test"
		headerValue = "configured"
	)
	originSrv, originRec, otherSrv, otherRec := headerTestServers(t)
	client := httpClientWithHeaders(map[string]string{headerName: headerValue})

	t.Run("ConfiguredHeaderWins", func(t *testing.T) {
		t.Parallel()
		doGet(t, client, originSrv.URL+"/client", http.Header{headerName: {"from-client"}})

		seen := originRec.headers("/client")
		require.Len(t, seen, 1)
		assert.Equal(t, []string{headerValue}, seen[0].Values(headerName))
	})

	t.Run("OtherOriginGetsHeader", func(t *testing.T) {
		t.Parallel()
		doGet(t, client, otherSrv.URL+"/direct", nil)

		seen := otherRec.headers("/direct")
		require.Len(t, seen, 1)
		assert.Equal(t, []string{headerValue}, seen[0].Values(headerName))
	})

	t.Run("CrossOriginRedirectGetsHeader", func(t *testing.T) {
		t.Parallel()
		doGet(t, client, originSrv.URL+"/redirect-other", nil)

		seen := otherRec.headers("/landed")
		require.Len(t, seen, 1)
		assert.Equal(t, []string{headerValue}, seen[0].Values(headerName))
	})
}

func TestSameOriginHeaderClient(t *testing.T) {
	t.Parallel()

	const (
		headerName  = "X-Mcp-Test"
		headerValue = "configured"
	)
	originSrv, originRec, otherSrv, otherRec := headerTestServers(t)
	client, err := sameOriginHeaderClient(slogtest.Make(t, nil), originSrv.URL+"/mcp", map[string]string{headerName: headerValue})
	require.NoError(t, err)

	t.Run("SameOriginGetsHeader", func(t *testing.T) {
		t.Parallel()
		doGet(t, client, originSrv.URL+"/same", nil)

		seen := originRec.headers("/same")
		require.Len(t, seen, 1)
		assert.Equal(t, []string{headerValue}, seen[0].Values(headerName))
	})

	t.Run("ClientHeaderWins", func(t *testing.T) {
		t.Parallel()
		doGet(t, client, originSrv.URL+"/client", http.Header{headerName: {"from-client"}})

		seen := originRec.headers("/client")
		require.Len(t, seen, 1)
		assert.Equal(t, []string{"from-client"}, seen[0].Values(headerName))
	})

	t.Run("OtherOriginWithoutHeader", func(t *testing.T) {
		t.Parallel()
		doGet(t, client, otherSrv.URL+"/direct", nil)

		seen := otherRec.headers("/direct")
		require.Len(t, seen, 1)
		assert.Empty(t, seen[0].Values(headerName))
	})

	t.Run("CrossOriginRedirectFollowedWithoutHeader", func(t *testing.T) {
		t.Parallel()
		doGet(t, client, originSrv.URL+"/redirect-other", nil)

		seen := otherRec.headers("/landed")
		require.Len(t, seen, 1)
		assert.Empty(t, seen[0].Values(headerName))
	})

	t.Run("SameOriginRedirectKeepsHeader", func(t *testing.T) {
		t.Parallel()
		doGet(t, client, originSrv.URL+"/redirect-same", nil)

		seen := originRec.headers("/landed-same")
		require.Len(t, seen, 1)
		assert.Equal(t, []string{headerValue}, seen[0].Values(headerName))
	})
}

func TestSameOriginHeaderClient_SkipsTransportManagedHeaders(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			_, _ = io.WriteString(w, `{"ok":true}`)
			return
		}
		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		_, _ = io.WriteString(gz, `{"ok":true}`)
		_ = gz.Close()
	}))
	t.Cleanup(srv.Close)

	client, err := sameOriginHeaderClient(slogtest.Make(t, nil), srv.URL, map[string]string{
		"accept-encoding": "gzip",
		"Connection":      "close",
	})
	require.NoError(t, err)

	ctx := testutil.Context(t, testutil.WaitShort)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, `{"ok":true}`, string(body))
}

func TestSameOriginHeaderClient_WarnsOnceWithOrigins(t *testing.T) {
	t.Parallel()

	originSrv, _, otherSrv, _ := headerTestServers(t)
	sink := &entrySink{}
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).AppendSinks(sink)
	client, err := sameOriginHeaderClient(logger, originSrv.URL+"/mcp", map[string]string{"Authorization": "Bearer x"})
	require.NoError(t, err)

	doGet(t, client, originSrv.URL+"/same", nil)
	require.Empty(t, sink.warnings())

	doGet(t, client, otherSrv.URL+"/msg?session=SECRETQ", nil)
	doGet(t, client, otherSrv.URL+"/msg?session=SECRETQ", nil)

	warnings := sink.warnings()
	require.Len(t, warnings, 1)
	assert.Equal(t, originOf(testutil.MustURL(t, originSrv.URL)).String(), fieldValue(warnings[0], "server_origin"))
	assert.Equal(t, originOf(testutil.MustURL(t, otherSrv.URL)).String(), fieldValue(warnings[0], "request_origin"))
	for _, f := range warnings[0].Fields {
		assert.NotContains(t, fmt.Sprint(f.Value), "SECRETQ")
		assert.NotContains(t, fmt.Sprint(f.Value), "/msg")
	}
}

func TestOriginOf(t *testing.T) {
	t.Parallel()

	base := originOf(testutil.MustURL(t, "https://mcp.example.com/mcp"))
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
			assert.Equal(t, tt.same, originOf(testutil.MustURL(t, tt.target)) == base)
		})
	}
}

func TestSameOriginHeaderClient_InvalidURLOmitsURL(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		"http://user:pa55word@[::1/mcp?api_key=SECRETQ",
		"https://deploy:Zm9v/YmFy@mcp.example.com/mcp",
	} {
		_, err := sameOriginHeaderClient(slogtest.Make(t, nil), raw, nil)
		require.EqualError(t, err, "invalid server url", raw)
	}
}
