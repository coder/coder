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
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

func TestStartWorkspace(t *testing.T) {
	t.Parallel()

	t.Run("NoWorkspace", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		db, _ := dbtestutil.NewDB(t)

		user := dbgen.User(t, db, database.User{})
		modelCfg := seedModelConfig(ctx, t, db, user.ID)
		org := dbgen.Organization(t, db, database.Organization{})
		_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
			UserID:         user.ID,
			OrganizationID: org.ID,
		})

		chat, err := db.InsertChat(ctx, database.InsertChatParams{
			OrganizationID:    org.ID,
			Status:            database.ChatStatusWaiting,
			OwnerID:           user.ID,
			LastModelConfigID: modelCfg.ID,
			Title:             "test-no-workspace",
		})
		require.NoError(t, err)

		tool := chattool.StartWorkspace(chattool.StartWorkspaceOptions{
			DB:     db,
			ChatID: chat.ID,
			StartFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ codersdk.CreateWorkspaceBuildRequest) (codersdk.WorkspaceBuild, error) {
				t.Fatal("StartFn should not be called")
				return codersdk.WorkspaceBuild{}, nil
			},
			WorkspaceMu: &sync.Mutex{},
		})

		resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "call-1", Name: "start_workspace", Input: "{}"})
		require.NoError(t, err)
		require.Contains(t, resp.Content, "no workspace")
	})

	t.Run("AlreadyRunning", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		db, _ := dbtestutil.NewDB(t)

		user := dbgen.User(t, db, database.User{})
		modelCfg := seedModelConfig(ctx, t, db, user.ID)
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

		chat, err := db.InsertChat(ctx, database.InsertChatParams{
			OrganizationID:    org.ID,
			Status:            database.ChatStatusWaiting,
			OwnerID:           user.ID,
			WorkspaceID:       uuid.NullUUID{UUID: ws.ID, Valid: true},
			LastModelConfigID: modelCfg.ID,
			Title:             "test-already-running",
		})
		require.NoError(t, err)

		agentConnFn := func(_ context.Context, _ uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			return nil, func() {}, nil
		}

		tool := chattool.StartWorkspace(chattool.StartWorkspaceOptions{
			DB:          db,
			OwnerID:     user.ID,
			ChatID:      chat.ID,
			AgentConnFn: agentConnFn,
			StartFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ codersdk.CreateWorkspaceBuildRequest) (codersdk.WorkspaceBuild, error) {
				t.Fatal("StartFn should not be called for already-running workspace")
				return codersdk.WorkspaceBuild{}, nil
			},
			WorkspaceMu: &sync.Mutex{},
		})

		resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "call-1", Name: "start_workspace", Input: "{}"})
		require.NoError(t, err)

		var result map[string]any
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		started, ok := result["started"].(bool)
		require.True(t, ok)
		require.True(t, started)
		require.Nil(t, result["build_id"], "build_id should not be present when workspace was already running")
		require.Equal(t, true, result["no_build"], "no_build should be true when workspace was already running")
	})

	t.Run("AlreadyRunningPrefersChatSuffixAgent", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		db, _ := dbtestutil.NewDB(t)

		user := dbgen.User(t, db, database.User{})
		modelCfg := seedModelConfig(ctx, t, db, user.ID)
		org := dbgen.Organization(t, db, database.Organization{})
		_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
			UserID:         user.ID,
			OrganizationID: org.ID,
		})
		wsResp := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OwnerID:        user.ID,
			OrganizationID: org.ID,
		}).WithAgent(func(agents []*sdkproto.Agent) []*sdkproto.Agent {
			agents[0].Name = "dev"
			return append(agents, &sdkproto.Agent{
				Id:   uuid.NewString(),
				Name: "dev-coderd-chat",
				Auth: &sdkproto.Agent_Token{Token: uuid.NewString()},
				Env:  map[string]string{},
			})
		}).Seed(database.WorkspaceBuild{
			Transition: database.WorkspaceTransitionStart,
		}).Do()
		ws := wsResp.Workspace

		now := time.Now().UTC()
		preferredAgentID := uuid.Nil
		for _, agent := range wsResp.Agents {
			if agent.Name == "dev-coderd-chat" {
				preferredAgentID = agent.ID
			}
			err := db.UpdateWorkspaceAgentLifecycleStateByID(ctx, database.UpdateWorkspaceAgentLifecycleStateByIDParams{
				ID:             agent.ID,
				LifecycleState: database.WorkspaceAgentLifecycleStateReady,
				StartedAt:      sql.NullTime{Time: now, Valid: true},
				ReadyAt:        sql.NullTime{Time: now, Valid: true},
			})
			require.NoError(t, err)
		}
		require.NotEqual(t, uuid.Nil, preferredAgentID)

		chat, err := db.InsertChat(ctx, database.InsertChatParams{
			OrganizationID:    org.ID,
			Status:            database.ChatStatusWaiting,
			OwnerID:           user.ID,
			WorkspaceID:       uuid.NullUUID{UUID: ws.ID, Valid: true},
			LastModelConfigID: modelCfg.ID,
			Title:             "test-running-preferred-agent",
		})
		require.NoError(t, err)

		var connectedAgentID uuid.UUID
		agentConnFn := func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			connectedAgentID = agentID
			return nil, func() {}, nil
		}

		tool := chattool.StartWorkspace(chattool.StartWorkspaceOptions{
			DB:          db,
			OwnerID:     user.ID,
			ChatID:      chat.ID,
			AgentConnFn: agentConnFn,
			StartFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ codersdk.CreateWorkspaceBuildRequest) (codersdk.WorkspaceBuild, error) {
				t.Fatal("StartFn should not be called for already-running workspace")
				return codersdk.WorkspaceBuild{}, nil
			},
			WorkspaceMu: &sync.Mutex{},
		})

		resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "call-1", Name: "start_workspace", Input: "{}"})
		require.NoError(t, err)
		require.Equal(t, preferredAgentID, connectedAgentID)

		var result map[string]any
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		started, ok := result["started"].(bool)
		require.True(t, ok)
		require.True(t, started)
	})

	t.Run("AlreadyRunningWithoutAgentsReturnsNoAgent", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		db, _ := dbtestutil.NewDB(t)

		user := dbgen.User(t, db, database.User{})
		modelCfg := seedModelConfig(ctx, t, db, user.ID)
		org := dbgen.Organization(t, db, database.Organization{})
		_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
			UserID:         user.ID,
			OrganizationID: org.ID,
		})
		wsResp := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OwnerID:        user.ID,
			OrganizationID: org.ID,
		}).WithAgent(func(_ []*sdkproto.Agent) []*sdkproto.Agent {
			return nil
		}).Seed(database.WorkspaceBuild{
			Transition: database.WorkspaceTransitionStart,
		}).Do()
		ws := wsResp.Workspace

		chat, err := db.InsertChat(ctx, database.InsertChatParams{
			OrganizationID:    org.ID,
			Status:            database.ChatStatusWaiting,
			OwnerID:           user.ID,
			WorkspaceID:       uuid.NullUUID{UUID: ws.ID, Valid: true},
			LastModelConfigID: modelCfg.ID,
			Title:             "test-running-no-agent",
		})
		require.NoError(t, err)

		tool := chattool.StartWorkspace(chattool.StartWorkspaceOptions{
			DB:      db,
			OwnerID: user.ID,
			ChatID:  chat.ID,
			AgentConnFn: func(_ context.Context, _ uuid.UUID) (workspacesdk.AgentConn, func(), error) {
				t.Fatal("AgentConnFn should not be called when no agents exist")
				return nil, func() {}, nil
			},
			StartFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ codersdk.CreateWorkspaceBuildRequest) (codersdk.WorkspaceBuild, error) {
				t.Fatal("StartFn should not be called for already-running workspace")
				return codersdk.WorkspaceBuild{}, nil
			},
			WorkspaceMu: &sync.Mutex{},
		})

		resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "call-1", Name: "start_workspace", Input: "{}"})
		require.NoError(t, err)

		var result map[string]any
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		started, ok := result["started"].(bool)
		require.True(t, ok)
		require.True(t, started)
		require.Equal(t, "no_agent", result["agent_status"])
	})

	t.Run("AlreadyRunningPreservesAgentSelectionError", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		db, _ := dbtestutil.NewDB(t)

		user := dbgen.User(t, db, database.User{})
		modelCfg := seedModelConfig(ctx, t, db, user.ID)
		org := dbgen.Organization(t, db, database.Organization{})
		_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
			UserID:         user.ID,
			OrganizationID: org.ID,
		})
		wsResp := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OwnerID:        user.ID,
			OrganizationID: org.ID,
		}).WithAgent(func(agents []*sdkproto.Agent) []*sdkproto.Agent {
			agents[0].Name = "alpha-coderd-chat"
			return append(agents, &sdkproto.Agent{
				Id:   uuid.NewString(),
				Name: "beta-coderd-chat",
				Auth: &sdkproto.Agent_Token{Token: uuid.NewString()},
				Env:  map[string]string{},
			})
		}).Seed(database.WorkspaceBuild{
			Transition: database.WorkspaceTransitionStart,
		}).Do()
		ws := wsResp.Workspace

		chat, err := db.InsertChat(ctx, database.InsertChatParams{
			OrganizationID:    org.ID,
			Status:            database.ChatStatusWaiting,
			OwnerID:           user.ID,
			WorkspaceID:       uuid.NullUUID{UUID: ws.ID, Valid: true},
			LastModelConfigID: modelCfg.ID,
			Title:             "test-running-selection-error",
		})
		require.NoError(t, err)

		tool := chattool.StartWorkspace(chattool.StartWorkspaceOptions{
			DB:      db,
			OwnerID: user.ID,
			ChatID:  chat.ID,
			AgentConnFn: func(_ context.Context, _ uuid.UUID) (workspacesdk.AgentConn, func(), error) {
				t.Fatal("AgentConnFn should not be called when agent selection fails")
				return nil, func() {}, nil
			},
			StartFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ codersdk.CreateWorkspaceBuildRequest) (codersdk.WorkspaceBuild, error) {
				t.Fatal("StartFn should not be called for already-running workspace")
				return codersdk.WorkspaceBuild{}, nil
			},
			WorkspaceMu: &sync.Mutex{},
		})

		resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "call-1", Name: "start_workspace", Input: "{}"})
		require.NoError(t, err)

		var result map[string]any
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		started, ok := result["started"].(bool)
		require.True(t, ok)
		require.True(t, started)
		require.Equal(t, "selection_error", result["agent_status"])
		require.Contains(t, result["agent_error"], "multiple agents match the chat suffix")
	})

	t.Run("StoppedWorkspace", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		db, _ := dbtestutil.NewDB(t)

		user := dbgen.User(t, db, database.User{})
		modelCfg := seedModelConfig(ctx, t, db, user.ID)
		org := dbgen.Organization(t, db, database.Organization{})
		_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
			UserID:         user.ID,
			OrganizationID: org.ID,
		})
		// Create a completed "stop" build so the workspace is stopped.
		wsResp := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OwnerID:        user.ID,
			OrganizationID: org.ID,
		}).Seed(database.WorkspaceBuild{
			Transition: database.WorkspaceTransitionStop,
		}).Do()
		ws := wsResp.Workspace

		chat, err := db.InsertChat(ctx, database.InsertChatParams{
			OrganizationID:    org.ID,
			Status:            database.ChatStatusWaiting,
			OwnerID:           user.ID,
			WorkspaceID:       uuid.NullUUID{UUID: ws.ID, Valid: true},
			LastModelConfigID: modelCfg.ID,
			Title:             "test-stopped-workspace",
		})
		require.NoError(t, err)

		var startCalled bool
		var startBuildID uuid.UUID
		startFn := func(_ context.Context, _ uuid.UUID, wsID uuid.UUID, req codersdk.CreateWorkspaceBuildRequest) (codersdk.WorkspaceBuild, error) {
			startCalled = true
			require.Equal(t, codersdk.WorkspaceTransitionStart, req.Transition)
			require.Equal(t, ws.ID, wsID)
			// Simulate start by inserting a new completed "start" build.
			buildResp := dbfake.WorkspaceBuild(t, db, ws).Seed(database.WorkspaceBuild{
				Transition:  database.WorkspaceTransitionStart,
				BuildNumber: 2,
			}).Do()
			startBuildID = buildResp.Build.ID
			return codersdk.WorkspaceBuild{ID: buildResp.Build.ID}, nil
		}

		agentConnFn := func(_ context.Context, _ uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			return nil, func() {}, nil
		}

		tool := chattool.StartWorkspace(chattool.StartWorkspaceOptions{
			DB:          db,
			OwnerID:     user.ID,
			ChatID:      chat.ID,
			StartFn:     startFn,
			AgentConnFn: agentConnFn,
			WorkspaceMu: &sync.Mutex{},
		})

		resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "call-1", Name: "start_workspace", Input: "{}"})
		require.NoError(t, err)
		require.True(t, startCalled, "expected StartFn to be called")

		var result map[string]any
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		started, ok := result["started"].(bool)
		require.True(t, ok)
		require.True(t, started)
		require.Equal(t, startBuildID.String(), result["build_id"])
		require.Nil(t, result["no_build"], "no_build should not be set when a build was triggered")
	})

	t.Run("InProgressBuild", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		db, _ := dbtestutil.NewDB(t)

		user := dbgen.User(t, db, database.User{})
		modelCfg := seedModelConfig(ctx, t, db, user.ID)
		org := dbgen.Organization(t, db, database.Organization{})
		_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
			UserID:         user.ID,
			OrganizationID: org.ID,
		})
		// Create a workspace with a build that is still running.
		wsResp := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OwnerID:        user.ID,
			OrganizationID: org.ID,
		}).Seed(database.WorkspaceBuild{
			Transition: database.WorkspaceTransitionStart,
		}).Starting().Do()
		ws := wsResp.Workspace

		chat, err := db.InsertChat(ctx, database.InsertChatParams{
			OrganizationID:    org.ID,
			Status:            database.ChatStatusWaiting,
			OwnerID:           user.ID,
			WorkspaceID:       uuid.NullUUID{UUID: ws.ID, Valid: true},
			LastModelConfigID: modelCfg.ID,
			Title:             "test-in-progress-build",
		})
		require.NoError(t, err)

		// Wrap the DB so we know exactly when the tool reads
		// the job status. The interceptor signals AFTER the
		// first GetProvisionerJobByID read completes, so the
		// main goroutine can safely complete the build knowing
		// the tool already observed Running.
		jobRead := make(chan struct{}, 1)
		wrappedDB := &jobInterceptStore{Store: db, jobRead: jobRead}

		agentConnFn := func(_ context.Context, _ uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			return nil, func() {}, nil
		}

		var onChatUpdatedCalled atomic.Bool
		tool := chattool.StartWorkspace(chattool.StartWorkspaceOptions{
			DB:          wrappedDB,
			OwnerID:     user.ID,
			ChatID:      chat.ID,
			AgentConnFn: agentConnFn,
			StartFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ codersdk.CreateWorkspaceBuildRequest) (codersdk.WorkspaceBuild, error) {
				t.Fatal("StartFn should not be called for an in-progress build")
				return codersdk.WorkspaceBuild{}, nil
			},
			WorkspaceMu:   &sync.Mutex{},
			Logger:        slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
			OnChatUpdated: func(_ database.Chat) { onChatUpdatedCalled.Store(true) },
		})

		// Run tool.Run in a goroutine. It will see the job as
		// Running and enter waitForBuild which polls every 2s.
		type toolResult struct {
			resp fantasy.ToolResponse
			err  error
		}
		done := make(chan toolResult, 1)
		go func() {
			resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "call-1", Name: "start_workspace", Input: "{}"})
			done <- toolResult{resp, err}
		}()

		// Wait for the tool to read the job status (Running).
		testutil.TryReceive(ctx, t, jobRead)

		// Now complete the build. The next poll in waitForBuild
		// will see Succeeded and return the build ID.
		now := time.Now().UTC()
		require.NoError(t, db.UpdateProvisionerJobWithCompleteByID(ctx, database.UpdateProvisionerJobWithCompleteByIDParams{
			ID:          wsResp.Build.JobID,
			UpdatedAt:   now,
			CompletedAt: sql.NullTime{Time: now, Valid: true},
		}))

		res := testutil.TryReceive(ctx, t, done)
		require.NoError(t, res.err)
		resp := res.resp

		var result map[string]any
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		started, ok := result["started"].(bool)
		require.True(t, ok)
		require.True(t, started)
		require.Equal(t, wsResp.Build.ID.String(), result["build_id"])
		require.True(t, onChatUpdatedCalled.Load(), "OnChatUpdated should be called to notify frontend of build ID")
	})

	t.Run("FailedBuild", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, _ := dbtestutil.NewDB(t)

		user := dbgen.User(t, db, database.User{})
		modelCfg := seedModelConfig(ctx, t, db, user.ID)
		org := dbgen.Organization(t, db, database.Organization{})
		_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
			UserID:         user.ID,
			OrganizationID: org.ID,
		})
		// Create a workspace with a build that is still running.
		wsResp := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OwnerID:        user.ID,
			OrganizationID: org.ID,
		}).Seed(database.WorkspaceBuild{
			Transition: database.WorkspaceTransitionStart,
		}).Starting().Do()
		ws := wsResp.Workspace

		chat, err := db.InsertChat(ctx, database.InsertChatParams{
			OrganizationID:    org.ID,
			Status:            database.ChatStatusWaiting,
			OwnerID:           user.ID,
			WorkspaceID:       uuid.NullUUID{UUID: ws.ID, Valid: true},
			LastModelConfigID: modelCfg.ID,
			Title:             "test-failed-build",
		})
		require.NoError(t, err)

		jobRead := make(chan struct{}, 1)
		wrappedDB := &jobInterceptStore{Store: db, jobRead: jobRead}

		tool := chattool.StartWorkspace(chattool.StartWorkspaceOptions{
			DB:      wrappedDB,
			OwnerID: user.ID,
			ChatID:  chat.ID,
			AgentConnFn: func(_ context.Context, _ uuid.UUID) (workspacesdk.AgentConn, func(), error) {
				return nil, func() {}, nil
			},
			StartFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ codersdk.CreateWorkspaceBuildRequest) (codersdk.WorkspaceBuild, error) {
				t.Fatal("StartFn should not be called for an in-progress build")
				return codersdk.WorkspaceBuild{}, nil
			},
			WorkspaceMu: &sync.Mutex{},
			Logger:      slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		})

		type toolResult struct {
			resp fantasy.ToolResponse
			err  error
		}
		done := make(chan toolResult, 1)
		go func() {
			resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "call-1", Name: "start_workspace", Input: "{}"})
			done <- toolResult{resp, err}
		}()

		// Wait for the tool to observe the running job.
		testutil.TryReceive(ctx, t, jobRead)

		// Fail the build.
		now := time.Now().UTC()
		require.NoError(t, db.UpdateProvisionerJobWithCompleteByID(ctx, database.UpdateProvisionerJobWithCompleteByIDParams{
			ID:          wsResp.Build.JobID,
			UpdatedAt:   now,
			CompletedAt: sql.NullTime{Time: now, Valid: true},
			Error:       sql.NullString{String: "terraform apply failed", Valid: true},
		}))

		res := testutil.TryReceive(ctx, t, done)
		require.NoError(t, res.err)

		var result map[string]any
		require.NoError(t, json.Unmarshal([]byte(res.resp.Content), &result))
		require.Contains(t, result["error"], "waiting for in-progress build")
		require.Equal(t, wsResp.Build.ID.String(), result["build_id"])
		require.False(t, res.resp.IsError,
			"buildToolResponse must not set IsError; chatprompt strips structured fields from error responses")
	})

	t.Run("StartTriggeredBuildFailure", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		db, _ := dbtestutil.NewDB(t)

		user := dbgen.User(t, db, database.User{})
		modelCfg := seedModelConfig(ctx, t, db, user.ID)
		org := dbgen.Organization(t, db, database.Organization{})
		_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
			UserID:         user.ID,
			OrganizationID: org.ID,
		})
		// Create a stopped workspace (succeeded stop transition).
		wsResp := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OwnerID:        user.ID,
			OrganizationID: org.ID,
		}).Seed(database.WorkspaceBuild{
			Transition: database.WorkspaceTransitionStop,
		}).Do()
		ws := wsResp.Workspace

		chat, err := db.InsertChat(ctx, database.InsertChatParams{
			OrganizationID:    org.ID,
			Status:            database.ChatStatusWaiting,
			OwnerID:           user.ID,
			WorkspaceID:       uuid.NullUUID{UUID: ws.ID, Valid: true},
			LastModelConfigID: modelCfg.ID,
			Title:             "test-start-triggered-build-failure",
		})
		require.NoError(t, err)

		// StartFn creates a real in-progress build via dbfake.
		var startBuildJobID uuid.UUID
		var startBuildID uuid.UUID
		startFn := func(_ context.Context, _ uuid.UUID, wsID uuid.UUID, req codersdk.CreateWorkspaceBuildRequest) (codersdk.WorkspaceBuild, error) {
			require.Equal(t, codersdk.WorkspaceTransitionStart, req.Transition)
			require.Equal(t, ws.ID, wsID)
			buildResp := dbfake.WorkspaceBuild(t, db, ws).Seed(database.WorkspaceBuild{
				Transition:  database.WorkspaceTransitionStart,
				BuildNumber: 2,
			}).Starting().Do()
			startBuildJobID = buildResp.Build.JobID
			startBuildID = buildResp.Build.ID
			return codersdk.WorkspaceBuild{ID: buildResp.Build.ID}, nil
		}

		jobRead := make(chan struct{}, 2)
		wrappedDB := &jobInterceptStore{Store: db, jobRead: jobRead}

		tool := chattool.StartWorkspace(chattool.StartWorkspaceOptions{
			DB:      wrappedDB,
			OwnerID: user.ID,
			ChatID:  chat.ID,
			StartFn: startFn,
			AgentConnFn: func(_ context.Context, _ uuid.UUID) (workspacesdk.AgentConn, func(), error) {
				return nil, func() {}, nil
			},
			WorkspaceMu: &sync.Mutex{},
			Logger:      slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		})

		type toolResult struct {
			resp fantasy.ToolResponse
			err  error
		}
		done := make(chan toolResult, 1)
		go func() {
			resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "call-1", Name: "start_workspace", Input: "{}"})
			done <- toolResult{resp, err}
		}()

		// First signal: initial GetProvisionerJobByID for the
		// old stop build. Second signal: waitForBuild's first
		// poll for the new start build.
		testutil.TryReceive(ctx, t, jobRead)
		testutil.TryReceive(ctx, t, jobRead)

		// Fail the provisioner job.
		now := time.Now().UTC()
		require.NoError(t, db.UpdateProvisionerJobWithCompleteByID(ctx, database.UpdateProvisionerJobWithCompleteByIDParams{
			ID:          startBuildJobID,
			UpdatedAt:   now,
			CompletedAt: sql.NullTime{Time: now, Valid: true},
			Error:       sql.NullString{String: "terraform apply failed", Valid: true},
		}))

		res := testutil.TryReceive(ctx, t, done)
		require.NoError(t, res.err)

		var result map[string]any
		require.NoError(t, json.Unmarshal([]byte(res.resp.Content), &result))
		require.Contains(t, result["error"], "workspace start build failed")
		require.Equal(t, startBuildID.String(), result["build_id"])
		require.False(t, res.resp.IsError,
			"buildToolResponse must not set IsError; chatprompt strips structured fields from error responses")
	})

	t.Run("DeletedWorkspace", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		db, _ := dbtestutil.NewDB(t)

		user := dbgen.User(t, db, database.User{})
		modelCfg := seedModelConfig(ctx, t, db, user.ID)
		org := dbgen.Organization(t, db, database.Organization{})
		_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
			UserID:         user.ID,
			OrganizationID: org.ID,
		})
		// Create a workspace that has been soft-deleted.
		wsResp := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OwnerID:        user.ID,
			OrganizationID: org.ID,
			Deleted:        true,
		}).Seed(database.WorkspaceBuild{
			Transition: database.WorkspaceTransitionDelete,
		}).Do()
		ws := wsResp.Workspace

		chat, err := db.InsertChat(ctx, database.InsertChatParams{
			OrganizationID:    org.ID,
			Status:            database.ChatStatusWaiting,
			OwnerID:           user.ID,
			WorkspaceID:       uuid.NullUUID{UUID: ws.ID, Valid: true},
			LastModelConfigID: modelCfg.ID,
			Title:             "test-deleted-workspace",
		})
		require.NoError(t, err)

		tool := chattool.StartWorkspace(chattool.StartWorkspaceOptions{
			DB:     db,
			ChatID: chat.ID,
			StartFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ codersdk.CreateWorkspaceBuildRequest) (codersdk.WorkspaceBuild, error) {
				t.Fatal("StartFn should not be called for deleted workspace")
				return codersdk.WorkspaceBuild{}, nil
			},
			WorkspaceMu: &sync.Mutex{},
		})

		resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "call-1", Name: "start_workspace", Input: "{}"})
		require.NoError(t, err)
		require.Contains(t, resp.Content, "workspace was deleted")
	})
}

