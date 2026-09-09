package agentapi_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"storj.io/drpc/drpcerr"
	"storj.io/drpc/drpcmux"
	"storj.io/drpc/drpcserver"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/agentapi"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk/drpcsdk"
	"github.com/coder/coder/v2/testutil"
)

func TestUpdateLifecycleMissingMessage(t *testing.T) {
	t.Parallel()

	for name, req := range map[string]*agentproto.UpdateLifecycleRequest{"NilRequest": nil, "MissingLifecycle": {}} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			api := &agentapi.LifecycleAPI{
				AgentFn: func(context.Context) (database.WorkspaceAgent, error) {
					return database.WorkspaceAgent{}, nil
				},
				Log: testutil.Logger(t),
			}
			require.NotPanics(t, func() {
				resp, err := api.UpdateLifecycle(context.Background(), req)
				require.ErrorContains(t, err, "lifecycle is required")
				require.EqualValues(t, codes.InvalidArgument, drpcerr.Code(err))
				require.Nil(t, resp)
			})
		})
	}
}

func TestUpdateStartupMissingMessage(t *testing.T) {
	t.Parallel()

	for name, req := range map[string]*agentproto.UpdateStartupRequest{"NilRequest": nil, "MissingStartup": {}} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			api := &agentapi.LifecycleAPI{
				AgentFn: func(context.Context) (database.WorkspaceAgent, error) {
					return database.WorkspaceAgent{}, nil
				},
				Log: testutil.Logger(t),
			}
			ctx := agentapi.WithAPIVersion(context.Background(), "2.0")
			require.NotPanics(t, func() {
				resp, err := api.UpdateStartup(ctx, req)
				require.ErrorContains(t, err, "startup is required")
				require.EqualValues(t, codes.InvalidArgument, drpcerr.Code(err))
				require.Nil(t, resp)
			})
		})
	}
}

func TestBatchUpdateMetadataMissingMessage(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		req  *agentproto.BatchUpdateMetadataRequest
	}{
		{name: "NilRequest"},
		{name: "NilMetadata", req: &agentproto.BatchUpdateMetadataRequest{Metadata: []*agentproto.Metadata{nil}}},
		{name: "MissingResult", req: &agentproto.BatchUpdateMetadataRequest{Metadata: []*agentproto.Metadata{{Key: "key"}}}},
		{name: "MixedBatch", req: &agentproto.BatchUpdateMetadataRequest{Metadata: []*agentproto.Metadata{
			{Key: "valid", Result: &agentproto.WorkspaceAgentMetadata_Result{Value: " value "}},
			{Key: "invalid"},
		}}},
		{name: "MissingResultAfterKeyLimit", req: &agentproto.BatchUpdateMetadataRequest{Metadata: []*agentproto.Metadata{
			{Key: strings.Repeat("k", 6145), Result: &agentproto.WorkspaceAgentMetadata_Result{Value: " value "}},
			{Key: "invalid"},
		}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			original := proto.Clone(tt.req)
			// A nil batcher also catches attempts to enqueue a partial batch.
			api := &agentapi.MetadataAPI{Log: testutil.Logger(t)}
			require.NotPanics(t, func() {
				resp, err := api.BatchUpdateMetadata(context.Background(), tt.req)
				require.Error(t, err)
				require.EqualValues(t, codes.InvalidArgument, drpcerr.Code(err))
				require.Nil(t, resp)
			})
			require.True(t, proto.Equal(original, tt.req), "invalid batches must not be processed")
		})
	}
}

func TestAgentRPCMissingMessages(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	serverCtx, cancel := context.WithCancel(agentapi.WithAPIVersion(ctx, "2.0"))
	defer cancel()
	conn, listener := drpcsdk.MemTransportPipe()
	defer conn.Close()
	defer listener.Close()

	api := &agentapi.API{
		LifecycleAPI: &agentapi.LifecycleAPI{
			AgentFn: func(context.Context) (database.WorkspaceAgent, error) {
				return database.WorkspaceAgent{}, nil
			},
			Log: testutil.Logger(t),
		},
		MetadataAPI: &agentapi.MetadataAPI{},
		StatsAPI:    &agentapi.StatsAPI{},
	}
	mux := drpcmux.New()
	require.NoError(t, agentproto.DRPCRegisterAgent(mux, api))
	server := drpcsdk.NewServer(testutil.NewFakeSink(t).Logger(), mux, drpcserver.Options{
		Manager: drpcsdk.DefaultDRPCOptions(nil),
	})
	done := make(chan error, 1)
	go func() {
		done <- server.Serve(serverCtx, listener)
	}()
	client := agentproto.NewDRPCAgentClient(conn)

	_, err := client.UpdateLifecycle(ctx, &agentproto.UpdateLifecycleRequest{})
	require.ErrorContains(t, err, "lifecycle is required")
	require.EqualValues(t, codes.InvalidArgument, drpcerr.Code(err))
	_, err = client.UpdateStartup(ctx, &agentproto.UpdateStartupRequest{})
	require.ErrorContains(t, err, "startup is required")
	require.EqualValues(t, codes.InvalidArgument, drpcerr.Code(err))
	_, err = client.BatchUpdateMetadata(ctx, &agentproto.BatchUpdateMetadataRequest{
		Metadata: []*agentproto.Metadata{{Key: "key"}},
	})
	require.ErrorContains(t, err, "metadata result at index 0 is required")
	require.EqualValues(t, codes.InvalidArgument, drpcerr.Code(err))

	_, err = client.UpdateStats(ctx, &agentproto.UpdateStatsRequest{})
	require.NoError(t, err)
	_, err = client.BatchUpdateMetadata(ctx, &agentproto.BatchUpdateMetadataRequest{})
	require.NoError(t, err)
	cancel()
	require.NoError(t, testutil.RequireReceive(ctx, t, done))
}
