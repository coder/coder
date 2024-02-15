package coderd

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/tailnettest"
	"github.com/coder/coder/v2/testutil"
)

// TestServerTailnet_Reconnect tests that ServerTailnet calls SetAllPeersLost on the Coordinatee
// (tailnet.Conn in production) when it disconnects from the Coordinator (via MultiAgentConn) and
// reconnects.
func TestServerTailnet_Reconnect(t *testing.T) {
	t.Parallel()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctrl := gomock.NewController(t)
	ctx := testutil.Context(t, testutil.WaitShort)

	mMultiAgent0 := tailnettest.NewMockMultiAgentConn(ctrl)
	mMultiAgent1 := tailnettest.NewMockMultiAgentConn(ctrl)
	mac := make(chan tailnet.MultiAgentConn, 2)
	mac <- mMultiAgent0
	mac <- mMultiAgent1
	mCoord := tailnettest.NewMockCoordinatee(ctrl)

	uut := &ServerTailnet{
		ctx:         ctx,
		logger:      logger,
		coordinatee: mCoord,
		getMultiAgent: func(ctx context.Context) (tailnet.MultiAgentConn, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case m := <-mac:
				return m, nil
			}
		},
		agentConn:            atomic.Pointer[tailnet.MultiAgentConn]{},
		agentConnectionTimes: make(map[uuid.UUID]time.Time),
	}
	// reinit the Coordinator once, to load mMultiAgent0
	mCoord.EXPECT().SetNodeCallback(gomock.Any()).Times(1)
	uut.reinitCoordinator()

	mMultiAgent0.EXPECT().NextUpdate(gomock.Any()).
		Times(1).
		Return(nil, false) // this indicates there are no more updates
	closed0 := mMultiAgent0.EXPECT().IsClosed().
		Times(1).
		Return(true) // this triggers reconnect
	setLost := mCoord.EXPECT().SetAllPeersLost().Times(1).After(closed0)
	mCoord.EXPECT().SetNodeCallback(gomock.Any()).Times(1).After(closed0)
	mMultiAgent1.EXPECT().NextUpdate(gomock.Any()).
		Times(1).
		After(setLost).
		Return(nil, false)
	mMultiAgent1.EXPECT().IsClosed().
		Times(1).
		Return(false) // this causes us to exit and not reconnect

	done := make(chan struct{})
	go func() {
		uut.watchAgentUpdates()
		close(done)
	}()

	testutil.RequireRecvCtx(ctx, t, done)
}
