package testutil_test

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

// TestNewUnstartedHTTPServerDisablesKeepAlives verifies the helper
// disables keep-alives on the underlying server, which is the property
// that prevents stale pooled-connection reuse (see AIGOV-430). A server
// with keep-alives disabled closes the connection after a response, so a
// second request on the same raw connection must fail.
func TestNewUnstartedHTTPServerDisablesKeepAlives(t *testing.T) {
	t.Parallel()

	srv := testutil.NewUnstartedHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	srv.Start()

	conn, err := net.Dial("tcp", srv.Listener.Addr().String())
	require.NoError(t, err, "dial")
	defer conn.Close()

	send := func() error {
		_, err := fmt.Fprintf(conn, "GET / HTTP/1.1\r\nHost: x\r\n\r\n")
		if err != nil {
			return err
		}
		resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
		if err != nil {
			return err
		}
		_ = resp.Body.Close()
		return nil
	}

	// First request on this connection succeeds.
	require.NoError(t, send(), "first request")
	// Keep-alives disabled means the server closes the conn, so the
	// second request on the same conn fails.
	require.Error(t, send(), "second request on reused connection must fail")
}
