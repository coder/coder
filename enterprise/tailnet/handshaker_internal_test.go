package tailnet

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/testutil"
)

func Test_handshaker_NoPermission(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mDB := dbmock.NewMockStore(ctrl)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitSuperLong)
	defer cancel()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)

	rfhCh := make(chan readyForHandshake)
	ready := make(chan struct{})
	close(ready)

	srcID, dstID := uuid.New(), uuid.New()

	newHandshaker(ctx, logger, uuid.New(), mDB, rfhCh, ready)

	called := make(chan struct{})
	mDB.EXPECT().GetTailnetTunnelPeerIDs(gomock.Any(), srcID).
		DoAndReturn(func(context.Context, uuid.UUID) ([]database.GetTailnetTunnelPeerIDsRow, error) {
			close(called)
			return []database.GetTailnetTunnelPeerIDsRow{}, nil
		})
	rfhCh <- readyForHandshake{hKey{src: srcID, dst: dstID}}
	<-called
	// the handshaker should not attempt to broadcast the rfh. if it does, the
	// mock will catch an unmocked call.
}
