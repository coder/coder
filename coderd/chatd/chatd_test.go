package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestMergeProviderAPIKeys(t *testing.T) {
	t.Parallel()

	merged := MergeProviderAPIKeys(
		ProviderAPIKeys{
			OpenAI:    " deployment-openai ",
			Anthropic: "deployment-anthropic",
			ByProvider: map[string]string{
				"openrouter": " deployment-openrouter ",
			},
		},
		[]ConfiguredProvider{
			{Provider: "openai", APIKey: "   "},
			{Provider: "anthropic", APIKey: " provider-anthropic "},
			{Provider: "openrouter", APIKey: "provider-openrouter"},
		},
	)

	require.Equal(t, "deployment-openai", merged.OpenAI)
	require.Equal(t, "provider-anthropic", merged.Anthropic)
	require.Equal(t, "provider-openrouter", merged.APIKey("openrouter"))
}

func TestModelCatalogListConfiguredModels_UsesFallbackAPIKeys(t *testing.T) {
	t.Parallel()

	catalog := NewModelCatalog(
		testutil.Logger(t),
		nil,
		ProviderAPIKeys{
			OpenAI: "deployment-openai",
		},
		ModelCatalogConfig{},
	)

	response, ok := catalog.ListConfiguredModels(
		[]ConfiguredProvider{
			{Provider: "openai", APIKey: "   "},
		},
		[]ConfiguredModel{
			{
				Provider:    "openai",
				Model:       "gpt-5.2",
				DisplayName: "GPT 5.2",
			},
		},
	)
	require.True(t, ok)
	require.Len(t, response.Providers, 1)

	provider := response.Providers[0]
	require.Equal(t, "openai", provider.Provider)
	require.True(t, provider.Available)
	require.Empty(t, provider.UnavailableReason)
	require.Equal(
		t,
		[]codersdk.ChatModel{{
			ID:          "openai:gpt-5.2",
			Provider:    "openai",
			Model:       "gpt-5.2",
			DisplayName: "GPT 5.2",
		}},
		provider.Models,
	)
}

func TestModelCatalogListConfiguredModels_NoEnabledModels(t *testing.T) {
	t.Parallel()

	catalog := NewModelCatalog(
		testutil.Logger(t),
		nil,
		ProviderAPIKeys{
			OpenAI: "deployment-openai",
		},
		ModelCatalogConfig{},
	)

	response, ok := catalog.ListConfiguredModels(
		[]ConfiguredProvider{
			{Provider: "openai", APIKey: ""},
		},
		nil,
	)
	require.False(t, ok)
	require.Equal(t, codersdk.ChatModelsResponse{}, response)
}

func TestNormalizeProviderSupportsFantasyProviders(t *testing.T) {
	t.Parallel()

	require.Equal(t, []string{
		"anthropic",
		"azure",
		"bedrock",
		"google",
		"openai",
		"openai-compat",
		"openrouter",
		"vercel",
	}, SupportedProviders())

	for _, provider := range SupportedProviders() {
		require.Equal(t, provider, NormalizeProvider(provider))
		require.Equal(t, provider, NormalizeProvider(strings.ToUpper(provider)))
	}
}

func TestModelCatalogListConfiguredProviderAvailability_AllSupported(t *testing.T) {
	t.Parallel()

	catalog := NewModelCatalog(
		testutil.Logger(t),
		nil,
		ProviderAPIKeys{
			OpenAI: "deployment-openai",
		},
		ModelCatalogConfig{},
	)

	response := catalog.ListConfiguredProviderAvailability(
		[]ConfiguredProvider{
			{Provider: "openrouter", APIKey: "openrouter-key"},
		},
	)
	require.Len(t, response.Providers, len(SupportedProviders()))

	availability := make(map[string]codersdk.ChatModelProvider, len(response.Providers))
	for _, provider := range response.Providers {
		availability[provider.Provider] = provider
	}

	require.True(t, availability["openai"].Available)
	require.True(t, availability["openrouter"].Available)
	require.False(t, availability["anthropic"].Available)
}

func TestModelFromConfig_OpenRouter(t *testing.T) {
	t.Parallel()

	model, err := modelFromConfig(
		chatModelConfig{
			Provider: "openrouter",
			Model:    "gpt-4o-mini",
		},
		ProviderAPIKeys{
			ByProvider: map[string]string{
				"openrouter": "openrouter-key",
			},
		},
	)
	require.NoError(t, err)
	require.Equal(t, "openrouter", model.Provider())
	require.Equal(t, "gpt-4o-mini", model.Model())
}

func TestModelFromConfig_AzureRequiresBaseURL(t *testing.T) {
	t.Parallel()

	_, err := modelFromConfig(
		chatModelConfig{
			Provider: "azure",
			Model:    "gpt-4o-mini",
		},
		ProviderAPIKeys{
			ByProvider: map[string]string{
				"azure": "azure-key",
			},
		},
	)
	require.EqualError(
		t,
		err,
		"azure provider requires a base URL, but chat provider configs do not support base URLs yet",
	)
}

