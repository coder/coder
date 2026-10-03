package agentapi_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"storj.io/drpc/drpcerr"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/agentapi"
	"github.com/coder/coder/v2/codersdk/drpcsdk"
	"github.com/coder/coder/v2/testutil"
)

func TestUpdateLifecycleMissingMessage(t *testing.T) {
	t.Parallel()

	resp, err := (&agentapi.LifecycleAPI{}).UpdateLifecycle(context.Background(), &agentproto.UpdateLifecycleRequest{})
	require.ErrorContains(t, err, "lifecycle is required")
	require.EqualValues(t, codes.InvalidArgument, drpcerr.Code(err))
	require.Nil(t, resp)
}

func TestUpdateStartupMissingMessage(t *testing.T) {
	t.Parallel()

	resp, err := (&agentapi.LifecycleAPI{}).UpdateStartup(context.Background(), &agentproto.UpdateStartupRequest{})
	require.ErrorContains(t, err, "startup is required")
	require.EqualValues(t, codes.InvalidArgument, drpcerr.Code(err))
	require.Nil(t, resp)
}

func TestBatchUpdateMetadataMissingMessage(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		req  *agentproto.BatchUpdateMetadataRequest
	}{
		{name: "NilRequest"},
		{name: "MixedBatch", req: &agentproto.BatchUpdateMetadataRequest{Metadata: []*agentproto.Metadata{
			{Key: "valid", Result: &agentproto.WorkspaceAgentMetadata_Result{Value: "value"}},
			{Key: "invalid"},
		}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resp, err := (&agentapi.MetadataAPI{}).BatchUpdateMetadata(context.Background(), tt.req)
			require.Error(t, err)
			require.EqualValues(t, codes.InvalidArgument, drpcerr.Code(err))
			require.Nil(t, resp)
		})
	}
}

func TestAgentRPCMissingMessage(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	serverCtx, cancel := context.WithCancel(agentapi.WithAPIVersion(ctx, "2.0"))
	defer cancel()
	conn, listener := drpcsdk.MemTransportPipe()
	defer conn.Close()
	defer listener.Close()

	api := &agentapi.API{
		LifecycleAPI: &agentapi.LifecycleAPI{},
		StatsAPI:     &agentapi.StatsAPI{},
	}
	server, err := api.Server(serverCtx)
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() {
		done <- server.Serve(serverCtx, listener)
	}()
	client := agentproto.NewDRPCAgentClient(conn)

	_, err = client.UpdateLifecycle(ctx, &agentproto.UpdateLifecycleRequest{})
	require.ErrorContains(t, err, "lifecycle is required")
	require.EqualValues(t, codes.InvalidArgument, drpcerr.Code(err))
	stats, err := client.UpdateStats(ctx, &agentproto.UpdateStatsRequest{})
	require.NoError(t, err)
	require.NotNil(t, stats.ReportInterval)

	cancel()
	require.NoError(t, testutil.RequireReceive(ctx, t, done))
}
