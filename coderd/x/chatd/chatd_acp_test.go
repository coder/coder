package chatd_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

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
	for _, harness := range chatacp.Harnesses() {
		t.Run(string(harness.Runtime), func(t *testing.T) {
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
	// providerID is an enabled provider of the harness type holding
	// acpTestProviderKey; otherProviderID is an enabled provider of a
	// different type, whose model configs the runtime must reject.
	providerID      uuid.UUID
	otherProviderID uuid.UUID
	// pinnedConfig is the enabled model config on providerID that the
	// runtime config's acpTestPinnedModel pin resolves to.
	pinnedConfig database.ChatModelConfig
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
	seedWorkspaceBuild(t, db, ws, tv.ID, transition, 1)

	pinnedConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		OrganizationID: org.ID,
		Model:          acpTestPinnedModel,
		AIProviderID:   uuid.NullUUID{UUID: provider.ID, Valid: true},
	})
	_, err := db.UpsertChatRuntimeConfig(ctx, database.UpsertChatRuntimeConfigParams{
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
		providerID:      provider.ID,
		otherProviderID: otherProvider.ID,
		pinnedConfig:    pinnedConfig,
	}
}

// seedWorkspaceBuild records a completed build of the given transition
// with one ready agent whose directory the runtime uses as its cwd.
func seedWorkspaceBuild(t *testing.T, db database.Store, workspace database.WorkspaceTable, templateVersionID uuid.UUID, transition database.WorkspaceTransition, buildNumber int32) {
	t.Helper()
	job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		InitiatorID:    workspace.OwnerID,
		OrganizationID: workspace.OrganizationID,
		StartedAt:      sql.NullTime{Time: dbtime.Now(), Valid: true},
		CompletedAt:    sql.NullTime{Time: dbtime.Now(), Valid: true},
	})
	dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		TemplateVersionID: templateVersionID,
		WorkspaceID:       workspace.ID,
		JobID:             job.ID,
		Transition:        transition,
		BuildNumber:       buildNumber,
	})
	resource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
		Transition: transition,
		JobID:      job.ID,
	})
	agent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
		ResourceID:      resource.ID,
		Directory:       "/home/coder/project",
		OperatingSystem: "linux",
	})
	require.NoError(t, db.UpdateWorkspaceAgentStartupByID(context.Background(), database.UpdateWorkspaceAgentStartupByIDParams{
		ID:                agent.ID,
		Version:           "v1.0.0",
		ExpandedDirectory: "/home/coder/project",
	}))
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

// acpModelConfig seeds an enabled model config in the chat's
// organization on the given provider.
func acpModelConfig(t *testing.T, db database.Store, setup acpTestSetup, providerID uuid.UUID, model string, munge ...func(*database.InsertChatModelConfigParams)) database.ChatModelConfig {
	t.Helper()
	return dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		OrganizationID: setup.org.ID,
		Model:          model,
		AIProviderID:   uuid.NullUUID{UUID: providerID, Valid: true},
	}, munge...)
}

// seedSecondHarnessProvider adds a second enabled, keyed provider of
// the harness type so the runtime default chain has two candidates.
func seedSecondHarnessProvider(t *testing.T, db database.Store, setup acpTestSetup) database.ChatProvider {
	t.Helper()
	return dbgen.ChatProvider(t, db, database.ChatProvider{
		Provider:    string(setup.harness.ProviderType),
		DisplayName: "second",
		Enabled:     true,
		BaseUrl:     "https://second.example.com",
	}, func(p *database.InsertChatProviderParams) {
		p.APIKey = "second-provider-key"
	})
}

// acpConfigOverrides routes turns to the fake agent and checks that the
// adapter environment chatd hands the transport is what the harness
// builds for the expected credentials.
func acpConfigOverrides(t *testing.T, setup acpTestSetup, agent *chatacptest.FakeAgent, wantCreds chatacp.TurnCredentials) func(*chatd.Config) {
	t.Helper()
	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	mockConn.EXPECT().SetExtraHeaders(gomock.Any()).AnyTimes()

	return func(cfg *chatd.Config) {
		cfg.AgentConn = func(_ context.Context, _ uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			return mockConn, func() {}, nil
		}
		cfg.ACPTransport = func(_ context.Context, _ workspacesdk.AgentConn, _ database.WorkspaceAgent, harness chatacp.Harness, env map[string]string, _ time.Time) (chatacp.Transport, func(), error) {
			assert.Equal(t, setup.harness.Runtime, harness.Runtime)
			assert.Equal(t, setup.harness.Env(wantCreds), env)
			return &chatacptest.PipeTransport{Agent: agent}, func() {}, nil
		}
	}
}

