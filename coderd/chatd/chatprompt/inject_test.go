package chatprompt_test

import (
	"encoding/json"
	"testing"

	"charm.land/fantasy"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/database"
)

// TestInjectMissingToolResults_SkipsProviderExecuted verifies that
// provider-executed tool calls (e.g. web_search) do not receive
// synthetic error results when their results are missing from the
// contiguous tool messages. This scenario happens when the
// provider-executed result is persisted in a later step.
func TestInjectMissingToolResults_SkipsProviderExecuted(t *testing.T) {
	t.Parallel()

	// Step 1: assistant calls spawn_agent (local) + web_search
	// (provider_executed). Only the local tool has a result.
	assistantContent := mustMarshalContent(t, []fantasy.Content{
		fantasy.ToolCallContent{
			ToolCallID: "toolu_local",
			ToolName:   "spawn_agent",
			Input:      `{"prompt":"test"}`,
		},
		fantasy.ToolCallContent{
			ToolCallID:       "srvtoolu_websearch",
			ToolName:         "web_search",
			Input:            `{"query":"test"}`,
			ProviderExecuted: true,
		},
	})

	localResult := mustMarshalToolResult(t,
		"toolu_local", "spawn_agent",
		json.RawMessage(`{"status":"done"}`),
		false, false,
	)

	prompt, err := chatprompt.ConvertMessages([]database.ChatMessage{
		{
			Role:       "assistant",
			Visibility: database.ChatMessageVisibilityBoth,
			Content:    assistantContent,
		},
		{
			Role:       "tool",
			Visibility: database.ChatMessageVisibilityBoth,
			Content:    localResult,
		},
	})
	require.NoError(t, err)

	// Expected: assistant + tool(local result). No synthetic error
	// for the provider-executed tool call.
	require.Len(t, prompt, 2, "expected assistant + tool, no synthetic error")
	require.Equal(t, fantasy.MessageRoleAssistant, prompt[0].Role)
	require.Equal(t, fantasy.MessageRoleTool, prompt[1].Role)

	// The tool message should have exactly one result (the local one).
	var resultIDs []string
	for _, part := range prompt[1].Content {
		tr, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](part)
		if ok {
			resultIDs = append(resultIDs, tr.ToolCallID)
		}
	}
	require.Equal(t, []string{"toolu_local"}, resultIDs)
}

// TestInjectMissingToolUses_DropsProviderExecutedOrphans verifies that
// provider-executed tool results that end up after the wrong assistant
// message (because they were persisted in a later step) are dropped
// rather than triggering synthetic tool_use injection.
func TestInjectMissingToolUses_DropsProviderExecutedOrphans(t *testing.T) {
	t.Parallel()

	// Step 1: assistant calls spawn_agent x2 + web_search (PE).
	step1Assistant := mustMarshalContent(t, []fantasy.Content{
		fantasy.ToolCallContent{
			ToolCallID: "toolu_A",
			ToolName:   "spawn_agent",
			Input:      `{"prompt":"a"}`,
		},
		fantasy.ToolCallContent{
			ToolCallID: "toolu_B",
			ToolName:   "spawn_agent",
			Input:      `{"prompt":"b"}`,
		},
		fantasy.ToolCallContent{
			ToolCallID:       "srvtoolu_C",
			ToolName:         "web_search",
			Input:            `{"query":"test"}`,
			ProviderExecuted: true,
		},
	})

	resultA := mustMarshalToolResult(t,
		"toolu_A", "spawn_agent",
		json.RawMessage(`{"status":"done"}`),
		false, false,
	)
	resultB := mustMarshalToolResult(t,
		"toolu_B", "spawn_agent",
		json.RawMessage(`{"status":"done"}`),
		false, false,
	)

	// Step 2: assistant with sources/text + wait_agent x2.
	// The web_search result from step 1 ended up here.
	step2Assistant := mustMarshalContent(t, []fantasy.Content{
		fantasy.TextContent{Text: "Here are the results."},
		fantasy.ToolCallContent{
			ToolCallID: "toolu_D",
			ToolName:   "wait_agent",
			Input:      `{"chat_id":"abc"}`,
		},
		fantasy.ToolCallContent{
			ToolCallID: "toolu_E",
			ToolName:   "wait_agent",
			Input:      `{"chat_id":"def"}`,
		},
	})

	// The provider-executed result C is persisted in step 2's batch.
	resultC := mustMarshalToolResult(t,
		"srvtoolu_C", "web_search",
		json.RawMessage(`{}`),
		false, true, // provider_executed = true
	)
	resultD := mustMarshalToolResult(t,
		"toolu_D", "wait_agent",
		json.RawMessage(`{"report":"done"}`),
		false, false,
	)
	resultE := mustMarshalToolResult(t,
		"toolu_E", "wait_agent",
		json.RawMessage(`{"report":"done"}`),
		false, false,
	)

	prompt, err := chatprompt.ConvertMessages([]database.ChatMessage{
		// Step 1
		{Role: "assistant", Visibility: database.ChatMessageVisibilityBoth, Content: step1Assistant},
		{Role: "tool", Visibility: database.ChatMessageVisibilityBoth, Content: resultA},
		{Role: "tool", Visibility: database.ChatMessageVisibilityBoth, Content: resultB},
		// Step 2
		{Role: "assistant", Visibility: database.ChatMessageVisibilityBoth, Content: step2Assistant},
		{Role: "tool", Visibility: database.ChatMessageVisibilityBoth, Content: resultC},
		{Role: "tool", Visibility: database.ChatMessageVisibilityBoth, Content: resultD},
		{Role: "tool", Visibility: database.ChatMessageVisibilityBoth, Content: resultE},
		// User follow-up
		{Role: "user", Visibility: database.ChatMessageVisibilityBoth, Content: mustMarshalContent(t, []fantasy.Content{
			fantasy.TextContent{Text: "?"},
		})},
	})
	require.NoError(t, err)

	// Expected message sequence:
	// [0] assistant [tool_use A, B, C(PE)]
	// [1] tool [result A]
	// [2] tool [result B]
	// [3] assistant [text, tool_use D, E]
	// [4] tool [result D]
	// [5] tool [result E]
	// [6] user ["?"]
	require.Len(t, prompt, 7, "expected 7 messages after repair")

	require.Equal(t, fantasy.MessageRoleAssistant, prompt[0].Role)
	require.Equal(t, fantasy.MessageRoleTool, prompt[1].Role)
	require.Equal(t, fantasy.MessageRoleTool, prompt[2].Role)
	require.Equal(t, fantasy.MessageRoleAssistant, prompt[3].Role)
	require.Equal(t, fantasy.MessageRoleTool, prompt[4].Role)
	require.Equal(t, fantasy.MessageRoleTool, prompt[5].Role)
	require.Equal(t, fantasy.MessageRoleUser, prompt[6].Role)

	// Verify step 1 has no synthetic error for C.
	step1ToolIDs := extractToolResultIDs(t, prompt[1], prompt[2])
	require.ElementsMatch(t, []string{"toolu_A", "toolu_B"}, step1ToolIDs)

	// Verify step 2 tool results contain only D and E (C is dropped).
	step2ToolIDs := extractToolResultIDs(t, prompt[4], prompt[5])
	require.ElementsMatch(t, []string{"toolu_D", "toolu_E"}, step2ToolIDs)

	// Verify no synthetic assistant messages were injected.
	for i, msg := range prompt {
		if msg.Role == fantasy.MessageRoleAssistant {
			for _, part := range msg.Content {
				tc, ok := fantasy.AsMessagePart[fantasy.ToolCallPart](part)
				if ok && tc.Input == "{}" && tc.ToolCallID == "srvtoolu_C" {
					t.Errorf("message[%d]: unexpected synthetic tool_use for srvtoolu_C", i)
				}
			}
		}
	}
}

