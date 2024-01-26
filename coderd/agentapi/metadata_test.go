package agentapi_test

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/types/known/timestamppb"

	"cdr.dev/slog/sloggers/slogtest"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/agentapi"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/pubsub"
)

type fakePublisher struct {
	// Nil pointer to pass interface check.
	pubsub.Pubsub
	publishes [][]byte
}

var _ pubsub.Pubsub = &fakePublisher{}

func (f *fakePublisher) Publish(_ string, message []byte) error {
	f.publishes = append(f.publishes, message)
	return nil
}

func TestBatchUpdateMetadata(t *testing.T) {
	t.Parallel()

	agent := database.WorkspaceAgent{
		ID: uuid.New(),
	}

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		dbM := dbmock.NewMockStore(gomock.NewController(t))
		pub := &fakePublisher{}

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

		dbM.EXPECT().UpdateWorkspaceAgentMetadata(gomock.Any(), database.UpdateWorkspaceAgentMetadataParams{
			WorkspaceAgentID: agent.ID,
			Key:              []string{req.Metadata[0].Key, req.Metadata[1].Key},
			Value:            []string{req.Metadata[0].Result.Value, req.Metadata[1].Result.Value},
			Error:            []string{req.Metadata[0].Result.Error, req.Metadata[1].Result.Error},
			// The value from the agent is ignored.
			CollectedAt: []time.Time{now, now},
		}).Return(nil)

		api := &agentapi.MetadataAPI{
			AgentFn: func(context.Context) (database.WorkspaceAgent, error) {
				return agent, nil
			},
			Database: dbM,
			Pubsub:   pub,
			Log:      slogtest.Make(t, nil),
			TimeNowFn: func() time.Time {
				return now
			},
		}

		resp, err := api.BatchUpdateMetadata(context.Background(), req)
		require.NoError(t, err)
		require.Equal(t, &agentproto.BatchUpdateMetadataResponse{}, resp)

		require.Equal(t, 1, len(pub.publishes))
		var gotEvent agentapi.WorkspaceAgentMetadataChannelPayload
		require.NoError(t, json.Unmarshal(pub.publishes[0], &gotEvent))
		require.Equal(t, agentapi.WorkspaceAgentMetadataChannelPayload{
			CollectedAt: now,
			Keys:        []string{req.Metadata[0].Key, req.Metadata[1].Key},
		}, gotEvent)
	})

	t.Run("ExceededLength", func(t *testing.T) {
		t.Parallel()

		dbM := dbmock.NewMockStore(gomock.NewController(t))
		pub := pubsub.NewInMemory()

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

		dbM.EXPECT().UpdateWorkspaceAgentMetadata(gomock.Any(), database.UpdateWorkspaceAgentMetadataParams{
			WorkspaceAgentID: agent.ID,
			Key:              []string{req.Metadata[0].Key, req.Metadata[1].Key, req.Metadata[2].Key, req.Metadata[3].Key},
			Value: []string{
				almostLongValue,
				almostLongValue, // truncated
				"",
				"",
			},
			Error: []string{
				"",
				"value of 2049 bytes exceeded 2048 bytes",
				almostLongValue,
				"error of 2049 bytes exceeded 2048 bytes", // replaced
			},
			// The value from the agent is ignored.
			CollectedAt: []time.Time{now, now, now, now},
		}).Return(nil)

		api := &agentapi.MetadataAPI{
			AgentFn: func(context.Context) (database.WorkspaceAgent, error) {
				return agent, nil
			},
			Database: dbM,
			Pubsub:   pub,
			Log:      slogtest.Make(t, nil),
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

		dbM := dbmock.NewMockStore(gomock.NewController(t))
		pub := pubsub.NewInMemory()

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

		dbM.EXPECT().UpdateWorkspaceAgentMetadata(gomock.Any(), database.UpdateWorkspaceAgentMetadataParams{
			WorkspaceAgentID: agent.ID,
			// No key 4.
			Key:   []string{req.Metadata[0].Key, req.Metadata[1].Key, req.Metadata[2].Key},
			Value: []string{req.Metadata[0].Result.Value, req.Metadata[1].Result.Value, req.Metadata[2].Result.Value},
			Error: []string{req.Metadata[0].Result.Error, req.Metadata[1].Result.Error, req.Metadata[2].Result.Error},
			// The value from the agent is ignored.
			CollectedAt: []time.Time{now, now, now},
		}).Return(nil)

		api := &agentapi.MetadataAPI{
			AgentFn: func(context.Context) (database.WorkspaceAgent, error) {
				return agent, nil
			},
			Database: dbM,
			Pubsub:   pub,
			Log:      slogtest.Make(t, nil),
			TimeNowFn: func() time.Time {
				return now
			},
		}

		// Watch the pubsub for events.
		var (
			eventCount int64
			gotEvent   agentapi.WorkspaceAgentMetadataChannelPayload
		)
		cancel, err := pub.Subscribe(agentapi.WatchWorkspaceAgentMetadataChannel(agent.ID), func(ctx context.Context, message []byte) {
			if atomic.AddInt64(&eventCount, 1) > 1 {
				return
			}
			require.NoError(t, json.Unmarshal(message, &gotEvent))
		})
		require.NoError(t, err)
		defer cancel()

		resp, err := api.BatchUpdateMetadata(context.Background(), req)
		require.Error(t, err)
		require.Equal(t, "metadata keys of 6145 bytes exceeded 6144 bytes", err.Error())
		require.Nil(t, resp)

		require.Equal(t, int64(1), atomic.LoadInt64(&eventCount))
		require.Equal(t, agentapi.WorkspaceAgentMetadataChannelPayload{
			CollectedAt: now,
			// No key 4.
			Keys: []string{req.Metadata[0].Key, req.Metadata[1].Key, req.Metadata[2].Key},
		}, gotEvent)
	})
}
