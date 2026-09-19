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
		idleTrap.MustWait(ctx).MustRelease(ctx)
		for _, payload := range []string{"hello", "", "again"} {
			_, err := client.Write(frame([]byte(payload)))
			require.NoError(t, err)
			require.Equal(t, payload, string(readTestFrame(t, br)))
		}
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
	t.Run("OversizeDatagram", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		h := newProxyHarness(t, "default: allow\n")
		client, br, _, done := h.connectStatus(ctx, t, startUDPEcho(t).String(), http.StatusOK, udpHeader)
		connect := testutil.RequireReceive(ctx, t, h.flows.ch)
		go func() { _, _ = client.Write(frame(make([]byte, 65535))) }()
		requireEOF(t, br)
		testutil.RequireReceive(ctx, t, done)
		terminal := testutil.RequireReceive(ctx, t, h.flows.ch)
		require.Equal(t, connect.FlowID, terminal.FlowID)
		require.NotNil(t, terminal.DisconnectTime)
		require.Zero(t, terminal.BytesIn)
		require.Zero(t, terminal.BytesOut)
	})
	t.Run("PolicyDeny", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
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
