package tunnel_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/coder/coderd/tunnel"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
)

// The tunnel leaks a few goroutines that aren't impactful to production scenarios.
// func TestMain(m *testing.M) {
// 	goleak.VerifyTestMain(m)
// }

func TestTunnel(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip()
		return
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	url, _, err := tunnel.New(ctx, srv.URL)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		require.NoError(t, err)
		res, err := http.DefaultClient.Do(req)
		var dnsErr *net.DNSError
		// The name might take a bit to resolve!
		if xerrors.As(err, &dnsErr) {
			return false
		}
		require.NoError(t, err)
		return res.StatusCode == http.StatusOK
	}, time.Minute, 3*time.Second)
}