// seedModelConfig inserts a provider and model config for testing.
func seedModelConfig(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	userID uuid.UUID,
) database.ChatModelConfig {
	t.Helper()

	_, err := db.InsertChatProvider(ctx, database.InsertChatProviderParams{
		Provider:             "openai",
		DisplayName:          "OpenAI",
		APIKey:               "test-key",
		BaseUrl:              "",
		ApiKeyKeyID:          sql.NullString{},
		CreatedBy:            uuid.NullUUID{UUID: userID, Valid: true},
		Enabled:              true,
		CentralApiKeyEnabled: true,
	})
	require.NoError(t, err)

	model, err := db.InsertChatModelConfig(ctx, database.InsertChatModelConfigParams{
		Provider:             "openai",
		Model:                "gpt-4o-mini",
		DisplayName:          "Test Model",
		CreatedBy:            uuid.NullUUID{UUID: userID, Valid: true},
		UpdatedBy:            uuid.NullUUID{UUID: userID, Valid: true},
		Enabled:              true,
		IsDefault:            true,
		ContextLimit:         128000,
		CompressionThreshold: 70,
		Options:              json.RawMessage(`{}`),
	})
	require.NoError(t, err)
	return model
}

// jobInterceptStore wraps a database.Store and signals a
// channel after the first GetProvisionerJobByID read completes.
// This lets the test synchronize: the tool observes the Running
// job status before the main goroutine completes the build.
type jobInterceptStore struct {
	database.Store
	jobRead chan struct{}
}

func (s *jobInterceptStore) GetProvisionerJobByID(ctx context.Context, id uuid.UUID) (database.ProvisionerJob, error) {
	result, err := s.Store.GetProvisionerJobByID(ctx, id)
	select {
	case s.jobRead <- struct{}{}:
	default:
	}
	return result, err
}