// acpTestCommands is the slash-command list replyingFakeAgent advertises
// on every prompt.
var acpTestCommands = []acp.AvailableCommand{
	{
		Name:        "review",
		Description: "Review the current diff",
		Input:       &acp.AvailableCommandInput{Unstructured: &acp.UnstructuredCommandInput{Hint: "pr number"}},
	},
	{Name: "init", Description: "Create a project guide"},
}

func replyingFakeAgent(text string) *chatacptest.FakeAgent {
	agent := &chatacptest.FakeAgent{}
	agent.OnPrompt = func(ctx context.Context, conn *acp.AgentSideConnection, params acp.PromptRequest) (acp.PromptResponse, error) {
		err := conn.SessionUpdate(ctx, acp.SessionNotification{
			SessionId: params.SessionId,
			Update: acp.SessionUpdate{
				AvailableCommandsUpdate: &acp.SessionAvailableCommandsUpdate{AvailableCommands: acpTestCommands},
			},
		})
		if err != nil {
			return acp.PromptResponse{}, err
		}
		err = conn.SessionUpdate(ctx, acp.SessionNotification{
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
		server := newActiveTestServer(t, db, ps, acpConfigOverrides(t, setup, fakeAgent, primaryCredentials(acpTestPinnedModel)))

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

// TestACPChatEditStartsNewSession verifies that editing a message does
// not resume the ACP session that still holds the discarded turns: the
// next turn starts a new session and reseeds it from the transcript.
func TestACPChatEditStartsNewSession(t *testing.T) {
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
		server := newActiveTestServer(t, db, ps, acpConfigOverrides(t, setup, fakeAgent, primaryCredentials(acpTestPinnedModel)))

		chat := waitForTerminalChat(ctx, t, db, created.Chat.ID)
		require.Equal(t, database.ChatStatusWaiting, chat.Status)
		require.Len(t, fakeAgent.NewSessions(), 1)

		_, err := server.EditMessage(ctx, chatd.EditMessageOptions{
			ChatID:          chat.ID,
			CreatedBy:       setup.user.ID,
			EditedMessageID: created.InitialMessages[0].ID,
			Content:         []codersdk.ChatMessagePart{codersdk.ChatMessageText("edited message")},
		})
		require.NoError(t, err)

		testutil.Eventually(ctx, t, func(ctx context.Context) bool {
			got, err := db.GetChatByID(ctx, chat.ID)
			if err != nil {
				return false
			}
			return got.Status == database.ChatStatusWaiting && got.HistoryVersion > chat.HistoryVersion
		}, testutil.IntervalFast)

		require.Empty(t, fakeAgent.ResumeSessions())
		require.Len(t, fakeAgent.NewSessions(), 2)
		prompts := fakeAgent.Prompts()
		require.Len(t, prompts, 2)
		require.Equal(t, "edited message", prompts[1].Prompt[0].Text.Text)
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

		// The worker goroutine only records the start request; the test
		// goroutine seeds the resulting build, which the worker polls for.
		type startRequest struct {
			ownerID, workspaceID uuid.UUID
			transition           codersdk.WorkspaceTransition
		}
		started := make(chan startRequest, 1)
		overrides := acpConfigOverrides(t, setup, fakeAgent, primaryCredentials(acpTestPinnedModel))
		_ = newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
			overrides(cfg)
			cfg.StartWorkspace = func(_ context.Context, ownerID uuid.UUID, workspaceID uuid.UUID, req codersdk.CreateWorkspaceBuildRequest) (codersdk.WorkspaceBuild, error) {
				started <- startRequest{ownerID, workspaceID, req.Transition}
				return codersdk.WorkspaceBuild{}, nil
			}
		})

		request := testutil.RequireReceive(ctx, t, started)
		stopped, err := db.GetLatestWorkspaceBuildByWorkspaceID(ctx, setup.workspace.ID)
		require.NoError(t, err)
		seedWorkspaceBuild(t, db, setup.workspace, stopped.TemplateVersionID, database.WorkspaceTransitionStart, stopped.BuildNumber+1)

		chat := waitForTerminalChat(ctx, t, db, created.Chat.ID)
		require.Equal(t, database.ChatStatusWaiting, chat.Status)
		require.False(t, chat.LastError.Valid)
		require.Equal(t, startRequest{setup.user.ID, setup.workspace.ID, codersdk.WorkspaceTransitionStart}, request)
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
		_ = newActiveTestServer(t, db, ps, acpConfigOverrides(t, setup, fakeAgent, primaryCredentials(acpTestPinnedModel)))

		chat := waitForTerminalChat(ctx, t, db, created.Chat.ID)
		require.Equal(t, database.ChatStatusError, chat.Status)
		require.True(t, chat.LastError.Valid)
		require.Contains(t, string(chat.LastError.RawMessage), "The "+harness.DisplayName+" runtime is not configured")
		require.Empty(t, fakeAgent.Prompts())
	})
}

