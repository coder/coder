package mcpclient_test

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
	"github.com/coder/coder/v2/testutil"
)

func TestWithInProcessRoute(t *testing.T) {
	t.Parallel()

	// The origin is a real loopback server so that requests which wrongly
	// fall through to the network would reach it; the default guarded
	// client blocks loopback, so they must fail instead.
	var networkHits atomic.Int64
	network := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		networkHits.Add(1)
		rw.WriteHeader(http.StatusTeapot)
	}))
	t.Cleanup(network.Close)
	origin, err := url.Parse(network.URL)
	require.NoError(t, err)

	type seen struct{ requestURI, host, header string }
	seenCh := make(chan seen, 1)
	route := mcpclient.InProcessRoute{
		Origin:     origin,
		PathPrefix: "/api/experimental/mcp/builtin/",
		Handler: http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			seenCh <- seen{requestURI: r.URL.RequestURI(), host: r.Host, header: r.Header.Get("X-Test")}
			rw.Header().Set("Content-Type", "text/plain")
			rw.WriteHeader(http.StatusAccepted)
			_, _ = rw.Write([]byte("in-process"))
		}),
	}
	client := mcpclient.WithInProcessRoute(mcpclient.NewHTTPClient(nil), route)

	t.Run("Matching", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, network.URL+"/api/experimental/mcp/builtin/slack?a=1", nil)
		require.NoError(t, err)
		req.Header.Set("X-Test", "yes")
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, http.StatusAccepted, resp.StatusCode)
		assert.Equal(t, "text/plain", resp.Header.Get("Content-Type"))
		assert.Equal(t, "in-process", string(body))
		got := testutil.TryReceive(ctx, t, seenCh)
		assert.Equal(t, seen{requestURI: "/api/experimental/mcp/builtin/slack?a=1", host: origin.Host, header: "yes"}, got)
	})

	t.Run("OtherRequestsStayGuarded", func(t *testing.T) {
		t.Parallel()
		otherPort := *origin
		otherPort.Host = origin.Hostname() + ":1"
		for _, target := range []string{
			network.URL + "/api/v2/users",
			network.URL + "/api/experimental/mcp/builtin",
			network.URL + "/api/experimental/mcp/builtin/../../../v2/users",
			network.URL + "/api/experimental/mcp/builtin//slack",
			network.URL + "/api/experimental/mcp/builtin/%2e%2e/v2",
			"https://" + origin.Host + "/api/experimental/mcp/builtin/slack",
			otherPort.String() + "/api/experimental/mcp/builtin/slack",
		} {
			ctx := testutil.Context(t, testutil.WaitShort)
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
			require.NoError(t, err)
			resp, err := client.Do(req)
			if err == nil {
				_ = resp.Body.Close()
			}
			require.Error(t, err, target)
			assert.Contains(t, err.Error(), "blocked", target)
		}
		assert.Zero(t, networkHits.Load())
	})
}

func TestWithInProcessRoute_Streams(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	route := mcpclient.InProcessRoute{
		Origin:     &url.URL{Scheme: "https", Host: "coder.invalid"},
		PathPrefix: "/mcp/",
		Handler: http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			rw.Header().Set("Content-Type", "text/event-stream")
			_, _ = rw.Write([]byte("data: first\n"))
			rw.(http.Flusher).Flush()
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
			_, _ = rw.Write([]byte("data: second\n"))
		}),
	}
	client := mcpclient.WithInProcessRoute(mcpclient.NewHTTPClient(nil), route)

	ctx := testutil.Context(t, testutil.WaitShort)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://coder.invalid/mcp/stream", nil)
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	line, err := reader.ReadString('\n')
	require.NoError(t, err)
	assert.Equal(t, "data: first\n", line, "the first event must arrive before the handler finishes")
	close(release)
	line, err = reader.ReadString('\n')
	require.NoError(t, err)
	assert.Equal(t, "data: second\n", line)
	_, err = reader.ReadByte()
	assert.ErrorIs(t, err, io.EOF)
}

func TestWithInProcessRoute_CanceledBeforeHeaders(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	route := mcpclient.InProcessRoute{
		Origin:     &url.URL{Scheme: "https", Host: "coder.invalid"},
		PathPrefix: "/mcp/",
		Handler: http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			close(started)
			<-r.Context().Done()
		}),
	}
	client := mcpclient.WithInProcessRoute(mcpclient.NewHTTPClient(nil), route)

	ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitShort))
	go func() {
		<-started
		cancel()
	}()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://coder.invalid/mcp/x", nil)
	require.NoError(t, err)
	resp, err := client.Do(req)
	if err == nil {
		_ = resp.Body.Close()
	}
	require.ErrorIs(t, err, context.Canceled)
}
