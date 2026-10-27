package chatd //nolint:testpackage // Exercises unexported re-derivation helpers.

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"slices"
	"strings"
	"testing"

	"charm.land/fantasy"
	fantasyopenai "charm.land/fantasy/providers/openai"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"cdr.dev/slog/v3/sloggers/slogtest"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chatadvisor"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/safedial"
)

func mustMarshalText(t *testing.T, parts ...string) pqtype.NullRawMessage {
	t.Helper()
	messageParts := make([]codersdk.ChatMessagePart, 0, len(parts))
	for _, p := range parts {
		messageParts = append(messageParts, codersdk.ChatMessageText(p))
	}
	content, err := chatprompt.MarshalParts(messageParts)
	require.NoError(t, err)
	return content
}

func textMessage(t *testing.T, id int64, role database.ChatMessageRole, parts ...string) database.ChatMessage {
	t.Helper()
	return database.ChatMessage{
		ID:             id,
		Role:           role,
		Content:        mustMarshalText(t, parts...),
		ContentVersion: chatprompt.CurrentContentVersion,
	}
}

func TestLatestAssistantText(t *testing.T) {
	t.Parallel()

	t.Run("ReturnsMostRecentAssistantMessage", func(t *testing.T) {
		t.Parallel()
		messages := []database.ChatMessage{
			textMessage(t, 1, database.ChatMessageRoleUser, "hi"),
			textMessage(t, 2, database.ChatMessageRoleAssistant, "first answer"),
			textMessage(t, 3, database.ChatMessageRoleTool, "tool result"),
			textMessage(t, 4, database.ChatMessageRoleAssistant, "  final answer  "),
		}
		require.Equal(t, "final answer", latestAssistantText(messages))
	})

	t.Run("ConcatenatesTextParts", func(t *testing.T) {
		t.Parallel()
		messages := []database.ChatMessage{
			textMessage(t, 1, database.ChatMessageRoleAssistant, "foo", "bar"),
		}
		require.Equal(t, "foobar", latestAssistantText(messages))
	})

	t.Run("NoAssistantMessage", func(t *testing.T) {
		t.Parallel()
		messages := []database.ChatMessage{
			textMessage(t, 1, database.ChatMessageRoleUser, "hi"),
			textMessage(t, 2, database.ChatMessageRoleTool, "tool result"),
		}
		require.Empty(t, latestAssistantText(messages))
	})

	t.Run("EmptyAssistantText", func(t *testing.T) {
		t.Parallel()
		messages := []database.ChatMessage{
			textMessage(t, 1, database.ChatMessageRoleAssistant, "   "),
		}
		require.Empty(t, latestAssistantText(messages))
	})

	t.Run("EmptyHistory", func(t *testing.T) {
		t.Parallel()
		require.Empty(t, latestAssistantText(nil))
	})
}

func TestBuildTurnEnvironmentClampsRequestedReasoningEffortToMax(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := chatdTestContext(t)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})
	provider := dbgen.AIProviderWithOptionalKey(t, db, database.AIProvider{
		Type: database.AIProviderTypeOpenai,
	}, "test-key")
	modelConfigRaw, err := json.Marshal(codersdk.ChatModelCallConfig{
		ProviderOptions: &codersdk.ChatModelProviderOptions{
			OpenAI: &codersdk.ChatModelOpenAIProviderOptions{
				User: new("turn-options-sentinel"),
			},
		},
		ReasoningEffort: &codersdk.ChatModelReasoningEffortConfig{
			Default: new(codersdk.ChatModelReasoningEffortLow),
			Max:     new(codersdk.ChatModelReasoningEffortMedium),
		},
	})
	require.NoError(t, err)
	modelConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		Model:          "gpt-4o-mini",
		Options:        modelConfigRaw,
		AIProviderID:   uuid.NullUUID{UUID: provider.ID, Valid: true},
		OrganizationID: org.ID,
	}, func(p *database.InsertChatModelConfigParams) {
		p.Enabled = true
	})

	created, err := chatstate.CreateChat(ctx, db, ps, chatstate.CreateChatInput{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: modelConfig.ID,
		Title:             "clamp reasoning effort",
		ClientType:        database.ChatClientTypeApi,
		InitialMessages: []chatstate.Message{
			{
				Role:          database.ChatMessageRoleUser,
				Content:       mustMarshalText(t, "hello"),
				Visibility:    database.ChatMessageVisibilityBoth,
				ModelConfigID: uuid.NullUUID{UUID: modelConfig.ID, Valid: true},
				ReasoningEffort: database.NullChatReasoningEffort{
					ChatReasoningEffort: database.ChatReasoningEffortHigh,
					Valid:               true,
				},
				CreatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
				ContentVersion: chatprompt.CurrentContentVersion,
			},
		},
	})
	require.NoError(t, err)

	server := newInternalTestServer(
		t,
		db,
		ps,
		chatprovider.ProviderAPIKeys{},
		withInternalTestServerTransportFactory(&aibridgeTestFactory{}),
	)
	prepared, err := server.buildTurnEnvironment(ctx, generationPrepareInput{
		Chat:            created.Chat,
		Messages:        created.InitialMessages,
		TurnExperiments: &turnExperimentDecisions{},
	})
	require.NoError(t, err)
	t.Cleanup(prepared.Close)

	providerOptions, ok := prepared.CallTemplate.ProviderOptions[fantasyopenai.Name].(*fantasyopenai.ResponsesProviderOptions)
	require.True(t, ok, "%T", prepared.CallTemplate.ProviderOptions[fantasyopenai.Name])
	require.NotNil(t, providerOptions.ReasoningEffort)
	require.Equal(t, fantasyopenai.ReasoningEffortMedium, *providerOptions.ReasoningEffort)
	require.NotNil(t, prepared.Compaction.Options.ToolDefinitions)
	require.Equal(t,
		chatloop.BuildToolDefinitions(prepared.Tools, prepared.ActiveTools, prepared.ProviderTools),
		prepared.Compaction.Options.ToolDefinitions,
	)

	require.NotNil(t, providerOptions.User)
	require.Equal(t, "turn-options-sentinel", *providerOptions.User)
	require.NotNil(t, prepared.CallTemplate.MaxOutputTokens)
	require.Equal(t, defaultChatMaxOutputTokens, *prepared.CallTemplate.MaxOutputTokens)

	require.NotNil(t, prepared.Compaction)
	summaryCall := prepared.Compaction.Options.SummaryCall
	require.Equal(t, prepared.CallTemplate.ProviderOptions, summaryCall.ProviderOptions)
	require.NotNil(t, summaryCall.ToolChoice)
	require.Equal(t, fantasy.ToolChoiceNone, *summaryCall.ToolChoice)
	require.NotNil(t, summaryCall.MaxOutputTokens)
	require.Equal(t, *prepared.CallTemplate.MaxOutputTokens, *summaryCall.MaxOutputTokens)
}

