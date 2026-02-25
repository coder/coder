package chatd

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"charm.land/fantasy"
	fantasyopenai "charm.land/fantasy/providers/openai"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk"
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

	toolResults, err := json.Marshal([]chatprompt.ToolResultBlock{{
		ToolCallID: legacyToolCallID,
		ToolName:   "read_file",
		Result:     map[string]any{"content": "hello"},
	}})
	require.NoError(t, err)

	prompt, err := chatprompt.ConvertMessages([]database.ChatMessage{
		{
			Role:    string(fantasy.MessageRoleAssistant),
			Content: pqtype.NullRawMessage{RawMessage: assistantContent, Valid: true},
		},
		{
			Role:    string(fantasy.MessageRoleTool),
			Content: pqtype.NullRawMessage{RawMessage: toolResults, Valid: true},
		},
	}, subagentReportToolCallIDPrefix)
	require.NoError(t, err)
	require.Len(t, prompt, 2)

	assistantToolCalls := chatprompt.ExtractToolCalls(prompt[0].Content)
	require.Len(t, assistantToolCalls, 1)
	require.Equal(t, sanitizedToolCallID, assistantToolCalls[0].ToolCallID)

	toolResultParts := messageToolResultParts(prompt[1])
	require.Len(t, toolResultParts, 1)
	require.Equal(t, sanitizedToolCallID, toolResultParts[0].ToolCallID)
}

func TestContentToMessageParts_PreservesReasoningProviderMetadata(t *testing.T) {
	t.Parallel()

	metadata := &fantasyopenai.ResponsesReasoningMetadata{
		ItemID:  "reasoning-item-1",
		Summary: []string{"Plan migration"},
	}

	parts := chatprompt.ToMessageParts([]fantasy.Content{
		fantasy.ReasoningContent{
			Text: "Plan migration",
			ProviderMetadata: fantasy.ProviderMetadata{
				fantasyopenai.Name: metadata,
			},
		},
	})
	require.Len(t, parts, 1)

	reasoningPart, ok := fantasy.AsMessagePart[fantasy.ReasoningPart](parts[0])
	require.True(t, ok)
	require.Equal(t, "Plan migration", reasoningPart.Text)

	gotMetadata := fantasyopenai.GetReasoningMetadata(reasoningPart.ProviderOptions)
	require.NotNil(t, gotMetadata)
	require.Equal(t, "reasoning-item-1", gotMetadata.ItemID)
	require.Equal(t, []string{"Plan migration"}, gotMetadata.Summary)
}

func TestExtractGitAuthRequiredMarker(t *testing.T) {
	t.Parallel()

	output := "" +
		"fatal: could not read Username for 'https://github.com': terminal prompts disabled\n" +
		"CODER_GITAUTH_REQUIRED:{\"provider_id\":\"github\",\"provider_type\":\"github\",\"provider_display_name\":\"GitHub\",\"authenticate_url\":\"https://coder.example.com/external-auth/github\",\"host\":\"https://github.com\"}\n" +
		"fatal: Authentication failed\n"

	marker, cleaned := extractGitAuthRequiredMarker(output)
	require.NotNil(t, marker)
	require.Equal(t, "github", marker.ProviderID)
	require.Equal(t, "https://coder.example.com/external-auth/github", marker.AuthenticateURL)
	require.NotContains(t, cleaned, gitAuthRequiredMarkerPrefix)
	require.Contains(t, cleaned, "fatal: Authentication failed")
}

func TestWaitForExternalAuth(t *testing.T) {
	t.Parallel()

	t.Run("Authenticated", func(t *testing.T) {
		t.Parallel()

		db := chatdTestDB(t)
		user := dbgen.User(t, db, database.User{})
		dbgen.ExternalAuthLink(t, db, database.ExternalAuthLink{
			ProviderID:       "github",
			UserID:           user.ID,
			OAuthAccessToken: "token",
			OAuthExpiry:      time.Now().Add(time.Minute),
		})

		p := &Processor{db: db}
		authenticated, timedOut, err := p.waitForExternalAuth(
			context.Background(),
			user.ID,
			"github",
			100*time.Millisecond,
		)
		require.NoError(t, err)
		require.True(t, authenticated)
		require.False(t, timedOut)
	})

	t.Run("TimedOut", func(t *testing.T) {
		t.Parallel()

		db := chatdTestDB(t)
		user := dbgen.User(t, db, database.User{})
		p := &Processor{db: db}

		authenticated, timedOut, err := p.waitForExternalAuth(
			context.Background(),
			user.ID,
			"missing-provider",
			50*time.Millisecond,
		)
		require.NoError(t, err)
		require.False(t, authenticated)
		require.True(t, timedOut)
	})
}

