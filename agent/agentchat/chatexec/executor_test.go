package chatexec_test

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
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentchat/chatexec"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/quartz"
)

func TestExecutor_HappyPath(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	runtimeContext := defaultRuntimeContext()
	runtimeContext.ChatID = chatID

	client := &mockChatRunnerClient{
		runtimeContextResp: runtimeContext,
		persistStepResp:    agentsdk.ChatRunnerPersistStepResponse{OK: true},
	}

	var buildCall buildModelCall
	var runOpts chatloop.RunOptions

	executor := newTestExecutor(
		t,
		client,
		func(
			providerHint string,
			modelName string,
			keys chatprovider.ProviderAPIKeys,
			userAgent string,
			extraHeaders map[string]string,
		) (fantasy.LanguageModel, error) {
			buildCall = buildModelCall{
				providerHint: providerHint,
				modelName:    modelName,
				keys:         keys,
				userAgent:    userAgent,
				extraHeaders: extraHeaders,
			}
			return fakeLanguageModel{provider: providerHint, model: modelName}, nil
		},
		func(ctx context.Context, opts chatloop.RunOptions) error {
			runOpts = opts
			require.NotNil(t, opts.PersistStep)
			require.NotNil(t, opts.PublishMessagePart)
			require.NotNil(t, opts.ReloadMessages)
			require.NotNil(t, opts.OnRetry)
			require.NotNil(t, opts.OnInterruptedPersistError)
			require.Len(t, opts.Messages, 1)
			require.Len(t, opts.Messages[0].Content, 1)

			userText, ok := fantasy.AsMessagePart[fantasy.TextPart](opts.Messages[0].Content[0])
			require.True(t, ok)
			require.Equal(t, fantasy.MessageRoleUser, opts.Messages[0].Role)
			require.Equal(t, "hello", userText.Text)

			return opts.PersistStep(ctx, chatloop.PersistedStep{
				Content: []fantasy.Content{fantasy.TextContent{Text: "hello back"}},
				Usage: fantasy.Usage{
					InputTokens:  10,
					OutputTokens: 20,
					TotalTokens:  30,
				},
				ContextLimit:       sql.NullInt64{Int64: 256000, Valid: true},
				ProviderResponseID: "response-1",
				Runtime:            1500 * time.Millisecond,
			})
		},
	)

	err := executor.Execute(context.Background(), chatID)
	require.NoError(t, err)

	runtimeCalls := client.runtimeContextCallsSnapshot()
	require.Len(t, runtimeCalls, 1)
	require.Equal(t, chatID, runtimeCalls[0].ChatID)

	require.Equal(t, runtimeContext.Provider, buildCall.providerHint)
	require.Equal(t, runtimeContext.Model, buildCall.modelName)
	require.Equal(t, runtimeContext.ProviderAPIKeys, buildCall.keys.ByProvider)
	require.Equal(t, runtimeContext.ProviderBaseURLs, buildCall.keys.BaseURLByProvider)
	require.Equal(t, chatprovider.UserAgent(), buildCall.userAgent)
	require.Nil(t, buildCall.extraHeaders)

	require.Equal(t, 1200, runOpts.MaxSteps)
	require.Empty(t, runOpts.Tools)
	require.Nil(t, runOpts.ActiveTools)
	require.Empty(t, runOpts.ProviderTools)
	require.Nil(t, runOpts.DynamicToolNames)

	persistCalls := client.persistStepCallsSnapshot()
	require.Len(t, persistCalls, 1)
	require.Equal(t, chatID, persistCalls[0].ChatID)
	require.Equal(t, runtimeContext.LeaseEpoch, persistCalls[0].LeaseEpoch)
	require.Equal(t, runtimeContext.ModelConfigID, persistCalls[0].ModelConfigID)
	require.Len(t, persistCalls[0].AssistantParts, 1)
	require.Equal(t, codersdk.ChatMessagePartTypeText, persistCalls[0].AssistantParts[0].Type)
	require.Equal(t, "hello back", persistCalls[0].AssistantParts[0].Text)
	require.Nil(t, persistCalls[0].ToolResults)
	require.NotNil(t, persistCalls[0].Usage)
	require.Equal(t, int64(10), persistCalls[0].Usage.InputTokens)
	require.Equal(t, int64(20), persistCalls[0].Usage.OutputTokens)
	require.Equal(t, int64(30), persistCalls[0].Usage.TotalTokens)
	require.NotNil(t, persistCalls[0].ContextLimit)
	require.Equal(t, int64(256000), *persistCalls[0].ContextLimit)
	require.Equal(t, "response-1", persistCalls[0].ProviderResponseID)
	require.Equal(t, int64(1500), persistCalls[0].RuntimeMs)
}

func TestExecutor_ReloadMessages(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	runtimeContext := defaultRuntimeContext()
	runtimeContext.ChatID = chatID

	client := &mockChatRunnerClient{
		runtimeContextResp: runtimeContext,
		reloadResp: agentsdk.ChatRunnerReloadMessagesResponse{
			Messages: []agentsdk.ChatRunnerMessage{{
				Role: string(codersdk.ChatMessageRoleAssistant),
				Text: "reloaded message",
			}},
		},
	}

	executor := newTestExecutor(
		t,
		client,
		nil,
		func(ctx context.Context, opts chatloop.RunOptions) error {
			messages, err := opts.ReloadMessages(ctx)
			require.NoError(t, err)
			require.NotEmpty(t, messages)
			require.Len(t, messages[0].Content, 1)

			text, ok := fantasy.AsMessagePart[fantasy.TextPart](messages[0].Content[0])
			require.True(t, ok)
			require.Equal(t, fantasy.MessageRoleAssistant, messages[0].Role)
			require.Equal(t, "reloaded message", text.Text)
			return nil
		},
	)

	err := executor.Execute(context.Background(), chatID)
	require.NoError(t, err)

	reloadCalls := client.reloadCallsSnapshot()
	require.Len(t, reloadCalls, 1)
	require.Equal(t, chatID, reloadCalls[0].ChatID)
	require.Equal(t, runtimeContext.LeaseEpoch, reloadCalls[0].LeaseEpoch)
}

