package chatd_test

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	acp "github.com/coder/acp-go-sdk"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chatacp"
	"github.com/coder/coder/v2/coderd/x/chatd/chatacp/chatacptest"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/coder/v2/testutil"
)

const (
	acpTestProviderKey = "test-provider-key"
	acpTestPinnedModel = "pinned-test-model"
	// acpTestProviderBaseURL is dbgen's default provider base URL.
	acpTestProviderBaseURL = "invalid://test.invalid/"
)

// primaryCredentials are the credentials a turn resolves from the
// seeded harness provider for the given model.
func primaryCredentials(model string) chatacp.TurnCredentials {
	return chatacp.TurnCredentials{APIKey: acpTestProviderKey, BaseURL: acpTestProviderBaseURL, Model: model}
}

// forEachHarness runs fn once per external runtime so that no runtime
// behavior is verified for a single adapter by accident.
func forEachHarness(t *testing.T, fn func(t *testing.T, harness chatacp.Harness)) {
	t.Helper()
	for _, runtime := range []codersdk.ChatRuntime{codersdk.ChatRuntimeClaudeCode, codersdk.ChatRuntimeCodex} {
		harness, ok := chatacp.HarnessFor(runtime)
		require.True(t, ok)
		t.Run(string(runtime), func(t *testing.T) {
			t.Parallel()
			fn(t, harness)
		})
	}
}

type acpTestSetup struct {
	harness   chatacp.Harness
	user      database.User
	org       database.Organization
	workspace database.WorkspaceTable
	agent     database.WorkspaceAgent
	// providerID is an enabled provider of the harness type holding
	// acpTestProviderKey; otherProviderID is an enabled provider of a
	// different type, whose model configs the runtime must reject.
	providerID      uuid.UUID
	otherProviderID uuid.UUID
}

func seedACPChatDependencies(t *testing.T, db database.Store, harness chatacp.Harness, transition database.WorkspaceTransition) acpTestSetup {
	t.Helper()
	ctx := context.Background()

	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})
	provider := dbgen.ChatProvider(t, db, database.ChatProvider{
		Provider:    string(harness.ProviderType),
		DisplayName: string(harness.ProviderType),
		Enabled:     true,
	}, func(p *database.InsertChatProviderParams) {
		p.APIKey = acpTestProviderKey
	})
	otherType := string(codersdk.AIProviderTypeOpenAI)
	if harness.ProviderType == codersdk.AIProviderTypeOpenAI {
		otherType = string(codersdk.AIProviderTypeAnthropic)
	}
	otherProvider := dbgen.ChatProvider(t, db, database.ChatProvider{
		Provider:    otherType,
		DisplayName: otherType,
		Enabled:     true,
	}, func(p *database.InsertChatProviderParams) {
		p.APIKey = "other-provider-key"
	})

	tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})
	tpl := dbgen.Template(t, db, database.Template{
		CreatedBy:       user.ID,
		OrganizationID:  org.ID,
		ActiveVersionID: tv.ID,
	})
	ws := dbgen.Workspace(t, db, database.WorkspaceTable{
		TemplateID:     tpl.ID,
		OwnerID:        user.ID,
		OrganizationID: org.ID,
	})

	pj := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		InitiatorID:    user.ID,
		OrganizationID: org.ID,
		StartedAt:      sql.NullTime{Time: dbtime.Now(), Valid: true},
		CompletedAt:    sql.NullTime{Time: dbtime.Now(), Valid: true},
	})
	_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		TemplateVersionID: tv.ID,
		WorkspaceID:       ws.ID,
		JobID:             pj.ID,
		Transition:        transition,
	})
	res := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
		Transition: transition,
		JobID:      pj.ID,
	})
	agent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
		ResourceID:      res.ID,
		Directory:       "/home/coder/project",
		OperatingSystem: "linux",
	})
	require.NoError(t, db.UpdateWorkspaceAgentStartupByID(ctx, database.UpdateWorkspaceAgentStartupByIDParams{
		ID:                agent.ID,
		Version:           "v1.0.0",
		ExpandedDirectory: "/home/coder/project",
	}))
	agent, err := db.GetWorkspaceAgentByID(ctx, agent.ID)
	require.NoError(t, err)

	_, err = db.UpsertChatRuntimeConfig(ctx, database.UpsertChatRuntimeConfigParams{
		OrganizationID: org.ID,
		Runtime:        database.ChatRuntime(harness.Runtime),
		TemplateID:     tpl.ID,
		Enabled:        true,
		Model:          acpTestPinnedModel,
		PermissionMode: "acceptEdits",
	})
	require.NoError(t, err)

	return acpTestSetup{
		harness:         harness,
		user:            user,
		org:             org,
		workspace:       ws,
		agent:           agent,
		providerID:      provider.ID,
		otherProviderID: otherProvider.ID,
	}
}

