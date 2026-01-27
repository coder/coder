package agentapi_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	prom_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/types/known/timestamppb"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/agentapi"
	"github.com/coder/coder/v2/coderd/agentapi/metadatabatcher"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/testutil"
)

func TestBatchUpdateMetadata(t *testing.T) {
	t.Parallel()

	agent := database.WorkspaceAgent{
		ID: uuid.New(),
	}

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)

		ctrl := gomock.NewController(t)
		store := dbmock.NewMockStore(ctrl)
		ps := pubsub.NewInMemory()
		reg := prometheus.NewRegistry()

		now := dbtime.Now()
		req := &agentproto.BatchUpdateMetadataRequest{
			Metadata: []*agentproto.Metadata{
				{
					Key: "awesome key",
					Result: &agentproto.WorkspaceAgentMetadata_Result{
						CollectedAt: timestamppb.New(now.Add(-10 * time.Second)),
						Age:         10,
						Value:       "awesome value",
						Error:       "",
					},
				},
				{
					Key: "uncool key",
					Result: &agentproto.WorkspaceAgentMetadata_Result{
						CollectedAt: timestamppb.New(now.Add(-3 * time.Second)),
						Age:         3,
						Value:       "",
						Error:       "uncool value",
					},
				},
			},
		}
		batchSize := len(req.Metadata)
		// This test sends 2 metadata entries. With batch size 2, we expect
		// exactly 1 capacity flush.
		store.EXPECT().
			BatchUpdateWorkspaceAgentMetadata(gomock.Any(), gomock.Any()).
			Return(nil).
			Times(1)

		// Create a real batcher for the test with batch size matching the number
		// of metadata entries to trigger exactly one capacity flush.
		batcher, err := metadatabatcher.NewBatcher(ctx, reg, store, ps,
			metadatabatcher.WithLogger(testutil.Logger(t)),
			metadatabatcher.WithBatchSize(batchSize),
		)
		require.NoError(t, err)
		t.Cleanup(batcher.Close)

		api := &agentapi.MetadataAPI{
			AgentFn: func(context.Context) (database.WorkspaceAgent, error) {
				return agent, nil
			},
			Workspace: &agentapi.CachedWorkspaceFields{},
			Log:       testutil.Logger(t),
			Batcher:   batcher,
			TimeNowFn: func() time.Time {
				return now
			},
		}

		resp, err := api.BatchUpdateMetadata(context.Background(), req)
		require.NoError(t, err)
		require.Equal(t, &agentproto.BatchUpdateMetadataResponse{}, resp)

		// Wait for the capacity flush to complete before test ends.
		testutil.Eventually(ctx, t, func(ctx context.Context) bool {
			return prom_testutil.ToFloat64(batcher.Metrics.MetadataTotal) == 2.0
		}, testutil.IntervalFast)
	})

	t.Run("ExceededLength", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		ctrl := gomock.NewController(t)
		store := dbmock.NewMockStore(ctrl)
		ps := pubsub.NewInMemory()
		reg := prometheus.NewRegistry()

		// This test sends 4 metadata entries with some exceeding length limits. We set the batchers batch size so that
		// we can reliably ensure a batch is sent within the WaitShort time period.
		store.EXPECT().
			BatchUpdateWorkspaceAgentMetadata(gomock.Any(), gomock.Any()).
			Return(nil).
			Times(1)

		now := dbtime.Now()
		almostLongValue := ""
		for i := 0; i < 2048; i++ {
			almostLongValue += "a"
		}
		req := &agentproto.BatchUpdateMetadataRequest{
			Metadata: []*agentproto.Metadata{
				{
					Key: "almost long value",
					Result: &agentproto.WorkspaceAgentMetadata_Result{
						Value: almostLongValue,
					},
				},
				{
					Key: "too long value",
					Result: &agentproto.WorkspaceAgentMetadata_Result{
						Value: almostLongValue + "a",
					},
				},
				{
					Key: "almost long error",
					Result: &agentproto.WorkspaceAgentMetadata_Result{
						Error: almostLongValue,
					},
				},
				{
					Key: "too long error",
					Result: &agentproto.WorkspaceAgentMetadata_Result{
						Error: almostLongValue + "a",
					},
				},
			},
		}
		batchSize := len(req.Metadata)
		batcher, err := metadatabatcher.NewBatcher(ctx, reg, store, ps,
			metadatabatcher.WithLogger(testutil.Logger(t)),
			metadatabatcher.WithBatchSize(batchSize),
		)
		require.NoError(t, err)
		t.Cleanup(batcher.Close)

		api := &agentapi.MetadataAPI{
			AgentFn: func(context.Context) (database.WorkspaceAgent, error) {
				return agent, nil
			},
			Workspace: &agentapi.CachedWorkspaceFields{},
			Log:       testutil.Logger(t),
			Batcher:   batcher,
			TimeNowFn: func() time.Time {
				return now
			},
		}

		resp, err := api.BatchUpdateMetadata(context.Background(), req)
		require.NoError(t, err)
		require.Equal(t, &agentproto.BatchUpdateMetadataResponse{}, resp)
		// Wait for the capacity flush to complete before test ends.
		testutil.Eventually(ctx, t, func(ctx context.Context) bool {
			return prom_testutil.ToFloat64(batcher.Metrics.MetadataTotal) == 4.0
		}, testutil.IntervalFast)
	})

	t.Run("KeysTooLong", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)

		ctrl := gomock.NewController(t)
		store := dbmock.NewMockStore(ctrl)
		ps := pubsub.NewInMemory()
		reg := prometheus.NewRegistry()

		now := dbtime.Now()
		req := &agentproto.BatchUpdateMetadataRequest{
			Metadata: []*agentproto.Metadata{
				{
					Key: "key 1",
					Result: &agentproto.WorkspaceAgentMetadata_Result{
						Value: "value 1",
					},
				},
				{
					Key: "key 2",
					Result: &agentproto.WorkspaceAgentMetadata_Result{
						Value: "value 2",
					},
				},
				{
					Key: func() string {
						key := "key 3 "
						for i := 0; i < (6144 - len("key 1") - len("key 2") - len("key 3") - 1); i++ {
							key += "a"
						}
						return key
					}(),
					Result: &agentproto.WorkspaceAgentMetadata_Result{
						Value: "value 3",
					},
				},
				{
					Key: "a", // should be ignored
					Result: &agentproto.WorkspaceAgentMetadata_Result{
						Value: "value 4",
					},
				},
			},
		}
		batchSize := len(req.Metadata)

		// This test sends 4 metadata entries but rejects the last one due to excessive key length.
		// We set the batchers batch size so that we can reliably ensure a batch is sent within the WaitShort time period.
		store.EXPECT().
			BatchUpdateWorkspaceAgentMetadata(gomock.Any(), gomock.Any()).
			Return(nil).
			Times(1)

		batcher, err := metadatabatcher.NewBatcher(ctx, reg, store, ps,
			metadatabatcher.WithLogger(testutil.Logger(t)),
			metadatabatcher.WithBatchSize(batchSize-1), // one of the keys will be rejected
		)
		require.NoError(t, err)
		t.Cleanup(batcher.Close)

		api := &agentapi.MetadataAPI{
			AgentFn: func(context.Context) (database.WorkspaceAgent, error) {
				return agent, nil
			},
			Workspace: &agentapi.CachedWorkspaceFields{},
			Log:       testutil.Logger(t),
			Batcher:   batcher,
			TimeNowFn: func() time.Time {
				return now
			},
		}

		resp, err := api.BatchUpdateMetadata(context.Background(), req)
		// Should return error because keys are too long.
		require.Error(t, err)
		require.Nil(t, resp)
		testutil.Eventually(ctx, t, func(ctx context.Context) bool {
			return prom_testutil.ToFloat64(batcher.Metrics.MetadataTotal) == 3.0
		}, testutil.IntervalFast)
	})
}