// Consolidated from conversion_test.go.
func TestChatMessagesToPrompt(t *testing.T) {
	t.Parallel()

	systemContent, err := json.Marshal("system")
	require.NoError(t, err)

	userContent, err := json.Marshal(contentFromText("hello"))
	require.NoError(t, err)

	assistantBlocks := append(contentFromText("working"), fantasy.ToolCallContent{
		ToolCallID: "tool-1",
		ToolName:   toolReadFile,
		Input:      `{"path":"hello.txt"}`,
	})
	assistantContent, err := json.Marshal(assistantBlocks)
	require.NoError(t, err)

	toolResults, err := json.Marshal([]ToolResultBlock{{
		ToolCallID: "tool-1",
		ToolName:   toolReadFile,
		Result:     map[string]any{"content": "hello"},
	}})
	require.NoError(t, err)

	messages := []database.ChatMessage{
		{
			Role:    string(fantasy.MessageRoleSystem),
			Content: pqtype.NullRawMessage{RawMessage: systemContent, Valid: true},
		},
		{
			Role:    string(fantasy.MessageRoleUser),
			Content: pqtype.NullRawMessage{RawMessage: userContent, Valid: true},
		},
		{
			Role:    string(fantasy.MessageRoleAssistant),
			Content: pqtype.NullRawMessage{RawMessage: assistantContent, Valid: true},
		},
		{
			Role:    string(fantasy.MessageRoleTool),
			Content: pqtype.NullRawMessage{RawMessage: toolResults, Valid: true},
		},
	}

	prompt, err := chatMessagesToPrompt(messages)
	require.NoError(t, err)
	require.Len(t, prompt, 4)
	require.Equal(t, fantasy.MessageRoleAssistant, prompt[2].Role)
	require.Len(t, extractToolCallsFromMessageParts(prompt[2].Content), 1)
}

func TestChatMessagesToPrompt_SanitizesToolCallIDs(t *testing.T) {
	t.Parallel()

	const (
		legacyToolCallID    = "subagent_report:123e4567-e89b-12d3-a456-426614174000"
		sanitizedToolCallID = "subagent_report_123e4567-e89b-12d3-a456-426614174000"
		compliantToolCallID = "subagent_report_123e4567-e89b-12d3-a456-426614174000"
	)

	tests := []struct {
		name       string
		toolCallID string
		wantID     string
	}{
		{
			name:       "LegacyInvalidID",
			toolCallID: legacyToolCallID,
			wantID:     sanitizedToolCallID,
		},
		{
			name:       "AlreadyCompliantID",
			toolCallID: compliantToolCallID,
			wantID:     compliantToolCallID,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assistantBlocks := append(contentFromText("working"), fantasy.ToolCallContent{
				ToolCallID: tt.toolCallID,
				ToolName:   toolReadFile,
				Input:      `{"path":"hello.txt"}`,
			})
			assistantContent, err := json.Marshal(assistantBlocks)
			require.NoError(t, err)

			toolResults, err := json.Marshal([]ToolResultBlock{{
				ToolCallID: tt.toolCallID,
				ToolName:   toolReadFile,
				Result:     map[string]any{"content": "hello"},
			}})
			require.NoError(t, err)

			messages := []database.ChatMessage{
				{
					Role:    string(fantasy.MessageRoleAssistant),
					Content: pqtype.NullRawMessage{RawMessage: assistantContent, Valid: true},
				},
				{
					Role:    string(fantasy.MessageRoleTool),
					Content: pqtype.NullRawMessage{RawMessage: toolResults, Valid: true},
				},
			}

			prompt, err := chatMessagesToPrompt(messages)
			require.NoError(t, err)
			require.Len(t, prompt, 2)

			assistantToolCalls := extractToolCallsFromMessageParts(prompt[0].Content)
			require.Len(t, assistantToolCalls, 1)
			require.Equal(t, tt.wantID, assistantToolCalls[0].ToolCallID)

			toolResultParts := messageToolResultParts(prompt[1])
			require.Len(t, toolResultParts, 1)
			require.Equal(t, tt.wantID, toolResultParts[0].ToolCallID)

			require.Contains(t, string(messages[0].Content.RawMessage), tt.toolCallID)
			require.Contains(t, string(messages[1].Content.RawMessage), tt.toolCallID)
		})
	}
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
		ToolName:   toolSubagentReport,
		Result: map[string]any{
			"chat_id":    uuid.NewString(),
			"request_id": uuid.NewString(),
			"report":     "done",
			"status":     "reported",
		},
	}})
	require.NoError(t, err)

	messages := []database.ChatMessage{
		{
			Role:    string(fantasy.MessageRoleUser),
			Content: pqtype.NullRawMessage{RawMessage: userContent, Valid: true},
		},
		{
			Role:    string(fantasy.MessageRoleTool),
			Content: pqtype.NullRawMessage{RawMessage: toolResults, Valid: true},
		},
	}

	prompt, err := chatMessagesToPrompt(messages)
	require.NoError(t, err)
	require.Len(t, prompt, 3)
	require.Equal(t, fantasy.MessageRoleAssistant, prompt[1].Role)
	require.Equal(t, fantasy.MessageRoleTool, prompt[2].Role)

	toolCalls := extractToolCallsFromMessageParts(prompt[1].Content)
	require.Len(t, toolCalls, 1)
	require.Equal(t, sanitizedToolCallID, toolCalls[0].ToolCallID)
	require.Equal(t, toolSubagentReport, toolCalls[0].ToolName)

	toolResultParts := messageToolResultParts(prompt[2])
	require.Len(t, toolResultParts, 1)
	require.Equal(t, sanitizedToolCallID, toolResultParts[0].ToolCallID)
}