// TestACPChatRejectedPermissionModeFails verifies that an adapter
// refusing the configured permission mode surfaces as a configuration
// error naming the mode, so administrators learn the setting is wrong.
func TestACPChatRejectedPermissionModeFails(t *testing.T) {
	t.Parallel()

	forEachHarness(t, func(t *testing.T, harness chatacp.Harness) {
		db, ps := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)

		setup := seedACPChatDependencies(t, db, harness, database.WorkspaceTransitionStart)
		fakeAgent := &chatacptest.FakeAgent{}
		fakeAgent.OnSetSessionMode = func(acp.SetSessionModeRequest) error {
			return xerrors.New("unknown mode")
		}
		created := createACPChat(ctx, t, db, ps, setup, "hello")
		_ = newActiveTestServer(t, db, ps, acpConfigOverrides(t, setup, fakeAgent, primaryCredentials(acpTestPinnedModel)))

		chat := waitForTerminalChat(ctx, t, db, created.Chat.ID)
		require.Equal(t, database.ChatStatusError, chat.Status)
		require.True(t, chat.LastError.Valid)
		var lastError codersdk.ChatError
		require.NoError(t, json.Unmarshal(chat.LastError.RawMessage, &lastError))
		require.Equal(t, codersdk.ChatErrorKindConfig, lastError.Kind)
		require.Equal(t, "The "+harness.DisplayName+` runtime's permission mode "acceptEdits" is not supported by the adapter; ask an administrator to change it in the runtime config.`, lastError.Message)
		require.Empty(t, fakeAgent.Prompts())
	})
}