func TestContentToMessageParts_PreservesProviderMetadataForOtherParts(t *testing.T) {
	t.Parallel()

	textMetadata := &fantasyopenai.ResponsesReasoningMetadata{
		ItemID:  "text-metadata",
		Summary: []string{"text"},
	}
	toolCallMetadata := &fantasyopenai.ResponsesReasoningMetadata{
		ItemID:  "tool-call-metadata",
		Summary: []string{"tool-call"},
	}
	fileMetadata := &fantasyopenai.ResponsesReasoningMetadata{
		ItemID:  "file-metadata",
		Summary: []string{"file"},
	}
	toolResultMetadata := &fantasyopenai.ResponsesReasoningMetadata{
		ItemID:  "tool-result-metadata",
		Summary: []string{"tool-result"},
	}

	parts := chatprompt.ToMessageParts([]fantasy.Content{
		fantasy.TextContent{
			Text: "hello",
			ProviderMetadata: fantasy.ProviderMetadata{
				fantasyopenai.Name: textMetadata,
			},
		},
		fantasy.ToolCallContent{
			ToolCallID: "call-1",
			ToolName:   "execute",
			Input:      `{"command":"pwd"}`,
			ProviderMetadata: fantasy.ProviderMetadata{
				fantasyopenai.Name: toolCallMetadata,
			},
		},
		fantasy.FileContent{
			MediaType: "text/plain",
			Data:      []byte("file"),
			ProviderMetadata: fantasy.ProviderMetadata{
				fantasyopenai.Name: fileMetadata,
			},
		},
		fantasy.ToolResultContent{
			ToolCallID: "call-1",
			ToolName:   "execute",
			Result: fantasy.ToolResultOutputContentText{
				Text: `{"output":"ok"}`,
			},
			ProviderMetadata: fantasy.ProviderMetadata{
				fantasyopenai.Name: toolResultMetadata,
			},
		},
	})
	require.Len(t, parts, 4)

	textPart, ok := fantasy.AsMessagePart[fantasy.TextPart](parts[0])
	require.True(t, ok)
	textPartMetadata := fantasyopenai.GetReasoningMetadata(textPart.ProviderOptions)
	require.NotNil(t, textPartMetadata)
	require.Equal(t, "text-metadata", textPartMetadata.ItemID)

	toolCallPart, ok := fantasy.AsMessagePart[fantasy.ToolCallPart](parts[1])
	require.True(t, ok)
	toolCallPartMetadata := fantasyopenai.GetReasoningMetadata(
		toolCallPart.ProviderOptions,
	)
	require.NotNil(t, toolCallPartMetadata)
	require.Equal(t, "tool-call-metadata", toolCallPartMetadata.ItemID)

	filePart, ok := fantasy.AsMessagePart[fantasy.FilePart](parts[2])
	require.True(t, ok)
	filePartMetadata := fantasyopenai.GetReasoningMetadata(filePart.ProviderOptions)
	require.NotNil(t, filePartMetadata)
	require.Equal(t, "file-metadata", filePartMetadata.ItemID)

	toolResultPart, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](parts[3])
	require.True(t, ok)
	toolResultPartMetadata := fantasyopenai.GetReasoningMetadata(
		toolResultPart.ProviderOptions,
	)
	require.NotNil(t, toolResultPartMetadata)
	require.Equal(t, "tool-result-metadata", toolResultPartMetadata.ItemID)
}