func TestChatMessagesToPrompt_InjectsMissingToolResults(t *testing.T) {
	t.Parallel()

	t.Run("InterruptedAfterToolCall", func(t *testing.T) {
		t.Parallel()

		// Simulate an interrupted chat: assistant made tool calls but
		// the processing was interrupted before tool results were saved.
		userContent, err := json.Marshal(contentFromText("hello"))
		require.NoError(t, err)

		assistantBlocks := append(contentFromText("let me check"),
			fantasy.ToolCallContent{
				ToolCallID: "call-1",
				ToolName:   toolReadFile,
				Input:      `{"path":"main.go"}`,
			},
			fantasy.ToolCallContent{
				ToolCallID: "call-2",
				ToolName:   toolExecute,
				Input:      `{"command":"ls"}`,
			},
		)
		assistantContent, err := json.Marshal(assistantBlocks)
		require.NoError(t, err)

		messages := []database.ChatMessage{
			{
				Role:    string(fantasy.MessageRoleUser),
				Content: pqtype.NullRawMessage{RawMessage: userContent, Valid: true},
			},
			{
				Role:    string(fantasy.MessageRoleAssistant),
				Content: pqtype.NullRawMessage{RawMessage: assistantContent, Valid: true},
			},
		}

		prompt, err := chatMessagesToPrompt(messages)
		require.NoError(t, err)

		// Should have injected a tool message after the assistant.
		require.Len(t, prompt, 3, "expected injected tool result message")

		toolMsg := prompt[2]
		require.Equal(t, fantasy.MessageRoleTool, toolMsg.Role)
		toolResults := messageToolResultParts(toolMsg)
		require.Len(t, toolResults, 2, "should have results for both tool calls")

		for _, result := range toolResults {
			_, ok := result.Output.(fantasy.ToolResultOutputContentError)
			require.True(t, ok, "injected result should be an error")
		}
		require.Equal(t, "call-1", toolResults[0].ToolCallID)
		require.Equal(t, "call-2", toolResults[1].ToolCallID)
	})

	t.Run("PartialToolResults", func(t *testing.T) {
		t.Parallel()

		// Assistant made two tool calls but only one result was saved
		// before interruption.
		userContent, err := json.Marshal(contentFromText("hello"))
		require.NoError(t, err)

		assistantBlocks := append(contentFromText("working"),
			fantasy.ToolCallContent{
				ToolCallID: "call-1",
				ToolName:   toolReadFile,
				Input:      `{"path":"a.go"}`,
			},
			fantasy.ToolCallContent{
				ToolCallID: "call-2",
				ToolName:   toolReadFile,
				Input:      `{"path":"b.go"}`,
			},
		)
		assistantContent, err := json.Marshal(assistantBlocks)
		require.NoError(t, err)

		toolResults, err := json.Marshal([]ToolResultBlock{{
			ToolCallID: "call-1",
			ToolName:   toolReadFile,
			Result:     map[string]any{"content": "file a"},
		}})
		require.NoError(t, err)

		messages := []database.ChatMessage{
			{
				Role:    string(fantasy.MessageRoleUser),
				Content: pqtype.NullRawMessage{RawMessage: userContent, Valid: true},
			},
			{
				Role:    string(fantasy.MessageRoleAssistant),
				Content: pqtype.NullRawMessage{RawMessage: assistantContent, Valid: true},
			},
			{
				Role:    string(fantasy.MessageRoleTool),
				Content: pqtype.NullRawMessage{RawMessage: toolResults, Valid: true},
			},
		}

		prompt, err := chatMessagesToPrompt(messages)
		require.NoError(t, err)

		// Original 3 messages + synthetic tool result for call-2.
		// injectMissingToolResults sees that call-2 has no result
		// and appends a synthetic error result. No extra assistant
		// message is needed because the original assistant already
		// contains both tool_use blocks.
		require.Len(t, prompt, 4)

		// First tool message carries the real call-1 result.
		firstToolMsg := prompt[2]
		require.Equal(t, fantasy.MessageRoleTool, firstToolMsg.Role)
		firstParts := messageToolResultParts(firstToolMsg)
		require.Len(t, firstParts, 1)
		require.Equal(t, "call-1", firstParts[0].ToolCallID)

		// Second tool message is the synthetic interrupted result
		// for call-2.
		injectedMsg := prompt[3]
		require.Equal(t, fantasy.MessageRoleTool, injectedMsg.Role)
		injectedParts := messageToolResultParts(injectedMsg)
		require.Len(t, injectedParts, 1)
		require.Equal(t, "call-2", injectedParts[0].ToolCallID)
		_, ok := injectedParts[0].Output.(fantasy.ToolResultOutputContentError)
		require.True(t, ok)
	})

	t.Run("NoToolCalls", func(t *testing.T) {
		t.Parallel()

		// Assistant message with no tool calls should not inject anything.
		userContent, err := json.Marshal(contentFromText("hi"))
		require.NoError(t, err)

		assistantContent, err := json.Marshal(contentFromText("hello back"))
		require.NoError(t, err)

		messages := []database.ChatMessage{
			{
				Role:    string(fantasy.MessageRoleUser),
				Content: pqtype.NullRawMessage{RawMessage: userContent, Valid: true},
			},
			{
				Role:    string(fantasy.MessageRoleAssistant),
				Content: pqtype.NullRawMessage{RawMessage: assistantContent, Valid: true},
			},
		}

		prompt, err := chatMessagesToPrompt(messages)
		require.NoError(t, err)
		require.Len(t, prompt, 2, "no injection expected when no tool calls")
	})

	t.Run("AllToolResultsPresent", func(t *testing.T) {
		t.Parallel()

		// All tool calls already have results; nothing to inject.
		userContent, err := json.Marshal(contentFromText("hello"))
		require.NoError(t, err)

		assistantBlocks := append(contentFromText("working"), fantasy.ToolCallContent{
			ToolCallID: "call-1",
			ToolName:   toolReadFile,
			Input:      `{"path":"x.go"}`,
		})
		assistantContent, err := json.Marshal(assistantBlocks)
		require.NoError(t, err)

		toolResults, err := json.Marshal([]ToolResultBlock{{
			ToolCallID: "call-1",
			ToolName:   toolReadFile,
			Result:     map[string]any{"content": "data"},
		}})
		require.NoError(t, err)

		messages := []database.ChatMessage{
			{
				Role:    string(fantasy.MessageRoleUser),
				Content: pqtype.NullRawMessage{RawMessage: userContent, Valid: true},
			},
			{
				Role:    string(fantasy.MessageRoleAssistant),
				Content: pqtype.NullRawMessage{RawMessage: assistantContent, Valid: true},
			},
			{
				Role:    string(fantasy.MessageRoleTool),
				Content: pqtype.NullRawMessage{RawMessage: toolResults, Valid: true},
			},
		}

		prompt, err := chatMessagesToPrompt(messages)
		require.NoError(t, err)
		require.Len(t, prompt, 3, "no injection when all results present")
	})
}
func TestChatMessagesToPrompt_SeparateToolResults(t *testing.T) {
	t.Parallel()

	// Reproduce the exact pattern from production: an assistant message
	// with 6 tool calls, followed by 6 SEPARATE tool result messages
	// (one per result, as the code persists them), then another assistant
	// with 1 tool call and 1 tool result.
	userContent, err := json.Marshal(contentFromText("analyze the repo"))
	require.NoError(t, err)

	// First assistant: 6 tool calls.
	assistantBlocks1 := append(contentFromText("I'll read files in parallel."),
		fantasy.ToolCallContent{ToolCallID: "call-1", ToolName: toolReadFile, Input: `{"path":"a.txt"}`},
		fantasy.ToolCallContent{ToolCallID: "call-2", ToolName: toolReadFile, Input: `{"path":"b.txt"}`},
		fantasy.ToolCallContent{ToolCallID: "call-3", ToolName: toolReadFile, Input: `{"path":"c.txt"}`},
		fantasy.ToolCallContent{ToolCallID: "call-4", ToolName: toolExecute, Input: `{"command":"ls"}`},
		fantasy.ToolCallContent{ToolCallID: "call-5", ToolName: toolExecute, Input: `{"command":"pwd"}`},
		fantasy.ToolCallContent{ToolCallID: "call-6", ToolName: toolReadFile, Input: `{"path":"d.txt"}`},
	)
	assistantContent1, err := json.Marshal(assistantBlocks1)
	require.NoError(t, err)

	// 6 separate tool result messages (one per tool call).
	makeToolResult := func(callID, toolName string, isError bool, result map[string]any) pqtype.NullRawMessage {
		data, err := json.Marshal([]ToolResultBlock{{
			ToolCallID: callID,
			ToolName:   toolName,
			Result:     result,
			IsError:    isError,
		}})
		require.NoError(t, err)
		return pqtype.NullRawMessage{RawMessage: data, Valid: true}
	}

	// Second assistant: 1 tool call.
	assistantBlocks2 := append(contentFromText("Let me check more."),
		fantasy.ToolCallContent{ToolCallID: "call-7", ToolName: toolExecute, Input: `{"command":"cat x"}`},
	)
	assistantContent2, err := json.Marshal(assistantBlocks2)
	require.NoError(t, err)

	messages := []database.ChatMessage{
		{Role: string(fantasy.MessageRoleUser), Content: pqtype.NullRawMessage{RawMessage: userContent, Valid: true}},
		{Role: string(fantasy.MessageRoleAssistant), Content: pqtype.NullRawMessage{RawMessage: assistantContent1, Valid: true}},
		{Role: string(fantasy.MessageRoleTool), Content: makeToolResult("call-1", toolReadFile, true, map[string]any{"error": "not found"})},
		{Role: string(fantasy.MessageRoleTool), Content: makeToolResult("call-2", toolReadFile, true, map[string]any{"error": "not found"})},
		{Role: string(fantasy.MessageRoleTool), Content: makeToolResult("call-3", toolReadFile, true, map[string]any{"error": "not found"})},
		{Role: string(fantasy.MessageRoleTool), Content: makeToolResult("call-4", toolExecute, false, map[string]any{"output": "file1\nfile2"})},
		{Role: string(fantasy.MessageRoleTool), Content: makeToolResult("call-5", toolExecute, false, map[string]any{"output": "/home"})},
		{Role: string(fantasy.MessageRoleTool), Content: makeToolResult("call-6", toolReadFile, true, map[string]any{"error": "not found"})},
		{Role: string(fantasy.MessageRoleAssistant), Content: pqtype.NullRawMessage{RawMessage: assistantContent2, Valid: true}},
		{Role: string(fantasy.MessageRoleTool), Content: makeToolResult("call-7", toolExecute, false, map[string]any{"output": "data"})},
	}

	prompt, err := chatMessagesToPrompt(messages)
	require.NoError(t, err)

	// Should have: user, assistant(6 calls), 6 tool msgs, assistant(1 call), 1 tool msg = 10 messages.
	require.Len(t, prompt, 10, "all messages should be present")

	// Verify structure.
	require.Equal(t, fantasy.MessageRoleUser, prompt[0].Role)
	require.Equal(t, fantasy.MessageRoleAssistant, prompt[1].Role)
	require.Len(t, extractToolCallsFromMessageParts(prompt[1].Content), 6)

	// Check all 6 tool results are present and have correct IDs.
	for i := 2; i <= 7; i++ {
		require.Equal(t, fantasy.MessageRoleTool, prompt[i].Role, "prompt[%d] should be tool", i)
		results := messageToolResultParts(prompt[i])
		require.Len(t, results, 1, "prompt[%d] should have 1 tool result", i)
	}

	// Collect all tool result IDs.
	answered := make(map[string]struct{})
	for i := 2; i <= 7; i++ {
		results := messageToolResultParts(prompt[i])
		answered[results[0].ToolCallID] = struct{}{}
	}
	for j := 1; j <= 6; j++ {
		_, ok := answered[fmt.Sprintf("call-%d", j)]
		require.True(t, ok, "call-%d should have a result", j)
	}

	require.Equal(t, fantasy.MessageRoleAssistant, prompt[8].Role)
	require.Len(t, extractToolCallsFromMessageParts(prompt[8].Content), 1)

	require.Equal(t, fantasy.MessageRoleTool, prompt[9].Role)
	results9 := messageToolResultParts(prompt[9])
	require.Len(t, results9, 1)
	require.Equal(t, "call-7", results9[0].ToolCallID)
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

// Consolidated from report_test.go.
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

	require.Equal(t, []string{toolSubagentReport}, result.ActiveTools)
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

func TestPrepareAgentStepResult_NonAnthropicUnchanged(t *testing.T) {
	t.Parallel()

	result := prepareAgentStepResult(
		[]fantasy.Message{
			textMessage(fantasy.MessageRoleSystem, "sys"),
			textMessage(fantasy.MessageRoleUser, "hello"),
			textMessage(fantasy.MessageRoleAssistant, "working"),
		},
		"__sentinel__",
		false,
		false,
	)

	require.Len(t, result.Messages, 3)
	for _, message := range result.Messages {
		require.Nil(t, message.ProviderOptions)
	}
}

func TestPrepareAgentStepResult_AnthropicCachingWithoutSystemMessage(t *testing.T) {
	t.Parallel()

	result := prepareAgentStepResult(
		[]fantasy.Message{
			textMessage(fantasy.MessageRoleUser, "first"),
			textMessage(fantasy.MessageRoleAssistant, "second"),
			textMessage(fantasy.MessageRoleUser, "third"),
		},
		"__sentinel__",
		false,
		true,
	)

	require.Len(t, result.Messages, 3)
	require.False(t, hasAnthropicEphemeralCacheControl(result.Messages[0]))
	require.True(t, hasAnthropicEphemeralCacheControl(result.Messages[1]))
	require.True(t, hasAnthropicEphemeralCacheControl(result.Messages[2]))
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

// Consolidated from schema_test.go.
func TestSchemaMap_NormalizesRequiredArrays(t *testing.T) {
	t.Parallel()

	schema := &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"workspace": {
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"name": {Type: "string"},
					"files": {
						Type: "array",
						Items: &jsonschema.Schema{
							Type: "object",
							Properties: map[string]*jsonschema.Schema{
								"path":    {Type: "string"},
								"content": {Type: "string"},
							},
							Required: []string{"path", "content"},
						},
					},
				},
				Required: []string{"name", "files"},
			},
		},
		Required: []string{"workspace"},
	}

	mapped := schemaMap(schema)
	assertRequiredArraysUseStringSlices(t, mapped, "$")

	properties := mapValue(t, mapped["properties"], "$.properties")
	workspace := mapValue(t, properties["workspace"], "$.properties.workspace")
	workspaceProperties := mapValue(t, workspace["properties"], "$.properties.workspace.properties")
	files := mapValue(t, workspaceProperties["files"], "$.properties.workspace.properties.files")
	items := mapValue(t, files["items"], "$.properties.workspace.properties.files.items")

	require.Equal(t, []string{"workspace"}, requiredStrings(t, mapped, "$"))
	require.Equal(t, []string{"name", "files"}, requiredStrings(t, workspace, "$.properties.workspace"))
	require.Equal(t, []string{"path", "content"}, requiredStrings(t, items, "$.properties.workspace.properties.files.items"))
}