// TestInjectMissingToolUses_DropsOnlyProviderExecutedMessage verifies
// that a tool message containing only a provider-executed result is
// entirely dropped.
func TestInjectMissingToolUses_DropsOnlyProviderExecutedMessage(t *testing.T) {
	t.Parallel()

	assistantContent := mustMarshalContent(t, []fantasy.Content{
		fantasy.ToolCallContent{
			ToolCallID: "toolu_local",
			ToolName:   "execute",
			Input:      `{"command":"ls"}`,
		},
	})

	localResult := mustMarshalToolResult(t,
		"toolu_local", "execute",
		json.RawMessage(`{"output":"file.txt"}`),
		false, false,
	)

	// Second assistant with only local tool call.
	assistant2Content := mustMarshalContent(t, []fantasy.Content{
		fantasy.TextContent{Text: "Done."},
	})

	// Orphaned provider-executed result after second assistant.
	peResult := mustMarshalToolResult(t,
		"srvtoolu_orphan", "web_search",
		json.RawMessage(`{}`),
		false, true,
	)

	prompt, err := chatprompt.ConvertMessages([]database.ChatMessage{
		{Role: "assistant", Visibility: database.ChatMessageVisibilityBoth, Content: assistantContent},
		{Role: "tool", Visibility: database.ChatMessageVisibilityBoth, Content: localResult},
		{Role: "assistant", Visibility: database.ChatMessageVisibilityBoth, Content: assistant2Content},
		{Role: "tool", Visibility: database.ChatMessageVisibilityBoth, Content: peResult},
	})
	require.NoError(t, err)

	// The PE-only tool message should be dropped entirely.
	// Expected: assistant, tool(local), assistant(text)
	require.Len(t, prompt, 3)
	require.Equal(t, fantasy.MessageRoleAssistant, prompt[0].Role)
	require.Equal(t, fantasy.MessageRoleTool, prompt[1].Role)
	require.Equal(t, fantasy.MessageRoleAssistant, prompt[2].Role)
}

func mustMarshalContent(t *testing.T, content []fantasy.Content) pqtype.NullRawMessage {
	t.Helper()
	result, err := chatprompt.MarshalContent(content, nil)
	require.NoError(t, err)
	return result
}

func mustMarshalToolResult(t *testing.T, toolCallID, toolName string, result json.RawMessage, isError, providerExecuted bool) pqtype.NullRawMessage {
	t.Helper()
	raw, err := chatprompt.MarshalToolResult(toolCallID, toolName, result, isError, providerExecuted, nil)
	require.NoError(t, err)
	return raw
}

func extractToolResultIDs(t *testing.T, msgs ...fantasy.Message) []string {
	t.Helper()
	var ids []string
	for _, msg := range msgs {
		for _, part := range msg.Content {
			tr, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](part)
			if ok {
				ids = append(ids, tr.ToolCallID)
			}
		}
	}
	return ids
}
