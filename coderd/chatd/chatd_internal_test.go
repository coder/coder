package chatd

import (
	"context"
	"encoding/json"
	"testing"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/testutil"
)

func TestChatMessagesToPrompt_SanitizesToolCallIDs(t *testing.T) {
	t.Parallel()

	const (
		legacyToolCallID    = "subagent_report:123e4567-e89b-12d3-a456-426614174000"
		sanitizedToolCallID = "subagent_report_123e4567-e89b-12d3-a456-426614174000"
	)

	assistantBlocks := append(contentFromText("working"), fantasy.ToolCallContent{
		ToolCallID: legacyToolCallID,
		ToolName:   "read_file",
		Input:      `{"path":"hello.txt"}`,
	})
	assistantContent, err := json.Marshal(assistantBlocks)
	require.NoError(t, err)

	toolResults, err := json.Marshal([]ToolResultBlock{{
		ToolCallID: legacyToolCallID,
		ToolName:   "read_file",
		Result:     map[string]any{"content": "hello"},
	}})
	require.NoError(t, err)

	prompt, err := chatMessagesToPrompt([]database.ChatMessage{
		{
			Role:    string(fantasy.MessageRoleAssistant),
			Content: pqtype.NullRawMessage{RawMessage: assistantContent, Valid: true},
		},
		{
			Role:    string(fantasy.MessageRoleTool),
			Content: pqtype.NullRawMessage{RawMessage: toolResults, Valid: true},
		},
	})
	require.NoError(t, err)
	require.Len(t, prompt, 2)

	assistantToolCalls := extractToolCallsFromMessageParts(prompt[0].Content)
	require.Len(t, assistantToolCalls, 1)
	require.Equal(t, sanitizedToolCallID, assistantToolCalls[0].ToolCallID)

	toolResultParts := messageToolResultParts(prompt[1])
	require.Len(t, toolResultParts, 1)
	require.Equal(t, sanitizedToolCallID, toolResultParts[0].ToolCallID)
}

func TestChatMessagesToPrompt_RepairsLegacyOrphanToolResult(t *testing.T) {
	t.Parallel()

	const (
		legacyToolCallID    = "subagent_report:123e4567-e89b-12d3-a456-426614174000"
		sanitizedToolCallID = "subagent_report_123e4567-e89b-12d3-a456-426614174000"
	)

	userContent, err := json.Marshal(contentFromText("status?"))
	require.NoError(t, err)

	toolResults, err := json.Marshal([]ToolResultBlock{{
		ToolCallID: legacyToolCallID,
		ToolName:   "subagent_report",
		Result: map[string]any{
			"chat_id":    uuid.NewString(),
			"request_id": uuid.NewString(),
			"report":     "done",
			"status":     "reported",
		},
	}})
	require.NoError(t, err)

	prompt, err := chatMessagesToPrompt([]database.ChatMessage{
		{
			Role:    string(fantasy.MessageRoleUser),
			Content: pqtype.NullRawMessage{RawMessage: userContent, Valid: true},
		},
		{
			Role:    string(fantasy.MessageRoleTool),
			Content: pqtype.NullRawMessage{RawMessage: toolResults, Valid: true},
		},
	})
	require.NoError(t, err)
	require.Len(t, prompt, 3)
	require.Equal(t, fantasy.MessageRoleAssistant, prompt[1].Role)
	require.Equal(t, fantasy.MessageRoleTool, prompt[2].Role)

	toolCalls := extractToolCallsFromMessageParts(prompt[1].Content)
	require.Len(t, toolCalls, 1)
	require.Equal(t, sanitizedToolCallID, toolCalls[0].ToolCallID)
	require.Equal(t, "subagent_report", toolCalls[0].ToolName)

	toolResultParts := messageToolResultParts(prompt[2])
	require.Len(t, toolResultParts, 1)
	require.Equal(t, sanitizedToolCallID, toolResultParts[0].ToolCallID)
}

func TestPrepareAgentStepResult_ReportOnly(t *testing.T) {
	t.Parallel()

	sentinel := "__sentinel__"
	result := prepareAgentStepResult(
		[]fantasy.Message{
			{
				Role: fantasy.MessageRoleUser,
				Content: []fantasy.MessagePart{
					fantasy.TextPart{Text: sentinel},
				},
			},
			{
				Role: fantasy.MessageRoleUser,
				Content: []fantasy.MessagePart{
					fantasy.TextPart{Text: "real message"},
				},
			},
		},
		sentinel,
		true,
		false,
	)

	require.Equal(t, []string{"subagent_report"}, result.ActiveTools)
	require.Len(t, result.Messages, 1)
	require.Equal(t, fantasy.MessageRoleUser, result.Messages[0].Role)
}