func TestBuildTurnEnvironmentSplitsPendingUserRowsFromCompactionPrompt(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := chatdTestContext(t)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user.ID, OrganizationID: org.ID})
	provider := dbgen.AIProviderWithOptionalKey(t, db, database.AIProvider{Type: database.AIProviderTypeOpenai}, "test-key")
	modelConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		Model:          "gpt-4o-mini",
		AIProviderID:   uuid.NullUUID{UUID: provider.ID, Valid: true},
		OrganizationID: org.ID,
	}, func(p *database.InsertChatModelConfigParams) {
		p.Enabled = true
	})
	message := func(role database.ChatMessageRole, text string) chatstate.Message {
		return chatstate.Message{
			Role:           role,
			Content:        mustMarshalText(t, text),
			Visibility:     database.ChatMessageVisibilityBoth,
			ModelConfigID:  uuid.NullUUID{UUID: modelConfig.ID, Valid: true},
			CreatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
			ContentVersion: chatprompt.CurrentContentVersion,
		}
	}
	created, err := chatstate.CreateChat(ctx, db, ps, chatstate.CreateChatInput{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: modelConfig.ID,
		Title:             "pending user rows",
		ClientType:        database.ChatClientTypeApi,
		InitialMessages: []chatstate.Message{
			message(database.ChatMessageRoleUser, "first"),
			message(database.ChatMessageRoleAssistant, "answered"),
			message(database.ChatMessageRoleUser, "unanswered"),
		},
	})
	require.NoError(t, err)

	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{},
		withInternalTestServerTransportFactory(&aibridgeTestFactory{}),
	)
	env, err := server.buildTurnEnvironment(ctx, generationPrepareInput{
		Chat:            created.Chat,
		Messages:        created.InitialMessages,
		TurnExperiments: &turnExperimentDecisions{},
	})
	require.NoError(t, err)
	t.Cleanup(env.Close)

	require.Len(t, env.Compaction.PendingUserRows, 1)
	require.Equal(t, created.InitialMessages[2].ID, env.Compaction.PendingUserRows[0].ID)
	// The unanswered tail is replayed verbatim after the compaction
	// boundary, so the summarizer input must stop before it.
	require.Equal(t, fantasy.MessageRoleUser, env.Prompt[len(env.Prompt)-1].Role)
	require.Equal(t, env.Prompt[:len(env.Prompt)-1], env.Compaction.Options.Messages)
}

func TestBuildTurnEnvironmentReplacesUnsupportedToolMedia(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		provider  database.AIProviderType
		model     string
		mediaType string
	}{
		// Anthropic is a native transport; Google is bridged through an
		// OpenAI-compatible transport that hides the vendor restriction.
		{name: "AnthropicAudio", provider: database.AIProviderTypeAnthropic, model: "claude-sonnet-4-5", mediaType: "audio/mpeg"},
		{name: "GoogleSVG", provider: database.AIProviderTypeGoogle, model: "gemini-2.5-flash", mediaType: "image/svg+xml"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			db, ps := dbtestutil.NewDB(t)
			ctx := chatdTestContext(t)
			user := dbgen.User(t, db, database.User{})
			org := dbgen.Organization(t, db, database.Organization{})
			dbgen.OrganizationMember(t, db, database.OrganizationMember{
				UserID:         user.ID,
				OrganizationID: org.ID,
			})
			provider := dbgen.AIProviderWithOptionalKey(t, db, database.AIProvider{
				Type: tc.provider,
			}, "test-key")
			modelConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
				Model:          tc.model,
				AIProviderID:   uuid.NullUUID{UUID: provider.ID, Valid: true},
				OrganizationID: org.ID,
			}, func(p *database.InsertChatModelConfigParams) {
				p.Enabled = true
			})

			const toolCallID = "call-media-1"
			mediaResult, err := json.Marshal(map[string]string{
				"data":      "AAAA",
				"mime_type": tc.mediaType,
				"text":      "Tool output",
			})
			require.NoError(t, err)
			assistantContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
				codersdk.ChatMessageToolCall(toolCallID, "media__render", json.RawMessage(`{}`)),
			})
			require.NoError(t, err)
			toolContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
				codersdk.ChatMessageToolResult(toolCallID, "media__render", mediaResult, false, true),
			})
			require.NoError(t, err)
			message := func(role database.ChatMessageRole, content pqtype.NullRawMessage) chatstate.Message {
				return chatstate.Message{
					Role:           role,
					Content:        content,
					Visibility:     database.ChatMessageVisibilityBoth,
					ModelConfigID:  uuid.NullUUID{UUID: modelConfig.ID, Valid: true},
					CreatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
					ContentVersion: chatprompt.CurrentContentVersion,
				}
			}
			created, err := chatstate.CreateChat(ctx, db, ps, chatstate.CreateChatInput{
				OrganizationID:    org.ID,
				OwnerID:           user.ID,
				LastModelConfigID: modelConfig.ID,
				Title:             "tool media after provider switch",
				ClientType:        database.ChatClientTypeApi,
				InitialMessages: []chatstate.Message{
					message(database.ChatMessageRoleUser, mustMarshalText(t, "render media")),
					message(database.ChatMessageRoleAssistant, assistantContent),
					message(database.ChatMessageRoleTool, toolContent),
					message(database.ChatMessageRoleUser, mustMarshalText(t, "what did it show?")),
				},
			})
			require.NoError(t, err)

			server := newInternalTestServer(
				t,
				db,
				ps,
				chatprovider.ProviderAPIKeys{},
				withInternalTestServerTransportFactory(&aibridgeTestFactory{}),
			)
			prepared, err := server.buildTurnEnvironment(ctx, generationPrepareInput{
				Chat:            created.Chat,
				Messages:        created.InitialMessages,
				TurnExperiments: &turnExperimentDecisions{},
			})
			require.NoError(t, err)
			t.Cleanup(prepared.Close)

			var sawToolResult bool
			for _, msg := range prepared.Prompt {
				for _, part := range msg.Content {
					result, ok := part.(fantasy.ToolResultPart)
					if !ok || result.ToolCallID != toolCallID {
						continue
					}
					sawToolResult = true
					text, ok := result.Output.(fantasy.ToolResultOutputContentText)
					require.True(t, ok, "expected text output, got %T", result.Output)
					require.Contains(t, text.Text, "Tool output")
					require.Contains(t, text.Text, "["+tc.mediaType+" content omitted: unsupported tool result media type]")
				}
			}
			require.True(t, sawToolResult, "tool result missing from prompt")
		})
	}
}

func TestBuildTurnEnvironmentComputerUseIgnoresChatTransportOverride(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := chatdTestContext(t)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})
	require.NoError(t, db.UpsertChatComputerUseProvider(ctx, string(codersdk.ChatComputerUseProviderOpenAI)))
	provider := dbgen.AIProviderWithOptionalKey(t, db, database.AIProvider{
		Type: database.AIProviderTypeOpenai,
	}, "test-key")
	forceCompletions := false
	modelConfigRaw, err := json.Marshal(codersdk.ChatModelCallConfig{
		OpenAIConfig: &codersdk.ChatModelOpenAIConfig{
			UseResponsesAPI: &forceCompletions,
		},
		ProviderOptions: &codersdk.ChatModelProviderOptions{
			OpenAI: &codersdk.ChatModelOpenAIProviderOptions{
				User: new("computer-use"),
			},
		},
	})
	require.NoError(t, err)
	modelConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		Model:          "gpt-4o-mini",
		Options:        modelConfigRaw,
		AIProviderID:   uuid.NullUUID{UUID: provider.ID, Valid: true},
		OrganizationID: org.ID,
	}, func(p *database.InsertChatModelConfigParams) {
		p.Enabled = true
	})

	const attachmentText = "text attachment body"
	file, err := db.InsertChatFile(ctx, database.InsertChatFileParams{
		OwnerID:        user.ID,
		OrganizationID: org.ID,
		Name:           "notes.txt",
		Mimetype:       "text/plain",
		Data:           []byte(attachmentText),
	})
	require.NoError(t, err)
	content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText("hello"),
		codersdk.ChatMessageFile(file.ID, "text/plain", "notes.txt"),
	})
	require.NoError(t, err)

	created, err := chatstate.CreateChat(ctx, db, ps, chatstate.CreateChatInput{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: modelConfig.ID,
		Title:             "computer use transport",
		ClientType:        database.ChatClientTypeApi,
		Mode:              database.NullChatMode{ChatMode: database.ChatModeComputerUse, Valid: true},
		InitialMessages: []chatstate.Message{
			{
				Role:           database.ChatMessageRoleUser,
				Content:        content,
				Visibility:     database.ChatMessageVisibilityBoth,
				ModelConfigID:  uuid.NullUUID{UUID: modelConfig.ID, Valid: true},
				CreatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
				ContentVersion: chatprompt.CurrentContentVersion,
			},
		},
	})
	require.NoError(t, err)

	server := newInternalTestServer(
		t,
		db,
		ps,
		chatprovider.ProviderAPIKeys{},
		withInternalTestServerTransportFactory(&aibridgeTestFactory{}),
	)
	prepared, err := server.buildTurnEnvironment(ctx, generationPrepareInput{
		Chat:            created.Chat,
		Messages:        created.InitialMessages,
		TurnExperiments: &turnExperimentDecisions{},
	})
	require.NoError(t, err)
	t.Cleanup(prepared.Close)

	// The computer-use model is Responses-selected by the SDK and its client
	// ignores the config's forced Chat Completions, so the options must be the
	// Responses type or the SDK discards them.
	_, ok := prepared.CallTemplate.ProviderOptions[fantasyopenai.Name].(*fantasyopenai.ResponsesProviderOptions)
	require.True(t, ok, "%T", prepared.CallTemplate.ProviderOptions[fantasyopenai.Name])

	// File classification must also key on the substituted model: the
	// Responses transport drops native text file parts, so the attachment
	// must be inlined as text rather than kept as a FilePart.
	var sawInlinedText bool
	for _, message := range prepared.Prompt {
		for _, part := range message.Content {
			if filePart, isFile := part.(fantasy.FilePart); isFile {
				t.Fatalf("text attachment survived as FilePart %q", filePart.Filename)
			}
			if textPart, isText := part.(fantasy.TextPart); isText &&
				strings.Contains(textPart.Text, attachmentText) {
				sawInlinedText = true
			}
		}
	}
	require.True(t, sawInlinedText, "attachment was not inlined as text")
}