func TestContentBlockToPart_ReasoningIncludesSummaryTitle(t *testing.T) {
	t.Parallel()

	metadata := &fantasyopenai.ResponsesReasoningMetadata{
		ItemID:  "reasoning-item-1",
		Summary: []string{"", "Plan migration"},
	}

	part := chatprompt.PartFromContent(fantasy.ReasoningContent{
		Text: "Plan migration",
		ProviderMetadata: fantasy.ProviderMetadata{
			fantasyopenai.Name: metadata,
		},
	})

	require.Equal(t, codersdk.ChatMessagePartTypeReasoning, part.Type)
	require.Equal(t, "Plan migration", part.Text)
	require.Equal(t, "Plan migration", part.Title)
}

func TestContentBlockToPart_ReasoningTitleTruncatesSummary(t *testing.T) {
	t.Parallel()

	metadata := &fantasyopenai.ResponsesReasoningMetadata{
		ItemID: "reasoning-item-1",
		Summary: []string{
			"Investigated workspace build failures and prepared step-by-step remediation plan for migrations",
		},
	}

	part := chatprompt.PartFromContent(fantasy.ReasoningContent{
		Text: "Investigated workspace build failures and prepared step-by-step remediation plan for migrations",
		ProviderMetadata: fantasy.ProviderMetadata{
			fantasyopenai.Name: metadata,
		},
	})

	require.Equal(t, codersdk.ChatMessagePartTypeReasoning, part.Type)
	require.Equal(
		t,
		"Investigated workspace build failures and prepared step-by-step remediation…",
		part.Title,
	)
}

func TestContentBlockToPart_ReasoningTitleUsesSummaryHeading(t *testing.T) {
	t.Parallel()

	metadata := &fantasyopenai.ResponsesReasoningMetadata{
		ItemID: "reasoning-item-1",
		Summary: []string{
			"**Implement migration safely**\n\nVerify schema updates, then apply changes in order.",
		},
	}

	part := chatprompt.PartFromContent(fantasy.ReasoningContent{
		Text: "Verify schema updates, then apply changes in order.",
		ProviderMetadata: fantasy.ProviderMetadata{
			fantasyopenai.Name: metadata,
		},
	})

	require.Equal(t, codersdk.ChatMessagePartTypeReasoning, part.Type)
	require.Equal(t, "Implement migration safely", part.Title)
}

func TestReasoningMarkdownTitleFromFirstLine(t *testing.T) {
	t.Parallel()

	t.Run("CompleteHeading", func(t *testing.T) {
		t.Parallel()
		title := chatprompt.ReasoningTitleFromFirstLine(
			"**Implement migration safely**\n\nVerify schema updates.",
		)
		require.Equal(t, "Implement migration safely", title)
	})

	t.Run("IncompleteHeading", func(t *testing.T) {
		t.Parallel()
		title := chatprompt.ReasoningTitleFromFirstLine("**Implement migration")
		require.Empty(t, title)
	})

	t.Run("NonHeadingReasoning", func(t *testing.T) {
		t.Parallel()
		title := chatprompt.ReasoningTitleFromFirstLine("Implement migration")
		require.Empty(t, title)
	})

	t.Run("HeadingWithSameLineSuffix", func(t *testing.T) {
		t.Parallel()
		title := chatprompt.ReasoningTitleFromFirstLine("**Implement migration** details")
		require.Empty(t, title)
	})
}