// TestACPChatTurn is the end-to-end turn contract and the behavior
// matrix over acpTurnConfig: which credentials, model, session mode, and
// message stamp result from the organization config and the message's
// model selection.
func TestACPChatTurn(t *testing.T) {
	t.Parallel()

	forEachHarness(t, func(t *testing.T, harness chatacp.Harness) {
		tests := []struct {
			name string
			// noPin clears the organization config's model pin and
			// permission mode so the adapter and harness defaults apply.
			noPin bool
			// seed returns the model config the chat's first message
			// selects; nil rows and uuid.Nil use the runtime default chain.
			seed      func(t *testing.T, db database.Store, setup acpTestSetup) uuid.UUID
			wantCreds chatacp.TurnCredentials
			// wantStamp expects the assistant message to carry the
			// selected model config id.
			wantStamp bool
			wantMode  string
			// wantError is the config error message that fails the turn
			// before any adapter request.
			wantError string
		}{
			{
				name:      "PinnedDefault",
				wantCreds: primaryCredentials(acpTestPinnedModel),
				wantMode:  "acceptEdits",
			},
			{
				name:      "NoPinUsesAdapterDefaults",
				noPin:     true,
				wantCreds: primaryCredentials(""),
				wantMode:  harness.DefaultSessionMode,
			},
			{
				name:  "AmbiguousDefaultProviders",
				noPin: true,
				seed: func(t *testing.T, db database.Store, setup acpTestSetup) uuid.UUID {
					seedSecondHarnessProvider(t, db, setup)
					return uuid.Nil
				},
				wantError: "Multiple " + harness.ProviderLabel + " providers are enabled, so the " + harness.DisplayName + " runtime cannot choose one. Select a model, or have an administrator keep a single " + harness.ProviderLabel + " provider enabled.",
			},
			{
				name: "PinnedWithTwoProviders",
				seed: func(t *testing.T, db database.Store, setup acpTestSetup) uuid.UUID {
					seedSecondHarnessProvider(t, db, setup)
					return uuid.Nil
				},
				wantCreds: primaryCredentials(acpTestPinnedModel),
				wantMode:  "acceptEdits",
			},
			{
				name: "PinnedDisabledFails",
				seed: func(t *testing.T, db database.Store, setup acpTestSetup) uuid.UUID {
					pinned := setup.pinnedConfig
					_, err := db.UpdateChatModelConfig(context.Background(), database.UpdateChatModelConfigParams{
						ID:                   pinned.ID,
						Model:                pinned.Model,
						DisplayName:          pinned.DisplayName,
						Enabled:              false,
						IsDefault:            pinned.IsDefault,
						ContextLimit:         pinned.ContextLimit,
						CompressionThreshold: pinned.CompressionThreshold,
						Options:              pinned.Options,
						AIProviderID:         pinned.AIProviderID,
					})
					require.NoError(t, err)
					return uuid.Nil
				},
				wantError: "The " + harness.DisplayName + ` runtime's pinned model "` + acpTestPinnedModel + `" is disabled or no longer available; an administrator must update the runtime configuration.`,
			},
			{
				name: "SelectedModel",
				seed: func(t *testing.T, db database.Store, setup acpTestSetup) uuid.UUID {
					return acpModelConfig(t, db, setup, setup.providerID, "selected-model").ID
				},
				wantCreds: primaryCredentials("selected-model"),
				wantStamp: true,
				wantMode:  "acceptEdits",
			},
			{
				name: "SelectedDisabledFallsBack",
				seed: func(t *testing.T, db database.Store, setup acpTestSetup) uuid.UUID {
					return acpModelConfig(t, db, setup, setup.providerID, "disabled-model", func(p *database.InsertChatModelConfigParams) {
						p.Enabled = false
					}).ID
				},
				wantCreds: primaryCredentials(acpTestPinnedModel),
				wantMode:  "acceptEdits",
			},
			{
				name: "SelectedProviderKey",
				seed: func(t *testing.T, db database.Store, setup acpTestSetup) uuid.UUID {
					second := seedSecondHarnessProvider(t, db, setup)
					return acpModelConfig(t, db, setup, second.ID, "second-model").ID
				},
				wantCreds: chatacp.TurnCredentials{APIKey: "second-provider-key", BaseURL: "https://second.example.com", Model: "second-model"},
				wantStamp: true,
				wantMode:  "acceptEdits",
			},
			{
				// A model is never paired with another provider's key.
				name: "SelectedKeylessProviderFallsBack",
				seed: func(t *testing.T, db database.Store, setup acpTestSetup) uuid.UUID {
					keyless := dbgen.ChatProvider(t, db, database.ChatProvider{
						Provider:    string(setup.harness.ProviderType),
						DisplayName: "keyless",
						Enabled:     true,
					}, func(p *database.InsertChatProviderParams) {
						p.APIKey = ""
					})
					return acpModelConfig(t, db, setup, keyless.ID, "keyless-model").ID
				},
				wantCreds: primaryCredentials(acpTestPinnedModel),
				wantMode:  "acceptEdits",
			},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				db, ps := dbtestutil.NewDB(t)
				ctx := testutil.Context(t, testutil.WaitLong)

				setup := seedACPChatDependencies(t, db, harness, database.WorkspaceTransitionStart)
				if tc.noPin {
					_, err := db.UpsertChatRuntimeConfig(ctx, database.UpsertChatRuntimeConfigParams{
						OrganizationID: setup.org.ID,
						Runtime:        database.ChatRuntime(harness.Runtime),
						TemplateID:     setup.workspace.TemplateID,
						Enabled:        true,
					})
					require.NoError(t, err)
				}
				var mutators []func(*chatstate.CreateChatInput)
				var wantStamp uuid.NullUUID
				if tc.seed != nil {
					if selected := tc.seed(t, db, setup); selected != uuid.Nil {
						mutators = append(mutators, withInitialModelConfig(selected))
						if tc.wantStamp {
							wantStamp = uuid.NullUUID{UUID: selected, Valid: true}
						}
					}
				}

				fakeAgent := replyingFakeAgent("reply")
				created := createACPChat(ctx, t, db, ps, setup, "hello", mutators...)
				_ = newActiveTestServer(t, db, ps, acpConfigOverrides(t, setup, fakeAgent, tc.wantCreds))

				chat := waitForTerminalChat(ctx, t, db, created.Chat.ID)
				if tc.wantError != "" {
					require.Equal(t, database.ChatStatusError, chat.Status)
					var lastError codersdk.ChatError
					require.NoError(t, json.Unmarshal(chat.LastError.RawMessage, &lastError))
					require.Equal(t, codersdk.ChatErrorKindConfig, lastError.Kind)
					require.Equal(t, tc.wantError, lastError.Message)
					require.Empty(t, fakeAgent.Prompts())
					require.Empty(t, fakeAgent.NewSessions())
					return
				}
				require.Equal(t, database.ChatStatusWaiting, chat.Status)
				require.False(t, chat.LastError.Valid)
				prompts := fakeAgent.Prompts()
				require.Len(t, prompts, 1)
				require.Equal(t, "hello", prompts[0].Prompt[0].Text.Text)
				sessions := fakeAgent.NewSessions()
				require.Len(t, sessions, 1)
				require.Equal(t, "/home/coder/project", sessions[0].Cwd)

				modes := fakeAgent.Modes()
				if tc.wantMode == "" {
					require.Empty(t, modes)
				} else {
					require.Len(t, modes, 1)
					require.Equal(t, acp.SessionModeId(tc.wantMode), modes[0].ModeId)
				}

				messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: created.Chat.ID})
				require.NoError(t, err)
				require.Len(t, messages, 2)
				require.Equal(t, database.ChatMessageRoleAssistant, messages[1].Role)
				parts, err := chatprompt.ParseContent(messages[1])
				require.NoError(t, err)
				require.Equal(t, []codersdk.ChatMessagePart{codersdk.ChatMessageText("reply")}, parts)
				require.Equal(t, int64(11), messages[1].InputTokens.Int64)
				require.Equal(t, int64(7), messages[1].OutputTokens.Int64)
				require.Equal(t, wantStamp, messages[1].ModelConfigID)
				// The applied selection groups token analytics; these turns
				// carry no cost.
				require.False(t, messages[1].TotalCostMicros.Valid)

				state := chatacp.ParseRuntimeState(chat.RuntimeState.RawMessage)
				require.Equal(t, "session-new", state.SessionID)
				require.Equal(t, "/home/coder/project", state.Cwd)
				require.Equal(t, []codersdk.ChatRuntimeCommand{
					{Name: "review", Description: "Review the current diff", InputHint: "pr number"},
					{Name: "init", Description: "Create a project guide"},
				}, state.AvailableCommands)
			})
		}
	})
}

