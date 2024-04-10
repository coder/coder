package tailnet_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"tailscale.com/types/key"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/enterprise/tailnet"
	agpl "github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/coder/v2/testutil"
)

func TestPGCoordinatorDual_ReadyForHandshake_OK(t *testing.T) {
	t.Parallel()
	if !dbtestutil.WillUsePostgres() {
		t.Skip("test only with postgres")
	}
	store, ps := dbtestutil.NewDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitSuperLong)
	defer cancel()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	coord1, err := tailnet.NewPGCoord(ctx, logger.Named("coord1"), ps, store)
	require.NoError(t, err)
	defer coord1.Close()
	coord2, err := tailnet.NewPGCoord(ctx, logger.Named("coord2"), ps, store)
	require.NoError(t, err)
	defer coord2.Close()

	clientID, agentID := uuid.New(), uuid.New()

	cReq, cRes := coord1.Coordinate(ctx, clientID, clientID.String(), agpl.ClientCoordinateeAuth{AgentID: agentID})
	aReq, aRes := coord2.Coordinate(ctx, agentID, agentID.String(), agpl.AgentCoordinateeAuth{ID: agentID})

	{
		nk, err := key.NewNode().Public().MarshalBinary()
		require.NoError(t, err)
		dk, err := key.NewDisco().Public().MarshalText()
		require.NoError(t, err)
		cReq <- &proto.CoordinateRequest{UpdateSelf: &proto.CoordinateRequest_UpdateSelf{
			Node: &proto.Node{
				Id:    3,
				Key:   nk,
				Disco: string(dk),
			},
		}}
	}

	cReq <- &proto.CoordinateRequest{AddTunnel: &proto.CoordinateRequest_Tunnel{
		Id: agentID[:],
	}}

	testutil.RequireRecvCtx(ctx, t, aRes)

	aReq <- &proto.CoordinateRequest{ReadyForHandshake: []*proto.CoordinateRequest_ReadyForHandshake{{
		Id: clientID[:],
	}}}
	ack := testutil.RequireRecvCtx(ctx, t, cRes)
	require.NotNil(t, ack.PeerUpdates)
	require.Len(t, ack.PeerUpdates, 1)
	require.Equal(t, proto.CoordinateResponse_PeerUpdate_READY_FOR_HANDSHAKE, ack.PeerUpdates[0].Kind)
	require.Equal(t, agentID[:], ack.PeerUpdates[0].Id)
}