func TestNormalizeRequiredArrays_ConvertsEmptyRequiredArray(t *testing.T) {
	t.Parallel()

	input := map[string]any{
		"type":     "object",
		"required": []any{},
		"properties": map[string]any{
			"nested": map[string]any{
				"type":     "object",
				"required": []any{"name"},
			},
		},
	}

	normalizeRequiredArrays(input)

	require.Equal(t, []string{}, requiredStrings(t, input, "$"))

	properties := mapValue(t, input["properties"], "$.properties")
	nested := mapValue(t, properties["nested"], "$.properties.nested")
	require.Equal(t, []string{"name"}, requiredStrings(t, nested, "$.properties.nested"))
}

func assertRequiredArraysUseStringSlices(t *testing.T, value any, path string) {
	t.Helper()

	switch v := value.(type) {
	case map[string]any:
		if required, ok := v["required"]; ok {
			_, isStringSlice := required.([]string)
			require.Truef(t, isStringSlice, "required at %s has type %T", path, required)
		}
		for key, child := range v {
			assertRequiredArraysUseStringSlices(t, child, path+"."+key)
		}
	case []any:
		for i, child := range v {
			assertRequiredArraysUseStringSlices(t, child, fmt.Sprintf("%s[%d]", path, i))
		}
	}
}