func TestBuildTurnEnvironmentMemory(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		project            bool
		subagent           bool
		experimentsEnabled bool
		wantMemoryBlock    bool
		wantMemoryTools    bool
		intro              string
	}{
		{name: "Project", project: true, experimentsEnabled: true, wantMemoryBlock: true, wantMemoryTools: true, intro: `project "platform"`},
		{name: "NoProject", experimentsEnabled: true},
		{name: "ExperimentDisabled"},
		{name: "Subagent", experimentsEnabled: true, subagent: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			db, ps := dbtestutil.NewDB(t)
			ctx := chatdTestContext(t)
			user := dbgen.User(t, db, database.User{})
			org := dbgen.Organization(t, db, database.Organization{})
			dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user.ID, OrganizationID: org.ID})
			provider := dbgen.AIProviderWithOptionalKey(t, db, database.AIProvider{Type: database.AIProviderTypeOpenai}, "test-key")
			modelConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{Model: "gpt-4o-mini", AIProviderID: uuid.NullUUID{UUID: provider.ID, Valid: true}, OrganizationID: org.ID}, func(p *database.InsertChatModelConfigParams) { p.Enabled = true })

			projectID := uuid.NullUUID{}
			if tt.project {
				project := dbgen.ChatProject(t, db, database.ChatProject{OrganizationID: org.ID, OwnerID: user.ID, Name: "platform"})
				projectID = uuid.NullUUID{UUID: project.ID, Valid: true}
				dbgen.ChatProjectMemory(t, db, database.ChatProjectMemory{ProjectID: project.ID, OrganizationID: org.ID, CreatedBy: user.ID, Name: "release_notes", Description: "Durable release process", Body: "Run the release checklist."})
				_, err := db.UpsertChatProjectInstructions(ctx, database.UpsertChatProjectInstructionsParams{ProjectID: project.ID, UpdatedBy: user.ID, Instructions: "Ship on Tuesdays."})
				require.NoError(t, err)
			}

			parentChatID, rootChatID := uuid.NullUUID{}, uuid.NullUUID{}
			if tt.subagent {
				parent := dbgen.Chat(t, db, database.Chat{OrganizationID: org.ID, OwnerID: user.ID, LastModelConfigID: modelConfig.ID})
				parentChatID = uuid.NullUUID{UUID: parent.ID, Valid: true}
				rootChatID = parentChatID
			}
			created, err := chatstate.CreateChat(ctx, db, ps, chatstate.CreateChatInput{
				OrganizationID: org.ID, OwnerID: user.ID, ProjectID: projectID, ParentChatID: parentChatID, RootChatID: rootChatID,
				LastModelConfigID: modelConfig.ID, Title: "memory preparation", ClientType: database.ChatClientTypeApi,
				InitialMessages: []chatstate.Message{{Role: database.ChatMessageRoleUser, Content: mustMarshalText(t, "inspect the release process"), Visibility: database.ChatMessageVisibilityBoth, ModelConfigID: uuid.NullUUID{UUID: modelConfig.ID, Valid: true}, CreatedBy: uuid.NullUUID{UUID: user.ID, Valid: true}, ContentVersion: chatprompt.CurrentContentVersion}},
			})
			require.NoError(t, err)

			serverOpts := []internalTestServerOpt{withInternalTestServerTransportFactory(&aibridgeTestFactory{})}
			if !tt.experimentsEnabled {
				serverOpts = append(serverOpts, withInternalTestServerExperiments([]codersdk.Experiment{}))
			}
			server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{}, serverOpts...)
			prepared, err := server.buildTurnEnvironment(ctx, generationPrepareInput{Chat: created.Chat, Messages: created.InitialMessages})
			require.NoError(t, err)
			t.Cleanup(prepared.Close)

			var systemPrompt strings.Builder
			for _, message := range prepared.Prompt {
				if message.Role != fantasy.MessageRoleSystem {
					continue
				}
				for _, part := range message.Content {
					if text, ok := part.(fantasy.TextPart); ok {
						systemPrompt.WriteString(text.Text)
						systemPrompt.WriteString("\n")
					}
				}
			}
			gotSystemPrompt := systemPrompt.String()
			require.Equal(t, tt.wantMemoryBlock, strings.Contains(gotSystemPrompt, "<memory>"))
			require.Equal(t, tt.project, strings.Contains(gotSystemPrompt, "Ship on Tuesdays."))
			// The index travels as conversation messages, never in the
			// system prompt or tools, so the cached prefix survives writes.
			require.NotContains(t, gotSystemPrompt, "- release_notes: Durable release process")
			if tt.wantMemoryBlock {
				require.Contains(t, gotSystemPrompt, tt.intro)
			}

			toolDescriptions := make(map[string]string, len(prepared.Tools))
			for _, tool := range prepared.Tools {
				toolDescriptions[tool.Info().Name] = tool.Info().Description
			}
			for _, name := range []string{chattool.ReadMemoryToolName, chattool.SaveMemoryToolName, chattool.DeleteMemoryToolName, chattool.ConsolidateMemoryToolName} {
				description, ok := toolDescriptions[name]
				require.Equal(t, tt.wantMemoryTools, ok, name)
				require.NotContains(t, description, "release_notes", name)
			}
		})
	}
}

type workspaceLookupErrorStore struct {
	database.Store

	workspaceID uuid.UUID
	err         error
}

func (s *workspaceLookupErrorStore) GetWorkspaceByID(ctx context.Context, id uuid.UUID) (database.Workspace, error) {
	if id == s.workspaceID {
		return database.Workspace{}, s.err
	}
	return s.Store.GetWorkspaceByID(ctx, id)
}

