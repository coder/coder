//go:build linux
// +build linux

package integration

import (
	"net/http"
	"net/netip"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/testutil"
)

// nolint:revive
func sendRestart(t *testing.T, serverURL *url.URL, derp bool, coordinator bool) {
	t.Helper()
	ctx := testutil.Context(t, 2*time.Second)

	serverURL, err := url.Parse(serverURL.String() + "/restart")
	q := serverURL.Query()
	if derp {
		q.Set("derp", "true")
	}
	if coordinator {
		q.Set("coordinator", "true")
	}
	serverURL.RawQuery = q.Encode()
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, serverURL.String(), nil)
	require.NoError(t, err)
	client := &http.Client{}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode, "unexpected status code %d", resp.StatusCode)
}

// TODO: instead of reusing one conn for each suite, maybe we should make a new
// one for each subtest?
func TestSuite(t *testing.T, _ slog.Logger, serverURL *url.URL, conn *tailnet.Conn, _, peer Client) {
	t.Parallel()

	t.Run("Connectivity", func(t *testing.T) {
		t.Parallel()
		peerIP := tailnet.TailscaleServicePrefix.AddrFromUUID(peer.ID)
		_, _, _, err := conn.Ping(testutil.Context(t, testutil.WaitLong), peerIP)
		require.NoError(t, err, "ping peer")
	})

	t.Run("RestartDERP", func(t *testing.T) {
		peerIP := tailnet.TailscaleServicePrefix.AddrFromUUID(peer.ID)
		_, _, _, err := conn.Ping(testutil.Context(t, testutil.WaitLong), peerIP)
		require.NoError(t, err, "ping peer")
		sendRestart(t, serverURL, true, false)
		_, _, _, err = conn.Ping(testutil.Context(t, testutil.WaitLong), peerIP)
		require.NoError(t, err, "ping peer after derp restart")
	})

	t.Run("RestartCoordinator", func(t *testing.T) {
		peerIP := tailnet.TailscaleServicePrefix.AddrFromUUID(peer.ID)
		_, _, _, err := conn.Ping(testutil.Context(t, testutil.WaitLong), peerIP)
		require.NoError(t, err, "ping peer")
		sendRestart(t, serverURL, false, true)
		_, _, _, err = conn.Ping(testutil.Context(t, testutil.WaitLong), peerIP)
		require.NoError(t, err, "ping peer after coordinator restart")
	})

	t.Run("RestartBoth", func(t *testing.T) {
		peerIP := tailnet.TailscaleServicePrefix.AddrFromUUID(peer.ID)
		_, _, _, err := conn.Ping(testutil.Context(t, testutil.WaitLong), peerIP)
		require.NoError(t, err, "ping peer")
		sendRestart(t, serverURL, true, true)
		_, _, _, err = conn.Ping(testutil.Context(t, testutil.WaitLong), peerIP)
		require.NoError(t, err, "ping peer after restart")
	})
}

func TestBigUDP(t *testing.T, logger slog.Logger, _ *url.URL, conn *tailnet.Conn, _, peer Client) {
	t.Run("UDPEcho", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitShort)

		peerIP := tailnet.TailscaleServicePrefix.AddrFromUUID(peer.ID)
		udpConn, err := conn.DialContextUDP(ctx, netip.AddrPortFrom(peerIP, uint16(EchoPort)))
		require.NoError(t, err)
		defer udpConn.Close()

		// 1280 max tunnel packet size
		//  -40
		//   -8 UDP header
		// ----------------------------
		// 1232 data size
		logger.Info(ctx, "sending UDP test packet")
		packet := make([]byte, 1232)
		for i := range packet {
			packet[i] = byte(i % 256)
		}
		err = udpConn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		require.NoError(t, err)
		n, err := udpConn.Write(packet)
		require.NoError(t, err)
		require.Equal(t, len(packet), n)

		// read the echo
		logger.Info(ctx, "attempting to read UDP reply")
		buf := make([]byte, 1280)
		err = udpConn.SetReadDeadline(time.Now().Add(5 * time.Second))
		require.NoError(t, err)
		n, err = udpConn.Read(buf)
		require.NoError(t, err)
		require.Equal(t, len(packet), n)
		require.Equal(t, packet, buf[:n])
	})
}