func mapValue(t *testing.T, value any, path string) map[string]any {
	t.Helper()

	m, ok := value.(map[string]any)
	require.True(t, ok, "value at %s has unexpected type %T", path, value)
	return m
}

func requiredStrings(t *testing.T, schema map[string]any, path string) []string {
	t.Helper()

	required, ok := schema["required"].([]string)
	require.True(t, ok, "required at %s has unexpected type %T", path, schema["required"])
	return required
}

func schemaMap(schema *jsonschema.Schema) map[string]any {
	data, err := json.Marshal(schema)
	if err != nil {
		return map[string]any{}
	}

	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return map[string]any{}
	}
	normalizeRequiredArrays(out)
	return out
}

func normalizeRequiredArrays(value any) {
	switch v := value.(type) {
	case map[string]any:
		normalizeMap(v)
	case []any:
		for _, item := range v {
			normalizeRequiredArrays(item)
		}
	}
}

func normalizeMap(m map[string]any) {
	if req, ok := m["required"]; ok {
		if arr, ok := req.([]any); ok {
			converted := make([]string, 0, len(arr))
			for _, item := range arr {
				s, isString := item.(string)
				if !isString {
					converted = nil
					break
				}
				converted = append(converted, s)
			}
			if converted != nil {
				m["required"] = converted
			}
		}
	}
	for _, v := range m {
		normalizeRequiredArrays(v)
	}
}