func createACPChat(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	ps pubsub.Pubsub,
	setup acpTestSetup,
	prompt string,
	mutators ...func(*chatstate.CreateChatInput),
) chatstate.CreateChatResult {
	t.Helper()

	content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText(prompt),
	})
	require.NoError(t, err)
	input := chatstate.CreateChatInput{
		OrganizationID: setup.org.ID,
		OwnerID:        setup.user.ID,
		WorkspaceID:    uuid.NullUUID{UUID: setup.workspace.ID, Valid: true},
		Runtime:        database.ChatRuntime(setup.harness.Runtime),
		Title:          setup.harness.DisplayName + " chat",
		ClientType:     database.ChatClientTypeUi,
		InitialMessages: []chatstate.Message{
			{
				Role:           database.ChatMessageRoleUser,
				Content:        content,
				Visibility:     database.ChatMessageVisibilityBoth,
				ContentVersion: chatprompt.CurrentContentVersion,
				CreatedBy:      uuid.NullUUID{UUID: setup.user.ID, Valid: true},
			},
		},
	}
	for _, mutate := range mutators {
		mutate(&input)
	}
	created, err := chatstate.CreateChat(ctx, db, ps, input)
	require.NoError(t, err)
	return created
}

func withInitialModelConfig(id uuid.UUID) func(*chatstate.CreateChatInput) {
	return func(input *chatstate.CreateChatInput) {
		input.LastModelConfigID = uuid.NullUUID{UUID: id, Valid: true}
		for i := range input.InitialMessages {
			input.InitialMessages[i].ModelConfigID = uuid.NullUUID{UUID: id, Valid: true}
		}
	}
}

// acpConfigOverrides routes turns to the fake agent and asserts the
// adapter environment carries the pinned model and provider key through
// the harness env builder.
func acpConfigOverrides(t *testing.T, setup acpTestSetup, agent *chatacptest.FakeAgent) func(*chatd.Config) {
	t.Helper()
	return acpConfigOverridesWithEnv(t, setup, agent, func(env map[string]string) {
		requireTurnEnv(t, setup.harness, env, primaryCredentials(acpTestPinnedModel))
	})
}

// requireTurnEnv checks the environment chatd handed the transport
// against what the harness builds for the expected credentials.
func requireTurnEnv(t *testing.T, harness chatacp.Harness, env map[string]string, creds chatacp.TurnCredentials) {
	t.Helper()
	require.Equal(t, harness.Env(creds), env)
}

type acpEnvRecorder struct {
	mu   sync.Mutex
	envs []map[string]string
}

func (r *acpEnvRecorder) record(env map[string]string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.envs = append(r.envs, env)
}

func (r *acpEnvRecorder) last(t *testing.T) map[string]string {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	require.NotEmpty(t, r.envs)
	return r.envs[len(r.envs)-1]
}

func acpConfigOverridesCaptureEnv(t *testing.T, setup acpTestSetup, agent *chatacptest.FakeAgent, recorder *acpEnvRecorder) func(*chatd.Config) {
	t.Helper()
	return acpConfigOverridesWithEnv(t, setup, agent, recorder.record)
}

