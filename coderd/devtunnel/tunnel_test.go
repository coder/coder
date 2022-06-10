package devtunnel_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/coderd/devtunnel"
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

	ctx, cancelTun := context.WithCancel(context.Background())
	defer cancelTun()

	server := http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
		BaseContext: func(_ net.Listener) context.Context {
			return ctx
		},
	}

	cfg, err := devtunnel.GenerateConfig()
	require.NoError(t, err)

	tun, errCh, err := devtunnel.NewWithConfig(ctx, slogtest.Make(t, nil), cfg)
	require.NoError(t, err)
	t.Log(tun.URL)

	go server.Serve(tun.Listener)
	defer tun.Listener.Close()

	transport, _ := http.DefaultTransport.(*http.Transport)

	httpClient := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}

	require.Eventually(t, func() bool {
		req, err := http.NewRequestWithContext(ctx, "GET", tun.URL, nil)
		require.NoError(t, err)

		res, err := httpClient.Do(req)
		require.NoError(t, err)
		defer res.Body.Close()
		_, _ = io.Copy(io.Discard, res.Body)

		return res.StatusCode == http.StatusOK
	}, time.Minute, time.Second)

	httpClient.CloseIdleConnections()
	assert.NoError(t, server.Close())
	cancelTun()

	select {
	case <-errCh:
	case <-time.After(10 * time.Second):
		t.Error("tunnel did not close after 10 seconds")
	}
}