// Consolidated from stream_test.go.
func TestSDKChatMessage_ToolResultPartMetadata(t *testing.T) {
	t.Parallel()

	content, err := marshalToolResults([]ToolResultBlock{{
		ToolCallID: "call-3",
		ToolName:   toolExecute,
		Result: map[string]any{
			"output":    "completed",
			"exit_code": 17,
		},
	}})
	require.NoError(t, err)

	message := SDKChatMessage(database.ChatMessage{
		ID:        42,
		ChatID:    uuid.New(),
		CreatedAt: time.Now(),
		Role:      string(fantasy.MessageRoleTool),
		Content:   content,
		ToolCallID: sql.NullString{
			String: "call-3",
			Valid:  true,
		},
	})

	require.Len(t, message.Parts, 1)
	part := message.Parts[0]
	require.Equal(t, codersdk.ChatMessagePartTypeToolResult, part.Type)
	require.Equal(t, "call-3", part.ToolCallID)
	require.Equal(t, toolExecute, part.ToolName)
	require.NotEmpty(t, part.Result)
	require.NotNil(t, part.ResultMeta)
	require.Equal(t, "completed", part.ResultMeta.Output)
	require.NotNil(t, part.ResultMeta.ExitCode)
	require.Equal(t, 17, *part.ResultMeta.ExitCode)
}