func TestBuildTurnEnvironmentWorkspaceAgentLookup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		lookupErr error
		wantErr   bool
	}{
		{name: "DatabaseErrorFailsTurn", lookupErr: sql.ErrConnDone, wantErr: true},
		{name: "MissingWorkspaceKeepsTurn", lookupErr: sql.ErrNoRows},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			db, ps := dbtestutil.NewDB(t)
			ctx := chatdTestContext(t)
			user := dbgen.User(t, db, database.User{})
			org := dbgen.Organization(t, db, database.Organization{})
			dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user.ID, OrganizationID: org.ID})
			provider := dbgen.AIProviderWithOptionalKey(t, db, database.AIProvider{Type: database.AIProviderTypeOpenai}, "test-key")
			modelConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{Model: "gpt-4o-mini", AIProviderID: uuid.NullUUID{UUID: provider.ID, Valid: true}, OrganizationID: org.ID}, func(p *database.InsertChatModelConfigParams) { p.Enabled = true })
			tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{OrganizationID: org.ID, CreatedBy: user.ID})
			tmpl := dbgen.Template(t, db, database.Template{OrganizationID: org.ID, ActiveVersionID: tv.ID, CreatedBy: user.ID})
			ws := dbgen.Workspace(t, db, database.WorkspaceTable{OwnerID: user.ID, OrganizationID: org.ID, TemplateID: tmpl.ID})

			created, err := chatstate.CreateChat(ctx, db, ps, chatstate.CreateChatInput{
				OrganizationID: org.ID, OwnerID: user.ID, WorkspaceID: uuid.NullUUID{UUID: ws.ID, Valid: true},
				LastModelConfigID: modelConfig.ID, Title: "workspace agent lookup", ClientType: database.ChatClientTypeApi,
				InitialMessages: []chatstate.Message{{Role: database.ChatMessageRoleUser, Content: mustMarshalText(t, "hello"), Visibility: database.ChatMessageVisibilityBoth, ModelConfigID: uuid.NullUUID{UUID: modelConfig.ID, Valid: true}, CreatedBy: uuid.NullUUID{UUID: user.ID, Valid: true}, ContentVersion: chatprompt.CurrentContentVersion}},
			})
			require.NoError(t, err)

			store := &workspaceLookupErrorStore{Store: db, workspaceID: ws.ID, err: tt.lookupErr}
			server := newInternalTestServer(t, store, ps, chatprovider.ProviderAPIKeys{}, withInternalTestServerTransportFactory(&aibridgeTestFactory{}))
			prepared, err := server.buildTurnEnvironment(ctx, generationPrepareInput{Chat: created.Chat, Messages: created.InitialMessages})
			if tt.wantErr {
				require.ErrorIs(t, err, tt.lookupErr)
				require.ErrorContains(t, err, "get workspace agent")
				return
			}
			require.NoError(t, err)
			t.Cleanup(prepared.Close)
		})
	}
}

func TestBuildTurnEnvironmentSubagentUsesOwnerSyntheticAPIKey(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := chatdTestContext(t)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})
	provider := dbgen.AIProviderWithOptionalKey(t, db, database.AIProvider{
		Type: database.AIProviderTypeOpenai,
	}, "test-key")
	modelConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		Model:          "gpt-4o-mini",
		AIProviderID:   uuid.NullUUID{UUID: provider.ID, Valid: true},
		OrganizationID: org.ID,
	}, func(p *database.InsertChatModelConfigParams) {
		p.Enabled = true
	})
	parent := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: modelConfig.ID,
	})
	created, err := chatstate.CreateChat(ctx, db, ps, chatstate.CreateChatInput{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		ParentChatID:      uuid.NullUUID{UUID: parent.ID, Valid: true},
		RootChatID:        uuid.NullUUID{UUID: parent.ID, Valid: true},
		LastModelConfigID: modelConfig.ID,
		Title:             "subagent attribution",
		ClientType:        database.ChatClientTypeApi,
		InitialMessages: []chatstate.Message{
			{
				Role:           database.ChatMessageRoleUser,
				Content:        mustMarshalText(t, "inspect the workspace"),
				Visibility:     database.ChatMessageVisibilityBoth,
				ModelConfigID:  uuid.NullUUID{UUID: modelConfig.ID, Valid: true},
				CreatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
				ContentVersion: chatprompt.CurrentContentVersion,
			},
		},
	})
	require.NoError(t, err)

	server := newInternalTestServer(
		t,
		db,
		ps,
		chatprovider.ProviderAPIKeys{},
		withInternalTestServerTransportFactory(&aibridgeTestFactory{}),
	)
	prepared, err := server.buildTurnEnvironment(ctx, generationPrepareInput{
		Chat:            created.Chat,
		Messages:        created.InitialMessages,
		TurnExperiments: &turnExperimentDecisions{},
	})
	require.NoError(t, err)
	t.Cleanup(prepared.Close)

	gatewayKey, err := db.GetChatGatewayAPIKey(ctx, database.GetChatGatewayAPIKeyParams{
		UserID:    user.ID,
		TokenName: GatewayTokenName(user.ID),
	})
	require.NoError(t, err)
	require.Equal(t, gatewayKey.ID, prepared.ModelBuildOptions.ActiveAPIKeyID)
}