func acpConfigOverridesWithEnv(t *testing.T, setup acpTestSetup, agent *chatacptest.FakeAgent, inspectEnv func(map[string]string)) func(*chatd.Config) {
	t.Helper()
	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	mockConn.EXPECT().SetExtraHeaders(gomock.Any()).AnyTimes()

	return func(cfg *chatd.Config) {
		cfg.AgentConn = func(_ context.Context, _ uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			return mockConn, func() {}, nil
		}
		cfg.ACPTransport = func(_ context.Context, _ workspacesdk.AgentConn, _ database.WorkspaceAgent, harness chatacp.Harness, env map[string]string, _ time.Time) (chatacp.Transport, func(), error) {
			require.Equal(t, setup.harness.Runtime, harness.Runtime)
			inspectEnv(env)
			return &chatacptest.PipeTransport{Agent: agent}, func() {}, nil
		}
	}
}

func replyingFakeAgent(text string) *chatacptest.FakeAgent {
	agent := &chatacptest.FakeAgent{}
	agent.OnPrompt = func(ctx context.Context, conn *acp.AgentSideConnection, params acp.PromptRequest) (acp.PromptResponse, error) {
		err := conn.SessionUpdate(ctx, acp.SessionNotification{
			SessionId: params.SessionId,
			Update: acp.SessionUpdate{
				AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{Content: acp.TextBlock(text)},
			},
		})
		if err != nil {
			return acp.PromptResponse{}, err
		}
		return acp.PromptResponse{
			StopReason: acp.StopReasonEndTurn,
			Usage:      &acp.Usage{InputTokens: 11, OutputTokens: 7, TotalTokens: 18},
		}, nil
	}
	return agent
}

func TestACPChatTurn(t *testing.T) {
	t.Parallel()

	forEachHarness(t, func(t *testing.T, harness chatacp.Harness) {
		db, ps := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)

		setup := seedACPChatDependencies(t, db, harness, database.WorkspaceTransitionStart)
		fakeAgent := replyingFakeAgent("Hello from the agent")

		created := createACPChat(ctx, t, db, ps, setup, "hello agent")
		_ = newActiveTestServer(t, db, ps, acpConfigOverrides(t, setup, fakeAgent))

		chat := waitForTerminalChat(ctx, t, db, created.Chat.ID)
		require.Equal(t, database.ChatStatusWaiting, chat.Status)
		require.False(t, chat.LastError.Valid)

		modes := fakeAgent.Modes()
		require.Len(t, modes, 1)
		require.Equal(t, acp.SessionModeId("acceptEdits"), modes[0].ModeId)

		prompts := fakeAgent.Prompts()
		require.Len(t, prompts, 1)
		require.Equal(t, "hello agent", prompts[0].Prompt[0].Text.Text)
		sessions := fakeAgent.NewSessions()
		require.Len(t, sessions, 1)
		require.Equal(t, "/home/coder/project", sessions[0].Cwd)

		messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: created.Chat.ID})
		require.NoError(t, err)
		require.Len(t, messages, 2)
		require.Equal(t, database.ChatMessageRoleAssistant, messages[1].Role)
		parts, err := chatprompt.ParseContent(messages[1])
		require.NoError(t, err)
		require.Len(t, parts, 1)
		require.Equal(t, codersdk.ChatMessagePartTypeText, parts[0].Type)
		require.Equal(t, "Hello from the agent", parts[0].Text)
		require.Equal(t, int64(11), messages[1].InputTokens.Int64)
		require.Equal(t, int64(7), messages[1].OutputTokens.Int64)

		state := chatacp.ParseRuntimeState(chat.RuntimeState.RawMessage)
		require.Equal(t, "session-new", state.SessionID)
		require.Equal(t, "/home/coder/project", state.Cwd)
	})
}