func TestExecutor_StaleLeaseConflict(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	runtimeContext := defaultRuntimeContext()
	runtimeContext.ChatID = chatID

	client := &mockChatRunnerClient{
		runtimeContextResp: runtimeContext,
		persistStepErr:     xerrors.New("stale lease"),
	}

	executor := newTestExecutor(
		t,
		client,
		nil,
		func(ctx context.Context, opts chatloop.RunOptions) error {
			return opts.PersistStep(ctx, chatloop.PersistedStep{
				Content: []fantasy.Content{fantasy.TextContent{Text: "retry me"}},
			})
		},
	)

	err := executor.Execute(context.Background(), chatID)
	require.Error(t, err)
	require.ErrorContains(t, err, "persist step")
	require.ErrorContains(t, err, "stale lease")
}

func TestExecutor_ContextCancellation(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := &mockChatRunnerClient{}
	executor := newTestExecutor(
		t,
		client,
		func(
			providerHint string,
			modelName string,
			keys chatprovider.ProviderAPIKeys,
			userAgent string,
			extraHeaders map[string]string,
		) (fantasy.LanguageModel, error) {
			t.Fatal("buildModel should not be called when runtime context fetch fails")
			return fakeLanguageModel{}, xerrors.New("unexpected buildModel call")
		},
		func(context.Context, chatloop.RunOptions) error {
			t.Fatal("runLoop should not be called when runtime context fetch fails")
			return nil
		},
	)

	err := executor.Execute(ctx, chatID)
	require.Error(t, err)
	require.ErrorContains(t, err, "fetch runtime context")
	require.ErrorIs(t, err, context.Canceled)
}

func TestExecutor_PublishBestEffort(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	runtimeContext := defaultRuntimeContext()
	runtimeContext.ChatID = chatID

	client := &mockChatRunnerClient{
		runtimeContextResp: runtimeContext,
		publishPartsErr:    xerrors.New("publish failed"),
	}

	clock := quartz.NewMock(t)
	executor := newTestExecutor(
		t,
		client,
		nil,
		func(ctx context.Context, opts chatloop.RunOptions) error {
			opts.PublishMessagePart(
				codersdk.ChatMessageRoleAssistant,
				codersdk.ChatMessageText("partial"),
			)
			return nil
		},
	)
	executor.SetClock(clock)

	err := executor.Execute(context.Background(), chatID)
	require.NoError(t, err)

	publishCalls := client.publishPartsCallsSnapshot()
	require.Len(t, publishCalls, 1)
	requirePublishPartsMetadata(t, publishCalls, chatID, runtimeContext.LeaseEpoch)
	require.Equal(t, []agentsdk.ChatRunnerPublishStreamPart{{
		Role: codersdk.ChatMessageRoleAssistant,
		Part: codersdk.ChatMessageText("partial"),
	}}, publishCalls[0].Parts)
}

func TestExecutor_BatchPublish(t *testing.T) {
	t.Parallel()

	t.Run("Coalescing", func(t *testing.T) {
		t.Parallel()

		chatID := uuid.New()
		runtimeContext := defaultRuntimeContext()
		runtimeContext.ChatID = chatID

		client := &mockChatRunnerClient{runtimeContextResp: runtimeContext}
		clock := quartz.NewMock(t)
		expected := assistantStreamParts("part-1", "part-2", "part-3", "part-4", "part-5")

		executor := newTestExecutor(
			t,
			client,
			nil,
			func(_ context.Context, opts chatloop.RunOptions) error {
				for _, part := range expected {
					opts.PublishMessagePart(part.Role, part.Part)
				}
				clock.Advance(49 * time.Millisecond).MustWait(t.Context())
				require.Empty(t, client.publishPartsCallsSnapshot())
				clock.Advance(time.Millisecond).MustWait(t.Context())
				return nil
			},
		)
		executor.SetClock(clock)

		err := executor.Execute(context.Background(), chatID)
		require.NoError(t, err)

		calls := client.publishPartsCallsSnapshot()
		require.Len(t, calls, 1)
		require.Less(t, len(calls), len(expected))
		requirePublishPartsMetadata(t, calls, chatID, runtimeContext.LeaseEpoch)
		require.Equal(t, expected, flattenPublishParts(calls))
	})

	t.Run("CloseFlush", func(t *testing.T) {
		t.Parallel()

		chatID := uuid.New()
		runtimeContext := defaultRuntimeContext()
		runtimeContext.ChatID = chatID

		client := &mockChatRunnerClient{runtimeContextResp: runtimeContext}
		clock := quartz.NewMock(t)
		expected := assistantStreamParts("tail-1", "tail-2", "tail-3")

		executor := newTestExecutor(
			t,
			client,
			nil,
			func(_ context.Context, opts chatloop.RunOptions) error {
				for _, part := range expected {
					opts.PublishMessagePart(part.Role, part.Part)
				}
				return nil
			},
		)
		executor.SetClock(clock)

		err := executor.Execute(context.Background(), chatID)
		require.NoError(t, err)

		calls := client.publishPartsCallsSnapshot()
		require.Len(t, calls, 1)
		requirePublishPartsMetadata(t, calls, chatID, runtimeContext.LeaseEpoch)
		require.Equal(t, expected, calls[0].Parts)
	})

	t.Run("Ordering", func(t *testing.T) {
		t.Parallel()

		chatID := uuid.New()
		runtimeContext := defaultRuntimeContext()
		runtimeContext.ChatID = chatID

		client := &mockChatRunnerClient{runtimeContextResp: runtimeContext}
		clock := quartz.NewMock(t)
		firstBatch := assistantStreamParts("first-1", "first-2")
		secondBatch := assistantStreamParts("second-1", "second-2", "second-3")
		expected := append(append([]agentsdk.ChatRunnerPublishStreamPart(nil), firstBatch...), secondBatch...)

		executor := newTestExecutor(
			t,
			client,
			nil,
			func(_ context.Context, opts chatloop.RunOptions) error {
				for _, part := range firstBatch {
					opts.PublishMessagePart(part.Role, part.Part)
				}
				clock.Advance(50 * time.Millisecond).MustWait(t.Context())
				require.Len(t, client.publishPartsCallsSnapshot(), 1)
				for _, part := range secondBatch {
					opts.PublishMessagePart(part.Role, part.Part)
				}
				clock.Advance(50 * time.Millisecond).MustWait(t.Context())
				return nil
			},
		)
		executor.SetClock(clock)

		err := executor.Execute(context.Background(), chatID)
		require.NoError(t, err)

		calls := client.publishPartsCallsSnapshot()
		require.Len(t, calls, 2)
		requirePublishPartsMetadata(t, calls, chatID, runtimeContext.LeaseEpoch)
		require.Equal(t, expected, flattenPublishParts(calls))
	})

	t.Run("ErrorBestEffort", func(t *testing.T) {
		t.Parallel()

		chatID := uuid.New()
		runtimeContext := defaultRuntimeContext()
		runtimeContext.ChatID = chatID

		client := &mockChatRunnerClient{
			runtimeContextResp: runtimeContext,
			publishPartsErr:    xerrors.New("publish failed"),
		}
		clock := quartz.NewMock(t)
		expected := assistantStreamParts("best-effort-1", "best-effort-2")

		executor := newTestExecutor(
			t,
			client,
			nil,
			func(_ context.Context, opts chatloop.RunOptions) error {
				for _, part := range expected {
					opts.PublishMessagePart(part.Role, part.Part)
				}
				clock.Advance(50 * time.Millisecond).MustWait(t.Context())
				return nil
			},
		)
		executor.SetClock(clock)

		err := executor.Execute(context.Background(), chatID)
		require.NoError(t, err)

		calls := client.publishPartsCallsSnapshot()
		require.Len(t, calls, 1)
		requirePublishPartsMetadata(t, calls, chatID, runtimeContext.LeaseEpoch)
		require.Equal(t, expected, flattenPublishParts(calls))
	})
}