// TestBuildTurnEnvironmentToolPolicy checks the tools and system overlays each
// chat shape sends to the model. Every chat selects the same MCP servers and
// binds the same workspace, so only the turn policy differs between cases.
func TestBuildTurnEnvironmentToolPolicy(t *testing.T) {
	t.Parallel()

	newMCPServerURL := func(name string, tools ...string) string {
		srv := mcp.NewServer(&mcp.Implementation{Name: name, Version: "1.0.0"}, nil)
		for _, tool := range tools {
			srv.AddTool(&mcp.Tool{Name: tool, InputSchema: map[string]any{"type": "object"}},
				func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					return &mcp.CallToolResult{}, nil
				})
		}
		ts := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
			return srv
		}, &mcp.StreamableHTTPOptions{Stateless: true}))
		t.Cleanup(ts.Close)
		return ts.URL
	}
	echoURL := newMCPServerURL("echo", "echo")
	filteredURL := newMCPServerURL("filtered", "visible", "hidden")

	db, ps := dbtestutil.NewDB(t)
	ctx := chatdTestContext(t)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user.ID, OrganizationID: org.ID})
	provider := dbgen.AIProviderWithOptionalKey(t, db, database.AIProvider{Type: database.AIProviderTypeOpenai}, "test-key")
	modelOptions, err := json.Marshal(codersdk.ChatModelCallConfig{
		ProviderOptions: &codersdk.ChatModelProviderOptions{
			OpenAI: &codersdk.ChatModelOpenAIProviderOptions{WebSearchEnabled: new(true)},
		},
	})
	require.NoError(t, err)
	modelConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		Model:          "gpt-4o-mini",
		Options:        modelOptions,
		AIProviderID:   uuid.NullUUID{UUID: provider.ID, Valid: true},
		OrganizationID: org.ID,
	}, func(p *database.InsertChatModelConfigParams) {
		p.Enabled = true
	})
	mcpConfig := func(seed database.MCPServerConfig) uuid.UUID {
		seed.OrganizationID = org.ID
		seed.CreatedBy = uuid.NullUUID{UUID: user.ID, Valid: true}
		return dbgen.MCPServerConfig(t, db, seed).ID
	}
	selectedMCP := []uuid.UUID{
		mcpConfig(database.MCPServerConfig{Slug: "approved", Url: echoURL, AllowInPlanMode: true}),
		mcpConfig(database.MCPServerConfig{Slug: "blocked", Url: echoURL}),
		mcpConfig(database.MCPServerConfig{Slug: "filtered", Url: filteredURL, AllowInPlanMode: true, ToolAllowList: []string{"visible"}}),
	}
	mcpConfig(database.MCPServerConfig{Slug: "forced", Url: echoURL, Availability: "force_on"})
	unselectedMCP := mcpConfig(database.MCPServerConfig{Slug: "unselected", Url: echoURL})

	workspace, _, agent := seedWorkspaceBinding(t, db, user.ID)
	workspaceMCPBody, err := protojson.Marshal(&agentproto.MCPServerBody{
		ServerName: "workspace",
		Tools:      []*agentproto.MCPTool{{Name: "echo"}},
	})
	require.NoError(t, err)
	seedAgentContext(ctx, t, db, agent.ID, "workspace", []byte("workspace"),
		database.WorkspaceAgentContextBodyKindMcpServer, workspaceMCPBody)
	parent := dbgen.Chat(t, db, database.Chat{OrganizationID: org.ID, OwnerID: user.ID, LastModelConfigID: modelConfig.ID})
	// Children load inline MCP servers from the root chat; this one does not allow sub-agents.
	dbgen.ChatMCPServer(t, db, database.ChatMCPServer{ChatID: parent.ID, Slug: "inline", Url: echoURL})
	// The stored config says enabled, so RootAdvisorOff shows that the experiment alone gates the advisor.
	advisorConfig, err := json.Marshal(codersdk.AdvisorConfig{Enabled: true, MaxUsesPerRun: 3, MaxOutputTokens: 1024})
	require.NoError(t, err)
	require.NoError(t, db.UpsertChatAdvisorConfig(ctx, string(advisorConfig)))

	newServer := func(experiments ...codersdk.Experiment) *Server {
		server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{},
			withInternalTestServerTransportFactory(&aibridgeTestFactory{}),
			withInternalTestServerExperiments(append(codersdk.Experiments{codersdk.ExperimentChatInlineMCPServers}, experiments...)),
		)
		server.mcpHTTPClient = mcpclient.NewHTTPClient(nil, safedial.WithAllowedPrefixes(netip.MustParsePrefix("127.0.0.0/8")))
		return server
	}
	advisorServer := newServer(codersdk.ExperimentChatAdvisor)
	advisorOffServer := newServer()

	explore := database.NullChatMode{ChatMode: database.ChatModeExplore, Valid: true}
	plan := database.NullChatPlanMode{ChatPlanMode: database.ChatPlanModePlan, Valid: true}
	const (
		exploreOverlay   = "You are in Explore Mode as a delegated sub-agent."
		planRootOverlay  = "You are in Plan Mode."
		planChildOverlay = "You are in Plan Mode as a delegated sub-agent."
		proposePlan      = "When the plan is ready, call propose_plan"
		advisorGuidance  = chatadvisor.ParentGuidanceBlock
	)
	rootOnlyTools := []string{
		"list_templates", "read_template", "create_workspace", "start_workspace", "stop_workspace",
		"spawn_agent", "wait_agent", "message_agent", "interrupt_agent", "list_agents",
	}
	tests := []struct {
		name     string
		mode     database.NullChatMode
		planMode database.NullChatPlanMode
		child    bool
		// advisorOff prepares the turn on a server without the advisor experiment.
		advisorOff bool
		// spawn delegates to a general sub-agent, which must keep the
		// parent's plan mode.
		spawn bool
		// replaceMCP sends a follow-up message that asks to replace the
		// MCP selection before the next turn is prepared.
		replaceMCP bool
		// prompt lists the instructions the system prompt must contain;
		// the others in the set must be absent.
		prompt   []string
		want     []string
		dontWant []string
	}{
		{
			name:   "Root",
			prompt: []string{advisorGuidance},
			want: append([]string{
				"write_file", "edit_files", "approved__echo", "blocked__echo", "filtered__visible",
				"forced__echo", "inline__echo", "workspace__echo", "web_search", chatadvisor.ToolName,
			}, rootOnlyTools...),
			dontWant: []string{"filtered__hidden", "unselected__echo", "ask_user_question", "propose_plan"},
		},
		{
			name:       "RootAdvisorOff",
			advisorOff: true,
			want:       []string{"spawn_agent"},
			dontWant:   []string{chatadvisor.ToolName},
		},
		{
			name:     "GeneralChild",
			child:    true,
			want:     []string{"read_file", "write_file", "edit_files", "execute"},
			dontWant: append([]string{"ask_user_question", chatadvisor.ToolName}, rootOnlyTools...),
		},
		{
			name:     "PlanRoot",
			planMode: plan,
			spawn:    true,
			prompt:   []string{planRootOverlay, proposePlan},
			want: []string{
				"write_file", "edit_files", "execute", "process_output", "ask_user_question", "propose_plan",
				"spawn_agent", "approved__echo", "filtered__visible", "inline__echo",
			},
			dontWant: []string{
				"blocked__echo", "filtered__hidden", "forced__echo", "workspace__echo", "web_search", chatadvisor.ToolName,
			},
		},
		{
			name:     "PlanChild",
			planMode: plan,
			child:    true,
			prompt:   []string{planChildOverlay},
			want:     []string{"read_file", "execute", "process_output"},
			dontWant: []string{
				"write_file", "edit_files", "ask_user_question", "approved__echo", "inline__echo",
				"workspace__echo", "web_search", chatadvisor.ToolName,
			},
		},
		{
			name:   "ExploreRoot",
			mode:   explore,
			prompt: []string{exploreOverlay},
			want:   []string{"read_file", "execute", "process_output"},
			dontWant: []string{
				"write_file", "edit_files", "approved__echo", "blocked__echo", "forced__echo", "inline__echo",
				"workspace__echo", "web_search", chatadvisor.ToolName,
			},
		},
		{
			name:   "ExploreChild",
			mode:   explore,
			child:  true,
			prompt: []string{exploreOverlay},
			want: []string{
				"read_file", "execute", "process_output", "approved__echo", "blocked__echo", "filtered__visible",
				"web_search",
			},
			dontWant: []string{
				"write_file", "edit_files", "spawn_agent", "wait_agent", "unselected__echo", "forced__echo",
				"inline__echo", "workspace__echo", chatadvisor.ToolName,
			},
		},
		{
			name:       "ExploreChildAfterMCPReplacement",
			mode:       explore,
			child:      true,
			replaceMCP: true,
			prompt:     []string{exploreOverlay},
			want:       []string{"approved__echo", "blocked__echo", "filtered__visible"},
			dontWant:   []string{"unselected__echo"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := chatdTestContext(t)
			input := chatstate.CreateChatInput{
				OrganizationID:    org.ID,
				OwnerID:           user.ID,
				WorkspaceID:       uuid.NullUUID{UUID: workspace.ID, Valid: true},
				AgentID:           uuid.NullUUID{UUID: agent.ID, Valid: true},
				LastModelConfigID: modelConfig.ID,
				Title:             tt.name,
				Mode:              tt.mode,
				PlanMode:          tt.planMode,
				MCPServerIDs:      selectedMCP,
				ClientType:        database.ChatClientTypeApi,
				InitialMessages: []chatstate.Message{{
					Role:           database.ChatMessageRoleUser,
					Content:        mustMarshalText(t, "list your tools"),
					Visibility:     database.ChatMessageVisibilityBoth,
					ModelConfigID:  uuid.NullUUID{UUID: modelConfig.ID, Valid: true},
					CreatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
					ContentVersion: chatprompt.CurrentContentVersion,
				}},
			}
			if tt.child {
				input.ParentChatID = uuid.NullUUID{UUID: parent.ID, Valid: true}
				input.RootChatID = input.ParentChatID
			}
			if tt.replaceMCP {
				// A waiting chat appends the follow-up message to the
				// history instead of queueing it.
				input.InitialStatus = database.ChatStatusWaiting
			}
			created, err := chatstate.CreateChat(ctx, db, ps, input)
			require.NoError(t, err)
			if !tt.child {
				dbgen.ChatMCPServer(t, db, database.ChatMCPServer{ChatID: created.Chat.ID, Slug: "inline", Url: echoURL})
			}
			server := advisorServer
			if tt.advisorOff {
				server = advisorOffServer
			}

			chat, messages := created.Chat, created.InitialMessages
			if tt.planMode.Valid {
				_, err = server.SendMessage(ctx, SendMessageOptions{
					ChatID:       chat.ID,
					Content:      []codersdk.ChatMessagePart{codersdk.ChatMessageText("continue")},
					BusyBehavior: SendMessageBusyBehaviorQueue,
				})
				require.NoError(t, err)
				chat, err = db.GetChatByID(ctx, chat.ID)
				require.NoError(t, err)
				require.Equal(t, tt.planMode, chat.PlanMode)
			}
			if tt.replaceMCP {
				_, err = server.SendMessage(ctx, SendMessageOptions{
					ChatID:       chat.ID,
					CreatedBy:    user.ID,
					Content:      []codersdk.ChatMessagePart{codersdk.ChatMessageText("use the other server")},
					MCPServerIDs: &[]uuid.UUID{unselectedMCP},
				})
				require.NoError(t, err)
				chat, err = db.GetChatByID(ctx, chat.ID)
				require.NoError(t, err)
				require.ElementsMatch(t, selectedMCP, chat.MCPServerIDs)
				messages, err = db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: chat.ID})
				require.NoError(t, err)
				require.Len(t, messages, len(created.InitialMessages)+1)
			}

			env, err := server.buildTurnEnvironment(ctx, generationPrepareInput{
				Chat:            chat,
				Messages:        messages,
				TurnExperiments: &turnExperimentDecisions{},
			})
			require.NoError(t, err)
			t.Cleanup(env.Close)

			var offered []string
			for _, definition := range chatloop.BuildToolDefinitions(env.Tools, env.ActiveTools, env.ProviderTools) {
				offered = append(offered, definition.GetName())
			}
			loaded := make(map[string]bool, len(env.Tools))
			for _, tool := range env.Tools {
				loaded[tool.Info().Name] = true
			}
			require.Subset(t, offered, tt.want)
			for _, name := range tt.dontWant {
				require.NotContains(t, offered, name)
				// Excluded MCP tools must be absent from the toolset, not only from the offered definitions.
				if strings.Contains(name, "__") {
					require.False(t, loaded[name], name)
				}
			}
			// chatloop offers every registered tool when the active list is
			// empty, so an Explore child must not register a tool it hides.
			if tt.child && tt.mode == explore {
				var hidden []string
				for _, tool := range env.Tools {
					if name := tool.Info().Name; !slices.Contains(offered, name) {
						hidden = append(hidden, name)
					}
				}
				require.Empty(t, hidden)
			}
			if slices.Contains(tt.want, chatadvisor.ToolName) {
				require.Equal(t, map[string]bool{chatadvisor.ToolName: true}, env.ExclusiveToolNames)
			} else {
				require.Empty(t, env.ExclusiveToolNames)
			}

			var system strings.Builder
			for _, message := range env.Prompt {
				for _, part := range message.Content {
					if text, ok := part.(fantasy.TextPart); ok && message.Role == fantasy.MessageRoleSystem {
						system.WriteString(text.Text)
					}
				}
			}
			for _, instruction := range []string{exploreOverlay, planRootOverlay, planChildOverlay, proposePlan, advisorGuidance} {
				require.Equal(t, slices.Contains(tt.prompt, instruction), strings.Contains(system.String(), instruction), instruction)
			}

			if tt.spawn {
				tool := findToolByName(env.Tools, spawnAgentToolName)
				require.NotNil(t, tool)
				response, err := tool.Run(ctx, fantasy.ToolCall{
					ID:    uuid.NewString(),
					Name:  spawnAgentToolName,
					Input: `{"type":"general","prompt":"inspect the codebase","title":"sub"}`,
				})
				require.NoError(t, err)
				childID := requireSpawnAgentChildChatID(t, response)
				child, err := db.GetChatByID(ctx, childID)
				require.NoError(t, err)
				require.Equal(t, chat.PlanMode, child.PlanMode)
			}
		})
	}
}

