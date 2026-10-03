package testutil_test

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

func TestNewUnstartedHTTPServer(t *testing.T) {
	t.Parallel()

	send := func(conn net.Conn) error {
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

	t.Run("defaults", func(t *testing.T) {
		t.Parallel()

		srv := testutil.NewHTTPTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		conn, err := net.Dial("tcp", srv.Listener.Addr().String())
		require.NoError(t, err, "dial")
		defer conn.Close()

		// Keepalives should be disabled: first request succeeds, second request on the same conn fails.
		require.NoError(t, send(conn), "keepalives disabled: first request on reused connection must succeed")
		require.Error(t, send(conn), "keepalives disabled: second request on reused connection must fail")
	})

	t.Run("override", func(t *testing.T) {
		t.Parallel()

		srv := testutil.NewHTTPTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}), func(srv *httptest.Server) {
			srv.Config.SetKeepAlivesEnabled(true)
		})

		conn, err := net.Dial("tcp", srv.Listener.Addr().String())
		require.NoError(t, err, "dial")
		defer conn.Close()

		// Keepalives should be enabled: multiple requests on the same conn succeed.
		require.NoError(t, send(conn), "keepalives enabled: first request on reused connection must succeed")
		require.NoError(t, send(conn), "keepalives enabled: second request on reused connection must succeed")
	})
}
