package chattool_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/hashicorp/hcl/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/dynamicparameters"
	"github.com/coder/coder/v2/coderd/httpapi/httperror"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/wsbuilder"
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
			Title:             "test-no-workspace",
		})

		tool := chattool.StartWorkspace(db, chat.ID, chattool.StartWorkspaceOptions{
			BuildFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ codersdk.CreateWorkspaceBuildRequest) (codersdk.WorkspaceBuild, error) {
				t.Fatal("BuildFn should not be called")
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
			Title:             "test-already-running",
		})

		agentConnFn := func(_ context.Context, _ uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			return nil, func() {}, nil
		}

		tool := chattool.StartWorkspace(db, chat.ID, chattool.StartWorkspaceOptions{
			OwnerID:     user.ID,
			AgentConnFn: agentConnFn,
			BuildFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ codersdk.CreateWorkspaceBuildRequest) (codersdk.WorkspaceBuild, error) {
				t.Fatal("BuildFn should not be called for already-running workspace")
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

		bound, err := db.GetChatByID(ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, uuid.NullUUID{UUID: wsResp.Build.ID, Valid: true}, bound.BuildID, "a stale binding must be repaired to the latest build")
	})

	t.Run("AlreadyRunningPrefersChatSuffixAgent", func(t *testing.T) {
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

		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    org.ID,
			OwnerID:           user.ID,
			WorkspaceID:       uuid.NullUUID{UUID: ws.ID, Valid: true},
			LastModelConfigID: modelCfg.ID,
			Title:             "test-running-preferred-agent",
		})

		var connectedAgentID uuid.UUID
		agentConnFn := func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			connectedAgentID = agentID
			return nil, func() {}, nil
		}

		tool := chattool.StartWorkspace(db, chat.ID, chattool.StartWorkspaceOptions{
			OwnerID:     user.ID,
			AgentConnFn: agentConnFn,
			BuildFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ codersdk.CreateWorkspaceBuildRequest) (codersdk.WorkspaceBuild, error) {
				t.Fatal("BuildFn should not be called for already-running workspace")
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
		modelCfg := seedModelConfig(t, db)
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

		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    org.ID,
			OwnerID:           user.ID,
			WorkspaceID:       uuid.NullUUID{UUID: ws.ID, Valid: true},
			LastModelConfigID: modelCfg.ID,
			Title:             "test-running-no-agent",
		})

		tool := chattool.StartWorkspace(db, chat.ID, chattool.StartWorkspaceOptions{
			OwnerID: user.ID,
			AgentConnFn: func(_ context.Context, _ uuid.UUID) (workspacesdk.AgentConn, func(), error) {
				t.Fatal("AgentConnFn should not be called when no agents exist")
				return nil, func() {}, nil
			},
			BuildFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ codersdk.CreateWorkspaceBuildRequest) (codersdk.WorkspaceBuild, error) {
				t.Fatal("BuildFn should not be called for already-running workspace")
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
		modelCfg := seedModelConfig(t, db)
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

		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    org.ID,
			OwnerID:           user.ID,
			WorkspaceID:       uuid.NullUUID{UUID: ws.ID, Valid: true},
			LastModelConfigID: modelCfg.ID,
			Title:             "test-running-selection-error",
		})

		tool := chattool.StartWorkspace(db, chat.ID, chattool.StartWorkspaceOptions{
			OwnerID: user.ID,
			AgentConnFn: func(_ context.Context, _ uuid.UUID) (workspacesdk.AgentConn, func(), error) {
				t.Fatal("AgentConnFn should not be called when agent selection fails")
				return nil, func() {}, nil
			},
			BuildFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ codersdk.CreateWorkspaceBuildRequest) (codersdk.WorkspaceBuild, error) {
				t.Fatal("BuildFn should not be called for already-running workspace")
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
		modelCfg := seedModelConfig(t, db)
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

		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    org.ID,
			OwnerID:           user.ID,
			WorkspaceID:       uuid.NullUUID{UUID: ws.ID, Valid: true},
			LastModelConfigID: modelCfg.ID,
			Title:             "test-stopped-workspace",
		})

		var startCalled bool
		var startBuildID uuid.UUID
		startFn := func(_ context.Context, _ uuid.UUID, wsID uuid.UUID, req codersdk.CreateWorkspaceBuildRequest) (codersdk.WorkspaceBuild, error) {
			startCalled = true
			require.Equal(t, codersdk.WorkspaceTransitionStart, req.Transition)
			require.Equal(t, ws.ID, wsID)
			require.Empty(t, req.RichParameterValues, "no parameters should be forwarded for bare start")
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

		tool := chattool.StartWorkspace(db, chat.ID, chattool.StartWorkspaceOptions{
			OwnerID:     user.ID,
			BuildFn:     startFn,
			AgentConnFn: agentConnFn,
			WorkspaceMu: &sync.Mutex{},
		})

		resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "call-1", Name: "start_workspace", Input: "{}"})
		require.NoError(t, err)
		require.True(t, startCalled, "expected BuildFn to be called")

		var result map[string]any
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		started, ok := result["started"].(bool)
		require.True(t, ok)
		require.True(t, started)
		require.Equal(t, startBuildID.String(), result["build_id"])
		require.Nil(t, result["no_build"], "no_build should not be set when a build was triggered")
	})

	t.Run("ActiveVersionPolicy", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)

		user := dbgen.User(t, db, database.User{})
		modelCfg := seedModelConfig(t, db)
		org := dbgen.Organization(t, db, database.Organization{})
		_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
			UserID:         user.ID,
			OrganizationID: org.ID,
		})

		tests := []struct {
			name                 string
			requireActiveVersion bool
			outdated             bool
			wantActiveVersion    bool
			wantUpdated          bool
		}{
			{
				name:     "NotRequired",
				outdated: true,
			},
			{
				name:                 "RequiredAndCurrent",
				requireActiveVersion: true,
				wantActiveVersion:    true,
			},
			{
				name:                 "RequiredAndOutdated",
				requireActiveVersion: true,
				outdated:             true,
				wantActiveVersion:    true,
				wantUpdated:          true,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				ctx := testutil.Context(t, testutil.WaitLong)

				wsResp := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
					OwnerID:        user.ID,
					OrganizationID: org.ID,
				}).Seed(database.WorkspaceBuild{
					Transition: database.WorkspaceTransitionStop,
				}).Do()
				ws := wsResp.Workspace
				if tt.requireActiveVersion {
					setRequireActiveVersion(ctx, t, db, ws.TemplateID)
				}
				activeVersionID := wsResp.Build.TemplateVersionID
				if tt.outdated {
					activeVersionID = promoteNewTemplateVersion(t, db, ws)
				}

				chat := dbgen.Chat(t, db, database.Chat{
					OrganizationID:    org.ID,
					OwnerID:           user.ID,
					WorkspaceID:       uuid.NullUUID{UUID: ws.ID, Valid: true},
					LastModelConfigID: modelCfg.ID,
					Title:             "test-start-workspace-active-version-policy",
				})

				var gotVersionID uuid.UUID
				startFn := func(_ context.Context, _ uuid.UUID, _ uuid.UUID, req codersdk.CreateWorkspaceBuildRequest) (codersdk.WorkspaceBuild, error) {
					gotVersionID = req.TemplateVersionID
					buildResp := dbfake.WorkspaceBuild(t, db, ws).Seed(database.WorkspaceBuild{
						Transition:  database.WorkspaceTransitionStart,
						BuildNumber: 2,
					}).Do()
					return codersdk.WorkspaceBuild{ID: buildResp.Build.ID}, nil
				}

				tool := chattool.StartWorkspace(db, chat.ID, chattool.StartWorkspaceOptions{
					OwnerID: user.ID,
					BuildFn: startFn,
					AgentConnFn: func(_ context.Context, _ uuid.UUID) (workspacesdk.AgentConn, func(), error) {
						return nil, func() {}, nil
					},
					WorkspaceMu: &sync.Mutex{},
				})

				resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "call-1", Name: "start_workspace", Input: "{}"})
				require.NoError(t, err)

				wantVersionID := uuid.Nil
				if tt.wantActiveVersion {
					wantVersionID = activeVersionID
				}
				require.Equal(t, wantVersionID, gotVersionID)

				var result map[string]any
				require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
				require.Equal(t, true, result["started"])
				if !tt.wantUpdated {
					require.NotContains(t, result, "updated_to_active_version")
					return
				}
				require.Equal(t, true, result["updated_to_active_version"])
				require.Equal(t, "template requires active versions", result["update_reason"])
				require.Contains(t, result["message"], "updated to the active template version")
			})
		}
	})

	t.Run("PassesParameters", func(t *testing.T) {
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
			Title:             "test-start-workspace-passes-parameters",
		})

		expectedParams := []codersdk.WorkspaceBuildParameter{
			{Name: "region", Value: "us-east-1"},
			{Name: "size", Value: "large"},
		}
		startFn := func(_ context.Context, _ uuid.UUID, wsID uuid.UUID, req codersdk.CreateWorkspaceBuildRequest) (codersdk.WorkspaceBuild, error) {
			require.Equal(t, codersdk.WorkspaceTransitionStart, req.Transition)
			require.Equal(t, ws.ID, wsID)
			require.ElementsMatch(t, expectedParams, req.RichParameterValues)
			buildResp := dbfake.WorkspaceBuild(t, db, ws).Seed(database.WorkspaceBuild{
				Transition:  database.WorkspaceTransitionStart,
				BuildNumber: 2,
			}).Do()
			return codersdk.WorkspaceBuild{ID: buildResp.Build.ID}, nil
		}

		tool := chattool.StartWorkspace(db, chat.ID, chattool.StartWorkspaceOptions{
			OwnerID: user.ID,
			BuildFn: startFn,
			AgentConnFn: func(_ context.Context, _ uuid.UUID) (workspacesdk.AgentConn, func(), error) {
				return nil, func() {}, nil
			},
			WorkspaceMu: &sync.Mutex{},
		})

		resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "call-1", Name: "start_workspace", Input: `{"parameters":{"region":"us-east-1","size":"large"}}`})
		require.NoError(t, err)

		var result map[string]any
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		require.Equal(t, true, result["started"])
	})

	t.Run("RejectedStart", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)

		user := dbgen.User(t, db, database.User{})
		modelCfg := seedModelConfig(t, db)
		org := dbgen.Organization(t, db, database.Organization{})
		_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
			UserID:         user.ID,
			OrganizationID: org.ID,
		})
		regionRequired := hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  "Required parameter not provided",
			Detail:   "region must be set",
		}}
		regionValidations := []codersdk.ValidationError{{
			Field:  "region",
			Detail: "Required parameter not provided; region must be set",
		}}
		keyedDiagnostics := wsbuilder.BuildError{
			Status:  http.StatusBadRequest,
			Message: "resolve parameters",
			Wrapped: &dynamicparameters.DiagnosticError{
				Message:          "Unable to validate parameters",
				KeyedDiagnostics: map[string]hcl.Diagnostics{"region": regionRequired},
			},
		}

		tests := []struct {
			name                 string
			requireActiveVersion bool
			err                  error
			// wantError is the exact error. Empty means the retry
			// instructions.
			wantError       string
			wantDetail      string
			wantValidations []codersdk.ValidationError
			wantTemplateID  bool
		}{
			{
				name:                 "KeyedDiagnostics",
				requireActiveVersion: true,
				err:                  keyedDiagnostics,
				wantDetail:           keyedDiagnostics.Error(),
				wantValidations:      regionValidations,
				wantTemplateID:       true,
			},
			{
				name:                 "KeyedAndTopLevelDiagnostics",
				requireActiveVersion: true,
				err: wsbuilder.BuildError{
					Status:  http.StatusBadRequest,
					Message: "resolve parameters",
					Wrapped: &dynamicparameters.DiagnosticError{
						Message: "Unable to validate parameters",
						Diagnostics: hcl.Diagnostics{{
							Severity: hcl.DiagError,
							Summary:  "Invalid template",
							Detail:   "owner data is unavailable",
						}},
						KeyedDiagnostics: map[string]hcl.Diagnostics{"region": regionRequired},
					},
				},
				wantDetail:      "Invalid template; owner data is unavailable",
				wantValidations: regionValidations,
				wantTemplateID:  true,
			},
			{
				name:                 "ClassicParameter",
				requireActiveVersion: true,
				err: wsbuilder.BuildError{
					Status:  http.StatusBadRequest,
					Message: `Unable to validate parameter "region"`,
					Wrapped: errors.Join(dynamicparameters.ErrParameterValidation, xerrors.New("region is required")),
				},
				wantDetail:     `Unable to validate parameter "region": ` + "parameter validation failed\nregion is required",
				wantTemplateID: true,
			},
			{
				name:                 "MessageOnly",
				requireActiveVersion: true,
				err:                  &dynamicparameters.DiagnosticError{Message: "Unable to validate parameters"},
				wantDetail:           "Unable to validate parameters",
				wantTemplateID:       true,
			},
			{
				name:                 "NotAParameterError",
				requireActiveVersion: true,
				err: httperror.NewResponseError(http.StatusBadGateway, codersdk.Response{
					Message: "workspace start failed",
					Detail:  "temporary provisioner outage",
				}),
				wantError:  "workspace start failed",
				wantDetail: "temporary provisioner outage",
			},
			{
				name:            "ParameterErrorWithoutUpdate",
				err:             keyedDiagnostics,
				wantError:       "Unable to validate parameters",
				wantValidations: regionValidations,
				wantTemplateID:  true,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				ctx := testutil.Context(t, testutil.WaitLong)

				ws := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
					OwnerID:        user.ID,
					OrganizationID: org.ID,
				}).Seed(database.WorkspaceBuild{
					Transition: database.WorkspaceTransitionStop,
				}).Do().Workspace
				if tt.requireActiveVersion {
					setRequireActiveVersion(ctx, t, db, ws.TemplateID)
				}
				promoteNewTemplateVersion(t, db, ws)

				chat := dbgen.Chat(t, db, database.Chat{
					OrganizationID:    org.ID,
					OwnerID:           user.ID,
					WorkspaceID:       uuid.NullUUID{UUID: ws.ID, Valid: true},
					LastModelConfigID: modelCfg.ID,
					Title:             "test-start-workspace-rejected-start",
				})

				tool := chattool.StartWorkspace(db, chat.ID, chattool.StartWorkspaceOptions{
					OwnerID: user.ID,
					BuildFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ codersdk.CreateWorkspaceBuildRequest) (codersdk.WorkspaceBuild, error) {
						return codersdk.WorkspaceBuild{}, tt.err
					},
					WorkspaceMu: &sync.Mutex{},
				})

				resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "call-1", Name: "start_workspace", Input: "{}"})
				require.NoError(t, err)
				require.False(t, resp.IsError)

				var result struct {
					Error       string                     `json:"error"`
					Detail      string                     `json:"detail"`
					TemplateID  string                     `json:"template_id"`
					Validations []codersdk.ValidationError `json:"validations"`
				}
				require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
				if tt.wantError == "" {
					require.Contains(t, result.Error, "read_template")
					require.Contains(t, result.Error, "retry start_workspace")
				} else {
					require.Equal(t, tt.wantError, result.Error)
				}
				require.Equal(t, tt.wantDetail, result.Detail)
				require.Equal(t, tt.wantValidations, result.Validations)
				wantTemplateID := ""
				if tt.wantTemplateID {
					wantTemplateID = ws.TemplateID.String()
				}
				require.Equal(t, wantTemplateID, result.TemplateID)
			})
		}
	})

	t.Run("InProgressBuild", func(t *testing.T) {
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
		// Create a workspace with a build that is still running.
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
			Title:             "test-in-progress-build",
		})

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
		tool := chattool.StartWorkspace(wrappedDB, chat.ID, chattool.StartWorkspaceOptions{
			OwnerID:     user.ID,
			AgentConnFn: agentConnFn,
			BuildFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ codersdk.CreateWorkspaceBuildRequest) (codersdk.WorkspaceBuild, error) {
				t.Fatal("BuildFn should not be called for an in-progress build")
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

	t.Run("FailedBuildQuota", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, _ := dbtestutil.NewDB(t)

		user := dbgen.User(t, db, database.User{})
		modelCfg := seedModelConfig(t, db)
		orgResp := dbfake.Organization(t, db).
			EveryoneAllowance(40).
			Members(user).
			Do()
		org := orgResp.Org
		// Create a workspace with a build that is still running.
		wsResp := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OwnerID:        user.ID,
			OrganizationID: org.ID,
		}).Seed(database.WorkspaceBuild{
			Transition: database.WorkspaceTransitionStart,
			DailyCost:  40,
		}).Starting().Do()
		ws := wsResp.Workspace

		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    org.ID,
			OwnerID:           user.ID,
			WorkspaceID:       uuid.NullUUID{UUID: ws.ID, Valid: true},
			LastModelConfigID: modelCfg.ID,
			Title:             "test-failed-build",
		})

		authzDB := dbauthz.New(
			db,
			rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry()),
			slogtest.Make(t, nil),
			testAccessControlStorePointer(),
		)
		jobRead := make(chan struct{}, 1)
		wrappedDB := &jobInterceptStore{Store: authzDB, jobRead: jobRead}

		tool := chattool.StartWorkspace(wrappedDB, chat.ID, chattool.StartWorkspaceOptions{
			OwnerID: user.ID,
			AgentConnFn: func(_ context.Context, _ uuid.UUID) (workspacesdk.AgentConn, func(), error) {
				return nil, func() {}, nil
			},
			BuildFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ codersdk.CreateWorkspaceBuildRequest) (codersdk.WorkspaceBuild, error) {
				t.Fatal("BuildFn should not be called for an in-progress build")
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
			resp, err := tool.Run(
				dbauthz.AsChatd(ctx),
				fantasy.ToolCall{ID: "call-1", Name: "start_workspace", Input: "{}"},
			)
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
			Error:       sql.NullString{String: "insufficient quota", Valid: true},
			ErrorCode: sql.NullString{
				String: string(codersdk.InsufficientQuota),
				Valid:  true,
			},
		}))

		res := testutil.TryReceive(ctx, t, done)
		require.NoError(t, res.err)

		var result map[string]any
		require.NoError(t, json.Unmarshal([]byte(res.resp.Content), &result))
		require.Contains(t, result["error"], "waiting for in-progress build")
		require.Equal(t, string(codersdk.InsufficientQuota), result["error_code"])
		require.Equal(t, "Workspace quota reached", result["title"])
		require.Contains(t, result["message"], "workspace quota is full")
		require.Equal(t, wsResp.Build.ID.String(), result["build_id"])
		quota, ok := result["quota"].(map[string]any)
		require.True(t, ok)
		require.Equal(t, float64(40), quota["credits_consumed"])
		require.Equal(t, float64(40), quota["budget"])
		require.False(t, res.resp.IsError,
			"quota responses must not set IsError; chatprompt strips structured fields from error responses")
	})

	t.Run("StartTriggeredBuildFailure", func(t *testing.T) {
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
		// Create a stopped workspace with a succeeded stop transition.
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
			Title:             "test-start-triggered-generic-build-failure",
		})

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

		tool := chattool.StartWorkspace(wrappedDB, chat.ID, chattool.StartWorkspaceOptions{
			OwnerID: user.ID,
			BuildFn: startFn,
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

		testutil.TryReceive(ctx, t, jobRead)
		testutil.TryReceive(ctx, t, jobRead)

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
		require.NotContains(t, result, "error_code")
		require.NotContains(t, result, "quota")
		require.False(t, res.resp.IsError,
			"buildToolResponse must not set IsError; chatprompt strips structured fields from error responses")
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
		// Create a workspace that has been soft-deleted.
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
			Title:             "test-deleted-workspace",
		})

		tool := chattool.StartWorkspace(db, chat.ID, chattool.StartWorkspaceOptions{
			BuildFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ codersdk.CreateWorkspaceBuildRequest) (codersdk.WorkspaceBuild, error) {
				t.Fatal("BuildFn should not be called for deleted workspace")
				return codersdk.WorkspaceBuild{}, nil
			},
			WorkspaceMu: &sync.Mutex{},
		})

		resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "call-1", Name: "start_workspace", Input: "{}"})
		require.NoError(t, err)
		require.Contains(t, resp.Content, "workspace was deleted")
	})

	// Dormancy auto-delete leaves dormant_at set, which switches the row's
	// RBAC object to workspace_dormant. The chatd actor must still read it
	// so the deleted-workspace guidance is reached instead of a permission
	// failure.
	t.Run("DormantDeletedWorkspace", func(t *testing.T) {
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
			DormantAt:      sql.NullTime{Time: time.Now(), Valid: true},
		}).Seed(database.WorkspaceBuild{
			Transition: database.WorkspaceTransitionDelete,
		}).Do()
		ws := wsResp.Workspace

		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    org.ID,
			OwnerID:           user.ID,
			WorkspaceID:       uuid.NullUUID{UUID: ws.ID, Valid: true},
			LastModelConfigID: modelCfg.ID,
			Title:             "test-dormant-deleted-workspace",
		})

		authzDB := dbauthz.New(
			db,
			rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry()),
			slogtest.Make(t, nil),
			testAccessControlStorePointer(),
		)
		tool := chattool.StartWorkspace(authzDB, chat.ID, chattool.StartWorkspaceOptions{
			OwnerID: user.ID,
			BuildFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ codersdk.CreateWorkspaceBuildRequest) (codersdk.WorkspaceBuild, error) {
				t.Fatal("BuildFn should not be called for deleted workspace")
				return codersdk.WorkspaceBuild{}, nil
			},
			WorkspaceMu: &sync.Mutex{},
		})

		resp, err := tool.Run(
			dbauthz.AsChatd(ctx),
			fantasy.ToolCall{ID: "call-1", Name: "start_workspace", Input: "{}"},
		)
		require.NoError(t, err)
		require.Contains(t, resp.Content, "workspace was deleted")
		require.Contains(t, resp.Content, "create_workspace")
		require.NotContains(t, resp.Content, "forbidden")
	})

	t.Run("LoadWorkspaceErrorGuidance", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		db, _ := dbtestutil.NewDB(t)

		user := dbgen.User(t, db, database.User{})
		modelCfg := seedModelConfig(t, db)
		org := dbgen.Organization(t, db, database.Organization{})
		ws := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OwnerID:        user.ID,
			OrganizationID: org.ID,
		}).Do().Workspace
		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    org.ID,
			OwnerID:           user.ID,
			WorkspaceID:       uuid.NullUUID{UUID: ws.ID, Valid: true},
			LastModelConfigID: modelCfg.ID,
			Title:             "test-load-workspace-error",
		})

		tool := chattool.StartWorkspace(&failingWorkspaceStore{Store: db}, chat.ID, chattool.StartWorkspaceOptions{
			BuildFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ codersdk.CreateWorkspaceBuildRequest) (codersdk.WorkspaceBuild, error) {
				t.Fatal("BuildFn should not be called when the workspace cannot be loaded")
				return codersdk.WorkspaceBuild{}, nil
			},
			WorkspaceMu: &sync.Mutex{},
		})

		resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "call-1", Name: "start_workspace", Input: "{}"})
		require.NoError(t, err)
		require.True(t, resp.IsError)
		require.Contains(t, resp.Content, "load workspace: workspace store offline")
		require.Contains(t, resp.Content, chattool.WorkspaceUnavailableHint)
	})
}