func TestExecutor_RuntimeContextError(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	client := &mockChatRunnerClient{runtimeContextErr: xerrors.New("runtime context unavailable")}
	executor := newTestExecutor(
		t,
		client,
		func(
			providerHint string,
			modelName string,
			keys chatprovider.ProviderAPIKeys,
			userAgent string,
			extraHeaders map[string]string,
		) (fantasy.LanguageModel, error) {
			t.Fatal("buildModel should not be called when runtime context fetch fails")
			return fakeLanguageModel{}, xerrors.New("unexpected buildModel call")
		},
		func(context.Context, chatloop.RunOptions) error {
			t.Fatal("runLoop should not be called when runtime context fetch fails")
			return nil
		},
	)

	err := executor.Execute(context.Background(), chatID)
	require.Error(t, err)
	require.ErrorContains(t, err, "fetch runtime context")
	require.ErrorContains(t, err, "runtime context unavailable")
}

func TestExecutor_Compaction(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	runtimeContext := defaultRuntimeContext()
	runtimeContext.ChatID = chatID
	runtimeContext.CompactionThresholdPercent = 80

	client := &mockChatRunnerClient{runtimeContextResp: runtimeContext}
	executor := newTestExecutor(
		t,
		client,
		nil,
		func(_ context.Context, opts chatloop.RunOptions) error {
			require.NotNil(t, opts.Compaction)
			require.Equal(t, int32(80), opts.Compaction.ThresholdPercent)
			require.Equal(t, runtimeContext.ContextLimit, opts.Compaction.ContextLimit)
			require.Equal(t, "compaction-tool-call", opts.Compaction.ToolCallID)
			require.Equal(t, "coder_chat_compaction", opts.Compaction.ToolName)
			require.NotNil(t, opts.Compaction.PublishMessagePart)
			require.NotNil(t, opts.Compaction.OnError)
			return nil
		},
	)

	err := executor.Execute(context.Background(), chatID)
	require.NoError(t, err)
}

func TestExecutor_CompactionDisabled(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	runtimeContext := defaultRuntimeContext()
	runtimeContext.ChatID = chatID
	runtimeContext.CompactionThresholdPercent = 0

	client := &mockChatRunnerClient{runtimeContextResp: runtimeContext}
	executor := newTestExecutor(
		t,
		client,
		nil,
		func(_ context.Context, opts chatloop.RunOptions) error {
			require.Nil(t, opts.Compaction)
			return nil
		},
	)

	err := executor.Execute(context.Background(), chatID)
	require.NoError(t, err)
}

func TestExecutor_BuildsSupportedLocalTools(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	runtimeContext := defaultRuntimeContext()
	runtimeContext.ChatID = chatID
	runtimeContext.BuiltinTools = []agentsdk.ChatRunnerToolDefinition{
		{Name: "read_file"},
		{Name: "write_file"},
		{Name: "edit_files"},
		{Name: "execute"},
		{Name: "process_output"},
		{Name: "process_list"},
		{Name: "process_signal"},
		{Name: "read_skill"},
		{Name: "create_workspace"},
	}

	client := &mockChatRunnerClient{runtimeContextResp: runtimeContext}
	localConnCalls := 0
	executor := newTestExecutor(
		t,
		client,
		nil,
		func(_ context.Context, opts chatloop.RunOptions) error {
			require.Equal(t, []string{
				"read_file",
				"write_file",
				"edit_files",
				"execute",
				"process_output",
				"process_list",
				"process_signal",
			}, agentToolNames(opts.Tools))
			require.Empty(t, opts.ProviderTools)
			require.Zero(t, localConnCalls)
			return nil
		},
		func(context.Context) (workspacesdk.AgentConn, error) {
			localConnCalls++
			return nil, xerrors.New("unexpected local conn call")
		},
	)

	err := executor.Execute(context.Background(), chatID)
	require.NoError(t, err)
	require.Zero(t, localConnCalls)
}

