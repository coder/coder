package vpn

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"testing/synctest"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/testutil"
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

func TestTunnel_StopBlockedUpdater(t *testing.T) {
	t.Parallel()

	for _, sender := range []struct {
		name string
		send func(*Tunnel)
	}{
		{name: "WorkspaceUpdate", send: func(tun *Tunnel) { _ = tun.Update(tailnet.WorkspaceUpdate{}) }},
		{name: "AgentUpdate", send: func(tun *Tunnel) { tun.sendAgentUpdate() }},
		{name: "Log", send: func(tun *Tunnel) { tun.LogEntry(context.Background(), slog.SinkEntry{}) }},
	} {
		t.Run(sender.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
				defer cancel()
				updateCtx, updateCancel := context.WithCancel(ctx)
				defer updateCancel()
				sendCh := make(chan *TunnelMessage)
				tun := &Tunnel{
					ctx:     ctx,
					speaker: speaker[*TunnelMessage, *ManagerMessage, ManagerMessage]{sendCh: sendCh},
					updater: updater{
						ctx: updateCtx, cancel: updateCancel, uSendCh: sendCh,
						agents: map[uuid.UUID]agentWithPing{uuid.New(): {}},
						logger: testutil.Logger(t),
					},
				}
				done := make(chan struct{})
				go func() {
					sender.send(tun)
					close(done)
				}()
				synctest.Wait()
				select {
				case <-done:
					t.Fatal("sender returned before Stop")
				default:
				}
				require.NoError(t, tun.stop(&StopRequest{}))
				synctest.Wait()
				select {
				case <-done:
				default:
					t.Fatal("Stop did not release the blocked sender")
				}
				require.NoError(t, ctx.Err(), "Stop must release senders before the parent context ends")
			})
		})
	}
}
