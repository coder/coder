package chatd

import (
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/testutil"
)

func TestSameCompactionProviderIdentity(t *testing.T) {
	t.Parallel()

	providerID := uuid.New()

	require.True(t, sameCompactionProviderIdentity(configWithProvider(providerID), configWithProvider(providerID)))
	require.False(t, sameCompactionProviderIdentity(configWithProvider(providerID), configWithProvider(uuid.New())))
	// Legacy configs without a provider FK compare as different (fail closed).
	require.False(t, sameCompactionProviderIdentity(database.ChatModelConfig{}, configWithProvider(providerID)))
	require.False(t, sameCompactionProviderIdentity(database.ChatModelConfig{}, database.ChatModelConfig{}))
}

func configWithProvider(id uuid.UUID) database.ChatModelConfig {
	return database.ChatModelConfig{AIProviderID: uuid.NullUUID{UUID: id, Valid: true}}
}

func TestSanitizeCompactionPrompt_FlattensForeignProviderExecutedToolParts(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	prompt := []fantasy.Message{
		{
			Role: fantasy.MessageRoleUser,
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: "search the web"},
			},
		},
		{
			Role: fantasy.MessageRoleAssistant,
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: "searching"},
				fantasy.ToolCallPart{
					ToolCallID:       "ws-1",
					ToolName:         "web_search",
					Input:            `{"query":"coder"}`,
					ProviderExecuted: true,
				},
				fantasy.ToolResultPart{
					ToolCallID:       "ws-1",
					Output:           fantasy.ToolResultOutputContentText{Text: "results"},
					ProviderExecuted: true,
				},
			},
		},
		{
			Role: fantasy.MessageRoleAssistant,
			Content: []fantasy.MessagePart{
				fantasy.ToolCallPart{
					ToolCallID: "local-1",
					ToolName:   "read_file",
					Input:      `{"path":"/tmp/a.txt"}`,
				},
			},
		},
	}

	compactionModel := &chattest.FakeModel{ProviderName: "openai", ModelName: "gpt-4.1-mini"}
	sanitized := sanitizeCompactionPrompt(ctx, logger, prompt, compactionModel, configWithProvider(uuid.New()), configWithProvider(uuid.New()))

	require.Len(t, sanitized, 3)
	// Provider-executed parts are flattened to text so the summary keeps
	// their content without the provider-specific wire shape.
	require.Len(t, sanitized[1].Content, 3)
	require.Equal(t, fantasy.TextPart{Text: "searching"}, sanitized[1].Content[0])
	require.Equal(t, fantasy.TextPart{Text: `[Server tool call: web_search] {"query":"coder"}`}, sanitized[1].Content[1])
	require.Equal(t, fantasy.TextPart{Text: "[Server tool result: web_search] results"}, sanitized[1].Content[2])
	// Local tool calls replay fine across providers and must survive.
	require.Len(t, sanitized[2].Content, 1)
	require.Equal(t, "read_file", sanitized[2].Content[0].(fantasy.ToolCallPart).ToolName)

	// The original prompt used for assistant generation is untouched.
	require.Equal(t, fantasy.ToolCallPart{
		ToolCallID:       "ws-1",
		ToolName:         "web_search",
		Input:            `{"query":"coder"}`,
		ProviderExecuted: true,
	}, prompt[1].Content[1])
}

func TestSanitizeCompactionPrompt_DropsNonAssistantProviderExecutedParts(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	// Provider-executed parts outside assistant messages are anomalous; a
	// flattened text part is not valid tool-message content, so they drop.
	prompt := []fantasy.Message{
		{
			Role: fantasy.MessageRoleTool,
			Content: []fantasy.MessagePart{
				fantasy.ToolResultPart{
					ToolCallID:       "ws-1",
					Output:           fantasy.ToolResultOutputContentText{Text: "results"},
					ProviderExecuted: true,
				},
			},
		},
		{
			Role: fantasy.MessageRoleUser,
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: "hello"},
			},
		},
	}

	compactionModel := &chattest.FakeModel{ProviderName: "openai", ModelName: "gpt-4.1-mini"}
	sanitized := sanitizeCompactionPrompt(ctx, logger, prompt, compactionModel, configWithProvider(uuid.New()), configWithProvider(uuid.New()))

	require.Len(t, sanitized, 1)
	require.Equal(t, fantasy.MessageRoleUser, sanitized[0].Role)
	require.Len(t, prompt, 2)
}

func TestSanitizeCompactionPrompt_ReplacesUnsupportedFileParts(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	prompt := []fantasy.Message{
		{
			Role: fantasy.MessageRoleUser,
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: "look at this"},
				fantasy.FilePart{
					Filename:  "diagram.pdf",
					Data:      []byte("%PDF-"),
					MediaType: "application/pdf",
				},
			},
		},
	}

	// Mistral accepts images but not PDFs, so the PDF part must become a
	// placeholder while the prompt stays otherwise intact.
	compactionModel := &chattest.FakeModel{ProviderName: "mistral", ModelName: "mistral-large"}
	sharedProviderID := uuid.New()
	sanitized := sanitizeCompactionPrompt(ctx, logger, prompt, compactionModel, configWithProvider(sharedProviderID), configWithProvider(sharedProviderID))

	require.Len(t, sanitized, 1)
	require.Len(t, sanitized[0].Content, 2)
	textPart, ok := sanitized[0].Content[1].(fantasy.TextPart)
	require.True(t, ok)
	require.Contains(t, textPart.Text, "diagram.pdf")
	require.Contains(t, textPart.Text, "not supported by the compaction model")

	// The original prompt keeps its file part.
	_, ok = prompt[0].Content[1].(fantasy.FilePart)
	require.True(t, ok)
}

func TestSanitizeCompactionPrompt_SameProviderKeepsProviderExecutedParts(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	prompt := []fantasy.Message{
		{
			Role: fantasy.MessageRoleAssistant,
			Content: []fantasy.MessagePart{
				fantasy.ToolCallPart{
					ToolCallID:       "ws-1",
					ToolName:         "web_search",
					Input:            `{"query":"coder"}`,
					ProviderExecuted: true,
				},
				fantasy.ToolResultPart{
					ToolCallID:       "ws-1",
					Output:           fantasy.ToolResultOutputContentText{Text: "results"},
					ProviderExecuted: true,
				},
			},
		},
	}

	compactionModel := &chattest.FakeModel{ProviderName: "openai", ModelName: "gpt-4.1-mini"}
	sharedProviderID := uuid.New()
	sanitized := sanitizeCompactionPrompt(ctx, logger, prompt, compactionModel, configWithProvider(sharedProviderID), configWithProvider(sharedProviderID))

	require.Len(t, sanitized, 1)
	require.Len(t, sanitized[0].Content, 2)
}