func TestExecutor_BuildsProviderTools(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	runtimeContext := defaultRuntimeContext()
	runtimeContext.ChatID = chatID
	runtimeContext.ProviderTools = []agentsdk.ChatRunnerToolDefinition{
		providerToolDefinition(t, fantasy.ProviderDefinedTool{
			ID:   "web_search",
			Name: "web_search",
			Args: map[string]any{"allowed_domains": []string{"example.com"}},
		}),
		providerToolDefinition(t, chattool.ComputerUseProviderTool(1280, 800)),
	}

	client := &mockChatRunnerClient{runtimeContextResp: runtimeContext}
	localConnCalls := 0
	executor := newTestExecutor(
		t,
		client,
		nil,
		func(_ context.Context, opts chatloop.RunOptions) error {
			require.Empty(t, opts.Tools)
			require.Len(t, opts.ProviderTools, 2)
			require.Zero(t, localConnCalls)

			webSearch := providerDefinedTool(t, opts.ProviderTools[0].Definition)
			require.Equal(t, "web_search", webSearch.ID)
			require.Equal(t, "web_search", webSearch.Name)
			domains, ok := webSearch.Args["allowed_domains"].([]any)
			require.True(t, ok)
			require.Equal(t, []any{"example.com"}, domains)
			require.Nil(t, opts.ProviderTools[0].Runner)

			computer := providerDefinedTool(t, opts.ProviderTools[1].Definition)
			require.Equal(t, "anthropic.computer", computer.ID)
			require.Equal(t, "computer", computer.Name)
			require.NotNil(t, opts.ProviderTools[1].Runner)
			require.Equal(t, "computer", opts.ProviderTools[1].Runner.Info().Name)
			requireProviderIntArg(t, computer.Args, "display_width_px", 1280)
			requireProviderIntArg(t, computer.Args, "display_height_px", 800)
			return nil
		},
		func(context.Context) (workspacesdk.AgentConn, error) {
			localConnCalls++
			return nil, xerrors.New("unexpected local conn call")
		},
	)

	err := executor.Execute(context.Background(), chatID)
	require.NoError(t, err)
	require.Zero(t, localConnCalls)
}

func TestExecutor_LocalToolExecutionUsesLocalConn(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	runtimeContext := defaultRuntimeContext()
	runtimeContext.ChatID = chatID
	runtimeContext.BuiltinTools = []agentsdk.ChatRunnerToolDefinition{{Name: "process_list"}}

	client := &mockChatRunnerClient{runtimeContextResp: runtimeContext}
	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	mockConn.EXPECT().ListProcesses(gomock.Any()).Return(workspacesdk.ListProcessesResponse{
		Processes: []workspacesdk.ProcessInfo{{
			ID:        "proc-1",
			Command:   "sleep 1",
			Running:   true,
			StartedAt: 123,
		}},
	}, nil)

	localConnCalls := 0
	executor := newTestExecutor(
		t,
		client,
		nil,
		func(ctx context.Context, opts chatloop.RunOptions) error {
			require.Len(t, opts.Tools, 1)

			resp, err := opts.Tools[0].Run(ctx, fantasy.ToolCall{ID: "call-1", Name: "process_list", Input: `{}`})
			require.NoError(t, err)
			require.False(t, resp.IsError)

			var parsed workspacesdk.ListProcessesResponse
			require.NoError(t, json.Unmarshal([]byte(resp.Content), &parsed))
			require.Len(t, parsed.Processes, 1)
			require.Equal(t, "proc-1", parsed.Processes[0].ID)
			require.Equal(t, "sleep 1", parsed.Processes[0].Command)
			return nil
		},
		func(context.Context) (workspacesdk.AgentConn, error) {
			localConnCalls++
			return mockConn, nil
		},
	)

	err := executor.Execute(context.Background(), chatID)
	require.NoError(t, err)
	require.Equal(t, 1, localConnCalls)
}