func TestPrepareAgentStepResult_AnthropicCaching(t *testing.T) {
	t.Parallel()

	result := prepareAgentStepResult(
		[]fantasy.Message{
			textMessage(fantasy.MessageRoleSystem, "sys-1"),
			textMessage(fantasy.MessageRoleSystem, "sys-2"),
			textMessage(fantasy.MessageRoleUser, "hello"),
			textMessage(fantasy.MessageRoleAssistant, "working"),
			textMessage(fantasy.MessageRoleUser, "continue"),
		},
		"__sentinel__",
		false,
		true,
	)

	require.Len(t, result.Messages, 5)
	require.False(t, hasAnthropicEphemeralCacheControl(result.Messages[0]))
	require.True(t, hasAnthropicEphemeralCacheControl(result.Messages[1]))
	require.False(t, hasAnthropicEphemeralCacheControl(result.Messages[2]))
	require.True(t, hasAnthropicEphemeralCacheControl(result.Messages[3]))
	require.True(t, hasAnthropicEphemeralCacheControl(result.Messages[4]))
}

func TestAnyAvailableModel(t *testing.T) {
	t.Parallel()

	t.Run("OpenAIOnly", func(t *testing.T) {
		t.Parallel()

		model, err := anyAvailableModel(ProviderAPIKeys{OpenAI: "openai-key"})
		require.NoError(t, err)
		require.Equal(t, "openai", model.Provider())
		require.Equal(t, "gpt-4o-mini", model.Model())
	})

	t.Run("None", func(t *testing.T) {
		t.Parallel()

		model, err := anyAvailableModel(ProviderAPIKeys{})
		require.Nil(t, model)
		require.EqualError(t, err, "no AI provider API keys are configured")
	})
}

func TestGenerateChatTitle(t *testing.T) {
	t.Parallel()

	t.Run("SuccessNormalizesOutput", func(t *testing.T) {
		t.Parallel()

		model := &titleTestModel{
			generateFn: func(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
				return &fantasy.Response{
					Content: []fantasy.Content{
						fantasy.TextContent{Text: `  " Debugging   Flaky   Go Tests "  `},
					},
				}, nil
			},
		}

		p := &Processor{
			resolveProviderAPIKeysFn: func(context.Context) (ProviderAPIKeys, error) {
				return ProviderAPIKeys{OpenAI: "openai-key"}, nil
			},
			titleGeneration: TitleGenerationConfig{Prompt: "custom title prompt"},
			titleModelLookup: func(ProviderAPIKeys) (fantasy.LanguageModel, error) {
				return model, nil
			},
		}

		title, err := p.generateChatTitle(context.Background(), "How can I debug this flaky Go test?")
		require.NoError(t, err)
		require.Equal(t, "Debugging Flaky Go Tests", title)
	})

	t.Run("EmptyOutput", func(t *testing.T) {
		t.Parallel()

		model := &titleTestModel{
			generateFn: func(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
				return &fantasy.Response{
					Content: []fantasy.Content{
						fantasy.TextContent{Text: "   "},
					},
				}, nil
			},
		}

		p := &Processor{
			resolveProviderAPIKeysFn: func(context.Context) (ProviderAPIKeys, error) {
				return ProviderAPIKeys{OpenAI: "openai-key"}, nil
			},
			titleModelLookup: func(ProviderAPIKeys) (fantasy.LanguageModel, error) {
				return model, nil
			},
		}

		title, err := p.generateChatTitle(context.Background(), "hello")
		require.EqualError(t, err, "generated title was empty")
		require.Empty(t, title)
	})

	t.Run("GenerateError", func(t *testing.T) {
		t.Parallel()

		model := &titleTestModel{
			generateFn: func(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
				return nil, xerrors.New("model failed")
			},
		}

		p := &Processor{
			resolveProviderAPIKeysFn: func(context.Context) (ProviderAPIKeys, error) {
				return ProviderAPIKeys{OpenAI: "openai-key"}, nil
			},
			titleModelLookup: func(ProviderAPIKeys) (fantasy.LanguageModel, error) {
				return model, nil
			},
		}

		title, err := p.generateChatTitle(context.Background(), "hello")
		require.EqualError(t, err, "generate title text: model failed")
		require.Empty(t, title)
	})
}