func TestACPModelSelectionValidation(t *testing.T) {
	t.Parallel()

	forEachHarness(t, func(t *testing.T, harness chatacp.Harness) {
		db, ps := dbtestutil.NewDB(t)
		replica := newTestServer(t, db, ps, uuid.New())
		setupCtx := testutil.Context(t, testutil.WaitLong)

		setup := seedACPChatDependencies(t, db, harness, database.WorkspaceTransitionStart)
		validCfg := acpModelConfig(t, db, setup, setup.providerID, "valid-model")
		otherCfg := acpModelConfig(t, db, setup, setup.otherProviderID, "other-provider-model", func(p *database.InsertChatModelConfigParams) {
			p.IsDefault = true
		})
		disabledCfg := acpModelConfig(t, db, setup, setup.providerID, "disabled-model", func(p *database.InsertChatModelConfigParams) {
			p.Enabled = false
		})
		otherOrgCfg := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
			OrganizationID: dbgen.Organization(t, db, database.Organization{}).ID,
			Model:          "other-org-model",
			AIProviderID:   uuid.NullUUID{UUID: setup.providerID, Valid: true},
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
			"OtherProviderRejected":     otherCfg.ID,
			"OtherOrganizationRejected": otherOrgCfg.ID,
			"DisabledRejected":          disabledCfg.ID,
			"UnknownRejected":           uuid.New(),
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
	})
}