func TestExecutor_BuildsDynamicToolsAndNames(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	runtimeContext := defaultRuntimeContext()
	runtimeContext.ChatID = chatID
	runtimeContext.DynamicTools = []agentsdk.ChatRunnerToolDefinition{
		{
			Name:        "dynamic_lookup",
			Description: "Look up records",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`),
		},
		{
			Name:        "dynamic_create",
			Description: "Create records",
		},
	}

	client := &mockChatRunnerClient{runtimeContextResp: runtimeContext}
	executor := newTestExecutor(
		t,
		client,
		nil,
		func(_ context.Context, opts chatloop.RunOptions) error {
			require.Equal(t, []string{"dynamic_lookup", "dynamic_create"}, agentToolNames(opts.Tools))
			require.Equal(t, map[string]bool{
				"dynamic_lookup": true,
				"dynamic_create": true,
			}, opts.DynamicToolNames)

			lookup := opts.Tools[0].Info()
			require.Equal(t, "dynamic_lookup", lookup.Name)
			require.Equal(t, "Look up records", lookup.Description)
			require.Contains(t, lookup.Parameters, "query")
			require.Equal(t, []string{"query"}, lookup.Required)
			return nil
		},
	)

	err := executor.Execute(context.Background(), chatID)
	require.NoError(t, err)
}

func TestExecutor_DynamicToolCollisionsPreferExistingTools(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	runtimeContext := defaultRuntimeContext()
	runtimeContext.ChatID = chatID
	runtimeContext.BuiltinTools = []agentsdk.ChatRunnerToolDefinition{
		{Name: "read_file"},
		{Name: "list_templates"},
	}
	runtimeContext.ProviderTools = []agentsdk.ChatRunnerToolDefinition{
		providerToolDefinition(t, fantasy.ProviderDefinedTool{
			ID:   "web_search",
			Name: "web_search",
			Args: map[string]any{"allowed_domains": []string{"example.com"}},
		}),
	}
	runtimeContext.DynamicTools = []agentsdk.ChatRunnerToolDefinition{
		{Name: "read_file", Description: "Dynamic read file"},
		{Name: "list_templates", Description: "Dynamic template list"},
		{Name: "web_search", Description: "Dynamic web search"},
		{Name: "dynamic_lookup", Description: "Dynamic lookup"},
	}

	client := &mockChatRunnerClient{runtimeContextResp: runtimeContext}
	executor := newTestExecutor(
		t,
		client,
		nil,
		func(_ context.Context, opts chatloop.RunOptions) error {
			require.Equal(t, []string{"read_file", "list_templates", "dynamic_lookup"}, agentToolNames(opts.Tools))
			require.Equal(t, map[string]bool{"dynamic_lookup": true}, opts.DynamicToolNames)
			require.Len(t, opts.ProviderTools, 1)
			require.Equal(t, "web_search", opts.ProviderTools[0].Definition.GetName())
			return nil
		},
		func(context.Context) (workspacesdk.AgentConn, error) {
			return nil, xerrors.New("unexpected local conn call")
		},
	)

	err := executor.Execute(context.Background(), chatID)
	require.NoError(t, err)
}

func TestExecutor_BuildsMCPToolsAndExecutesThroughProxy(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	serverConfigID := uuid.New()
	runtimeContext := defaultRuntimeContext()
	runtimeContext.ChatID = chatID
	runtimeContext.MCPTools = []agentsdk.ChatRunnerMCPTool{{
		MCPServerConfigID: serverConfigID,
		ToolName:          "github__search_issues",
		Description:       "Search GitHub issues",
		InputSchema:       json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`),
		ServerDisplayName: "GitHub",
	}}

	client := &mockChatRunnerClient{
		runtimeContextResp: runtimeContext,
		mcpToolCallResp: agentsdk.ChatRunnerMCPToolCallResponse{
			Result: json.RawMessage(`"{\"hits\":1}"`),
		},
	}
	localConnCalls := 0

	executor := newTestExecutor(
		t,
		client,
		nil,
		func(ctx context.Context, opts chatloop.RunOptions) error {
			require.Equal(t, []string{"github__search_issues"}, agentToolNames(opts.Tools))
			require.Empty(t, opts.ProviderTools)
			require.Nil(t, opts.DynamicToolNames)
			require.Zero(t, localConnCalls)

			toolInfo := opts.Tools[0].Info()
			require.Equal(t, "github__search_issues", toolInfo.Name)
			require.Equal(t, "Search GitHub issues", toolInfo.Description)
			require.Contains(t, toolInfo.Parameters, "query")
			require.Equal(t, []string{"query"}, toolInfo.Required)
			require.True(t, toolInfo.Parallel)

			resp, err := opts.Tools[0].Run(ctx, fantasy.ToolCall{
				ID:    "call-1",
				Name:  "github__search_issues",
				Input: `{"query":"coder"}`,
			})
			require.NoError(t, err)
			require.False(t, resp.IsError)
			require.Equal(t, `{"hits":1}`, resp.Content)
			return nil
		},
		func(context.Context) (workspacesdk.AgentConn, error) {
			localConnCalls++
			return nil, xerrors.New("unexpected local conn call")
		},
	)

	err := executor.Execute(context.Background(), chatID)
	require.NoError(t, err)
	require.Zero(t, localConnCalls)

	calls := client.mcpToolCallCallsSnapshot()
	require.Len(t, calls, 1)
	require.Equal(t, chatID, calls[0].ChatID)
	require.Equal(t, runtimeContext.LeaseEpoch, calls[0].LeaseEpoch)
	require.Equal(t, serverConfigID, calls[0].MCPServerConfigID)
	require.Equal(t, "github__search_issues", calls[0].ToolName)
	require.JSONEq(t, `{"query":"coder"}`, string(calls[0].Args))
}

func TestExecutor_MCPToolProxyErrorsBecomeToolErrors(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	serverConfigID := uuid.New()
	runtimeContext := defaultRuntimeContext()
	runtimeContext.ChatID = chatID
	runtimeContext.MCPTools = []agentsdk.ChatRunnerMCPTool{{
		MCPServerConfigID: serverConfigID,
		ToolName:          "github__search_issues",
		Description:       "Search GitHub issues",
	}}

	client := &mockChatRunnerClient{
		runtimeContextResp: runtimeContext,
		mcpToolCallResp: agentsdk.ChatRunnerMCPToolCallResponse{
			Result:  json.RawMessage(`"rate limited"`),
			IsError: true,
		},
	}

	executor := newTestExecutor(
		t,
		client,
		nil,
		func(ctx context.Context, opts chatloop.RunOptions) error {
			resp, err := opts.Tools[0].Run(ctx, fantasy.ToolCall{
				ID:    "call-1",
				Name:  "github__search_issues",
				Input: `{"query":"coder"}`,
			})
			require.NoError(t, err)
			require.True(t, resp.IsError)
			require.Equal(t, "rate limited", resp.Content)
			return nil
		},
	)

	err := executor.Execute(context.Background(), chatID)
	require.NoError(t, err)

	calls := client.mcpToolCallCallsSnapshot()
	require.Len(t, calls, 1)
	require.Equal(t, serverConfigID, calls[0].MCPServerConfigID)
}

func TestExecutor_MCPToolTransportErrorsPropagate(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	runtimeContext := defaultRuntimeContext()
	runtimeContext.ChatID = chatID
	runtimeContext.MCPTools = []agentsdk.ChatRunnerMCPTool{{
		MCPServerConfigID: uuid.New(),
		ToolName:          "github__search_issues",
		Description:       "Search GitHub issues",
	}}

	client := &mockChatRunnerClient{
		runtimeContextResp: runtimeContext,
		mcpToolCallErr:     xerrors.New("chat lease changed"),
	}

	executor := newTestExecutor(
		t,
		client,
		nil,
		func(ctx context.Context, opts chatloop.RunOptions) error {
			_, err := opts.Tools[0].Run(ctx, fantasy.ToolCall{
				ID:    "call-1",
				Name:  "github__search_issues",
				Input: `{"query":"coder"}`,
			})
			require.Error(t, err)
			return err
		},
	)

	err := executor.Execute(context.Background(), chatID)
	require.Error(t, err)
	require.ErrorContains(t, err, "chat lease changed")
}

