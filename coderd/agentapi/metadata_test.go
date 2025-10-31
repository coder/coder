package agentapi_test

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/types/known/timestamppb"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/agentapi"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/testutil"
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
			RBACContextFn: func(ctx context.Context) context.Context {
				workspace := database.Workspace{
					ID:             uuid.New(),
					OwnerID:        uuid.New(),
					OrganizationID: uuid.New(),
				}
				return dbauthz.WithWorkspaceRBAC(ctx, workspace.RBACObject())
			},
			Database: dbM,
			Pubsub:   pub,
			Log:      testutil.Logger(t),
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
			RBACContextFn: func(ctx context.Context) context.Context {
				workspace := database.Workspace{
					ID:             uuid.New(),
					OwnerID:        uuid.New(),
					OrganizationID: uuid.New(),
				}
				return dbauthz.WithWorkspaceRBAC(ctx, workspace.RBACObject())
			},
			Database: dbM,
			Pubsub:   pub,
			Log:      testutil.Logger(t),
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
			RBACContextFn: func(ctx context.Context) context.Context {
				workspace := database.Workspace{
					ID:             uuid.New(),
					OwnerID:        uuid.New(),
					OrganizationID: uuid.New(),
				}
				return dbauthz.WithWorkspaceRBAC(ctx, workspace.RBACObject())
			},
			Database: dbM,
			Pubsub:   pub,
			Log:      testutil.Logger(t),
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

	// Test RBAC fast path with valid RBAC object - should NOT call GetWorkspaceByAgentID
	// This test verifies that when a valid RBAC object is present in context, the dbauthz layer
	// uses the fast path and skips the GetWorkspaceByAgentID database call.
	t.Run("RBACFastPath_ValidObject", func(t *testing.T) {
		t.Parallel()

		var (
			ctrl = gomock.NewController(t)
			dbM  = dbmock.NewMockStore(ctrl)
			pub  = &fakePublisher{}
			now  = dbtime.Now()
			// Set up consistent IDs that represent a valid workspace->agent relationship
			workspaceID = uuid.MustParse("12345678-1234-1234-1234-123456789012")
			ownerID     = uuid.MustParse("87654321-4321-4321-4321-210987654321")
			orgID       = uuid.MustParse("11111111-1111-1111-1111-111111111111")
			agentID     = uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
		)

		agent := database.WorkspaceAgent{
			ID: agentID,
			// In a real scenario, this agent would belong to a resource in the workspace above
		}

		req := &agentproto.BatchUpdateMetadataRequest{
			Metadata: []*agentproto.Metadata{
				{
					Key: "test_key",
					Result: &agentproto.WorkspaceAgentMetadata_Result{
						CollectedAt: timestamppb.New(now.Add(-time.Second)),
						Age:         1,
						Value:       "test_value",
					},
				},
			},
		}

		// Expect UpdateWorkspaceAgentMetadata to be called
		dbM.EXPECT().UpdateWorkspaceAgentMetadata(gomock.Any(), database.UpdateWorkspaceAgentMetadataParams{
			WorkspaceAgentID: agent.ID,
			Key:              []string{"test_key"},
			Value:            []string{"test_value"},
			Error:            []string{""},
			CollectedAt:      []time.Time{now},
		}).Return(nil)

		// DO NOT expect GetWorkspaceByAgentID - the fast path should skip this call
		// If GetWorkspaceByAgentID is called, the test will fail with "unexpected call"

		// dbauthz will call Wrappers() to check for wrapped databases
		dbM.EXPECT().Wrappers().Return([]string{}).AnyTimes()

		// Set up dbauthz to test the actual authorization layer
		auth := rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry())
		accessControlStore := &atomic.Pointer[dbauthz.AccessControlStore]{}
		var acs dbauthz.AccessControlStore = dbauthz.AGPLTemplateAccessControlStore{}
		accessControlStore.Store(&acs)

		api := &agentapi.MetadataAPI{
			AgentFn: func(_ context.Context) (database.WorkspaceAgent, error) {
				return agent, nil
			},
			RBACContextFn: func(ctx context.Context) context.Context {
				// Create a valid RBAC object with proper workspace ID, owner ID, and org ID
				// These IDs match the workspace/owner/org that this agent belongs to
				workspace := database.WorkspaceTable{
					ID:             workspaceID,
					OwnerID:        ownerID,
					OrganizationID: orgID,
				}
				return dbauthz.WithWorkspaceRBAC(ctx, workspace.RBACObject())
			},
			Database: dbauthz.New(dbM, auth, testutil.Logger(t), accessControlStore),
			Pubsub:   pub,
			Log:      testutil.Logger(t),
			TimeNowFn: func() time.Time {
				return now
			},
		}

		// Create context with system actor so authorization passes
		ctx := dbauthz.AsSystemRestricted(context.Background())
		resp, err := api.BatchUpdateMetadata(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
	})
	// Test RBAC slow path - invalid RBAC object should fall back to GetWorkspaceByAgentID
	// This test verifies that when the RBAC object has invalid IDs (nil UUIDs), the dbauthz layer
	// falls back to the slow path and calls GetWorkspaceByAgentID.
	t.Run("RBACSlowPath_InvalidObject", func(t *testing.T) {
		t.Parallel()

		var (
			ctrl        = gomock.NewController(t)
			dbM         = dbmock.NewMockStore(ctrl)
			pub         = &fakePublisher{}
			now         = dbtime.Now()
			workspaceID = uuid.MustParse("12345678-1234-1234-1234-123456789012")
			ownerID     = uuid.MustParse("87654321-4321-4321-4321-210987654321")
			orgID       = uuid.MustParse("11111111-1111-1111-1111-111111111111")
			agentID     = uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")
		)

		agent := database.WorkspaceAgent{
			ID: agentID,
		}

		req := &agentproto.BatchUpdateMetadataRequest{
			Metadata: []*agentproto.Metadata{
				{
					Key: "test_key",
					Result: &agentproto.WorkspaceAgentMetadata_Result{
						CollectedAt: timestamppb.New(now.Add(-time.Second)),
						Age:         1,
						Value:       "test_value",
					},
				},
			},
		}

		// EXPECT GetWorkspaceByAgentID to be called because the RBAC fast path validation fails
		dbM.EXPECT().GetWorkspaceByAgentID(gomock.Any(), agentID).Return(database.Workspace{
			ID:             workspaceID,
			OwnerID:        ownerID,
			OrganizationID: orgID,
		}, nil)

		// Expect UpdateWorkspaceAgentMetadata to be called after authorization
		dbM.EXPECT().UpdateWorkspaceAgentMetadata(gomock.Any(), database.UpdateWorkspaceAgentMetadataParams{
			WorkspaceAgentID: agent.ID,
			Key:              []string{"test_key"},
			Value:            []string{"test_value"},
			Error:            []string{""},
			CollectedAt:      []time.Time{now},
		}).Return(nil)

		// dbauthz will call Wrappers() to check for wrapped databases
		dbM.EXPECT().Wrappers().Return([]string{}).AnyTimes()

		// Set up dbauthz to test the actual authorization layer
		auth := rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry())
		accessControlStore := &atomic.Pointer[dbauthz.AccessControlStore]{}
		var acs dbauthz.AccessControlStore = dbauthz.AGPLTemplateAccessControlStore{}
		accessControlStore.Store(&acs)

		api := &agentapi.MetadataAPI{
			AgentFn: func(_ context.Context) (database.WorkspaceAgent, error) {
				return agent, nil
			},
			RBACContextFn: func(ctx context.Context) context.Context {
				// Create an invalid RBAC object with nil UUIDs for owner/org
				// This will fail dbauthz fast path validation and trigger GetWorkspaceByAgentID
				workspace := database.WorkspaceTable{
					ID:             uuid.MustParse("cccccccc-cccc-cccc-cccc-cccccccccccc"),
					OwnerID:        uuid.Nil, // Invalid: fails dbauthz fast path validation
					OrganizationID: uuid.Nil, // Invalid: fails dbauthz fast path validation
				}
				return dbauthz.WithWorkspaceRBAC(ctx, workspace.RBACObject())
			},
			Database: dbauthz.New(dbM, auth, testutil.Logger(t), accessControlStore),
			Pubsub:   pub,
			Log:      testutil.Logger(t),
			TimeNowFn: func() time.Time {
				return now
			},
		}

		// Create context with system actor so authorization passes
		ctx := dbauthz.AsSystemRestricted(context.Background())
		resp, err := api.BatchUpdateMetadata(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
	})
	// Test RBAC slow path - no RBAC object in context
	// This test verifies that when no RBAC object is present in context, the dbauthz layer
	// falls back to the slow path and calls GetWorkspaceByAgentID.
	t.Run("RBACSlowPath_NoObject", func(t *testing.T) {
		t.Parallel()

		var (
			ctrl        = gomock.NewController(t)
			dbM         = dbmock.NewMockStore(ctrl)
			pub         = &fakePublisher{}
			now         = dbtime.Now()
			workspaceID = uuid.MustParse("12345678-1234-1234-1234-123456789012")
			ownerID     = uuid.MustParse("87654321-4321-4321-4321-210987654321")
			orgID       = uuid.MustParse("11111111-1111-1111-1111-111111111111")
			agentID     = uuid.MustParse("dddddddd-dddd-dddd-dddd-dddddddddddd")
		)

		agent := database.WorkspaceAgent{
			ID: agentID,
		}

		req := &agentproto.BatchUpdateMetadataRequest{
			Metadata: []*agentproto.Metadata{
				{
					Key: "test_key",
					Result: &agentproto.WorkspaceAgentMetadata_Result{
						CollectedAt: timestamppb.New(now.Add(-time.Second)),
						Age:         1,
						Value:       "test_value",
					},
				},
			},
		}

		// EXPECT GetWorkspaceByAgentID to be called because no RBAC object is in context
		dbM.EXPECT().GetWorkspaceByAgentID(gomock.Any(), agentID).Return(database.Workspace{
			ID:             workspaceID,
			OwnerID:        ownerID,
			OrganizationID: orgID,
		}, nil)

		// Expect UpdateWorkspaceAgentMetadata to be called after authorization
		dbM.EXPECT().UpdateWorkspaceAgentMetadata(gomock.Any(), database.UpdateWorkspaceAgentMetadataParams{
			WorkspaceAgentID: agent.ID,
			Key:              []string{"test_key"},
			Value:            []string{"test_value"},
			Error:            []string{""},
			CollectedAt:      []time.Time{now},
		}).Return(nil)

		// dbauthz will call Wrappers() to check for wrapped databases
		dbM.EXPECT().Wrappers().Return([]string{}).AnyTimes()

		// Set up dbauthz to test the actual authorization layer
		auth := rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry())
		accessControlStore := &atomic.Pointer[dbauthz.AccessControlStore]{}
		var acs dbauthz.AccessControlStore = dbauthz.AGPLTemplateAccessControlStore{}
		accessControlStore.Store(&acs)

		api := &agentapi.MetadataAPI{
			AgentFn: func(_ context.Context) (database.WorkspaceAgent, error) {
				return agent, nil
			},
			RBACContextFn: func(ctx context.Context) context.Context {
				// Don't attach any RBAC object - return context as-is
				// This should trigger the slow path since no cached RBAC object exists
				return ctx
			},
			Database: dbauthz.New(dbM, auth, testutil.Logger(t), accessControlStore),
			Pubsub:   pub,
			Log:      testutil.Logger(t),
			TimeNowFn: func() time.Time {
				return now
			},
		}

		// Create context with system actor so authorization passes
		ctx := dbauthz.AsSystemRestricted(context.Background())
		resp, err := api.BatchUpdateMetadata(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
	})
}