func TestMarshalContentBlocks_ReasoningPersistsTitle(t *testing.T) {
	t.Parallel()

	blocks := []fantasy.Content{
		fantasy.ReasoningContent{
			Text: "**Implement migration safely**\n\nVerify schema updates, then apply changes in order.",
			ProviderMetadata: fantasy.ProviderMetadata{
				fantasyopenai.Name: &fantasyopenai.ResponsesReasoningMetadata{
					ItemID: "reasoning-item-1",
					Summary: []string{
						"Fallback metadata title",
					},
				},
			},
		},
	}

	raw, err := chatprompt.MarshalContent(blocks)
	require.NoError(t, err)
	require.True(t, raw.Valid)

	var encoded []json.RawMessage
	require.NoError(t, json.Unmarshal(raw.RawMessage, &encoded))
	require.Len(t, encoded, 1)

	var persisted struct {
		Type string `json:"type"`
		Data struct {
			Title string `json:"title"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(encoded[0], &persisted))
	require.Equal(t, string(fantasy.ContentTypeReasoning), persisted.Type)
	require.Equal(t, "Implement migration safely", persisted.Data.Title)
}

func TestMarshalContentBlocks_ReasoningSkipsTitleWithoutMarkdownHeading(t *testing.T) {
	t.Parallel()

	blocks := []fantasy.Content{
		fantasy.ReasoningContent{
			Text: "Plan migration\n\nVerify schema updates, then apply changes in order.",
			ProviderMetadata: fantasy.ProviderMetadata{
				fantasyopenai.Name: &fantasyopenai.ResponsesReasoningMetadata{
					ItemID: "reasoning-item-1",
					Summary: []string{
						"**Metadata title should not be used**",
					},
				},
			},
		},
	}

	raw, err := chatprompt.MarshalContent(blocks)
	require.NoError(t, err)
	require.True(t, raw.Valid)

	var encoded []json.RawMessage
	require.NoError(t, json.Unmarshal(raw.RawMessage, &encoded))
	require.Len(t, encoded, 1)

	var persisted struct {
		Type string         `json:"type"`
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(encoded[0], &persisted))
	require.Equal(t, string(fantasy.ContentTypeReasoning), persisted.Type)
	_, hasTitle := persisted.Data["title"]
	require.False(t, hasTitle)
}

func TestChatMessagesToPrompt_RepairsLegacyOrphanToolResult(t *testing.T) {
	t.Parallel()

	const (
		legacyToolCallID    = "subagent_report:123e4567-e89b-12d3-a456-426614174000"
		sanitizedToolCallID = "subagent_report_123e4567-e89b-12d3-a456-426614174000"
	)

	userContent, err := json.Marshal(contentFromText("status?"))
	require.NoError(t, err)

	toolResults, err := json.Marshal([]chatprompt.ToolResultBlock{{
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

	prompt, err := chatprompt.ConvertMessages([]database.ChatMessage{
		{
			Role:    string(fantasy.MessageRoleUser),
			Content: pqtype.NullRawMessage{RawMessage: userContent, Valid: true},
		},
		{
			Role:    string(fantasy.MessageRoleTool),
			Content: pqtype.NullRawMessage{RawMessage: toolResults, Valid: true},
		},
	}, subagentReportToolCallIDPrefix)
	require.NoError(t, err)
	require.Len(t, prompt, 3)
	require.Equal(t, fantasy.MessageRoleAssistant, prompt[1].Role)
	require.Equal(t, fantasy.MessageRoleTool, prompt[2].Role)

	toolCalls := chatprompt.ExtractToolCalls(prompt[1].Content)
	require.Len(t, toolCalls, 1)
	require.Equal(t, sanitizedToolCallID, toolCalls[0].ToolCallID)
	require.Equal(t, "subagent_report", toolCalls[0].ToolName)

	toolResultParts := messageToolResultParts(prompt[2])
	require.Len(t, toolResultParts, 1)
	require.Equal(t, sanitizedToolCallID, toolResultParts[0].ToolCallID)
}

func TestParseChatModelConfig_ParsesCallConfig(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{
		"provider":"openai",
		"model":"gpt-5.2",
		"workspace_mode":"workspace",
		"context_limit":131072,
		"max_output_tokens":2048,
		"temperature":0.4,
		"top_p":0.9,
		"top_k":40,
		"presence_penalty":0.1,
		"frequency_penalty":0.2,
		"provider_options":{
			"openai":{
				"parallel_tool_calls":true,
				"reasoning_effort":"medium"
			}
		}
	}`)

	config, err := parseChatModelConfig(raw)
	require.NoError(t, err)
	require.Equal(t, "openai", config.Provider)
	require.Equal(t, "gpt-5.2", config.Model)
	require.Equal(t, int64(131072), config.ContextLimit)
	require.Equal(t, codersdk.ChatWorkspaceModeWorkspace, config.WorkspaceMode)
	require.NotNil(t, config.MaxOutputTokens)
	require.Equal(t, int64(2048), *config.MaxOutputTokens)
	require.NotNil(t, config.Temperature)
	require.Equal(t, 0.4, *config.Temperature)
	require.NotNil(t, config.TopP)
	require.Equal(t, 0.9, *config.TopP)
	require.NotNil(t, config.TopK)
	require.Equal(t, int64(40), *config.TopK)
	require.NotNil(t, config.PresencePenalty)
	require.Equal(t, 0.1, *config.PresencePenalty)
	require.NotNil(t, config.FrequencyPenalty)
	require.Equal(t, 0.2, *config.FrequencyPenalty)
	require.NotNil(t, config.ProviderOptions)
	require.NotNil(t, config.ProviderOptions.OpenAI)
	require.NotNil(t, config.ProviderOptions.OpenAI.ParallelToolCalls)
	require.True(t, *config.ProviderOptions.OpenAI.ParallelToolCalls)
	require.NotNil(t, config.ProviderOptions.OpenAI.ReasoningEffort)
	require.Equal(
		t,
		"medium",
		*config.ProviderOptions.OpenAI.ReasoningEffort,
	)
}

func TestParseChatModelConfig_DoesNotInterpretWrappedProviderOptions(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{
		"provider":"anthropic",
		"model":"claude-opus-4-6",
		"provider_options":{
			"anthropic":{
				"type":"anthropic.options",
				"data":{
					"send_reasoning":true,
					"effort":"high"
				}
			}
		}
	}`)

	config, err := parseChatModelConfig(raw)
	require.NoError(t, err)
	require.Equal(t, "anthropic", config.Provider)
	require.Equal(t, "claude-opus-4-6", config.Model)
	require.NotNil(t, config.ProviderOptions)
	require.NotNil(t, config.ProviderOptions.Anthropic)
	require.Nil(t, config.ProviderOptions.Anthropic.SendReasoning)
	require.Nil(t, config.ProviderOptions.Anthropic.Effort)
}

func TestApplyFallbackChatModelConfig_MergesMissingCallConfigValues(t *testing.T) {
	t.Parallel()

	maxOutputTokens := int64(4096)
	temperature := 0.2

	config := chatModelConfig{
		ChatModelCallConfig: codersdk.ChatModelCallConfig{
			Temperature: &temperature,
		},
	}
	fallbackConfig := database.ChatModelConfig{
		Provider: "anthropic",
		Model:    "claude-opus-4-6",
		ModelConfig: json.RawMessage(`{
			"max_output_tokens": 4096,
			"provider_options": {
				"anthropic": {
					"send_reasoning": true,
					"effort": "high"
				}
			}
		}`),
	}

	merged := applyFallbackChatModelConfig(config, fallbackConfig)
	require.Equal(t, "anthropic", merged.Provider)
	require.Equal(t, "claude-opus-4-6", merged.Model)
	require.NotNil(t, merged.MaxOutputTokens)
	require.Equal(t, maxOutputTokens, *merged.MaxOutputTokens)
	require.NotNil(t, merged.Temperature)
	require.Equal(t, temperature, *merged.Temperature)
	require.NotNil(t, merged.ProviderOptions)
	require.NotNil(t, merged.ProviderOptions.Anthropic)
	require.NotNil(t, merged.ProviderOptions.Anthropic.SendReasoning)
	require.True(t, *merged.ProviderOptions.Anthropic.SendReasoning)
	require.NotNil(t, merged.ProviderOptions.Anthropic.Effort)
	require.Equal(t, "high", *merged.ProviderOptions.Anthropic.Effort)
}

func TestApplyFallbackChatModelConfig_PreservesExistingProviderOptions(t *testing.T) {
	t.Parallel()

	sendReasoning := false
	config := chatModelConfig{
		ChatModelCallConfig: codersdk.ChatModelCallConfig{
			ProviderOptions: &codersdk.ChatModelProviderOptions{
				Anthropic: &codersdk.ChatModelAnthropicProviderOptions{
					SendReasoning: &sendReasoning,
				},
			},
		},
	}
	fallbackConfig := database.ChatModelConfig{
		Provider: "anthropic",
		Model:    "claude-opus-4-6",
		ModelConfig: json.RawMessage(`{
			"provider_options": {
				"anthropic": {
					"send_reasoning": true,
					"effort": "high"
				}
			}
		}`),
	}

	merged := applyFallbackChatModelConfig(config, fallbackConfig)
	require.NotNil(t, merged.ProviderOptions)
	require.NotNil(t, merged.ProviderOptions.Anthropic)
	require.NotNil(t, merged.ProviderOptions.Anthropic.SendReasoning)
	require.False(t, *merged.ProviderOptions.Anthropic.SendReasoning)
	require.NotNil(t, merged.ProviderOptions.Anthropic.Effort)
	require.Equal(t, "high", *merged.ProviderOptions.Anthropic.Effort)
}

func TestApplyFallbackChatModelConfig_MergesOpenAIProviderOptionsIntoEmptyOptions(t *testing.T) {
	t.Parallel()

	reasoningEffort := "high"
	config := chatModelConfig{
		ChatModelCallConfig: codersdk.ChatModelCallConfig{
			ProviderOptions: &codersdk.ChatModelProviderOptions{},
		},
	}
	fallbackConfig := database.ChatModelConfig{
		Provider: "openai",
		Model:    "gpt-5.2",
		ModelConfig: json.RawMessage(`{
			"provider_options": {
				"openai": {
					"reasoning_effort": "high"
				}
			}
		}`),
	}

	merged := applyFallbackChatModelConfig(config, fallbackConfig)
	require.NotNil(t, merged.ProviderOptions)
	require.NotNil(t, merged.ProviderOptions.OpenAI)
	require.NotNil(t, merged.ProviderOptions.OpenAI.ReasoningEffort)
	require.Equal(
		t,
		reasoningEffort,
		*merged.ProviderOptions.OpenAI.ReasoningEffort,
	)
}

func TestApplyFallbackChatModelConfig_MergesOpenAIMissingFields(t *testing.T) {
	t.Parallel()

	user := "alice"
	reasoningEffort := "high"
	config := chatModelConfig{
		ChatModelCallConfig: codersdk.ChatModelCallConfig{
			ProviderOptions: &codersdk.ChatModelProviderOptions{
				OpenAI: &codersdk.ChatModelOpenAIProviderOptions{
					User: &user,
				},
			},
		},
	}
	fallbackConfig := database.ChatModelConfig{
		Provider: "openai",
		Model:    "gpt-5.2",
		ModelConfig: json.RawMessage(`{
			"provider_options": {
				"openai": {
					"reasoning_effort": "high"
				}
			}
		}`),
	}

	merged := applyFallbackChatModelConfig(config, fallbackConfig)
	require.NotNil(t, merged.ProviderOptions)
	require.NotNil(t, merged.ProviderOptions.OpenAI)
	require.NotNil(t, merged.ProviderOptions.OpenAI.User)
	require.Equal(t, user, *merged.ProviderOptions.OpenAI.User)
	require.NotNil(t, merged.ProviderOptions.OpenAI.ReasoningEffort)
	require.Equal(
		t,
		reasoningEffort,
		*merged.ProviderOptions.OpenAI.ReasoningEffort,
	)
}

func TestStreamCallOptionsFromChatModelConfig_UsesMergedFallbackOpenAIOptions(t *testing.T) {
	t.Parallel()

	config := chatModelConfig{
		ChatModelCallConfig: codersdk.ChatModelCallConfig{
			ProviderOptions: &codersdk.ChatModelProviderOptions{},
		},
	}
	fallbackConfig := database.ChatModelConfig{
		Provider: "openai",
		Model:    "gpt-5.2",
		ModelConfig: json.RawMessage(`{
			"provider_options": {
				"openai": {
					"reasoning_effort": "high"
				}
			}
		}`),
	}

	merged := applyFallbackChatModelConfig(config, fallbackConfig)
	streamCall := streamCallOptionsFromChatModelConfig(
		&titleTestModel{provider: fantasyopenai.Name, model: "gpt-5.2"},
		merged,
	)

	require.NotNil(t, streamCall.ProviderOptions)
	openAIOptionsAny := streamCall.ProviderOptions[fantasyopenai.Name]
	require.NotNil(t, openAIOptionsAny)
	openAIOptions, ok := openAIOptionsAny.(*fantasyopenai.ResponsesProviderOptions)
	require.True(t, ok)
	require.NotNil(t, openAIOptions.ReasoningEffort)
	require.Equal(t, fantasyopenai.ReasoningEffortHigh, *openAIOptions.ReasoningEffort)
}

func TestStreamCallOptionsFromChatModelConfig_OpenAIResponses(t *testing.T) {
	t.Parallel()

	maxOutputTokens := int64(4096)
	temperature := 0.3
	topP := 0.92
	topK := int64(42)
	presencePenalty := 0.11
	frequencyPenalty := 0.23
	parallelToolCalls := true
	reasoningEffort := "medium"
	serviceTier := "priority"
	textVerbosity := "high"
	user := " user-123 "
	includes := []string{
		string(fantasyopenai.IncludeMessageOutputTextLogprobs),
		string(fantasyopenai.IncludeReasoningEncryptedContent),
	}

	streamCall := streamCallOptionsFromChatModelConfig(
		&titleTestModel{provider: fantasyopenai.Name, model: "gpt-5.2"},
		chatModelConfig{
			ChatModelCallConfig: codersdk.ChatModelCallConfig{
				MaxOutputTokens:  &maxOutputTokens,
				Temperature:      &temperature,
				TopP:             &topP,
				TopK:             &topK,
				PresencePenalty:  &presencePenalty,
				FrequencyPenalty: &frequencyPenalty,
				ProviderOptions: &codersdk.ChatModelProviderOptions{
					OpenAI: &codersdk.ChatModelOpenAIProviderOptions{
						ParallelToolCalls: &parallelToolCalls,
						ReasoningEffort:   &reasoningEffort,
						ServiceTier:       &serviceTier,
						TextVerbosity:     &textVerbosity,
						User:              &user,
						Include:           includes,
					},
				},
			},
		},
	)

	require.NotNil(t, streamCall.MaxOutputTokens)
	require.Equal(t, int64(4096), *streamCall.MaxOutputTokens)
	require.NotNil(t, streamCall.Temperature)
	require.Equal(t, 0.3, *streamCall.Temperature)
	require.NotNil(t, streamCall.TopP)
	require.Equal(t, 0.92, *streamCall.TopP)
	require.NotNil(t, streamCall.TopK)
	require.Equal(t, int64(42), *streamCall.TopK)
	require.NotNil(t, streamCall.PresencePenalty)
	require.Equal(t, 0.11, *streamCall.PresencePenalty)
	require.NotNil(t, streamCall.FrequencyPenalty)
	require.Equal(t, 0.23, *streamCall.FrequencyPenalty)

	openAIOptionsAny, ok := streamCall.ProviderOptions[fantasyopenai.Name]
	require.True(t, ok)
	openAIOptions, ok := openAIOptionsAny.(*fantasyopenai.ResponsesProviderOptions)
	require.True(t, ok)
	require.NotNil(t, openAIOptions.ParallelToolCalls)
	require.True(t, *openAIOptions.ParallelToolCalls)
	require.NotNil(t, openAIOptions.ReasoningEffort)
	require.Equal(t, fantasyopenai.ReasoningEffortMedium, *openAIOptions.ReasoningEffort)
	require.NotNil(t, openAIOptions.ServiceTier)
	require.Equal(t, fantasyopenai.ServiceTierPriority, *openAIOptions.ServiceTier)
	require.NotNil(t, openAIOptions.TextVerbosity)
	require.Equal(t, fantasyopenai.TextVerbosityHigh, *openAIOptions.TextVerbosity)
	require.NotNil(t, openAIOptions.User)
	require.Equal(t, "user-123", *openAIOptions.User)
	require.ElementsMatch(t, []fantasyopenai.IncludeType{
		fantasyopenai.IncludeMessageOutputTextLogprobs,
		fantasyopenai.IncludeReasoningEncryptedContent,
	}, openAIOptions.Include)
}

func TestStreamCallOptionsFromChatModelConfig_DefaultsAndLegacyOpenAI(t *testing.T) {
	t.Parallel()

	parallelToolCalls := true
	streamCall := streamCallOptionsFromChatModelConfig(
		&titleTestModel{provider: fantasyopenai.Name, model: "gpt-legacy-non-responses"},
		chatModelConfig{
			ChatModelCallConfig: codersdk.ChatModelCallConfig{
				ProviderOptions: &codersdk.ChatModelProviderOptions{
					OpenAI: &codersdk.ChatModelOpenAIProviderOptions{
						ParallelToolCalls: &parallelToolCalls,
					},
				},
			},
		},
	)

	require.NotNil(t, streamCall.MaxOutputTokens)
	require.Equal(t, int64(32000), *streamCall.MaxOutputTokens)
	openAIOptionsAny, ok := streamCall.ProviderOptions[fantasyopenai.Name]
	require.True(t, ok)
	_, ok = openAIOptionsAny.(*fantasyopenai.ProviderOptions)
	require.True(t, ok)
}

func TestStreamCallOptionsFromChatModelConfig_OpenAIResponses_ForceEncryptedReasoningInclude(t *testing.T) {
	t.Parallel()

	streamCall := streamCallOptionsFromChatModelConfig(
		&titleTestModel{provider: fantasyopenai.Name, model: "gpt-5.2"},
		chatModelConfig{
			ChatModelCallConfig: codersdk.ChatModelCallConfig{
				ProviderOptions: &codersdk.ChatModelProviderOptions{
					OpenAI: &codersdk.ChatModelOpenAIProviderOptions{},
				},
			},
		},
	)

	openAIOptionsAny, ok := streamCall.ProviderOptions[fantasyopenai.Name]
	require.True(t, ok)
	openAIOptions, ok := openAIOptionsAny.(*fantasyopenai.ResponsesProviderOptions)
	require.True(t, ok)
	require.Equal(t, []fantasyopenai.IncludeType{
		fantasyopenai.IncludeReasoningEncryptedContent,
	}, openAIOptions.Include)
}

func TestAnyAvailableModel(t *testing.T) {
	t.Parallel()

	t.Run("OpenAIOnly", func(t *testing.T) {
		t.Parallel()

		model, err := anyAvailableModel(chatprovider.ProviderAPIKeys{OpenAI: "openai-key"})
		require.NoError(t, err)
		require.Equal(t, "openai", model.Provider())
		require.Equal(t, "gpt-4o-mini", model.Model())
	})

	t.Run("None", func(t *testing.T) {
		t.Parallel()

		model, err := anyAvailableModel(chatprovider.ProviderAPIKeys{})
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
			resolveProviderAPIKeysFn: func(context.Context) (chatprovider.ProviderAPIKeys, error) {
				return chatprovider.ProviderAPIKeys{OpenAI: "openai-key"}, nil
			},
			titleGeneration: TitleGenerationConfig{Prompt: "custom title prompt"},
			titleModelLookup: func(chatprovider.ProviderAPIKeys) (fantasy.LanguageModel, error) {
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
			resolveProviderAPIKeysFn: func(context.Context) (chatprovider.ProviderAPIKeys, error) {
				return chatprovider.ProviderAPIKeys{OpenAI: "openai-key"}, nil
			},
			titleModelLookup: func(chatprovider.ProviderAPIKeys) (fantasy.LanguageModel, error) {
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
			resolveProviderAPIKeysFn: func(context.Context) (chatprovider.ProviderAPIKeys, error) {
				return chatprovider.ProviderAPIKeys{OpenAI: "openai-key"}, nil
			},
			titleModelLookup: func(chatprovider.ProviderAPIKeys) (fantasy.LanguageModel, error) {
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
			resolveProviderAPIKeysFn: func(context.Context) (chatprovider.ProviderAPIKeys, error) {
				return chatprovider.ProviderAPIKeys{OpenAI: "openai-key"}, nil
			},
			titleModelLookup: func(chatprovider.ProviderAPIKeys) (fantasy.LanguageModel, error) {
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
			resolveProviderAPIKeysFn: func(context.Context) (chatprovider.ProviderAPIKeys, error) {
				return chatprovider.ProviderAPIKeys{OpenAI: "openai-key"}, nil
			},
			titleModelLookup: func(chatprovider.ProviderAPIKeys) (fantasy.LanguageModel, error) {
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

type titleTestModel struct {
	generateFn func(context.Context, fantasy.Call) (*fantasy.Response, error)
	provider   string
	model      string
}

func (m *titleTestModel) Provider() string {
	if m.provider != "" {
		return m.provider
	}
	return "fake"
}

func (m *titleTestModel) Model() string {
	if m.model != "" {
		return m.model
	}
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