func TestExecutor_MCPToolCollisionsPreferExistingTools(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	runtimeContext := defaultRuntimeContext()
	runtimeContext.ChatID = chatID
	runtimeContext.BuiltinTools = []agentsdk.ChatRunnerToolDefinition{{Name: "read_file"}}
	runtimeContext.ProviderTools = []agentsdk.ChatRunnerToolDefinition{
		providerToolDefinition(t, fantasy.ProviderDefinedTool{
			ID:   "web_search",
			Name: "web_search",
			Args: map[string]any{"allowed_domains": []string{"example.com"}},
		}),
	}
	runtimeContext.DynamicTools = []agentsdk.ChatRunnerToolDefinition{{
		Name:        "dynamic_lookup",
		Description: "Dynamic lookup",
	}}
	runtimeContext.MCPTools = []agentsdk.ChatRunnerMCPTool{
		{MCPServerConfigID: uuid.New(), ToolName: "read_file", Description: "MCP read"},
		{MCPServerConfigID: uuid.New(), ToolName: "dynamic_lookup", Description: "MCP dynamic"},
		{MCPServerConfigID: uuid.New(), ToolName: "web_search", Description: "MCP web search"},
		{MCPServerConfigID: uuid.New(), ToolName: "github__search_issues", Description: "MCP search issues"},
	}

	client := &mockChatRunnerClient{runtimeContextResp: runtimeContext}
	executor := newTestExecutor(
		t,
		client,
		nil,
		func(_ context.Context, opts chatloop.RunOptions) error {
			require.Equal(t, []string{"read_file", "dynamic_lookup", "github__search_issues"}, agentToolNames(opts.Tools))
			require.Len(t, opts.ProviderTools, 1)
			require.Equal(t, "web_search", opts.ProviderTools[0].Definition.GetName())
			require.Equal(t, map[string]bool{"dynamic_lookup": true}, opts.DynamicToolNames)
			return nil
		},
		func(context.Context) (workspacesdk.AgentConn, error) {
			return nil, xerrors.New("unexpected local conn call")
		},
	)

	err := executor.Execute(context.Background(), chatID)
	require.NoError(t, err)
}

func TestExecutor_DynamicToolCallMapsToRequiresAction(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	runtimeContext := defaultRuntimeContext()
	runtimeContext.ChatID = chatID

	client := &mockChatRunnerClient{runtimeContextResp: runtimeContext}
	executor := newTestExecutor(
		t,
		client,
		nil,
		func(context.Context, chatloop.RunOptions) error {
			return xerrors.Errorf("wrapped dynamic tool call: %w", chatloop.ErrDynamicToolCall)
		},
	)

	err := executor.Execute(context.Background(), chatID)
	require.ErrorIs(t, err, chatexec.ErrRequiresAction)
}

type mockChatRunnerClient struct {
	mu sync.Mutex

	runtimeContextResp  agentsdk.ChatRunnerRuntimeContextResponse
	runtimeContextErr   error
	runtimeContextCalls []agentsdk.ChatRunnerRuntimeContextRequest

	persistStepResp  agentsdk.ChatRunnerPersistStepResponse
	persistStepErr   error
	persistStepCalls []agentsdk.ChatRunnerPersistStepRequest

	publishResp  agentsdk.ChatRunnerPublishStreamPartResponse
	publishErr   error
	publishCalls []agentsdk.ChatRunnerPublishStreamPartRequest

	publishPartsResp  agentsdk.ChatRunnerPublishStreamPartsResponse
	publishPartsErr   error
	publishPartsCalls []agentsdk.ChatRunnerPublishStreamPartsRequest

	reloadResp  agentsdk.ChatRunnerReloadMessagesResponse
	reloadErr   error
	reloadCalls []agentsdk.ChatRunnerReloadMessagesRequest

	listTemplatesResp  agentsdk.ChatRunnerListTemplatesResponse
	listTemplatesErr   error
	listTemplatesCalls []agentsdk.ChatRunnerListTemplatesRequest

	readTemplateResp  agentsdk.ChatRunnerReadTemplateResponse
	readTemplateErr   error
	readTemplateCalls []agentsdk.ChatRunnerReadTemplateRequest

	mcpToolCallResp  agentsdk.ChatRunnerMCPToolCallResponse
	mcpToolCallErr   error
	mcpToolCallCalls []agentsdk.ChatRunnerMCPToolCallRequest
}

var _ chatexec.ChatRunnerClient = (*mockChatRunnerClient)(nil)

func (m *mockChatRunnerClient) ChatRunnerRuntimeContext(
	ctx context.Context,
	req agentsdk.ChatRunnerRuntimeContextRequest,
) (agentsdk.ChatRunnerRuntimeContextResponse, error) {
	m.mu.Lock()
	m.runtimeContextCalls = append(m.runtimeContextCalls, req)
	resp := m.runtimeContextResp
	err := m.runtimeContextErr
	m.mu.Unlock()

	if err != nil {
		return agentsdk.ChatRunnerRuntimeContextResponse{}, err
	}
	if ctx.Err() != nil {
		return agentsdk.ChatRunnerRuntimeContextResponse{}, ctx.Err()
	}
	return resp, nil
}

func (m *mockChatRunnerClient) ChatRunnerPersistStep(
	ctx context.Context,
	req agentsdk.ChatRunnerPersistStepRequest,
) (agentsdk.ChatRunnerPersistStepResponse, error) {
	m.mu.Lock()
	m.persistStepCalls = append(m.persistStepCalls, req)
	resp := m.persistStepResp
	err := m.persistStepErr
	m.mu.Unlock()

	if err != nil {
		return agentsdk.ChatRunnerPersistStepResponse{}, err
	}
	if ctx.Err() != nil {
		return agentsdk.ChatRunnerPersistStepResponse{}, ctx.Err()
	}
	return resp, nil
}

