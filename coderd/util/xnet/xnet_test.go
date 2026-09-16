package xnet_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/util/xnet"
)

func TestIsTimeoutError(t *testing.T) {
	t.Parallel()

	require.False(t, xnet.IsTimeoutError(nil))
	require.False(t, xnet.IsTimeoutError(xerrors.New("other")))
	require.True(t, xnet.IsTimeoutError(context.DeadlineExceeded))
	require.True(t, xnet.IsTimeoutError(xerrors.Errorf("dial: %w", syscall.ETIMEDOUT)))
	require.True(t, xnet.IsTimeoutError(&net.OpError{Op: "dial", Err: syscall.ETIMEDOUT}))
}

func TestIsConnectionError(t *testing.T) {
	t.Parallel()

	require.False(t, xnet.IsConnectionError(nil))
	require.False(t, xnet.IsConnectionError(context.DeadlineExceeded))
	require.True(t, xnet.IsConnectionError(io.EOF))
	require.True(t, xnet.IsConnectionError(io.ErrUnexpectedEOF))
	require.True(t, xnet.IsConnectionError(xerrors.Errorf("write: %w", syscall.EPIPE)))
	require.True(t, xnet.IsConnectionError(&net.OpError{Op: "read", Err: syscall.ECONNRESET}))

	for _, err := range []error{
		http2.StreamError{Code: http2.ErrCodeNo},
		http2.StreamError{Code: http2.ErrCodeInternal},
		http2.StreamError{Code: http2.ErrCodeRefusedStream},
		http2.StreamError{Code: http2.ErrCodeCancel},
		http2.StreamError{Code: http2.ErrCodeEnhanceYourCalm},
		xerrors.Errorf("read response: %w", http2.StreamError{Code: http2.ErrCodeInternal}),
	} {
		require.True(t, xnet.IsConnectionError(err), err)
	}
	require.False(t, xnet.IsConnectionError(http2.StreamError{Code: http2.ErrCodeProtocol}))
	require.False(t, xnet.IsConnectionError(http2.StreamError{Code: http2.ErrCodeFlowControl}))
}

func TestIsConnectionErrorHTTPResponseAbort(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name        string
		enableHTTP2 bool
		proto       string
	}{
		{name: "http1", proto: "HTTP/1.1"},
		{name: "http2", enableHTTP2: true, proto: "HTTP/2.0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, err := w.Write([]byte(`{"partial":`))
				assert.NoError(t, err)
				assert.NoError(t, http.NewResponseController(w).Flush())
				panic(http.ErrAbortHandler)
			}))
			server.EnableHTTP2 = tc.enableHTTP2
			server.StartTLS()
			t.Cleanup(server.Close)

			request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, nil)
			require.NoError(t, err)
			response, err := server.Client().Do(request)
			require.NoError(t, err)
			require.Equal(t, tc.proto, response.Proto)
			_, err = io.ReadAll(response.Body)
			require.Error(t, err)
			require.NoError(t, response.Body.Close())
			require.True(t, xnet.IsConnectionError(err), err)
		})
	}
}
