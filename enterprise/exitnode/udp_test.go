package exitnode_test

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"testing"

	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/exitnode"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// startUDPEcho runs a UDP server that echoes every datagram to its sender
// until the test ends.
func startUDPEcho(t *testing.T) *net.UDPAddr {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = pc.Close() })
	go func() {
		buf := make([]byte, 65535)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			_, _ = pc.WriteTo(buf[:n], addr)
		}
	}()
	addr, ok := pc.LocalAddr().(*net.UDPAddr)
	require.True(t, ok)
	return addr
}

func frame(payload []byte) []byte {
	// #nosec G115 - Tests only build frames up to 65535 bytes.
	return append(binary.BigEndian.AppendUint16(nil, uint16(len(payload))), payload...)
}

func readTestFrame(t *testing.T, r *bufio.Reader) []byte {
	t.Helper()
	var hdr [2]byte
	_, err := io.ReadFull(r, hdr[:])
	require.NoError(t, err)
	payload := make([]byte, binary.BigEndian.Uint16(hdr[:]))
	_, err = io.ReadFull(r, payload)
	require.NoError(t, err)
	return payload
}

var udpHeader = http.Header{codersdk.ExitNodeProtocolHeader: []string{"udp"}}

func TestConnectProxy_UDP(t *testing.T) {
	t.Parallel()

	t.Run("RoundTripAndIdle", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		echo := startUDPEcho(t)

		mClock := quartz.NewMock(t)
		idleTrap := mClock.Trap().AfterFunc("relayDatagrams", "idle")
		defer idleTrap.Close()

		h := newProxyHarness(t, "default: allow\n", func(o *exitnode.ConnectProxyOptions) {
			o.Clock = mClock
		})
		client, br, _, done := h.connectStatus(ctx, t, echo.String(), http.StatusOK, udpHeader)

		connectReport := testutil.RequireReceive(ctx, t, h.flows.ch)
		require.Equal(t, codersdk.ExitNodeFlowAllow, connectReport.Decision)
		require.Equal(t, codersdk.ExitNodeProtocolUDP, connectReport.Protocol)
		require.Equal(t, echo.IP.String(), connectReport.DestinationIP)
		require.Equal(t, echo.Port, connectReport.DestinationPort)
		require.Nil(t, connectReport.DisconnectTime)

		// The idle timer is armed once the relay starts.
		idleTrap.MustWait(ctx).MustRelease(ctx)

		// The proxy reads frames continuously, so pipe writes complete
		// synchronously. Empty datagrams are legal and must round-trip.
		for _, payload := range []string{"hello", "", "again"} {
			_, err := client.Write(frame([]byte(payload)))
			require.NoError(t, err)
			require.Equal(t, payload, string(readTestFrame(t, br)))
		}

		// Nothing else happens until the idle timeout fires and closes both
		// ends.
		mClock.Advance(exitnode.DefaultUDPIdleTimeout).MustWait(ctx)
		requireEOF(t, br)
		testutil.RequireReceive(ctx, t, done)

		disconnectReport := testutil.RequireReceive(ctx, t, h.flows.ch)
		require.Equal(t, connectReport.FlowID, disconnectReport.FlowID)
		require.Equal(t, codersdk.ExitNodeProtocolUDP, disconnectReport.Protocol)
		require.NotNil(t, disconnectReport.DisconnectTime)
		require.Equal(t, int64(10), disconnectReport.BytesOut)
		require.Equal(t, int64(10), disconnectReport.BytesIn)
		require.Equal(t, float64(0), promtestutil.ToFloat64(h.metrics.ActiveFlows))
	})

	t.Run("OversizeDatagramClosesStream", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		echo := startUDPEcho(t)

		h := newProxyHarness(t, "default: allow\n")
		client, br, _, done := h.connectStatus(ctx, t, echo.String(), http.StatusOK, udpHeader)
		testutil.RequireReceive(ctx, t, h.flows.ch)

		// 65535 bytes is the largest frame the 16-bit length can describe
		// but more than any IPv4 datagram can carry, so the socket refuses
		// it and the proxy closes rather than silently dropping data.
		go func() {
			_, _ = client.Write(frame(make([]byte, 65535)))
		}()
		requireEOF(t, br)
		testutil.RequireReceive(ctx, t, done)

		disconnectReport := testutil.RequireReceive(ctx, t, h.flows.ch)
		require.NotNil(t, disconnectReport.DisconnectTime)
		require.Zero(t, disconnectReport.BytesOut)
	})

	t.Run("PolicyDeny", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		// A tcp listener stands in for a server that speaks both tcp and
		// udp on one port, like QUIC alongside HTTPS.
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		defer ln.Close()
		go func() {
			for {
				conn, err := ln.Accept()
				if err != nil {
					return
				}
				_, _ = io.WriteString(conn, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\nok")
				_ = conn.Close()
			}
		}()
		target := ln.Addr().String()

		h := newProxyHarness(t, fmt.Sprintf(`
default: allow
rules:
  - id: no-quic
    deny:
      protocols: [udp]
      ports: [%d]
`, netip.MustParseAddrPort(target).Port()))

		client, _, resp, done := h.connectStatus(ctx, t, target, http.StatusForbidden, udpHeader)
		require.Equal(t, "no-quic", resp.header.Get(codersdk.ExitNodeDenyRuleHeader))
		require.Equal(t, "matched rule no-quic", resp.header.Get(codersdk.ExitNodeDenyReasonHeader))
		requireEOF(t, client)
		testutil.RequireReceive(ctx, t, done)
		report := testutil.RequireReceive(ctx, t, h.flows.ch)
		require.Equal(t, codersdk.ExitNodeFlowDeny, report.Decision)
		require.Equal(t, codersdk.ExitNodeProtocolUDP, report.Protocol)
		require.Equal(t, "no-quic", report.RuleID)

		// The same destination over tcp is unaffected.
		client, br, _, done := h.connectStatus(ctx, t, target, http.StatusOK)
		go func() { _, _ = io.WriteString(client, "GET / HTTP/1.1\r\nHost: x\r\n\r\n") }()
		require.Equal(t, "ok", readTunneled(t, br))
		_ = client.Close()
		testutil.RequireReceive(ctx, t, done)
		report = testutil.RequireReceive(ctx, t, h.flows.ch)
		require.Equal(t, codersdk.ExitNodeFlowAllow, report.Decision)
		require.Equal(t, codersdk.ExitNodeProtocolTCP, report.Protocol)
	})
}