func (m *mockChatRunnerClient) ChatRunnerPublishStreamPart(
	ctx context.Context,
	req agentsdk.ChatRunnerPublishStreamPartRequest,
) (agentsdk.ChatRunnerPublishStreamPartResponse, error) {
	m.mu.Lock()
	m.publishCalls = append(m.publishCalls, req)
	resp := m.publishResp
	err := m.publishErr
	m.mu.Unlock()

	if err != nil {
		return agentsdk.ChatRunnerPublishStreamPartResponse{}, err
	}
	if ctx.Err() != nil {
		return agentsdk.ChatRunnerPublishStreamPartResponse{}, ctx.Err()
	}
	return resp, nil
}

func (m *mockChatRunnerClient) ChatRunnerPublishStreamParts(
	ctx context.Context,
	req agentsdk.ChatRunnerPublishStreamPartsRequest,
) (agentsdk.ChatRunnerPublishStreamPartsResponse, error) {
	m.mu.Lock()
	m.publishPartsCalls = append(m.publishPartsCalls, req)
	resp := m.publishPartsResp
	err := m.publishPartsErr
	m.mu.Unlock()

	if err != nil {
		return agentsdk.ChatRunnerPublishStreamPartsResponse{}, err
	}
	if ctx.Err() != nil {
		return agentsdk.ChatRunnerPublishStreamPartsResponse{}, ctx.Err()
	}
	return resp, nil
}

func (m *mockChatRunnerClient) ChatRunnerReloadMessages(
	ctx context.Context,
	req agentsdk.ChatRunnerReloadMessagesRequest,
) (agentsdk.ChatRunnerReloadMessagesResponse, error) {
	m.mu.Lock()
	m.reloadCalls = append(m.reloadCalls, req)
	resp := m.reloadResp
	err := m.reloadErr
	m.mu.Unlock()

	if err != nil {
		return agentsdk.ChatRunnerReloadMessagesResponse{}, err
	}
	if ctx.Err() != nil {
		return agentsdk.ChatRunnerReloadMessagesResponse{}, ctx.Err()
	}
	return resp, nil
}

func (m *mockChatRunnerClient) ChatRunnerListTemplates(
	ctx context.Context,
	req agentsdk.ChatRunnerListTemplatesRequest,
) (agentsdk.ChatRunnerListTemplatesResponse, error) {
	m.mu.Lock()
	m.listTemplatesCalls = append(m.listTemplatesCalls, req)
	resp := m.listTemplatesResp
	err := m.listTemplatesErr
	m.mu.Unlock()

	if err != nil {
		return agentsdk.ChatRunnerListTemplatesResponse{}, err
	}
	if ctx.Err() != nil {
		return agentsdk.ChatRunnerListTemplatesResponse{}, ctx.Err()
	}
	return resp, nil
}

func (m *mockChatRunnerClient) ChatRunnerReadTemplate(
	ctx context.Context,
	req agentsdk.ChatRunnerReadTemplateRequest,
) (agentsdk.ChatRunnerReadTemplateResponse, error) {
	m.mu.Lock()
	m.readTemplateCalls = append(m.readTemplateCalls, req)
	resp := m.readTemplateResp
	err := m.readTemplateErr
	m.mu.Unlock()

	if err != nil {
		return agentsdk.ChatRunnerReadTemplateResponse{}, err
	}
	if ctx.Err() != nil {
		return agentsdk.ChatRunnerReadTemplateResponse{}, ctx.Err()
	}
	return resp, nil
}

func (m *mockChatRunnerClient) ChatRunnerMCPToolCall(
	ctx context.Context,
	req agentsdk.ChatRunnerMCPToolCallRequest,
) (agentsdk.ChatRunnerMCPToolCallResponse, error) {
	m.mu.Lock()
	m.mcpToolCallCalls = append(m.mcpToolCallCalls, req)
	resp := m.mcpToolCallResp
	err := m.mcpToolCallErr
	m.mu.Unlock()

	if err != nil {
		return agentsdk.ChatRunnerMCPToolCallResponse{}, err
	}
	if ctx.Err() != nil {
		return agentsdk.ChatRunnerMCPToolCallResponse{}, ctx.Err()
	}
	return resp, nil
}

func (m *mockChatRunnerClient) runtimeContextCallsSnapshot() []agentsdk.ChatRunnerRuntimeContextRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]agentsdk.ChatRunnerRuntimeContextRequest(nil), m.runtimeContextCalls...)
}

func (m *mockChatRunnerClient) persistStepCallsSnapshot() []agentsdk.ChatRunnerPersistStepRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]agentsdk.ChatRunnerPersistStepRequest(nil), m.persistStepCalls...)
}

func (m *mockChatRunnerClient) publishPartsCallsSnapshot() []agentsdk.ChatRunnerPublishStreamPartsRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]agentsdk.ChatRunnerPublishStreamPartsRequest(nil), m.publishPartsCalls...)
}

func (m *mockChatRunnerClient) reloadCallsSnapshot() []agentsdk.ChatRunnerReloadMessagesRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]agentsdk.ChatRunnerReloadMessagesRequest(nil), m.reloadCalls...)
}

func (m *mockChatRunnerClient) listTemplatesCallsSnapshot() []agentsdk.ChatRunnerListTemplatesRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]agentsdk.ChatRunnerListTemplatesRequest(nil), m.listTemplatesCalls...)
}

func (m *mockChatRunnerClient) readTemplateCallsSnapshot() []agentsdk.ChatRunnerReadTemplateRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]agentsdk.ChatRunnerReadTemplateRequest(nil), m.readTemplateCalls...)
}