func TestCreateExploreChatPreservesMCPSelection(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)
	config := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
		OrganizationID: org.ID,
		CreatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
	})
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	chat, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "root-explore",
		ModelConfigID:      model.ID,
		ChatMode:           database.NullChatMode{ChatMode: database.ChatModeExplore, Valid: true},
		MCPServerIDs:       []uuid.UUID{config.ID},
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("inspect the codebase")},
	})
	require.NoError(t, err)

	storedChat, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.ElementsMatch(t, []uuid.UUID{config.ID}, storedChat.MCPServerIDs)
}

// TestDeriveFinalTurnRunResult exercises the re-derivation path that replaces
// the old in-memory generationSideEffects stash. The server here never ran
// buildTurnEnvironment, so a passing test proves the finish-turn inputs are
// rebuilt purely from persisted state.
func TestDeriveFinalTurnRunResult(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	// setup seeds an OpenAI provider with one enabled config per model. The
	// first model is the chat's default model; the rest are extra enabled
	// configs, returned by model name.
	setup := func(t *testing.T, chatModel string, extraModels ...string) (*Server, database.Chat, map[string]database.ChatModelConfig) {
		t.Helper()
		db, ps := dbtestutil.NewDB(t)
		ctx := chatdTestContext(t)

		user := dbgen.User(t, db, database.User{})
		org := dbgen.Organization(t, db, database.Organization{})
		dbgen.OrganizationMember(t, db, database.OrganizationMember{
			UserID:         user.ID,
			OrganizationID: org.ID,
		})
		dbgen.ChatProvider(t, db, database.ChatProvider{
			Provider:    "openai",
			DisplayName: "OpenAI",
			APIKey:      "test-key",
			Enabled:     true,
			CreatedBy:   uuid.NullUUID{UUID: user.ID, Valid: true},
		})
		insertModel := func(model string, isDefault bool) database.ChatModelConfig {
			return dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
				Model:          model,
				DisplayName:    model,
				Options:        json.RawMessage(`{"openai_config":{"use_responses_api":false}}`),
				OrganizationID: org.ID,
			}, func(p *database.InsertChatModelConfigParams) {
				p.Enabled = true
				p.IsDefault = isDefault
			})
		}
		modelCfg := insertModel(chatModel, true)
		extra := make(map[string]database.ChatModelConfig, len(extraModels))
		for _, model := range extraModels {
			extra[model] = insertModel(model, false)
		}

		created, err := chatstate.CreateChat(ctx, db, ps, chatstate.CreateChatInput{
			OrganizationID:    org.ID,
			OwnerID:           user.ID,
			LastModelConfigID: modelCfg.ID,
			Title:             "derive-chat",
			ClientType:        database.ChatClientTypeUi,
			InitialStatus:     database.ChatStatusRunning,
			InitialMessages: []chatstate.Message{
				{
					Role:           database.ChatMessageRoleUser,
					Content:        mustMarshalText(t, "what is the answer?"),
					Visibility:     database.ChatMessageVisibilityBoth,
					ContentVersion: chatprompt.CurrentContentVersion,
					CreatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
					ModelConfigID:  uuid.NullUUID{UUID: modelCfg.ID, Valid: true},
				},
			},
		})
		require.NoError(t, err)

		server := newInternalTestServer(
			t, db, ps, chatprovider.ProviderAPIKeys{},
			withInternalTestServerTransportFactory(&aibridgeTestFactory{}),
		)
		return server, created.Chat, extra
	}

	commitAssistant := func(t *testing.T, server *Server, chat database.Chat, text string) {
		t.Helper()
		ctx := chatdTestContext(t)
		machine := chatstate.NewChatMachine(server.db, server.pubsub, chat.ID)
		require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
			_, err := tx.CommitStep(chatstate.CommitStepInput{
				Messages: []chatstate.Message{
					{
						Role:           database.ChatMessageRoleAssistant,
						Content:        mustMarshalText(t, text),
						Visibility:     database.ChatMessageVisibilityBoth,
						ContentVersion: chatprompt.CurrentContentVersion,
						ModelConfigID:  uuid.NullUUID{UUID: chat.LastModelConfigID, Valid: true},
					},
				},
			})
			return err
		}))
	}

	t.Run("WaitingDerivesFromHistory", func(t *testing.T) {
		t.Parallel()
		server, chat, _ := setup(t, "gpt-4o-mini")
		ctx := chatdTestContext(t)
		commitAssistant(t, server, chat, "the answer is 42")

		rows, err := server.db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
		require.NoError(t, err)
		require.NotEmpty(t, rows)
		var lastUserID int64
		for _, row := range rows {
			if row.Role == database.ChatMessageRoleUser {
				lastUserID = row.ID
			}
		}
		tipID := rows[len(rows)-1].ID

		chat.Status = database.ChatStatusWaiting
		result := server.deriveFinalTurnRunResult(ctx, chat, logger)

		require.Equal(t, "the answer is 42", result.FinalAssistantText)
		require.Equal(t, lastUserID, result.TriggerMessageID)
		require.Equal(t, tipID, result.HistoryTipMessageID)
		require.NotNil(t, result.StatusLabelCall)
		require.True(t, result.StatusLabelCall.model.Valid())
		require.Equal(t, "openai", result.StatusLabelCall.resolvedProvider)
		require.Equal(t, "gpt-4o-mini", result.StatusLabelCall.resolvedModel)
		require.JSONEq(t, `{"openai_config":{"use_responses_api":false}}`, string(result.StatusLabelCall.dbConfig.Options))
	})

	// The status label is a structured side call, so it must prefer the
	// title generation model over a chat model that may reject forced tools.
	t.Run("WaitingPrefersSmallModelOverChatModel", func(t *testing.T) {
		t.Parallel()
		server, chat, extra := setup(t, "gpt-4.1", "gpt-4o-mini")
		ctx := chatdTestContext(t)
		commitAssistant(t, server, chat, "the answer is 42")

		chat.Status = database.ChatStatusWaiting
		result := server.deriveFinalTurnRunResult(ctx, chat, logger)

		require.NotNil(t, result.StatusLabelCall)
		require.Equal(t, extra["gpt-4o-mini"].ID, result.StatusLabelCall.dbConfig.ID)
		require.Equal(t, "gpt-4o-mini", result.StatusLabelCall.resolvedModel)
	})

	t.Run("WaitingPrefersTitleGenerationOverride", func(t *testing.T) {
		t.Parallel()
		server, chat, extra := setup(t, "gpt-4.1", "gpt-4o-mini", "gpt-4.1-nano")
		ctx := chatdTestContext(t)
		commitAssistant(t, server, chat, "the answer is 42")
		upsertInternalChatOrganizationModelOverride(
			t, server.db, chat.OrganizationID,
			codersdk.ChatModelOverrideContextTitleGeneration,
			extra["gpt-4.1-nano"].ID.String(),
		)

		chat.Status = database.ChatStatusWaiting
		result := server.deriveFinalTurnRunResult(ctx, chat, logger)

		require.NotNil(t, result.StatusLabelCall)
		require.Equal(t, extra["gpt-4.1-nano"].ID, result.StatusLabelCall.dbConfig.ID)
		require.Equal(t, "gpt-4.1-nano", result.StatusLabelCall.resolvedModel)
	})

	t.Run("NonWaitingReturnsEmpty", func(t *testing.T) {
		t.Parallel()
		server, chat, _ := setup(t, "gpt-4o-mini")
		ctx := chatdTestContext(t)
		commitAssistant(t, server, chat, "the answer is 42")

		chat.Status = database.ChatStatusError
		result := server.deriveFinalTurnRunResult(ctx, chat, logger)
		require.Equal(t, runChatResult{}, result)
	})

	t.Run("WaitingWithoutAssistantReturnsEmpty", func(t *testing.T) {
		t.Parallel()
		server, chat, _ := setup(t, "gpt-4o-mini")
		ctx := chatdTestContext(t)

		// No assistant message was committed, so there is nothing to label.
		chat.Status = database.ChatStatusWaiting
		result := server.deriveFinalTurnRunResult(ctx, chat, logger)
		require.Equal(t, runChatResult{}, result)
	})

	t.Run("ModelResolveErrorKeepsTextAndIDs", func(t *testing.T) {
		t.Parallel()
		db, ps := dbtestutil.NewDB(t)
		ctx := chatdTestContext(t)

		user := dbgen.User(t, db, database.User{})
		org := dbgen.Organization(t, db, database.Organization{})
		dbgen.OrganizationMember(t, db, database.OrganizationMember{
			UserID:         user.ID,
			OrganizationID: org.ID,
		})
		provider := insertInternalAIProvider(t, db, database.AIProviderTypeOpenai, "provider-api-key", false)
		modelCfg := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
			Model:          "gpt-4o-mini",
			DisplayName:    "gpt-4o-mini",
			AIProviderID:   uuid.NullUUID{UUID: provider.ID, Valid: true},
			OrganizationID: org.ID,
		})

		created, err := chatstate.CreateChat(ctx, db, ps, chatstate.CreateChatInput{
			OrganizationID:    org.ID,
			OwnerID:           user.ID,
			LastModelConfigID: modelCfg.ID,
			Title:             "derive-chat-error",
			ClientType:        database.ChatClientTypeUi,
			InitialStatus:     database.ChatStatusRunning,
			InitialMessages: []chatstate.Message{
				{
					Role:           database.ChatMessageRoleUser,
					Content:        mustMarshalText(t, "what is the answer?"),
					Visibility:     database.ChatMessageVisibilityBoth,
					ContentVersion: chatprompt.CurrentContentVersion,
					CreatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
					ModelConfigID:  uuid.NullUUID{UUID: modelCfg.ID, Valid: true},
				},
			},
		})
		require.NoError(t, err)
		chat := created.Chat

		server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})
		commitAssistant(t, server, chat, "the answer is 42")

		chat.Status = database.ChatStatusWaiting
		result := server.deriveFinalTurnRunResult(ctx, chat, logger)

		require.Equal(t, "the answer is 42", result.FinalAssistantText)
		require.NotZero(t, result.TriggerMessageID)
		require.NotZero(t, result.HistoryTipMessageID)
		require.Nil(t, result.StatusLabelCall)
	})
}

