package vpn

import (
	"context"
	"io"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/testutil"
)

func TestSpeaker_ResponseDuringCancellation(t *testing.T) {
	t.Parallel()

	for _, cancelSpeaker := range []bool{false, true} {
		name := "Caller"
		if cancelSpeaker {
			name = "Speaker"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				// Both a response and cancellation can be selectable. Exercise
				// either choice while holding response delivery in flight.
				for range 32 {
					func() {
						gate := newRPCSendGate()
						testCtx, tun, mgr := setupSpeakers(t, gate)
						defer tun.Close()
						defer mgr.Close()
						ctx, cancel := context.WithCancel(testCtx)
						defer cancel()

						done := make(chan error, 1)
						go func() {
							_, err := mgr.unaryRPC(ctx, &ManagerMessage{Msg: &ManagerMessage_Start{Start: &StartRequest{}}})
							done <- err
						}()
						req := testutil.TryReceive(testCtx, t, tun.requests)
						testutil.TryReceive(testCtx, t, gate.entered)
						require.NoError(t, req.sendReply(&TunnelMessage{Msg: &TunnelMessage_Start{Start: &StartResponse{Success: true}}}))
						synctest.Wait()

						if cancelSpeaker {
							mgr.cancel()
						} else {
							cancel()
						}
						close(gate.release)
						// Either the response or cancellation may win, but the RPC
						// must return instead of deadlocking against its dispatcher.
						err := testutil.TryReceive(testCtx, t, done)
						if err != nil {
							require.ErrorIs(t, err, context.Canceled)
						}
						require.NoError(t, testCtx.Err())
					}()
				}
			})
		})
	}
}

func TestSpeaker_ResponseBeforeEOF(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		for range 32 {
			func() {
				gate := newRPCSendGate()
				ctx, tun, mgr := setupSpeakers(t, gate)
				defer tun.Close()
				defer mgr.Close()
				type result struct {
					response *TunnelMessage
					err      error
				}
				done := make(chan result, 1)
				go func() {
					response, err := mgr.unaryRPC(ctx, &ManagerMessage{Msg: &ManagerMessage_Stop{Stop: &StopRequest{}}})
					done <- result{response: response, err: err}
				}()
				req := testutil.TryReceive(ctx, t, tun.requests)
				testutil.TryReceive(ctx, t, gate.entered)
				// A Stop response closes the transport after its frame is written.
				// Keep the caller paused until the response and EOF arrive.
				require.NoError(t, req.sendReply(&TunnelMessage{Msg: &TunnelMessage_Stop{Stop: &StopResponse{Success: true}}}))
				synctest.Wait()
				close(gate.release)
				got := testutil.TryReceive(ctx, t, done)
				require.NoError(t, got.err)
				require.True(t, got.response.GetStop().GetSuccess())
			}()
		}
	})
}

func TestSpeaker_RPCAfterWriterExits(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		ctx, tun, mgr := setupSpeakers(t)
		defer tun.Close()
		defer mgr.Close()
		// Leave a manager request undelivered, so the tunnel's receive side
		// cannot exit and close recvLoopDone.
		testutil.RequireSend(ctx, t, mgr.sendCh, &ManagerMessage{
			Rpc: &RPC{MsgId: 1},
			Msg: &ManagerMessage_Wake{Wake: &WakeRequest{}},
		})
		rpc := func() chan error {
			done := make(chan error, 1)
			go func() {
				_, err := tun.unaryRPC(ctx, &TunnelMessage{
					Msg: &TunnelMessage_NetworkSettings{NetworkSettings: &NetworkSettingsRequest{}},
				})
				done <- err
			}()
			return done
		}
		awaitingResponse := rpc()
		synctest.Wait()
		// Closing the manager makes the tunnel's next write fail.
		require.NoError(t, mgr.Close())
		failedWrite := rpc()
		synctest.Wait()
		unread := rpc()
		synctest.Wait()

		for _, done := range []chan error{awaitingResponse, failedWrite, unread} {
			select {
			case err := <-done:
				require.ErrorIs(t, err, io.ErrUnexpectedEOF)
			default:
				t.Fatal("RPC still blocked after the writer exited")
			}
		}
	})
}

func TestSpeaker_ResponseBeforeWriterExits(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		// The response and writer exit are both selectable. Repeat to
		// exercise either selection.
		for range 32 {
			func() {
				gate := newRPCSendGate()
				ctx, tun, mgr := setupSpeakers(t, gate)
				defer tun.Close()
				defer mgr.Close()
				type result struct {
					response *ManagerMessage
					err      error
				}
				done := make(chan result, 1)
				go func() {
					response, err := tun.unaryRPC(ctx, &TunnelMessage{
						Msg: &TunnelMessage_NetworkSettings{NetworkSettings: &NetworkSettingsRequest{}},
					})
					done <- result{response: response, err: err}
				}()
				req := testutil.TryReceive(ctx, t, mgr.requests)
				testutil.TryReceive(ctx, t, gate.entered)
				require.NoError(t, req.sendReply(&ManagerMessage{
					Msg: &ManagerMessage_NetworkSettings{NetworkSettings: &NetworkSettingsResponse{Success: true}},
				}))
				// Keep recvLoopDone open, so only writer exit competes with
				// the response.
				testutil.RequireSend(ctx, t, mgr.sendCh, &ManagerMessage{
					Rpc: &RPC{MsgId: 1},
					Msg: &ManagerMessage_Wake{Wake: &WakeRequest{}},
				})
				synctest.Wait()
				require.NoError(t, mgr.Close())
				// This write fails, so the writer exits.
				testutil.RequireSend(ctx, t, tun.sendCh, &TunnelMessage{Msg: &TunnelMessage_Log{Log: &Log{}}})
				synctest.Wait()
				close(gate.release)
				got := testutil.TryReceive(ctx, t, done)
				require.NoError(t, got.err)
				require.True(t, got.response.GetNetworkSettings().GetSuccess())
			}()
		}
	})
}

// rpcSendGate pauses the first RPC after sending but before awaiting its reply.
// The log sink lets the peer respond while the caller is not yet receiving.
type rpcSendGate struct {
	used    atomic.Bool
	entered chan struct{}
	release chan struct{}
}

func newRPCSendGate() *rpcSendGate {
	return &rpcSendGate{entered: make(chan struct{}), release: make(chan struct{})}
}

func (g *rpcSendGate) LogEntry(_ context.Context, entry slog.SinkEntry) {
	if entry.Message == "sent rpc request" && g.used.CompareAndSwap(false, true) {
		close(g.entered)
		<-g.release
	}
}

func (*rpcSendGate) Sync() {}
