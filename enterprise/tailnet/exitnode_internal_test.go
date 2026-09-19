package tailnet

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database/dbmock"
	agpl "github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/quartz"
)

func TestExitNodeCoordinateeAuth(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	store := dbmock.NewMockStore(ctrl)
	exitNodeID := uuid.New()
	peerID := uuid.New()
	boundID := uuid.New()
	unboundID := uuid.New()
	store.EXPECT().GetWorkspaceAgentIDsByExitNode(gomock.Any(), exitNodeID).
		Return([]uuid.UUID{boundID}, nil).
		Times(2)
	auth := &ExitNodeCoordinateeAuth{
		Database:   store,
		Clock:      quartz.NewMock(t),
		ExitNodeID: exitNodeID,
		PeerID:     peerID,
	}

	require.NoError(t, auth.Authorize(context.Background(), &proto.CoordinateRequest{
		AddTunnel: &proto.CoordinateRequest_Tunnel{Id: boundID[:]},
	}))
	require.Error(t, auth.Authorize(context.Background(), &proto.CoordinateRequest{
		AddTunnel: &proto.CoordinateRequest_Tunnel{Id: unboundID[:]},
	}))
	require.NoError(t, auth.Authorize(context.Background(), &proto.CoordinateRequest{
		UpdateSelf: &proto.CoordinateRequest_UpdateSelf{Node: &proto.Node{
			Addresses: []string{agpl.TailscaleServicePrefix.PrefixFromUUID(peerID).String()},
		}},
	}))
	require.Error(t, auth.Authorize(context.Background(), &proto.CoordinateRequest{
		UpdateSelf: &proto.CoordinateRequest_UpdateSelf{Node: &proto.Node{
			Addresses: []string{agpl.TailscaleServicePrefix.PrefixFromUUID(uuid.New()).String()},
		}},
	}))
}