// TestLatestPromptUsage verifies the compaction path reads the last
// persisted assistant usage, not a cumulative sum across steps.
// This is the chatd side of AIGOV-585: the issue blamed chatd for
// summing usage across steps, but latestPromptUsage returns the
// single most-recent non-zero usage. The real bug was in the
// aibridge streaming interceptor (fixed in ad100452d4).
func TestLatestPromptUsage(t *testing.T) {
	t.Parallel()

	t.Run("returns last assistant step not a sum", func(t *testing.T) {
		t.Parallel()
		// Three-step agentic turn: each step re-sends the whole
		// conversation, so InputTokens are 5000, 5200, 5400.
		// A sum would be 15600; the correct answer is 5400.
		messages := []database.ChatMessage{
			dbMessage(t, 1, database.ChatMessageRoleUser, false, codersdk.ChatMessageText("hi")),
			withUsage(dbMessage(t, 2, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageText("step1")), 5000, 0),
			dbMessage(t, 3, database.ChatMessageRoleTool, false, codersdk.ChatMessageText("result")),
			withUsage(dbMessage(t, 4, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageText("step2")), 5200, 0),
			dbMessage(t, 5, database.ChatMessageRoleTool, false, codersdk.ChatMessageText("result")),
			withUsage(dbMessage(t, 6, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageText("step3")), 5400, 0),
		}
		usage := latestPromptUsage(messages)
		assert.Equal(t, int64(5400), usage.InputTokens,
			"must return the last step's usage, not the sum across steps")
	})

	t.Run("returns zero when no messages have usage", func(t *testing.T) {
		t.Parallel()
		messages := []database.ChatMessage{
			dbMessage(t, 1, database.ChatMessageRoleUser, false, codersdk.ChatMessageText("hi")),
			dbMessage(t, 2, database.ChatMessageRoleAssistant, false, codersdk.ChatMessageText("hi")),
		}
		assert.Equal(t, fantasy.Usage{}, latestPromptUsage(messages))
	})

	t.Run("returns zero for empty message list", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, fantasy.Usage{}, latestPromptUsage(nil))
	})
}