// failingWorkspaceStore fails every workspace lookup so tests can observe
// the recovery guidance attached to load errors.
type failingWorkspaceStore struct {
	database.Store
}

func (*failingWorkspaceStore) GetWorkspaceByID(context.Context, uuid.UUID) (database.Workspace, error) {
	return database.Workspace{}, xerrors.New("workspace store offline")
}

// seedModelConfig inserts a provider and model config for testing.
func seedModelConfig(
	t *testing.T,
	db database.Store,
) database.ChatModelConfig {
	t.Helper()

	dbgen.ChatProvider(t, db, database.ChatProvider{})
	return dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		IsDefault: true,
	})
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

func setRequireActiveVersion(ctx context.Context, t *testing.T, db database.Store, templateID uuid.UUID) {
	t.Helper()
	require.NoError(t, db.UpdateTemplateAccessControlByID(ctx, database.UpdateTemplateAccessControlByIDParams{
		ID:                   templateID,
		RequireActiveVersion: true,
	}))
}

// promoteNewTemplateVersion makes a new version of the workspace's
// template the active version and returns its ID.
func promoteNewTemplateVersion(t *testing.T, db database.Store, ws database.WorkspaceTable) uuid.UUID {
	t.Helper()
	return dbfake.TemplateVersion(t, db).Seed(database.TemplateVersion{
		TemplateID:     uuid.NullUUID{UUID: ws.TemplateID, Valid: true},
		OrganizationID: ws.OrganizationID,
		CreatedBy:      ws.OwnerID,
	}).Do().TemplateVersion.ID
}

func testAccessControlStorePointer() *atomic.Pointer[dbauthz.AccessControlStore] {
	acs := &atomic.Pointer[dbauthz.AccessControlStore]{}
	var store dbauthz.AccessControlStore = dbauthz.AGPLTemplateAccessControlStore{}
	acs.Store(&store)
	return acs
}
