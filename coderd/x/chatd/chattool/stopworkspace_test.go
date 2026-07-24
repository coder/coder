package chattool_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestStopWorkspace(t *testing.T) {
	t.Parallel()

	t.Run("NoWorkspace", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		db, _ := dbtestutil.NewDB(t)

		user := dbgen.User(t, db, database.User{})
		modelCfg := seedModelConfig(t, db)
		org := dbgen.Organization(t, db, database.Organization{})
		_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
			UserID:         user.ID,
			OrganizationID: org.ID,
		})
		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    org.ID,
			OwnerID:           user.ID,
			LastModelConfigID: modelCfg.ID,
			Title:             "test-stop-no-workspace",
		})

		tool := chattool.StopWorkspace(db, chat.ID, chattool.StopWorkspaceOptions{
			StopFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ codersdk.CreateWorkspaceBuildRequest) (codersdk.WorkspaceBuild, error) {
				t.Fatal("StopFn should not be called")
				return codersdk.WorkspaceBuild{}, nil
			},
			WorkspaceMu: &sync.Mutex{},
		})

		resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "call-1", Name: "stop_workspace", Input: "{}"})
		require.NoError(t, err)
		require.Contains(t, resp.Content, "use create_workspace first")
	})

	t.Run("DeletedWorkspace", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		db, _ := dbtestutil.NewDB(t)

		user := dbgen.User(t, db, database.User{})
		modelCfg := seedModelConfig(t, db)
		org := dbgen.Organization(t, db, database.Organization{})
		_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
			UserID:         user.ID,
			OrganizationID: org.ID,
		})
		wsResp := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OwnerID:        user.ID,
			OrganizationID: org.ID,
			Deleted:        true,
		}).Seed(database.WorkspaceBuild{
			Transition: database.WorkspaceTransitionDelete,
		}).Do()
		ws := wsResp.Workspace

		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    org.ID,
			OwnerID:           user.ID,
			WorkspaceID:       uuid.NullUUID{UUID: ws.ID, Valid: true},
			LastModelConfigID: modelCfg.ID,
			Title:             "test-stop-deleted-workspace",
		})

		tool := chattool.StopWorkspace(db, chat.ID, chattool.StopWorkspaceOptions{
			StopFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ codersdk.CreateWorkspaceBuildRequest) (codersdk.WorkspaceBuild, error) {
				t.Fatal("StopFn should not be called for deleted workspace")
				return codersdk.WorkspaceBuild{}, nil
			},
			WorkspaceMu: &sync.Mutex{},
		})

		resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "call-1", Name: "stop_workspace", Input: "{}"})
		require.NoError(t, err)
		require.Contains(t, resp.Content, "workspace was deleted")
		require.Contains(t, resp.Content, "create_workspace")
	})

	t.Run("AlreadyStopped", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		db, _ := dbtestutil.NewDB(t)

		user := dbgen.User(t, db, database.User{})
		modelCfg := seedModelConfig(t, db)
		org := dbgen.Organization(t, db, database.Organization{})
		_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
			UserID:         user.ID,
			OrganizationID: org.ID,
		})
		wsResp := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OwnerID:        user.ID,
			OrganizationID: org.ID,
		}).Seed(database.WorkspaceBuild{
			Transition: database.WorkspaceTransitionStop,
		}).Do()
		ws := wsResp.Workspace

		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    org.ID,
			OwnerID:           user.ID,
			WorkspaceID:       uuid.NullUUID{UUID: ws.ID, Valid: true},
			LastModelConfigID: modelCfg.ID,
			Title:             "test-stop-already-stopped",
		})

		tool := chattool.StopWorkspace(db, chat.ID, chattool.StopWorkspaceOptions{
			OwnerID: user.ID,
			StopFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ codersdk.CreateWorkspaceBuildRequest) (codersdk.WorkspaceBuild, error) {
				t.Fatal("StopFn should not be called for already-stopped workspace")
				return codersdk.WorkspaceBuild{}, nil
			},
			WorkspaceMu: &sync.Mutex{},
		})

		resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "call-1", Name: "stop_workspace", Input: "{}"})
		require.NoError(t, err)

		var result map[string]any
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		require.Equal(t, true, result["stopped"])
		require.Equal(t, ws.Name, result["workspace_name"])
		require.Equal(t, true, result["no_build"])
		require.Nil(t, result["build_id"])
	})

	t.Run("RunningWorkspaceStops", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		db, _ := dbtestutil.NewDB(t)

		user := dbgen.User(t, db, database.User{})
		modelCfg := seedModelConfig(t, db)
		org := dbgen.Organization(t, db, database.Organization{})
		_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
			UserID:         user.ID,
			OrganizationID: org.ID,
		})
		wsResp := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OwnerID:        user.ID,
			OrganizationID: org.ID,
		}).Seed(database.WorkspaceBuild{
			Transition: database.WorkspaceTransitionStart,
		}).Do()
		ws := wsResp.Workspace

		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    org.ID,
			OwnerID:           user.ID,
			WorkspaceID:       uuid.NullUUID{UUID: ws.ID, Valid: true},
			LastModelConfigID: modelCfg.ID,
			Title:             "test-stop-running-workspace",
		})

		var stopCalled atomic.Bool
		var stopBuildID uuid.UUID
		var seenBuildID uuid.UUID
		var onChatUpdatedCalls atomic.Int32
		tool := chattool.StopWorkspace(db, chat.ID, chattool.StopWorkspaceOptions{
			OwnerID: user.ID,
			StopFn: func(_ context.Context, _ uuid.UUID, wsID uuid.UUID, req codersdk.CreateWorkspaceBuildRequest) (codersdk.WorkspaceBuild, error) {
				stopCalled.Store(true)
				require.Equal(t, ws.ID, wsID)
				require.Equal(t, codersdk.WorkspaceTransitionStop, req.Transition)
				buildResp := dbfake.WorkspaceBuild(t, db, ws).Seed(database.WorkspaceBuild{
					Transition:  database.WorkspaceTransitionStop,
					BuildNumber: 2,
				}).Do()
				stopBuildID = buildResp.Build.ID
				return codersdk.WorkspaceBuild{ID: buildResp.Build.ID}, nil
			},
			WorkspaceMu: &sync.Mutex{},
			OnChatUpdated: func(chat database.Chat) {
				onChatUpdatedCalls.Add(1)
				if chat.BuildID.Valid {
					seenBuildID = chat.BuildID.UUID
				}
			},
		})

		resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "call-1", Name: "stop_workspace", Input: "{}"})
		require.NoError(t, err)
		require.True(t, stopCalled.Load())

		var result map[string]any
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		require.Equal(t, true, result["stopped"])
		require.Equal(t, ws.Name, result["workspace_name"])
		require.Equal(t, stopBuildID.String(), result["build_id"])
		require.Nil(t, result["no_build"])

		require.GreaterOrEqual(t, onChatUpdatedCalls.Load(), int32(1))
		require.Equal(t, stopBuildID, seenBuildID)

		updatedChat, err := db.GetChatByID(ctx, chat.ID)
		require.NoError(t, err)
		require.True(t, updatedChat.BuildID.Valid)
		require.Equal(t, stopBuildID, updatedChat.BuildID.UUID)
	})

	t.Run("InProgressBuildWaitsThenStops", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		db, _ := dbtestutil.NewDB(t)

		user := dbgen.User(t, db, database.User{})
		modelCfg := seedModelConfig(t, db)
		org := dbgen.Organization(t, db, database.Organization{})
		_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
			UserID:         user.ID,
			OrganizationID: org.ID,
		})
		wsResp := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OwnerID:        user.ID,
			OrganizationID: org.ID,
		}).Seed(database.WorkspaceBuild{
			Transition: database.WorkspaceTransitionStart,
		}).Starting().Do()
		ws := wsResp.Workspace

		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    org.ID,
			OwnerID:           user.ID,
			WorkspaceID:       uuid.NullUUID{UUID: ws.ID, Valid: true},
			LastModelConfigID: modelCfg.ID,
			Title:             "test-stop-in-progress-build",
		})

		jobRead := make(chan struct{}, 1)
		wrappedDB := &jobInterceptStore{Store: db, jobRead: jobRead}
		var stopCalled atomic.Bool
		var stopBuildID uuid.UUID
		var onChatUpdatedCalled atomic.Bool
		tool := chattool.StopWorkspace(wrappedDB, chat.ID, chattool.StopWorkspaceOptions{
			OwnerID: user.ID,
			StopFn: func(_ context.Context, _ uuid.UUID, wsID uuid.UUID, req codersdk.CreateWorkspaceBuildRequest) (codersdk.WorkspaceBuild, error) {
				stopCalled.Store(true)
				require.Equal(t, ws.ID, wsID)
				require.Equal(t, codersdk.WorkspaceTransitionStop, req.Transition)
				buildResp := dbfake.WorkspaceBuild(t, db, ws).Seed(database.WorkspaceBuild{
					Transition:  database.WorkspaceTransitionStop,
					BuildNumber: 2,
				}).Do()
				stopBuildID = buildResp.Build.ID
				return codersdk.WorkspaceBuild{ID: buildResp.Build.ID}, nil
			},
			WorkspaceMu:   &sync.Mutex{},
			Logger:        slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
			OnChatUpdated: func(_ database.Chat) { onChatUpdatedCalled.Store(true) },
		})

		type toolResult struct {
			resp fantasy.ToolResponse
			err  error
		}
		done := make(chan toolResult, 1)
		go func() {
			resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "call-1", Name: "stop_workspace", Input: "{}"})
			done <- toolResult{resp: resp, err: err}
		}()

		testutil.TryReceive(ctx, t, jobRead)
		require.False(t, stopCalled.Load(), "StopFn must wait for the in-progress build")

		now := time.Now().UTC()
		require.NoError(t, db.UpdateProvisionerJobWithCompleteByID(ctx, database.UpdateProvisionerJobWithCompleteByIDParams{
			ID:          wsResp.Build.JobID,
			UpdatedAt:   now,
			CompletedAt: sql.NullTime{Time: now, Valid: true},
		}))

		res := testutil.TryReceive(ctx, t, done)
		require.NoError(t, res.err)
		require.True(t, stopCalled.Load())
		require.True(t, onChatUpdatedCalled.Load())

		var result map[string]any
		require.NoError(t, json.Unmarshal([]byte(res.resp.Content), &result))
		require.Equal(t, true, result["stopped"])
		require.Equal(t, stopBuildID.String(), result["build_id"])
	})

	t.Run("FailedLatestStopBuildStillStops", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		db, _ := dbtestutil.NewDB(t)

		user := dbgen.User(t, db, database.User{})
		modelCfg := seedModelConfig(t, db)
		org := dbgen.Organization(t, db, database.Organization{})
		_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
			UserID:         user.ID,
			OrganizationID: org.ID,
		})
		wsResp := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OwnerID:        user.ID,
			OrganizationID: org.ID,
		}).Seed(database.WorkspaceBuild{
			Transition: database.WorkspaceTransitionStop,
		}).Do()
		ws := wsResp.Workspace
		now := time.Now().UTC()
		require.NoError(t, db.UpdateProvisionerJobWithCompleteByID(ctx, database.UpdateProvisionerJobWithCompleteByIDParams{
			ID:          wsResp.Build.JobID,
			UpdatedAt:   now,
			CompletedAt: sql.NullTime{Time: now, Valid: true},
			Error:       sql.NullString{String: "latest build failed", Valid: true},
		}))

		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    org.ID,
			OwnerID:           user.ID,
			WorkspaceID:       uuid.NullUUID{UUID: ws.ID, Valid: true},
			LastModelConfigID: modelCfg.ID,
			Title:             "test-stop-failed-latest-build",
		})

		var stopCalled atomic.Bool
		tool := chattool.StopWorkspace(db, chat.ID, chattool.StopWorkspaceOptions{
			OwnerID: user.ID,
			StopFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID, req codersdk.CreateWorkspaceBuildRequest) (codersdk.WorkspaceBuild, error) {
				stopCalled.Store(true)
				require.Equal(t, codersdk.WorkspaceTransitionStop, req.Transition)
				buildResp := dbfake.WorkspaceBuild(t, db, ws).Seed(database.WorkspaceBuild{
					Transition:  database.WorkspaceTransitionStop,
					BuildNumber: 2,
				}).Do()
				return codersdk.WorkspaceBuild{ID: buildResp.Build.ID}, nil
			},
			WorkspaceMu: &sync.Mutex{},
		})

		resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "call-1", Name: "stop_workspace", Input: "{}"})
		require.NoError(t, err)
		require.True(t, stopCalled.Load())

		var result map[string]any
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		require.Equal(t, true, result["stopped"])
	})

	t.Run("StopTriggeredBuildFailure", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		db, _ := dbtestutil.NewDB(t)

		user := dbgen.User(t, db, database.User{})
		modelCfg := seedModelConfig(t, db)
		org := dbgen.Organization(t, db, database.Organization{})
		_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
			UserID:         user.ID,
			OrganizationID: org.ID,
		})
		wsResp := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OwnerID:        user.ID,
			OrganizationID: org.ID,
		}).Seed(database.WorkspaceBuild{
			Transition: database.WorkspaceTransitionStart,
		}).Do()
		ws := wsResp.Workspace

		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    org.ID,
			OwnerID:           user.ID,
			WorkspaceID:       uuid.NullUUID{UUID: ws.ID, Valid: true},
			LastModelConfigID: modelCfg.ID,
			Title:             "test-stop-triggered-build-failure",
		})

		var stopBuildJobID uuid.UUID
		var stopBuildID uuid.UUID
		stopFn := func(_ context.Context, _ uuid.UUID, wsID uuid.UUID, req codersdk.CreateWorkspaceBuildRequest) (codersdk.WorkspaceBuild, error) {
			require.Equal(t, ws.ID, wsID)
			require.Equal(t, codersdk.WorkspaceTransitionStop, req.Transition)
			buildResp := dbfake.WorkspaceBuild(t, db, ws).Seed(database.WorkspaceBuild{
				Transition:  database.WorkspaceTransitionStop,
				BuildNumber: 2,
			}).Starting().Do()
			stopBuildJobID = buildResp.Build.JobID
			stopBuildID = buildResp.Build.ID
			return codersdk.WorkspaceBuild{ID: buildResp.Build.ID}, nil
		}

		jobRead := make(chan struct{}, 2)
		wrappedDB := &jobInterceptStore{Store: db, jobRead: jobRead}
		tool := chattool.StopWorkspace(wrappedDB, chat.ID, chattool.StopWorkspaceOptions{
			OwnerID:     user.ID,
			StopFn:      stopFn,
			WorkspaceMu: &sync.Mutex{},
			Logger:      slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		})

		type toolResult struct {
			resp fantasy.ToolResponse
			err  error
		}
		done := make(chan toolResult, 1)
		go func() {
			resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "call-1", Name: "stop_workspace", Input: "{}"})
			done <- toolResult{resp: resp, err: err}
		}()

		testutil.TryReceive(ctx, t, jobRead)
		testutil.TryReceive(ctx, t, jobRead)

		now := time.Now().UTC()
		require.NoError(t, db.UpdateProvisionerJobWithCompleteByID(ctx, database.UpdateProvisionerJobWithCompleteByIDParams{
			ID:          stopBuildJobID,
			UpdatedAt:   now,
			CompletedAt: sql.NullTime{Time: now, Valid: true},
			Error:       sql.NullString{String: "terraform destroy failed", Valid: true},
		}))

		res := testutil.TryReceive(ctx, t, done)
		require.NoError(t, res.err)

		var result map[string]any
		require.NoError(t, json.Unmarshal([]byte(res.resp.Content), &result))
		require.Contains(t, result["error"], "workspace stop build failed")
		require.Equal(t, stopBuildID.String(), result["build_id"])
		require.False(t, res.resp.IsError,
			"buildToolResponse must not set IsError; chatprompt strips structured fields from error responses")
	})
}