// TestShouldCompactPromptUsage verifies the compaction threshold decision
// is correct for both the inflated values the aibridge bug produced and
// accurate per-step values.
func TestShouldCompactPromptUsage(t *testing.T) {
	t.Parallel()

	const contextLimit = int64(262144) // 256K, as in the poolside report

	t.Run("inflated cumulative usage triggers compaction", func(t *testing.T) {
		t.Parallel()
		// 417,012 tokens: what the aibridge cross-chunk sum bug
		// produced for a ~6,000-token conversation.
		assert.True(t, shouldCompactPromptUsage(
			fantasy.Usage{InputTokens: 417012, TotalTokens: 418846},
			contextLimit, 80))
	})

	t.Run("correct per-step usage does not trigger", func(t *testing.T) {
		t.Parallel()
		assert.False(t, shouldCompactPromptUsage(
			fantasy.Usage{InputTokens: 6000, TotalTokens: 6030},
			contextLimit, 80))
	})

	t.Run("threshold 100 disables compaction", func(t *testing.T) {
		t.Parallel()
		assert.False(t, shouldCompactPromptUsage(
			fantasy.Usage{InputTokens: 500000}, contextLimit, 100))
	})

	t.Run("zero context limit disables compaction", func(t *testing.T) {
		t.Parallel()
		assert.False(t, shouldCompactPromptUsage(
			fantasy.Usage{InputTokens: 6000}, 0, 80))
	})

	t.Run("counts cache read and creation tokens", func(t *testing.T) {
		t.Parallel()
		usage := fantasy.Usage{
			InputTokens:         6000,
			CacheReadTokens:     200000,
			CacheCreationTokens: 5000,
		}
		// 211,000 / 262,144 = ~80.5%
		assert.True(t, shouldCompactPromptUsage(usage, contextLimit, 80))
	})

	t.Run("falls back to TotalTokens when granular fields are missing", func(t *testing.T) {
		t.Parallel()
		assert.True(t, shouldCompactPromptUsage(
			fantasy.Usage{TotalTokens: 211000},
			contextLimit, 80))
	})
}

func TestBindingCompactionTriggerSource(t *testing.T) {
	t.Parallel()

	// Rows must match the binding tests in
	// site/src/pages/AgentsPage/compactionTriggers.test.ts.

	trigger := func(threshold int32, limit int64) compactionTrigger {
		return compactionTrigger{thresholdPercent: threshold, contextLimit: limit}
	}
	override := func(limit int64) compactionTrigger {
		return trigger(compactionOverrideWindowPercent, limit)
	}

	cases := []struct {
		name     string
		chat     compactionTrigger
		override compactionTrigger
		want     compactionTriggerSource
	}{
		{
			name:     "lower override point wins",
			chat:     trigger(70, 200_000),
			override: override(32_000),
			want:     compactionTriggerSourceOrganization,
		},
		{
			name:     "lower chat point wins over higher override point",
			chat:     trigger(70, 80_000),
			override: override(80_000),
			want:     compactionTriggerSourceChat,
		},
		{
			name:     "tie prefers the chat trigger",
			chat:     trigger(80, 100_000),
			override: override(100_000),
			want:     compactionTriggerSourceChat,
		},
		{
			name:     "chat trigger disabled by threshold 100 yields override",
			chat:     trigger(100, 200_000),
			override: override(32_000),
			want:     compactionTriggerSourceOrganization,
		},
		{
			name:     "chat trigger disabled by zero limit yields override",
			chat:     trigger(70, 0),
			override: override(32_000),
			want:     compactionTriggerSourceOrganization,
		},
		{
			name:     "override disabled by zero limit yields chat",
			chat:     trigger(70, 200_000),
			override: override(0),
			want:     compactionTriggerSourceChat,
		},
		{
			name:     "both disabled yields chat",
			chat:     trigger(100, 200_000),
			override: override(0),
			want:     compactionTriggerSourceChat,
		},
		{
			name:     "chat threshold zero fires immediately and wins",
			chat:     trigger(0, 200_000),
			override: override(32_000),
			want:     compactionTriggerSourceChat,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, bindingCompactionTriggerSource(tc.chat, tc.override))
		})
	}
}

func TestEnabledMCPServerConfigsForChatOrg(t *testing.T) {
	t.Parallel()

	newOrgWithConfig := func(t *testing.T, db database.Store) (database.Organization, database.MCPServerConfig) {
		t.Helper()
		org := dbgen.Organization(t, db, database.Organization{})
		cfg := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
			OrganizationID: org.ID,
			Enabled:        true,
		})
		return org, cfg
	}

	t.Run("DefaultOrgConfigExcluded", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		defaultOrg, err := db.GetDefaultOrganization(ctx)
		require.NoError(t, err)

		// Configs resolve only within the chat's organization.
		chatOrg, chatOrgCfg := newOrgWithConfig(t, db)
		defaultOrgCfg := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
			OrganizationID: defaultOrg.ID,
			Enabled:        true,
		})

		configs, err := enabledMCPServerConfigsForChatOrg(ctx, db, chatOrg.ID, []uuid.UUID{chatOrgCfg.ID, defaultOrgCfg.ID})
		require.NoError(t, err)
		require.Len(t, configs, 1)
		require.Equal(t, chatOrgCfg.ID, configs[0].ID)
	})

	t.Run("ThirdOrgConfigExcluded", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		chatOrg, chatOrgCfg := newOrgWithConfig(t, db)
		_, foreignCfg := newOrgWithConfig(t, db)

		configs, err := enabledMCPServerConfigsForChatOrg(ctx, db, chatOrg.ID, []uuid.UUID{chatOrgCfg.ID, foreignCfg.ID})
		require.NoError(t, err)
		require.Len(t, configs, 1)
		require.Equal(t, chatOrgCfg.ID, configs[0].ID)
	})

	t.Run("DisabledConfigExcluded", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		// dbgen.MCPServerConfig defaults Enabled to true, so insert the
		// disabled config directly.
		chatOrg := dbgen.Organization(t, db, database.Organization{})
		user := dbgen.User(t, db, database.User{})
		disabledCfg, err := db.InsertMCPServerConfig(ctx, database.InsertMCPServerConfigParams{
			ID:             uuid.New(),
			OrganizationID: chatOrg.ID,
			DisplayName:    "Disabled MCP Server",
			Slug:           testutil.GetRandomName(t),
			Url:            "https://mcp.example.com",
			Transport:      "streamable_http",
			AuthType:       "none",
			ToolAllowList:  []string{},
			ToolDenyList:   []string{},
			Availability:   "default_off",
			Enabled:        false,
			CreatedBy:      user.ID,
			UpdatedBy:      user.ID,
		})
		require.NoError(t, err)

		configs, err := enabledMCPServerConfigsForChatOrg(ctx, db, chatOrg.ID, []uuid.UUID{disabledCfg.ID})
		require.NoError(t, err)
		require.Empty(t, configs)
	})

	t.Run("DuplicateIDsYieldOneConfigPerUniqueID", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		// The ID array may contain duplicates, but the query returns one row
		// per unique ID ordered by display_name.
		chatOrg, cfgA := newOrgWithConfig(t, db)
		cfgB := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
			OrganizationID: chatOrg.ID,
			Enabled:        true,
		})

		// The requested order is the reverse of display_name order to
		// prove the output ordering comes from the SQL, not the request.
		requested := []uuid.UUID{cfgB.ID, cfgA.ID, cfgB.ID, cfgA.ID}

		configs, err := enabledMCPServerConfigsForChatOrg(ctx, db, chatOrg.ID, requested)
		require.NoError(t, err)
		require.Len(t, configs, 2)
		gotIDs := []uuid.UUID{configs[0].ID, configs[1].ID}
		wantOrder := []uuid.UUID{cfgA.ID, cfgB.ID}
		if cfgA.DisplayName > cfgB.DisplayName {
			wantOrder = []uuid.UUID{cfgB.ID, cfgA.ID}
		}
		require.Equal(t, wantOrder, gotIDs, "output must follow display_name order, not request order")
	})

	t.Run("ChatOrgWithNoConfigs", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		defaultOrg, err := db.GetDefaultOrganization(ctx)
		require.NoError(t, err)

		chatOrg := dbgen.Organization(t, db, database.Organization{})
		defaultOrgCfg := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
			OrganizationID: defaultOrg.ID,
			Enabled:        true,
		})

		configs, err := enabledMCPServerConfigsForChatOrg(ctx, db, chatOrg.ID, []uuid.UUID{defaultOrgCfg.ID})
		require.NoError(t, err)
		require.Empty(t, configs)
	})
}