func TestSDKChatMessage_IncludesUsageFields(t *testing.T) {
	t.Parallel()

	message := SDKChatMessage(database.ChatMessage{
		ID:        99,
		ChatID:    uuid.New(),
		CreatedAt: time.Now(),
		Role:      string(fantasy.MessageRoleAssistant),
		InputTokens: sql.NullInt64{
			Int64: 101,
			Valid: true,
		},
		OutputTokens: sql.NullInt64{
			Int64: 37,
			Valid: true,
		},
		ReasoningTokens: sql.NullInt64{
			Int64: 11,
			Valid: true,
		},
		CacheCreationTokens: sql.NullInt64{
			Int64: 5,
			Valid: true,
		},
		CacheReadTokens: sql.NullInt64{
			Int64: 2,
			Valid: true,
		},
		TotalTokens: sql.NullInt64{
			Int64: 138,
			Valid: true,
		},
		ContextLimit: sql.NullInt64{
			Int64: 200000,
			Valid: true,
		},
	})

	require.NotNil(t, message.InputTokens)
	require.Equal(t, int64(101), *message.InputTokens)
	require.NotNil(t, message.OutputTokens)
	require.Equal(t, int64(37), *message.OutputTokens)
	require.NotNil(t, message.ReasoningTokens)
	require.Equal(t, int64(11), *message.ReasoningTokens)
	require.NotNil(t, message.CacheCreationTokens)
	require.Equal(t, int64(5), *message.CacheCreationTokens)
	require.NotNil(t, message.CacheReadTokens)
	require.Equal(t, int64(2), *message.CacheReadTokens)
	require.NotNil(t, message.TotalTokens)
	require.Equal(t, int64(138), *message.TotalTokens)
	require.NotNil(t, message.ContextLimit)
	require.Equal(t, int64(200000), *message.ContextLimit)
}

func TestStreamManager_SnapshotBuffersOnlyMessageParts(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	manager := NewStreamManager(testutil.Logger(t))
	manager.StartStream(chatID)
	manager.Publish(chatID, codersdk.ChatStreamEvent{
		Type: codersdk.ChatStreamEventTypeStatus,
		Status: &codersdk.ChatStreamStatus{
			Status: codersdk.ChatStatusRunning,
		},
	})
	manager.Publish(chatID, codersdk.ChatStreamEvent{
		Type: codersdk.ChatStreamEventTypeMessagePart,
		MessagePart: &codersdk.ChatStreamMessagePart{
			Role: string(fantasy.MessageRoleAssistant),
			Part: codersdk.ChatMessagePart{
				Type: codersdk.ChatMessagePartTypeText,
				Text: "chunk",
			},
		},
	})
	manager.Publish(chatID, codersdk.ChatStreamEvent{
		Type: codersdk.ChatStreamEventTypeMessage,
		Message: &codersdk.ChatMessage{
			ID: 1,
		},
	})

	snapshot, _, cancel := manager.Subscribe(chatID)
	defer cancel()

	require.Len(t, snapshot, 1)
	require.Equal(t, codersdk.ChatStreamEventTypeMessagePart, snapshot[0].Type)
	require.NotNil(t, snapshot[0].MessagePart)
	require.Equal(t, "chunk", snapshot[0].MessagePart.Part.Text)
}

func TestToolResultMetadata_ReadFileFields(t *testing.T) {
	t.Parallel()

	raw, err := json.Marshal([]ToolResultBlock{{
		ToolCallID: "call-4",
		ToolName:   toolReadFile,
		Result: map[string]any{
			"content":   "hello",
			"mime_type": "text/plain",
		},
	}})
	require.NoError(t, err)

	message := SDKChatMessage(database.ChatMessage{
		Role: string(fantasy.MessageRoleTool),
		Content: pqtype.NullRawMessage{
			RawMessage: raw,
			Valid:      true,
		},
	})
	require.Len(t, message.Parts, 1)
	require.NotNil(t, message.Parts[0].ResultMeta)
	require.Equal(t, "hello", message.Parts[0].ResultMeta.Content)
	require.Equal(t, "text/plain", message.Parts[0].ResultMeta.MimeType)
}

// Consolidated from title_test.go.
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

