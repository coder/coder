package agentapi_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
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

		ctx := context.Background()
		ctrl := gomock.NewController(t)
		store := dbmock.NewMockStore(ctrl)
		ps := pubsub.NewInMemory()
		reg := prometheus.NewRegistry()

		// Mock the database calls that batcher will make when it flushes.
		store.EXPECT().
			BatchUpdateWorkspaceAgentMetadata(gomock.Any(), gomock.Any()).
			Return(nil).
			AnyTimes()

		// Create a real batcher for the test
		batcher, err := metadatabatcher.NewBatcher(ctx, reg, store, ps,
			metadatabatcher.WithLogger(testutil.Logger(t)),
		)
		require.NoError(t, err)
		t.Cleanup(batcher.Close)

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
	})

	t.Run("ExceededLength", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		ctrl := gomock.NewController(t)
		store := dbmock.NewMockStore(ctrl)
		ps := pubsub.NewInMemory()
		reg := prometheus.NewRegistry()

		// Mock the database calls that batcher will make when it flushes.
		store.EXPECT().
			BatchUpdateWorkspaceAgentMetadata(gomock.Any(), gomock.Any()).
			Return(nil).
			AnyTimes()

		batcher, err := metadatabatcher.NewBatcher(ctx, reg, store, ps,
			metadatabatcher.WithLogger(testutil.Logger(t)),
		)
		require.NoError(t, err)
		t.Cleanup(batcher.Close)

		almostLongValue := ""
		for i := 0; i < 2048; i++ {
			almostLongValue += "a"
		}

		now := dbtime.Now()
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
	})

	t.Run("KeysTooLong", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		ctrl := gomock.NewController(t)
		store := dbmock.NewMockStore(ctrl)
		ps := pubsub.NewInMemory()
		reg := prometheus.NewRegistry()

		// Mock the database calls that batcher will make when it flushes.
		store.EXPECT().
			BatchUpdateWorkspaceAgentMetadata(gomock.Any(), gomock.Any()).
			Return(nil).
			AnyTimes()

		batcher, err := metadatabatcher.NewBatcher(ctx, reg, store, ps,
			metadatabatcher.WithLogger(testutil.Logger(t)),
		)
		require.NoError(t, err)
		t.Cleanup(batcher.Close)

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
	})
}
