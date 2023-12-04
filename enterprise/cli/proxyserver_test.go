package cli_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/pty/ptytest"
)

func Test_Headers(t *testing.T) {
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