func TestMaybeGenerateChatTitle(t *testing.T) {
	t.Parallel()

	messageText := "How do I debug flaky Go tests?"
	initialTitle := fallbackChatTitle(messageText)

	t.Run("UpdatesTitle", func(t *testing.T) {
		t.Parallel()

		db := chatdTestDB(t)
		chat := insertChatForTesting(t, db, initialTitle)
		messages := []database.ChatMessage{mustUserChatMessage(t, messageText)}

		p := &Processor{
			db: db,
			resolveProviderAPIKeysFn: func(context.Context) (ProviderAPIKeys, error) {
				return ProviderAPIKeys{OpenAI: "openai-key"}, nil
			},
			titleModelLookup: func(ProviderAPIKeys) (fantasy.LanguageModel, error) {
				return &titleTestModel{
					generateFn: func(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
						return &fantasy.Response{
							Content: []fantasy.Content{
								fantasy.TextContent{Text: "Debugging Flaky Go Tests"},
							},
						}, nil
					},
				}, nil
			},
		}

		p.maybeGenerateChatTitle(context.Background(), chat, messages, testutil.Logger(t))

		updated, err := db.GetChatByID(context.Background(), chat.ID)
		require.NoError(t, err)
		require.Equal(t, "Debugging Flaky Go Tests", updated.Title)
	})

	t.Run("SkipsUpdateOnGenerationError", func(t *testing.T) {
		t.Parallel()

		db := chatdTestDB(t)
		chat := insertChatForTesting(t, db, initialTitle)
		messages := []database.ChatMessage{mustUserChatMessage(t, messageText)}

		p := &Processor{
			db: db,
			resolveProviderAPIKeysFn: func(context.Context) (ProviderAPIKeys, error) {
				return ProviderAPIKeys{OpenAI: "openai-key"}, nil
			},
			titleModelLookup: func(ProviderAPIKeys) (fantasy.LanguageModel, error) {
				return &titleTestModel{
					generateFn: func(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
						return nil, xerrors.New("title model failed")
					},
				}, nil
			},
		}

		p.maybeGenerateChatTitle(context.Background(), chat, messages, testutil.Logger(t))

		updated, err := db.GetChatByID(context.Background(), chat.ID)
		require.NoError(t, err)
		require.Equal(t, initialTitle, updated.Title)
	})
}

func messageToolResultParts(message fantasy.Message) []fantasy.ToolResultPart {
	results := make([]fantasy.ToolResultPart, 0, len(message.Content))
	for _, part := range message.Content {
		result, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](part)
		if !ok {
			continue
		}
		results = append(results, result)
	}
	return results
}

func textMessage(role fantasy.MessageRole, text string) fantasy.Message {
	return fantasy.Message{
		Role: role,
		Content: []fantasy.MessagePart{
			fantasy.TextPart{Text: text},
		},
	}
}

func hasAnthropicEphemeralCacheControl(message fantasy.Message) bool {
	if len(message.ProviderOptions) == 0 {
		return false
	}

	options, ok := message.ProviderOptions[fantasyanthropic.Name]
	if !ok {
		return false
	}

	cacheOptions, ok := options.(*fantasyanthropic.ProviderCacheControlOptions)
	return ok && cacheOptions.CacheControl.Type == "ephemeral"
}

type titleTestModel struct {
	generateFn func(context.Context, fantasy.Call) (*fantasy.Response, error)
}

func (*titleTestModel) Provider() string {
	return "fake"
}

func (*titleTestModel) Model() string {
	return "fake"
}

func (m *titleTestModel) Generate(
	ctx context.Context,
	call fantasy.Call,
) (*fantasy.Response, error) {
	if m.generateFn != nil {
		return m.generateFn(ctx, call)
	}
	return &fantasy.Response{}, nil
}

func (*titleTestModel) Stream(
	_ context.Context,
	_ fantasy.Call,
) (fantasy.StreamResponse, error) {
	return nil, xerrors.New("not implemented")
}

func (*titleTestModel) GenerateObject(context.Context, fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
	return nil, xerrors.New("not implemented")
}

func (*titleTestModel) StreamObject(context.Context, fantasy.ObjectCall) (fantasy.ObjectStreamResponse, error) {
	return nil, xerrors.New("not implemented")
}

func chatdTestDB(t *testing.T) database.Store {
	t.Helper()

	db, _ := dbtestutil.NewDB(t)
	return db
}

func insertChatForTesting(t *testing.T, db database.Store, title string) database.Chat {
	t.Helper()

	user := dbgen.User(t, db, database.User{})
	chat, err := db.InsertChat(context.Background(), database.InsertChatParams{
		OwnerID:     user.ID,
		Title:       title,
		ModelConfig: json.RawMessage(`{"provider":"openai","model":"gpt-4o-mini"}`),
	})
	require.NoError(t, err)
	return chat
}

func mustUserChatMessage(t *testing.T, text string) database.ChatMessage {
	t.Helper()

	raw, err := json.Marshal(contentFromText(text))
	require.NoError(t, err)

	return database.ChatMessage{
		Role: string(fantasy.MessageRoleUser),
		Content: pqtype.NullRawMessage{
			RawMessage: raw,
			Valid:      true,
		},
	}
}

func contentFromText(text string) []fantasy.Content {
	return []fantasy.Content{
		fantasy.TextContent{Text: text},
	}
}