func TestACPChatResumesSession(t *testing.T) {
	t.Parallel()

	forEachHarness(t, func(t *testing.T, harness chatacp.Harness) {
		db, ps := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)

		setup := seedACPChatDependencies(t, db, harness, database.WorkspaceTransitionStart)

		fakeAgent := &chatacptest.FakeAgent{
			Capabilities: acp.AgentCapabilities{
				SessionCapabilities: acp.SessionCapabilities{Resume: &acp.SessionResumeCapabilities{}},
			},
		}

		created := createACPChat(ctx, t, db, ps, setup, "first message")
		server := newActiveTestServer(t, db, ps, acpConfigOverrides(t, setup, fakeAgent))

		chat := waitForTerminalChat(ctx, t, db, created.Chat.ID)
		require.Equal(t, database.ChatStatusWaiting, chat.Status)
		require.Len(t, fakeAgent.NewSessions(), 1)

		_, err := server.SendMessage(ctx, chatd.SendMessageOptions{
			ChatID:    chat.ID,
			CreatedBy: setup.user.ID,
			Content: []codersdk.ChatMessagePart{
				codersdk.ChatMessageText("second message"),
			},
		})
		require.NoError(t, err)

		testutil.Eventually(ctx, t, func(ctx context.Context) bool {
			got, err := db.GetChatByID(ctx, chat.ID)
			if err != nil {
				return false
			}
			return got.Status == database.ChatStatusWaiting && got.HistoryVersion > chat.HistoryVersion
		}, testutil.IntervalFast)

		resumes := fakeAgent.ResumeSessions()
		require.Len(t, resumes, 1)
		require.Equal(t, acp.SessionId("session-new"), resumes[0].SessionId)
		require.Equal(t, "/home/coder/project", resumes[0].Cwd)
		require.Len(t, fakeAgent.NewSessions(), 1)
	})
}

func TestACPChatRestartsStoppedWorkspace(t *testing.T) {
	t.Parallel()

	forEachHarness(t, func(t *testing.T, harness chatacp.Harness) {
		db, ps := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)

		setup := seedACPChatDependencies(t, db, harness, database.WorkspaceTransitionStop)

		fakeAgent := &chatacptest.FakeAgent{}
		created := createACPChat(ctx, t, db, ps, setup, "wake up")

		overrides := acpConfigOverrides(t, setup, fakeAgent)
		_ = newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
			overrides(cfg)
			cfg.StartWorkspace = func(ctx context.Context, ownerID uuid.UUID, workspaceID uuid.UUID, req codersdk.CreateWorkspaceBuildRequest) (codersdk.WorkspaceBuild, error) {
				require.Equal(t, setup.user.ID, ownerID)
				require.Equal(t, setup.workspace.ID, workspaceID)
				require.Equal(t, codersdk.WorkspaceTransitionStart, req.Transition)
				build, err := db.GetLatestWorkspaceBuildByWorkspaceID(ctx, workspaceID)
				if err != nil {
					return codersdk.WorkspaceBuild{}, err
				}
				pj := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
					InitiatorID:    ownerID,
					OrganizationID: setup.org.ID,
					StartedAt:      sql.NullTime{Time: dbtime.Now(), Valid: true},
					CompletedAt:    sql.NullTime{Time: dbtime.Now(), Valid: true},
				})
				newBuild := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
					TemplateVersionID: build.TemplateVersionID,
					WorkspaceID:       workspaceID,
					JobID:             pj.ID,
					Transition:        database.WorkspaceTransitionStart,
					BuildNumber:       build.BuildNumber + 1,
				})
				res := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
					Transition: database.WorkspaceTransitionStart,
					JobID:      pj.ID,
				})
				agent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
					ResourceID:      res.ID,
					Directory:       "/home/coder/project",
					OperatingSystem: "linux",
				})
				if err := db.UpdateWorkspaceAgentStartupByID(ctx, database.UpdateWorkspaceAgentStartupByIDParams{
					ID:                agent.ID,
					Version:           "v1.0.0",
					ExpandedDirectory: "/home/coder/project",
				}); err != nil {
					return codersdk.WorkspaceBuild{}, err
				}
				return codersdk.WorkspaceBuild{ID: newBuild.ID}, nil
			}
		})

		chat := waitForTerminalChat(ctx, t, db, created.Chat.ID)
		require.Equal(t, database.ChatStatusWaiting, chat.Status)
		require.False(t, chat.LastError.Valid)
		require.Len(t, fakeAgent.Prompts(), 1)
	})
}

