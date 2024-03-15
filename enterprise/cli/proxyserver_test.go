package cli_test

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

func Test_ProxyServer_Headers(t *testing.T) {
	t.Parallel()

	const (
		headerName1 = "X-Test-Header-1"
		headerVal1  = "test-value-1"
		headerName2 = "X-Test-Header-2"
		headerVal2  = "test-value-2"
	)

	// We're not going to actually start a proxy, we're going to point it
	// towards a fake server that returns an unexpected status code. This'll
	// cause the proxy to exit with an error that we can check for.
	var called int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&called, 1)
		assert.Equal(t, headerVal1, r.Header.Get(headerName1))
		assert.Equal(t, headerVal2, r.Header.Get(headerName2))

		w.WriteHeader(http.StatusTeapot) // lol
	}))
	defer srv.Close()

	inv, _ := newCLI(t, "wsproxy", "server",
		"--primary-access-url", srv.URL,
		"--proxy-session-token", "test-token",
		"--access-url", "http://localhost:8080",
		"--header", fmt.Sprintf("%s=%s", headerName1, headerVal1),
		"--header-command", fmt.Sprintf("printf %s=%s", headerName2, headerVal2),
	)
	pty := ptytest.New(t)
	inv.Stdout = pty.Output()
	err := inv.Run()
	require.Error(t, err)
	require.ErrorContains(t, err, "unexpected status code 418")
	require.NoError(t, pty.Close())

	assert.EqualValues(t, 1, atomic.LoadInt64(&called))
}

func TestWorkspaceProxy_Server_PrometheusEnabled(t *testing.T) {
	t.Parallel()

	prometheusPort := testutil.RandomPort(t)

	var wg sync.WaitGroup
	wg.Add(1)

	// Start fake coderd
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/workspaceproxies/me/register" {
			// Give fake app_security_key (96 bytes)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"app_security_key": "012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789123456012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789123456"}`))
			return
		}
		if r.URL.Path == "/api/v2/workspaceproxies/me/coordinate" {
			// Slow down proxy registration, so that test runner can check if Prometheus endpoint is exposed.
			wg.Wait()

			// Does not matter, we are not going to implement a real workspace proxy.
			w.WriteHeader(http.StatusNotImplemented)
			return
		}

		w.Header().Add("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`)) // build info can be ignored
	}))
	defer srv.Close()
	defer wg.Done()

	// Configure CLI client
	inv, _ := newCLI(t, "wsproxy", "server",
		"--primary-access-url", srv.URL,
		"--proxy-session-token", "test-token",
		"--access-url", "http://foobar:3001",
		"--http-address", fmt.Sprintf("127.0.0.1:%d", testutil.RandomPort(t)),
		"--prometheus-enable",
		"--prometheus-address", fmt.Sprintf("127.0.0.1:%d", prometheusPort),
	)
	pty := ptytest.New(t).Attach(inv)

	ctx, cancel := context.WithTimeout(inv.Context(), testutil.WaitLong)
	defer cancel()

	// Start "wsproxy server" command
	clitest.StartWithAssert(t, inv, func(t *testing.T, err error) {
		assert.Error(t, err)
		assert.False(t, errors.Is(err, context.Canceled), "error was expected, but context was canceled")
	})
	pty.ExpectMatchContext(ctx, "Started HTTP listener at")

	// Fetch metrics from Prometheus endpoint
	var res *http.Response
	require.Eventually(t, func() bool {
		req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("http://127.0.0.1:%d", prometheusPort), nil)
		assert.NoError(t, err)
		// nolint:bodyclose
		res, err = http.DefaultClient.Do(req)
		return err == nil
	}, testutil.WaitShort, testutil.IntervalFast)
	defer res.Body.Close()

	// Scan for metric patterns
	scanner := bufio.NewScanner(res.Body)
	hasGoStats := false
	hasPromHTTP := false
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "go_goroutines") {
			hasGoStats = true
			continue
		}
		if strings.HasPrefix(scanner.Text(), "promhttp_metric_handler_requests_total") {
			hasPromHTTP = true
			continue
		}
		t.Logf("scanned %s", scanner.Text())
	}
	require.NoError(t, scanner.Err())

	// Verify patterns
	require.True(t, hasGoStats, "Go stats are missing")
	require.True(t, hasPromHTTP, "Prometheus HTTP metrics are missing")
}