func TestAnyAvailableModel(t *testing.T) {
	t.Parallel()

	t.Run("OpenAIOnly", func(t *testing.T) {
		t.Parallel()

		model, err := anyAvailableModel(ProviderAPIKeys{OpenAI: "openai-key"})
		require.NoError(t, err)
		require.Equal(t, "openai", model.Provider())
		require.Equal(t, "gpt-4o-mini", model.Model())
	})

	t.Run("AnthropicOnly", func(t *testing.T) {
		t.Parallel()

		model, err := anyAvailableModel(ProviderAPIKeys{Anthropic: "anthropic-key"})
		require.NoError(t, err)
		require.Equal(t, "anthropic", model.Provider())
		require.Equal(t, "claude-haiku-4-5", model.Model())
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

		var capturedPrompt []fantasy.Message
		var capturedToolChoice *fantasy.ToolChoice
		model := &titleTestModel{
			generateFn: func(_ context.Context, call fantasy.Call) (*fantasy.Response, error) {
				capturedPrompt = append([]fantasy.Message(nil), call.Prompt...)
				capturedToolChoice = call.ToolChoice
				return &fantasy.Response{
					Content: []fantasy.Content{
						fantasy.TextContent{Text: `  " Debugging   Flaky   Go Tests "  `},
					},
				}, nil
			},
		}

		p := &Processor{
			providerKeys: ProviderAPIKeys{OpenAI: "openai-key"},
			titleGeneration: TitleGenerationConfig{
				Prompt: "custom title prompt",
			},
			titleModelLookup: func(ProviderAPIKeys) (fantasy.LanguageModel, error) {
				return model, nil
			},
		}

		title, err := p.generateChatTitle(context.Background(), "How can I debug this flaky Go test?")
		require.NoError(t, err)
		require.Equal(t, "Debugging Flaky Go Tests", title)
		require.Len(t, capturedPrompt, 2)
		require.NotNil(t, capturedToolChoice)
		require.Equal(t, fantasy.ToolChoiceNone, *capturedToolChoice)

		require.Equal(t, fantasy.MessageRoleSystem, capturedPrompt[0].Role)
		require.Len(t, capturedPrompt[0].Content, 1)
		systemPart, ok := fantasy.AsMessagePart[fantasy.TextPart](capturedPrompt[0].Content[0])
		require.True(t, ok)
		require.Equal(t, "custom title prompt", systemPart.Text)

		require.Equal(t, fantasy.MessageRoleUser, capturedPrompt[1].Role)
		require.Len(t, capturedPrompt[1].Content, 1)
		userPart, ok := fantasy.AsMessagePart[fantasy.TextPart](capturedPrompt[1].Content[0])
		require.True(t, ok)
		require.Equal(t, "How can I debug this flaky Go test?", userPart.Text)
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
			providerKeys: ProviderAPIKeys{OpenAI: "openai-key"},
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
			providerKeys: ProviderAPIKeys{OpenAI: "openai-key"},
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
	chat := database.Chat{
		ID:    uuid.New(),
		Title: fallbackChatTitle(messageText),
	}
	messages := []database.ChatMessage{mustUserChatMessage(t, messageText)}

	t.Run("UpdatesTitle", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		db.EXPECT().UpdateChatByID(gomock.Any(), database.UpdateChatByIDParams{
			ID:    chat.ID,
			Title: "Debugging Flaky Go Tests",
		}).Return(database.Chat{}, nil)

		p := &Processor{
			db:           db,
			providerKeys: ProviderAPIKeys{OpenAI: "openai-key"},
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
	})

	t.Run("SkipsUpdateOnEmptyTitle", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)

		p := &Processor{
			db:           db,
			providerKeys: ProviderAPIKeys{OpenAI: "openai-key"},
			titleModelLookup: func(ProviderAPIKeys) (fantasy.LanguageModel, error) {
				return &titleTestModel{
					generateFn: func(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
						return &fantasy.Response{
							Content: []fantasy.Content{
								fantasy.TextContent{Text: "   "},
							},
						}, nil
					},
				}, nil
			},
		}

		p.maybeGenerateChatTitle(context.Background(), chat, messages, testutil.Logger(t))
	})

	t.Run("SkipsUpdateOnGenerationError", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)

		p := &Processor{
			db:           db,
			providerKeys: ProviderAPIKeys{OpenAI: "openai-key"},
			titleModelLookup: func(ProviderAPIKeys) (fantasy.LanguageModel, error) {
				return &titleTestModel{
					generateFn: func(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
						return nil, xerrors.New("title model failed")
					},
				}, nil
			},
		}

		p.maybeGenerateChatTitle(context.Background(), chat, messages, testutil.Logger(t))
	})

	t.Run("SkipsUpdateWhenTitleUnchanged", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)

		p := &Processor{
			db:           db,
			providerKeys: ProviderAPIKeys{OpenAI: "openai-key"},
			titleModelLookup: func(ProviderAPIKeys) (fantasy.LanguageModel, error) {
				return &titleTestModel{
					generateFn: func(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
						return &fantasy.Response{
							Content: []fantasy.Content{
								fantasy.TextContent{Text: chat.Title},
							},
						}, nil
					},
				}, nil
			},
		}

		p.maybeGenerateChatTitle(context.Background(), chat, messages, testutil.Logger(t))
	})
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