func TestACPChatMissingRuntimeConfigFails(t *testing.T) {
	t.Parallel()

	forEachHarness(t, func(t *testing.T, harness chatacp.Harness) {
		db, ps := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)

		setup := seedACPChatDependencies(t, db, harness, database.WorkspaceTransitionStart)
		require.NoError(t, db.DeleteChatRuntimeConfig(ctx, database.DeleteChatRuntimeConfigParams{
			OrganizationID: setup.org.ID,
			Runtime:        database.ChatRuntime(harness.Runtime),
		}))

		fakeAgent := &chatacptest.FakeAgent{}
		created := createACPChat(ctx, t, db, ps, setup, "hello")
		_ = newActiveTestServer(t, db, ps, acpConfigOverrides(t, setup, fakeAgent))

		chat := waitForTerminalChat(ctx, t, db, created.Chat.ID)
		require.Equal(t, database.ChatStatusError, chat.Status)
		require.True(t, chat.LastError.Valid)
		require.Contains(t, string(chat.LastError.RawMessage), "The "+harness.DisplayName+" runtime is not configured")
		require.Empty(t, fakeAgent.Prompts())
	})
}

func TestACPChatModelSelection(t *testing.T) {
	t.Parallel()

	forEachHarness(t, func(t *testing.T, harness chatacp.Harness) {
		db, ps := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)

		setup := seedACPChatDependencies(t, db, harness, database.WorkspaceTransitionStart)
		selected := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
			Model:        "selected-model",
			AIProviderID: uuid.NullUUID{UUID: setup.providerID, Valid: true},
		})

		recorder := &acpEnvRecorder{}
		created := createACPChat(ctx, t, db, ps, setup, "hello", withInitialModelConfig(selected.ID))
		_ = newActiveTestServer(t, db, ps, acpConfigOverridesCaptureEnv(t, setup, replyingFakeAgent("selected reply"), recorder))

		chat := waitForTerminalChat(ctx, t, db, created.Chat.ID)
		require.Equal(t, database.ChatStatusWaiting, chat.Status)
		require.False(t, chat.LastError.Valid)

		requireTurnEnv(t, harness, recorder.last(t), primaryCredentials("selected-model"))

		messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: created.Chat.ID})
		require.NoError(t, err)
		require.Len(t, messages, 2)
		require.Equal(t, database.ChatMessageRoleAssistant, messages[1].Role)
		require.Equal(t, uuid.NullUUID{UUID: selected.ID, Valid: true}, messages[1].ModelConfigID)
		require.False(t, messages[1].TotalCostMicros.Valid)
		require.Equal(t, int64(11), messages[1].InputTokens.Int64)
		require.Equal(t, uuid.NullUUID{UUID: selected.ID, Valid: true}, chat.LastModelConfigID)
	})
}

func TestACPChatModelSelectionUnavailableFallsBack(t *testing.T) {
	t.Parallel()

	forEachHarness(t, func(t *testing.T, harness chatacp.Harness) {
		db, ps := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)

		setup := seedACPChatDependencies(t, db, harness, database.WorkspaceTransitionStart)
		disabled := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
			Model:        "disabled-model",
			AIProviderID: uuid.NullUUID{UUID: setup.providerID, Valid: true},
		}, func(p *database.InsertChatModelConfigParams) {
			p.Enabled = false
		})

		recorder := &acpEnvRecorder{}
		created := createACPChat(ctx, t, db, ps, setup, "hello", withInitialModelConfig(disabled.ID))
		_ = newActiveTestServer(t, db, ps, acpConfigOverridesCaptureEnv(t, setup, replyingFakeAgent("fallback reply"), recorder))

		chat := waitForTerminalChat(ctx, t, db, created.Chat.ID)
		require.Equal(t, database.ChatStatusWaiting, chat.Status)
		require.False(t, chat.LastError.Valid)

		requireTurnEnv(t, harness, recorder.last(t), primaryCredentials(acpTestPinnedModel))
		messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: created.Chat.ID})
		require.NoError(t, err)
		require.Len(t, messages, 2)
		require.Equal(t, database.ChatMessageRoleAssistant, messages[1].Role)
		require.False(t, messages[1].ModelConfigID.Valid)
	})
}

