//go:build linux
// +build linux

package integration

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/testutil"
)

func sendRestart(t *testing.T, serverURL *url.URL, endpoint string) {
	const reqTimeout = 2 * time.Second

	serverURL, err := url.Parse(serverURL.String() + endpoint)
	require.NoError(t, err)

	client := http.Client{
		Timeout: reqTimeout,
	}

	//nolint:noctx
	resp, err := client.Post(serverURL.String(), "text/plain", nil)
	require.NoError(t, err)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status code: %d", resp.StatusCode)
	}
}

// TODO: instead of reusing one conn for each suite, maybe we should make a new
// one for each subtest?
func TestSuite(t *testing.T, _ slog.Logger, serverURL *url.URL, conn *tailnet.Conn, _, peer Client) {
	t.Parallel()

	t.Run("Connectivity", func(t *testing.T) {
		t.Parallel()
		peerIP := tailnet.IPFromUUID(peer.ID)
		_, _, _, err := conn.Ping(testutil.Context(t, testutil.WaitLong), peerIP)
		require.NoError(t, err, "ping peer")
	})

	t.Run("RestartDERP", func(t *testing.T) {
		peerIP := tailnet.IPFromUUID(peer.ID)
		_, _, _, err := conn.Ping(testutil.Context(t, testutil.WaitLong), peerIP)
		require.NoError(t, err, "ping peer")
		sendRestart(t, serverURL, "/derp/restart")
		_, _, _, err = conn.Ping(testutil.Context(t, testutil.WaitLong), peerIP)
		require.NoError(t, err, "ping peer after derp restart")
	})

	t.Run("RestartCoordinator", func(t *testing.T) {
		peerIP := tailnet.IPFromUUID(peer.ID)
		_, _, _, err := conn.Ping(testutil.Context(t, testutil.WaitLong), peerIP)
		require.NoError(t, err, "ping peer")
		sendRestart(t, serverURL, "/restart")
		_, _, _, err = conn.Ping(testutil.Context(t, testutil.WaitLong), peerIP)
		require.NoError(t, err, "ping peer after coordinator restart")
	})

	t.Run("RestartBoth", func(t *testing.T) {
		peerIP := tailnet.IPFromUUID(peer.ID)
		_, _, _, err := conn.Ping(testutil.Context(t, testutil.WaitLong), peerIP)
		require.NoError(t, err, "ping peer")
		sendRestart(t, serverURL, "/derp/restart")
		sendRestart(t, serverURL, "/restart")
		_, _, _, err = conn.Ping(testutil.Context(t, testutil.WaitLong), peerIP)
		require.NoError(t, err, "ping peer after restart")
	})
}
