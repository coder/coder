package vpn

import (
	"context"
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