func (m *mockChatRunnerClient) mcpToolCallCallsSnapshot() []agentsdk.ChatRunnerMCPToolCallRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]agentsdk.ChatRunnerMCPToolCallRequest(nil), m.mcpToolCallCalls...)
}

func assistantStreamParts(texts ...string) []agentsdk.ChatRunnerPublishStreamPart {
	parts := make([]agentsdk.ChatRunnerPublishStreamPart, 0, len(texts))
	for _, text := range texts {
		parts = append(parts, agentsdk.ChatRunnerPublishStreamPart{
			Role: codersdk.ChatMessageRoleAssistant,
			Part: codersdk.ChatMessageText(text),
		})
	}
	return parts
}

func flattenPublishParts(calls []agentsdk.ChatRunnerPublishStreamPartsRequest) []agentsdk.ChatRunnerPublishStreamPart {
	total := 0
	for _, call := range calls {
		total += len(call.Parts)
	}
	parts := make([]agentsdk.ChatRunnerPublishStreamPart, 0, total)
	for _, call := range calls {
		parts = append(parts, call.Parts...)
	}
	return parts
}

func requirePublishPartsMetadata(
	t testing.TB,
	calls []agentsdk.ChatRunnerPublishStreamPartsRequest,
	chatID uuid.UUID,
	leaseEpoch int64,
) {
	t.Helper()
	for _, call := range calls {
		require.Equal(t, chatID, call.ChatID)
		require.Equal(t, leaseEpoch, call.LeaseEpoch)
	}
}

type buildModelCall struct {
	providerHint string
	modelName    string
	keys         chatprovider.ProviderAPIKeys
	userAgent    string
	extraHeaders map[string]string
}

type fakeLanguageModel struct {
	provider string
	model    string
}

func (fakeLanguageModel) Generate(context.Context, fantasy.Call) (*fantasy.Response, error) {
	panic("fakeLanguageModel.Generate should not be called in executor tests")
}

func (fakeLanguageModel) Stream(context.Context, fantasy.Call) (fantasy.StreamResponse, error) {
	panic("fakeLanguageModel.Stream should not be called in executor tests")
}

func (fakeLanguageModel) GenerateObject(context.Context, fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
	panic("fakeLanguageModel.GenerateObject should not be called in executor tests")
}

func (fakeLanguageModel) StreamObject(context.Context, fantasy.ObjectCall) (fantasy.ObjectStreamResponse, error) {
	panic("fakeLanguageModel.StreamObject should not be called in executor tests")
}

func (m fakeLanguageModel) Provider() string {
	return m.provider
}

func (m fakeLanguageModel) Model() string {
	return m.model
}

func newTestExecutor(
	t testing.TB,
	client *mockChatRunnerClient,
	buildModel func(string, string, chatprovider.ProviderAPIKeys, string, map[string]string) (fantasy.LanguageModel, error),
	runLoop func(context.Context, chatloop.RunOptions) error,
	getLocalConns ...func(context.Context) (workspacesdk.AgentConn, error),
) *chatexec.Executor {
	t.Helper()

	if buildModel == nil {
		buildModel = func(
			providerHint string,
			modelName string,
			keys chatprovider.ProviderAPIKeys,
			userAgent string,
			extraHeaders map[string]string,
		) (fantasy.LanguageModel, error) {
			return fakeLanguageModel{provider: providerHint, model: modelName}, nil
		}
	}
	if runLoop == nil {
		runLoop = func(context.Context, chatloop.RunOptions) error { return nil }
	}

	var getLocalConn func(context.Context) (workspacesdk.AgentConn, error)
	if len(getLocalConns) > 0 {
		getLocalConn = getLocalConns[0]
	}

	executor := chatexec.New(client, slog.Make(), getLocalConn)
	executor.SetBuildModel(buildModel)
	executor.SetRunLoop(runLoop)
	return executor
}

func defaultRuntimeContext() agentsdk.ChatRunnerRuntimeContextResponse {
	return agentsdk.ChatRunnerRuntimeContextResponse{
		ChatID:           uuid.New(),
		Provider:         "anthropic",
		Model:            "claude-sonnet-4-20250514",
		ProviderAPIKeys:  map[string]string{"anthropic": "test-key"},
		ProviderBaseURLs: map[string]string{"anthropic": "https://example.test"},
		LeaseEpoch:       42,
		ModelConfigID:    uuid.New(),
		ContextLimit:     128000,
		Messages: []agentsdk.ChatRunnerMessage{{
			Role: string(codersdk.ChatMessageRoleUser),
			Content: []codersdk.ChatMessagePart{
				codersdk.ChatMessageText("hello"),
			},
		}},
	}
}

func agentToolNames(tools []fantasy.AgentTool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Info().Name)
	}
	return names
}

func providerToolDefinition(t testing.TB, tool fantasy.Tool) agentsdk.ChatRunnerToolDefinition {
	t.Helper()

	defined, ok := tool.(fantasy.ProviderDefinedTool)
	require.True(t, ok)

	providerConfig, err := json.Marshal(struct {
		ID   string         `json:"id"`
		Args map[string]any `json:"args"`
	}{
		ID:   defined.ID,
		Args: defined.Args,
	})
	require.NoError(t, err)
	return agentsdk.ChatRunnerToolDefinition{
		Name:           defined.Name,
		ProviderConfig: providerConfig,
	}
}

func providerDefinedTool(t testing.TB, tool fantasy.Tool) fantasy.ProviderDefinedTool {
	t.Helper()
	defined, ok := tool.(fantasy.ProviderDefinedTool)
	require.True(t, ok)
	return defined
}

func requireProviderIntArg(t testing.TB, args map[string]any, key string, want int64) {
	t.Helper()
	value, ok := args[key]
	require.True(t, ok)
	switch typed := value.(type) {
	case float64:
		require.Equal(t, want, int64(typed))
	case json.Number:
		parsed, err := typed.Int64()
		require.NoError(t, err)
		require.Equal(t, want, parsed)
	default:
		require.Failf(t, "unexpected provider arg type", "%s=%T", key, value)
	}
}
