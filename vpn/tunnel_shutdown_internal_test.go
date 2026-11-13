package vpn

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestTunnel_StopConcurrentSenders(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
		defer cancel()
		manager, tunnel := net.Pipe()
		defer manager.Close()
		defer tunnel.Close()

		var tun *Tunnel
		ready := make(chan error, 1)
		go func() {
			var err error
			tun, err = NewTunnel(ctx, testutil.Logger(t), tunnel, newFakeClient(ctx, t))
			ready <- err
		}()
		header := make([]byte, len(expectedHandshake))
		_, err := io.ReadFull(manager, header)
		require.NoError(t, err)
		require.Equal(t, expectedHandshake, string(header))
		_, err = manager.Write([]byte("codervpn manager 1.3\n"))
		require.NoError(t, err)
		require.NoError(t, <-ready)
		defer tun.Close()

		// Occupy the writer so both the network-settings RPC and the Stop
		// response must wait for a send on the unbuffered channel.
		tun.LogEntry(ctx, slog.SinkEntry{Message: "before stop"})
		rpcDone := make(chan error, 1)
		go func() {
			rpcDone <- tun.ApplyNetworkSettings(ctx, &NetworkSettingsRequest{})
		}()
		synctest.Wait()
		select {
		case err := <-rpcDone:
			t.Fatalf("RPC returned before shutdown: %v", err)
		default:
		}

		stop, err := proto.Marshal(&ManagerMessage{
			Rpc: &RPC{MsgId: 1},
			Msg: &ManagerMessage_Stop{Stop: &StopRequest{}},
		})
		require.NoError(t, err)
		// #nosec G115 - The Stop request is a small, fixed-size message.
		require.NoError(t, binary.Write(manager, binary.BigEndian, uint32(len(stop))))
		_, err = manager.Write(stop)
		require.NoError(t, err)
		synctest.Wait()

		// Queue another RPC behind Stop. The writer is still blocked, so
		// this send is in flight when requestLoop shuts down the protocol.
		lateRPCDone := make(chan error, 1)
		go func() {
			lateRPCDone <- tun.ApplyNetworkSettings(ctx, &NetworkSettingsRequest{})
		}()
		synctest.Wait()
		select {
		case err := <-lateRPCDone:
			t.Fatalf("queued RPC returned before shutdown: %v", err)
		default:
		}

		// Read until Stop, without replying to the in-flight RPC. Every
		// accepted message must be written in full before the pipe closes.
		for {
			var length uint32
			require.NoError(t, binary.Read(manager, binary.BigEndian, &length))
			// Let requestLoop finish handling the message while the writer
			// is blocked on its payload, before accepting another send.
			synctest.Wait()
			payload := make([]byte, length)
			_, err = io.ReadFull(manager, payload)
			require.NoError(t, err)
			msg := new(TunnelMessage)
			require.NoError(t, proto.Unmarshal(payload, msg))
			if msg.GetStop() != nil {
				require.True(t, msg.GetStop().GetSuccess())
				require.EqualValues(t, 1, msg.GetRpc().GetResponseTo())
				break
			}
		}
		synctest.Wait()
		require.Error(t, <-rpcDone)
		require.Error(t, <-lateRPCDone)
		var b [1]byte
		_, err = manager.Read(b[:])
		require.ErrorIs(t, err, io.EOF)

		// Stop is also valid before Start. It must terminate the updater
		// and make later senders return without a panic or a blocked send.
		tun.LogEntry(ctx, slog.SinkEntry{Message: "after stop"})
		require.ErrorIs(t, tun.Update(tailnet.WorkspaceUpdate{}), context.Canceled)
		require.Error(t, tun.ApplyNetworkSettings(ctx, &NetworkSettingsRequest{}))
		select {
		case <-tun.netLoopDone:
		default:
			t.Fatal("network status loop did not exit after Stop")
		}
		require.NoError(t, ctx.Err(), "shutdown must not depend on the parent context ending")
	})
}

func TestTunnel_StopForwardsCloseLogs(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	tun, mgr := setupTunnel(t, ctx, newFakeClient(ctx, t), quartz.NewMock(t))
	const closeLogs = 10
	conn := &loggingCloseConn{fakeConn: newFakeConn(tailnet.WorkspaceUpdate{}, time.Time{}), tun: tun, logs: closeLogs}
	require.True(t, tun.updater.setConn(conn))

	type result struct {
		resp *TunnelMessage
		err  error
	}
	done := make(chan result, 1)
	go func() {
		resp, err := mgr.unaryRPC(ctx, &ManagerMessage{Msg: &ManagerMessage_Stop{}})
		done <- result{resp: resp, err: err}
	}()
	// Logs sent before the Stop response are dispatched before it is.
	var received int
	var got result
	requests := mgr.requests
recv:
	for {
		select {
		case req, ok := <-requests:
			if !ok {
				requests = nil
			} else if req.msg.GetLog().GetMessage() == "closing conn" {
				received++
			}
		case got = <-done:
			break recv
		}
	}
	require.NoError(t, got.err)
	require.True(t, got.resp.GetStop().GetSuccess())
	require.Equal(t, closeLogs, received, "logs emitted while closing the conn must reach the manager")
}

// loggingCloseConn logs through the tunnel while closing, as a tailnet conn
// does when the tunnel is its log sink.
type loggingCloseConn struct {
	*fakeConn
	tun  *Tunnel
	logs int
}

func (c *loggingCloseConn) Close() error {
	for range c.logs {
		c.tun.LogEntry(context.Background(), slog.SinkEntry{Message: "closing conn"})
	}
	return c.fakeConn.Close()
}