// TestACPChatRuntimeDefaults covers an organization config that pins
// neither a model nor a permission mode: the adapter keeps its own
// model and the harness default session mode applies.
func TestACPChatRuntimeDefaults(t *testing.T) {
	t.Parallel()

	forEachHarness(t, func(t *testing.T, harness chatacp.Harness) {
		db, ps := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)

		setup := seedACPChatDependencies(t, db, harness, database.WorkspaceTransitionStart)
		_, err := db.UpsertChatRuntimeConfig(ctx, database.UpsertChatRuntimeConfigParams{
			OrganizationID: setup.org.ID,
			Runtime:        database.ChatRuntime(harness.Runtime),
			TemplateID:     setup.workspace.TemplateID,
			Enabled:        true,
			Model:          "",
			PermissionMode: "",
		})
		require.NoError(t, err)

		recorder := &acpEnvRecorder{}
		fakeAgent := replyingFakeAgent("default reply")
		created := createACPChat(ctx, t, db, ps, setup, "hello")
		_ = newActiveTestServer(t, db, ps, acpConfigOverridesCaptureEnv(t, setup, fakeAgent, recorder))

		chat := waitForTerminalChat(ctx, t, db, created.Chat.ID)
		require.Equal(t, database.ChatStatusWaiting, chat.Status)

		requireTurnEnv(t, harness, recorder.last(t), primaryCredentials(""))
		modes := fakeAgent.Modes()
		if harness.DefaultSessionMode == "" {
			require.Empty(t, modes)
		} else {
			require.Len(t, modes, 1)
			require.Equal(t, acp.SessionModeId(harness.DefaultSessionMode), modes[0].ModeId)
		}
	})
}

func TestACPChatSelectionCredentials(t *testing.T) {
	t.Parallel()

	forEachHarness(t, func(t *testing.T, harness chatacp.Harness) {
		t.Run("SelectedProviderKey", func(t *testing.T) {
			t.Parallel()

			db, ps := dbtestutil.NewDB(t)
			ctx := testutil.Context(t, testutil.WaitLong)

			setup := seedACPChatDependencies(t, db, harness, database.WorkspaceTransitionStart)
			second := dbgen.ChatProvider(t, db, database.ChatProvider{
				Provider:    string(harness.ProviderType),
				DisplayName: "second",
				Enabled:     true,
				BaseUrl:     "https://second.example.com",
			}, func(p *database.InsertChatProviderParams) {
				p.APIKey = "second-provider-key"
			})
			selected := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
				Model:        "second-model",
				AIProviderID: uuid.NullUUID{UUID: second.ID, Valid: true},
			})

			recorder := &acpEnvRecorder{}
			created := createACPChat(ctx, t, db, ps, setup, "hello", withInitialModelConfig(selected.ID))
			_ = newActiveTestServer(t, db, ps, acpConfigOverridesCaptureEnv(t, setup, replyingFakeAgent("second reply"), recorder))

			chat := waitForTerminalChat(ctx, t, db, created.Chat.ID)
			require.Equal(t, database.ChatStatusWaiting, chat.Status)

			requireTurnEnv(t, harness, recorder.last(t), chatacp.TurnCredentials{
				APIKey:  "second-provider-key",
				BaseURL: "https://second.example.com",
				Model:   "second-model",
			})
		})

		t.Run("KeylessProviderFallsBack", func(t *testing.T) {
			t.Parallel()

			db, ps := dbtestutil.NewDB(t)
			ctx := testutil.Context(t, testutil.WaitLong)

			setup := seedACPChatDependencies(t, db, harness, database.WorkspaceTransitionStart)
			keyless := dbgen.ChatProvider(t, db, database.ChatProvider{
				Provider:    string(harness.ProviderType),
				DisplayName: "keyless",
				Enabled:     true,
			}, func(p *database.InsertChatProviderParams) {
				p.APIKey = ""
			})
			selected := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
				Model:        "keyless-model",
				AIProviderID: uuid.NullUUID{UUID: keyless.ID, Valid: true},
			})

			recorder := &acpEnvRecorder{}
			created := createACPChat(ctx, t, db, ps, setup, "hello", withInitialModelConfig(selected.ID))
			_ = newActiveTestServer(t, db, ps, acpConfigOverridesCaptureEnv(t, setup, replyingFakeAgent("keyless reply"), recorder))

			chat := waitForTerminalChat(ctx, t, db, created.Chat.ID)
			require.Equal(t, database.ChatStatusWaiting, chat.Status)

			requireTurnEnv(t, harness, recorder.last(t), primaryCredentials("keyless-model"))
			messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: created.Chat.ID})
			require.NoError(t, err)
			require.Len(t, messages, 2)
			require.Equal(t, uuid.NullUUID{UUID: selected.ID, Valid: true}, messages[1].ModelConfigID)
		})
	})
}

