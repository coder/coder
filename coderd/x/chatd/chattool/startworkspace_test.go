package chattool_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

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

		chat, err := db.InsertChat(ctx, database.InsertChatParams{
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
			OwnerID:           user.ID,
			WorkspaceID:       uuid.NullUUID{UUID: ws.ID, Valid: true},
			LastModelConfigID: modelCfg.ID,
			Title:             "test-stopped-workspace",
		})
		require.NoError(t, err)

		var startCalled bool
		startFn := func(_ context.Context, _ uuid.UUID, wsID uuid.UUID, req codersdk.CreateWorkspaceBuildRequest) (codersdk.WorkspaceBuild, error) {
			startCalled = true
			require.Equal(t, codersdk.WorkspaceTransitionStart, req.Transition)
			require.Equal(t, ws.ID, wsID)

			// Simulate start by inserting a new completed "start" build.
			dbfake.WorkspaceBuild(t, db, ws).Seed(database.WorkspaceBuild{
				Transition:  database.WorkspaceTransitionStart,
				BuildNumber: 2,
			}).Do()
			return codersdk.WorkspaceBuild{}, nil
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
		Provider:    "openai",
		DisplayName: "OpenAI",
		APIKey:      "test-key",
		BaseUrl:     "",
		ApiKeyKeyID: sql.NullString{},
		CreatedBy:   uuid.NullUUID{UUID: userID, Valid: true},
		Enabled:     true,
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