func TestACPModelSelectionValidation(t *testing.T) {
	t.Parallel()

	forEachHarness(t, func(t *testing.T, harness chatacp.Harness) {
		db, ps := dbtestutil.NewDB(t)
		replica := newTestServer(t, db, ps, uuid.New())
		setupCtx := testutil.Context(t, testutil.WaitLong)

		setup := seedACPChatDependencies(t, db, harness, database.WorkspaceTransitionStart)
		validCfg := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
			Model:        "valid-model",
			AIProviderID: uuid.NullUUID{UUID: setup.providerID, Valid: true},
		})
		otherCfg := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
			Model:        "other-provider-model",
			AIProviderID: uuid.NullUUID{UUID: setup.otherProviderID, Valid: true},
			IsDefault:    true,
		})
		disabledCfg := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
			Model:        "disabled-model",
			AIProviderID: uuid.NullUUID{UUID: setup.providerID, Valid: true},
		}, func(p *database.InsertChatModelConfigParams) {
			p.Enabled = false
		})

		created := createACPChat(setupCtx, t, db, ps, setup, "hello")
		send := func(ctx context.Context, modelConfigID uuid.UUID) (chatd.SendMessageResult, error) {
			return replica.SendMessage(ctx, chatd.SendMessageOptions{
				ChatID:        created.Chat.ID,
				CreatedBy:     setup.user.ID,
				ModelConfigID: modelConfigID,
				Content:       []codersdk.ChatMessagePart{codersdk.ChatMessageText("next")},
			})
		}

		t.Run("HarnessProviderAccepted", func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)

			result, err := send(ctx, validCfg.ID)
			require.NoError(t, err)
			require.True(t, result.Queued)
			require.Equal(t, uuid.NullUUID{UUID: validCfg.ID, Valid: true}, result.QueuedMessage.ModelConfigID)
		})

		t.Run("AbsentStaysNullDespiteDefault", func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)

			result, err := send(ctx, uuid.Nil)
			require.NoError(t, err)
			require.True(t, result.Queued)
			require.False(t, result.QueuedMessage.ModelConfigID.Valid)
		})

		for name, id := range map[string]uuid.UUID{
			"OtherProviderRejected": otherCfg.ID,
			"DisabledRejected":      disabledCfg.ID,
			"UnknownRejected":       uuid.New(),
		} {
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				ctx := testutil.Context(t, testutil.WaitLong)

				_, err := send(ctx, id)
				require.ErrorIs(t, err, chatd.ErrInvalidModelConfigID)
			})
		}

		t.Run("EditRejectsOtherProvider", func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)

			messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: created.Chat.ID})
			require.NoError(t, err)
			require.NotEmpty(t, messages)
			_, err = replica.EditMessage(ctx, chatd.EditMessageOptions{
				ChatID:          created.Chat.ID,
				CreatedBy:       setup.user.ID,
				EditedMessageID: messages[0].ID,
				Content:         []codersdk.ChatMessagePart{codersdk.ChatMessageText("edited")},
				ModelConfigID:   otherCfg.ID,
			})
			require.ErrorIs(t, err, chatd.ErrInvalidModelConfigID)
		})

		t.Run("ValidateForCreatePath", func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)

			require.NoError(t, replica.ValidateACPModelConfigID(ctx, harness, validCfg.ID))
			require.ErrorIs(t, replica.ValidateACPModelConfigID(ctx, harness, otherCfg.ID), chatd.ErrInvalidModelConfigID)
			require.ErrorIs(t, replica.ValidateACPModelConfigID(ctx, harness, disabledCfg.ID), chatd.ErrInvalidModelConfigID)
			require.ErrorIs(t, replica.ValidateACPModelConfigID(ctx, harness, uuid.New()), chatd.ErrInvalidModelConfigID)
		})
	})
}
