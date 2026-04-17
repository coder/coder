package chatprompt_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// testMsg builds a database.ChatMessage for ParseContent tests.
// ContentVersion defaults to 0 (legacy), which exercises the
// heuristic detection path.
func testMsg(role codersdk.ChatMessageRole, raw pqtype.NullRawMessage) database.ChatMessage {
	return database.ChatMessage{
		Role:    database.ChatMessageRole(role),
		Content: raw,
	}
}

// testMsgV1 builds a database.ChatMessage with ContentVersion 1.
func testMsgV1(role codersdk.ChatMessageRole, raw pqtype.NullRawMessage) database.ChatMessage {
	return database.ChatMessage{
		Role:           database.ChatMessageRole(role),
		Content:        raw,
		ContentVersion: chatprompt.CurrentContentVersion,
	}
}

func TestConvertMessages_NormalizesAssistantToolCallInput(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty input",
			input:    "",
			expected: "{}",
		},
		{
			name:     "invalid json",
			input:    "{\"command\":",
			expected: "{}",
		},
		{
			name:     "non-object json",
			input:    "[]",
			expected: "{}",
		},
		{
			name:     "valid object json",
			input:    "{\"command\":\"ls\"}",
			expected: "{\"command\":\"ls\"}",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assistantContent, err := chatprompt.MarshalContent([]fantasy.Content{
				fantasy.ToolCallContent{
					ToolCallID: "toolu_01C4PqN6F2493pi7Ebag8Vg7",
					ToolName:   "execute",
					Input:      tc.input,
				},
			}, nil)
			require.NoError(t, err)

			toolContent, err := chatprompt.MarshalToolResult(
				"toolu_01C4PqN6F2493pi7Ebag8Vg7",
				"execute",
				json.RawMessage(`{"error":"tool call was interrupted before it produced a result"}`),
				true,
				false,
				false,
				nil,
			)
			require.NoError(t, err)

			prompt, err := chatprompt.ConvertMessages([]database.ChatMessage{
				{
					Role:       database.ChatMessageRoleAssistant,
					Visibility: database.ChatMessageVisibilityBoth,
					Content:    assistantContent,
				},
				{
					Role:       database.ChatMessageRoleTool,
					Visibility: database.ChatMessageVisibilityBoth,
					Content:    toolContent,
				},
			})
			require.NoError(t, err)
			require.Len(t, prompt, 2)

			require.Equal(t, fantasy.MessageRoleAssistant, prompt[0].Role)
			toolCalls := chatprompt.ExtractToolCalls(prompt[0].Content)
			require.Len(t, toolCalls, 1)
			require.Equal(t, tc.expected, toolCalls[0].Input)
			require.Equal(t, "execute", toolCalls[0].ToolName)
			require.Equal(t, "toolu_01C4PqN6F2493pi7Ebag8Vg7", toolCalls[0].ToolCallID)

			require.Equal(t, fantasy.MessageRoleTool, prompt[1].Role)
		})
	}
}

func TestConvertMessagesWithFiles_ResolvesFileData(t *testing.T) {
	t.Parallel()

	fileID := uuid.New()
	fileData := []byte("fake-image-bytes")

	// Build a user message with file_id but no inline data, as
	// would be stored after injectFileID strips the data.
	rawContent := mustJSON(t, []json.RawMessage{
		mustJSON(t, map[string]any{
			"type": "file",
			"data": map[string]any{
				"media_type": "image/png",
				"file_id":    fileID.String(),
			},
		}),
	})

	resolver := func(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]chatprompt.FileData, error) {
		result := make(map[uuid.UUID]chatprompt.FileData)
		for _, id := range ids {
			if id == fileID {
				result[id] = chatprompt.FileData{
					Data:      fileData,
					MediaType: "image/png",
				}
			}
		}
		return result, nil
	}

	prompt, err := chatprompt.ConvertMessagesWithFiles(
		context.Background(),
		[]database.ChatMessage{
			{
				Role:       database.ChatMessageRoleUser,
				Visibility: database.ChatMessageVisibilityBoth,
				Content:    pqtype.NullRawMessage{RawMessage: rawContent, Valid: true},
			},
		},
		resolver,
		slogtest.Make(t, nil),
	)
	require.NoError(t, err)
	require.Len(t, prompt, 1)
	require.Equal(t, fantasy.MessageRoleUser, prompt[0].Role)
	require.Len(t, prompt[0].Content, 1)

	filePart, ok := fantasy.AsMessagePart[fantasy.FilePart](prompt[0].Content[0])
	require.True(t, ok, "expected FilePart")
	require.Equal(t, fileData, filePart.Data)
	require.Equal(t, "image/png", filePart.MediaType)
}

func TestConvertMessagesWithFiles_BackwardCompat(t *testing.T) {
	t.Parallel()

	// A legacy message with inline data and a file_id: ParseContent
	// extracts the file_id and clears inline data (resolved at LLM
	// dispatch time). When a resolver provides data, the file part
	// in the LLM prompt should contain the resolved data.
	fileID := uuid.New()
	resolvedData := []byte("resolved-image-data")

	rawContent := mustJSON(t, []json.RawMessage{
		mustJSON(t, map[string]any{
			"type": "file",
			"data": map[string]any{
				"media_type": "image/png",
				"data":       []byte("inline-image-data"),
				"file_id":    fileID.String(),
			},
		}),
	})

	resolver := func(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]chatprompt.FileData, error) {
		result := make(map[uuid.UUID]chatprompt.FileData)
		for _, id := range ids {
			if id == fileID {
				result[id] = chatprompt.FileData{
					Data:      resolvedData,
					MediaType: "image/png",
				}
			}
		}
		return result, nil
	}

	prompt, err := chatprompt.ConvertMessagesWithFiles(
		context.Background(),
		[]database.ChatMessage{
			{
				Role:       database.ChatMessageRoleUser,
				Visibility: database.ChatMessageVisibilityBoth,
				Content:    pqtype.NullRawMessage{RawMessage: rawContent, Valid: true},
			},
		},
		resolver,
		slogtest.Make(t, nil),
	)
	require.NoError(t, err)
	require.Len(t, prompt, 1)
	require.Len(t, prompt[0].Content, 1)

	filePart, ok := fantasy.AsMessagePart[fantasy.FilePart](prompt[0].Content[0])
	require.True(t, ok, "expected FilePart")
	require.Equal(t, resolvedData, filePart.Data)
	require.Equal(t, "image/png", filePart.MediaType)
}

func TestInjectFileID_StripsInlineData(t *testing.T) {
	t.Parallel()

	fileID := uuid.New()
	imageData := []byte("raw-image-bytes")

	// Marshal a file content block with inline data, then inject
	// a file_id. The result should have file_id but no data.
	content, err := chatprompt.MarshalContent([]fantasy.Content{
		fantasy.FileContent{
			MediaType: "image/png",
			Data:      imageData,
		},
	}, map[int]uuid.UUID{0: fileID})
	require.NoError(t, err)

	// Parse the stored content to verify shape.
	var blocks []json.RawMessage
	require.NoError(t, json.Unmarshal(content.RawMessage, &blocks))
	require.Len(t, blocks, 1)

	var envelope struct {
		Type string `json:"type"`
		Data struct {
			MediaType string           `json:"media_type"`
			Data      *json.RawMessage `json:"data,omitempty"`
			FileID    string           `json:"file_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(blocks[0], &envelope))
	require.Equal(t, "file", envelope.Type)
	require.Equal(t, "image/png", envelope.Data.MediaType)
	require.Equal(t, fileID.String(), envelope.Data.FileID)
	// Data should be nil (omitted) since injectFileID strips it.
	require.Nil(t, envelope.Data.Data, "inline data should be stripped")
}

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
		false, false, false,
	)

	prompt, err := chatprompt.ConvertMessages([]database.ChatMessage{
		{
			Role:       database.ChatMessageRoleAssistant,
			Visibility: database.ChatMessageVisibilityBoth,
			Content:    assistantContent,
		},
		{
			Role:       database.ChatMessageRoleTool,
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
		false, false, false,
	)
	resultB := mustMarshalToolResult(t,
		"toolu_B", "spawn_agent",
		json.RawMessage(`{"status":"done"}`),
		false, false, false,
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
		false, false, true, // provider_executed = true
	)
	resultD := mustMarshalToolResult(t,
		"toolu_D", "wait_agent",
		json.RawMessage(`{"report":"done"}`),
		false, false, false,
	)
	resultE := mustMarshalToolResult(t,
		"toolu_E", "wait_agent",
		json.RawMessage(`{"report":"done"}`),
		false, false, false,
	)

	prompt, err := chatprompt.ConvertMessages([]database.ChatMessage{
		// Step 1
		{Role: database.ChatMessageRoleAssistant, Visibility: database.ChatMessageVisibilityBoth, Content: step1Assistant},
		{Role: database.ChatMessageRoleTool, Visibility: database.ChatMessageVisibilityBoth, Content: resultA},
		{Role: database.ChatMessageRoleTool, Visibility: database.ChatMessageVisibilityBoth, Content: resultB},
		// Step 2
		{Role: database.ChatMessageRoleAssistant, Visibility: database.ChatMessageVisibilityBoth, Content: step2Assistant},
		{Role: database.ChatMessageRoleTool, Visibility: database.ChatMessageVisibilityBoth, Content: resultC},
		{Role: database.ChatMessageRoleTool, Visibility: database.ChatMessageVisibilityBoth, Content: resultD},
		{Role: database.ChatMessageRoleTool, Visibility: database.ChatMessageVisibilityBoth, Content: resultE},
		// User follow-up
		{Role: database.ChatMessageRoleUser, Visibility: database.ChatMessageVisibilityBoth, Content: mustMarshalContent(t, []fantasy.Content{
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
		false, false, false,
	)

	// Second assistant with only local tool call.
	assistant2Content := mustMarshalContent(t, []fantasy.Content{
		fantasy.TextContent{Text: "Done."},
	})

	// Orphaned provider-executed result after second assistant.
	peResult := mustMarshalToolResult(t,
		"srvtoolu_orphan", "web_search",
		json.RawMessage(`{}`),
		false, false, true,
	)

	prompt, err := chatprompt.ConvertMessages([]database.ChatMessage{
		{Role: database.ChatMessageRoleAssistant, Visibility: database.ChatMessageVisibilityBoth, Content: assistantContent},
		{Role: database.ChatMessageRoleTool, Visibility: database.ChatMessageVisibilityBoth, Content: localResult},
		{Role: database.ChatMessageRoleAssistant, Visibility: database.ChatMessageVisibilityBoth, Content: assistant2Content},
		{Role: database.ChatMessageRoleTool, Visibility: database.ChatMessageVisibilityBoth, Content: peResult},
	})
	require.NoError(t, err)

	// The PE-only tool message should be dropped entirely.
	// Expected: assistant, tool(local), assistant(text)
	require.Len(t, prompt, 3)
	require.Equal(t, fantasy.MessageRoleAssistant, prompt[0].Role)
	require.Equal(t, fantasy.MessageRoleTool, prompt[1].Role)
	require.Equal(t, fantasy.MessageRoleAssistant, prompt[2].Role)
}

// TestProviderExecutedResultInAssistantContent verifies the
// round-trip for the new persistence model: provider-executed tool
// results (e.g. web_search) are stored inline in the assistant
// content row (not as separate tool-role messages). After marshal →
// parse → ToMessageParts, the ToolResultPart must carry
// ProviderExecuted = true so the fantasy Anthropic provider can
// reconstruct the web_search_tool_result block.
func TestProviderExecutedResultInAssistantContent(t *testing.T) {
	t.Parallel()

	// The assistant message contains a PE tool call, a PE tool result,
	// and a text block — mimicking a web_search step where persistStep
	// keeps the PE result inline.
	assistantContent := mustMarshalContent(t, []fantasy.Content{
		fantasy.ToolCallContent{
			ToolCallID:       "srvtoolu_WS",
			ToolName:         "web_search",
			Input:            `{"query":"golang testing"}`,
			ProviderExecuted: true,
		},
		fantasy.ToolResultContent{
			ToolCallID:       "srvtoolu_WS",
			ToolName:         "web_search",
			Result:           fantasy.ToolResultOutputContentText{Text: `{"results":"some search results"}`},
			ProviderExecuted: true,
		},
		fantasy.TextContent{Text: "Here is what I found."},
	})

	prompt, err := chatprompt.ConvertMessages([]database.ChatMessage{
		{Role: database.ChatMessageRoleAssistant, Visibility: database.ChatMessageVisibilityBoth, Content: assistantContent},
		{Role: database.ChatMessageRoleUser, Visibility: database.ChatMessageVisibilityBoth, Content: mustMarshalContent(t, []fantasy.Content{
			fantasy.TextContent{Text: "Thanks!"},
		})},
	})
	require.NoError(t, err)

	// Should be 2 messages: assistant + user.
	require.Len(t, prompt, 2)
	require.Equal(t, fantasy.MessageRoleAssistant, prompt[0].Role)
	require.Equal(t, fantasy.MessageRoleUser, prompt[1].Role)

	// The assistant message must contain 3 parts: tool_call, tool_result, text.
	var foundToolCall, foundToolResult, foundText bool
	for _, part := range prompt[0].Content {
		if tc, ok := fantasy.AsMessagePart[fantasy.ToolCallPart](part); ok {
			require.Equal(t, "srvtoolu_WS", tc.ToolCallID)
			require.True(t, tc.ProviderExecuted, "ToolCallPart.ProviderExecuted must be true")
			foundToolCall = true
		}
		if tr, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](part); ok {
			require.Equal(t, "srvtoolu_WS", tr.ToolCallID)
			require.True(t, tr.ProviderExecuted, "ToolResultPart.ProviderExecuted must be true")
			foundToolResult = true
		}
		if tp, ok := fantasy.AsMessagePart[fantasy.TextPart](part); ok {
			require.Equal(t, "Here is what I found.", tp.Text)
			foundText = true
		}
	}
	require.True(t, foundToolCall, "expected PE tool call in assistant message")
	require.True(t, foundToolResult, "expected PE tool result in assistant message")
	require.True(t, foundText, "expected text part in assistant message")
}

// TestProviderExecutedResult_LegacyToolRow verifies backward
// compatibility: PE tool results that were stored as separate
// tool-role rows (legacy persistence) are still handled correctly
// by the repair passes — orphaned PE results are dropped, and
// matching PE results in the same step work via the existing
// injectMissingToolUses logic.
func TestProviderExecutedResult_LegacyToolRow(t *testing.T) {
	t.Parallel()

	// Assistant with PE web_search + regular tool call.
	assistantContent := mustMarshalContent(t, []fantasy.Content{
		fantasy.ToolCallContent{
			ToolCallID:       "srvtoolu_WS",
			ToolName:         "web_search",
			Input:            `{"query":"test"}`,
			ProviderExecuted: true,
		},
		fantasy.ToolCallContent{
			ToolCallID: "toolu_exec",
			ToolName:   "execute",
			Input:      `{"command":"ls"}`,
		},
		fantasy.TextContent{Text: "Results."},
	})

	// Legacy: PE result stored as separate tool-role message.
	peResult := mustMarshalToolResult(t,
		"srvtoolu_WS", "web_search",
		json.RawMessage(`{"results":"cached"}`),
		false, false, true, // providerExecuted = true
	)
	execResult := mustMarshalToolResult(t,
		"toolu_exec", "execute",
		json.RawMessage(`{"output":"file.txt"}`),
		false, false, false,
	)

	prompt, err := chatprompt.ConvertMessages([]database.ChatMessage{
		{Role: database.ChatMessageRoleAssistant, Visibility: database.ChatMessageVisibilityBoth, Content: assistantContent},
		{Role: database.ChatMessageRoleTool, Visibility: database.ChatMessageVisibilityBoth, Content: peResult},
		{Role: database.ChatMessageRoleTool, Visibility: database.ChatMessageVisibilityBoth, Content: execResult},
		{Role: database.ChatMessageRoleUser, Visibility: database.ChatMessageVisibilityBoth, Content: mustMarshalContent(t, []fantasy.Content{
			fantasy.TextContent{Text: "next"},
		})},
	})
	require.NoError(t, err)

	// The PE tool result should be dropped by injectMissingToolUses,
	// leaving: assistant, tool(exec), user.
	require.Len(t, prompt, 3, "expected 3 messages after PE result is dropped")
	require.Equal(t, fantasy.MessageRoleAssistant, prompt[0].Role)
	require.Equal(t, fantasy.MessageRoleTool, prompt[1].Role)
	require.Equal(t, fantasy.MessageRoleUser, prompt[2].Role)

	// Tool message should only contain the exec result, not the PE one.
	toolIDs := extractToolResultIDs(t, prompt[1])
	require.Equal(t, []string{"toolu_exec"}, toolIDs)
}

// TestSDKPartsNeverProduceFantasyEnvelopeShape guards the structural
// invariant that isFantasyEnvelopeFormat relies on: no SDK part type
// serializes with a top-level "data" field containing a JSON object
// (starting with '{'). Fantasy envelopes always have
// "data":{object}, while ChatMessagePart.Data is []byte which
// serializes to a base64 string or is omitted. If this test fails,
// the format discriminator can no longer distinguish legacy fantasy
// content from SDK parts, and parseAssistantRole / parseUserRole
// would silently lose data on legacy rows.
func TestSDKPartsNeverProduceFantasyEnvelopeShape(t *testing.T) {
	t.Parallel()

	parts := []codersdk.ChatMessagePart{
		{Type: codersdk.ChatMessagePartTypeText, Text: "hello"},
		{Type: codersdk.ChatMessagePartTypeFile, FileID: uuid.NullUUID{UUID: uuid.New(), Valid: true}, MediaType: "image/png"},
		{Type: codersdk.ChatMessagePartTypeFile, MediaType: "image/png", Data: []byte("fake-image-data")},
		{Type: codersdk.ChatMessagePartTypeFileReference, FileName: "main.go", StartLine: 1, EndLine: 10, Content: "func main() {}"},
		{Type: codersdk.ChatMessagePartTypeReasoning, Text: "thinking..."},
		{Type: codersdk.ChatMessagePartTypeToolCall, ToolCallID: "abc", ToolName: "read_file", Args: json.RawMessage(`{"path":"main.go"}`)},
		{Type: codersdk.ChatMessagePartTypeToolResult, ToolCallID: "abc", ToolName: "read_file", Result: json.RawMessage(`{"output":"code"}`)},
		{Type: codersdk.ChatMessagePartTypeSource, SourceID: "s1", URL: "https://example.com", Title: "Example"},
	}
	for _, part := range parts {
		raw, err := json.Marshal(part)
		require.NoError(t, err)
		var fields map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(raw, &fields))
		if data, ok := fields["data"]; ok {
			trimmed := bytes.TrimSpace(data)
			require.NotEmpty(t, trimmed)
			assert.NotEqual(t, byte('{'), trimmed[0],
				"SDK part type %q serializes with data field starting with '{', "+
					"would be misidentified as fantasy envelope by isFantasyEnvelopeFormat",
				part.Type)
		}
	}
}

// nullRaw wraps raw JSON bytes in a NullRawMessage for test input.
func nullRaw(data json.RawMessage) pqtype.NullRawMessage {
	return pqtype.NullRawMessage{RawMessage: data, Valid: true}
}

func TestParseContent_BackwardCompat(t *testing.T) {
	t.Parallel()

	fileID := uuid.New()

	// Build legacy fantasy assistant content using MarshalContent.
	legacyAssistantReasoning, err := chatprompt.MarshalContent([]fantasy.Content{
		fantasy.ReasoningContent{
			Text: "let me think...",
			ProviderMetadata: fantasy.ProviderMetadata{
				"anthropic": &fantasyanthropic.ProviderCacheControlOptions{
					CacheControl: fantasyanthropic.CacheControl{Type: "ephemeral"},
				},
			},
		},
	}, nil)
	require.NoError(t, err)

	legacyAssistantSource, err := chatprompt.MarshalContent([]fantasy.Content{
		fantasy.SourceContent{
			ID:    "src_001",
			URL:   "https://example.com/doc",
			Title: "Example Doc",
		},
	}, nil)
	require.NoError(t, err)

	legacyAssistantToolCall, err := chatprompt.MarshalContent([]fantasy.Content{
		fantasy.ToolCallContent{
			ToolCallID: "call_123",
			ToolName:   "read_file",
			Input:      `{"path":"main.go"}`,
		},
	}, nil)
	require.NoError(t, err)

	// Build new SDK format using MarshalParts.
	sdkMetadata := json.RawMessage(`{"anthropic":{"type":"anthropic.cache_control_options","data":{"cache_control":{"type":"ephemeral"}}}}`)

	newAssistantWithMeta, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{{
		Type:             codersdk.ChatMessagePartTypeText,
		Text:             "here is my answer",
		ProviderMetadata: sdkMetadata,
	}})
	require.NoError(t, err)

	newAssistantToolCall, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{{
		Type:       codersdk.ChatMessagePartTypeToolCall,
		ToolCallID: "call_456",
		ToolName:   "execute",
		Args:       json.RawMessage(`{"cmd":"ls"}`),
	}})
	require.NoError(t, err)

	newToolResult, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{{
		Type:       codersdk.ChatMessagePartTypeToolResult,
		ToolCallID: "call_456",
		ToolName:   "execute",
		Result:     json.RawMessage(`{"output":"file1.go"}`),
	}})
	require.NoError(t, err)

	tests := []struct {
		name  string
		role  codersdk.ChatMessageRole
		raw   pqtype.NullRawMessage
		check func(t *testing.T, parts []codersdk.ChatMessagePart)
	}{
		{
			name: "system/plain_string",
			role: codersdk.ChatMessageRoleSystem,
			raw:  nullRaw(mustJSON(t, "You are helpful.")),
			check: func(t *testing.T, parts []codersdk.ChatMessagePart) {
				require.Len(t, parts, 1)
				assert.Equal(t, codersdk.ChatMessagePartTypeText, parts[0].Type)
				assert.Equal(t, "You are helpful.", parts[0].Text)
			},
		},
		{
			name: "user/fantasy_text",
			role: codersdk.ChatMessageRoleUser,
			raw: nullRaw(mustJSON(t, []json.RawMessage{
				mustJSON(t, map[string]any{
					"type": "text",
					"data": map[string]any{"text": "hello from user"},
				}),
			})),
			check: func(t *testing.T, parts []codersdk.ChatMessagePart) {
				require.Len(t, parts, 1)
				assert.Equal(t, codersdk.ChatMessagePartTypeText, parts[0].Type)
				assert.Equal(t, "hello from user", parts[0].Text)
			},
		},
		{
			name: "assistant/fantasy_text",
			role: codersdk.ChatMessageRoleAssistant,
			raw: nullRaw(mustJSON(t, []json.RawMessage{
				mustJSON(t, map[string]any{
					"type": "text",
					"data": map[string]any{"text": "hello from assistant"},
				}),
			})),
			check: func(t *testing.T, parts []codersdk.ChatMessagePart) {
				require.Len(t, parts, 1)
				assert.Equal(t, codersdk.ChatMessagePartTypeText, parts[0].Type)
				assert.Equal(t, "hello from assistant", parts[0].Text)
			},
		},
		{
			name: "user/plain_string",
			role: codersdk.ChatMessageRoleUser,
			raw:  nullRaw(mustJSON(t, "just a plain string")),
			check: func(t *testing.T, parts []codersdk.ChatMessagePart) {
				require.Len(t, parts, 1)
				assert.Equal(t, codersdk.ChatMessagePartTypeText, parts[0].Type)
				assert.Equal(t, "just a plain string", parts[0].Text)
			},
		},
		{
			name: "user/fantasy_file_with_file_id",
			role: codersdk.ChatMessageRoleUser,
			raw: nullRaw(mustJSON(t, []json.RawMessage{
				mustJSON(t, map[string]any{
					"type": "file",
					"data": map[string]any{
						"media_type": "image/png",
						"file_id":    fileID.String(),
					},
				}),
			})),
			check: func(t *testing.T, parts []codersdk.ChatMessagePart) {
				require.Len(t, parts, 1)
				assert.Equal(t, codersdk.ChatMessagePartTypeFile, parts[0].Type)
				assert.Equal(t, "image/png", parts[0].MediaType)
				assert.True(t, parts[0].FileID.Valid)
				assert.Equal(t, fileID, parts[0].FileID.UUID)
				assert.Nil(t, parts[0].Data, "inline data cleared when file_id present")
			},
		},
		{
			name: "assistant/fantasy_reasoning_with_metadata",
			role: codersdk.ChatMessageRoleAssistant,
			raw:  legacyAssistantReasoning,
			check: func(t *testing.T, parts []codersdk.ChatMessagePart) {
				require.Len(t, parts, 1)
				assert.Equal(t, codersdk.ChatMessagePartTypeReasoning, parts[0].Type)
				assert.Equal(t, "let me think...", parts[0].Text)
				require.NotNil(t, parts[0].ProviderMetadata, "ProviderMetadata must be preserved")
				assert.Contains(t, string(parts[0].ProviderMetadata), "anthropic")
			},
		},
		{
			name: "assistant/fantasy_source",
			role: codersdk.ChatMessageRoleAssistant,
			raw:  legacyAssistantSource,
			check: func(t *testing.T, parts []codersdk.ChatMessagePart) {
				require.Len(t, parts, 1)
				assert.Equal(t, codersdk.ChatMessagePartTypeSource, parts[0].Type)
				assert.Equal(t, "src_001", parts[0].SourceID)
				assert.Equal(t, "https://example.com/doc", parts[0].URL)
				assert.Equal(t, "Example Doc", parts[0].Title)
			},
		},
		{
			name: "assistant/fantasy_tool_call",
			role: codersdk.ChatMessageRoleAssistant,
			raw:  legacyAssistantToolCall,
			check: func(t *testing.T, parts []codersdk.ChatMessagePart) {
				require.Len(t, parts, 1)
				assert.Equal(t, codersdk.ChatMessagePartTypeToolCall, parts[0].Type)
				assert.Equal(t, "call_123", parts[0].ToolCallID)
				assert.Equal(t, "read_file", parts[0].ToolName)
				assert.JSONEq(t, `{"path":"main.go"}`, string(parts[0].Args))
			},
		},
		{
			name: "tool/legacy_result_row",
			role: codersdk.ChatMessageRoleTool,
			raw: nullRaw(mustJSON(t, []map[string]any{{
				"tool_call_id": "call_123",
				"tool_name":    "read_file",
				"result":       json.RawMessage(`{"output":"package main"}`),
			}})),
			check: func(t *testing.T, parts []codersdk.ChatMessagePart) {
				require.Len(t, parts, 1)
				assert.Equal(t, codersdk.ChatMessagePartTypeToolResult, parts[0].Type)
				assert.Equal(t, "call_123", parts[0].ToolCallID)
				assert.Equal(t, "read_file", parts[0].ToolName)
				assert.JSONEq(t, `{"output":"package main"}`, string(parts[0].Result))
			},
		},
		{
			name: "user/sdk_text",
			role: codersdk.ChatMessageRoleUser,
			raw: nullRaw(mustJSON(t, []codersdk.ChatMessagePart{
				{Type: codersdk.ChatMessagePartTypeText, Text: "hello sdk"},
			})),
			check: func(t *testing.T, parts []codersdk.ChatMessagePart) {
				require.Len(t, parts, 1)
				assert.Equal(t, codersdk.ChatMessagePartTypeText, parts[0].Type)
				assert.Equal(t, "hello sdk", parts[0].Text)
			},
		},
		{
			name: "user/sdk_file_reference",
			role: codersdk.ChatMessageRoleUser,
			raw: nullRaw(mustJSON(t, []codersdk.ChatMessagePart{
				{Type: codersdk.ChatMessagePartTypeFileReference, FileName: "main.go", StartLine: 1, EndLine: 10, Content: "func main() {}"},
			})),
			check: func(t *testing.T, parts []codersdk.ChatMessagePart) {
				require.Len(t, parts, 1)
				assert.Equal(t, codersdk.ChatMessagePartTypeFileReference, parts[0].Type)
				assert.Equal(t, "main.go", parts[0].FileName)
				assert.Equal(t, 1, parts[0].StartLine)
				assert.Equal(t, 10, parts[0].EndLine)
				assert.Equal(t, "func main() {}", parts[0].Content)
			},
		},
		{
			name: "user/sdk_file",
			role: codersdk.ChatMessageRoleUser,
			raw: nullRaw(mustJSON(t, []codersdk.ChatMessagePart{
				{Type: codersdk.ChatMessagePartTypeFile, FileID: uuid.NullUUID{UUID: fileID, Valid: true}, MediaType: "image/png"},
			})),
			check: func(t *testing.T, parts []codersdk.ChatMessagePart) {
				require.Len(t, parts, 1)
				assert.Equal(t, codersdk.ChatMessagePartTypeFile, parts[0].Type)
				assert.True(t, parts[0].FileID.Valid)
				assert.Equal(t, fileID, parts[0].FileID.UUID)
				assert.Equal(t, "image/png", parts[0].MediaType)
			},
		},
		{
			name: "assistant/sdk_text_with_metadata",
			role: codersdk.ChatMessageRoleAssistant,
			raw:  newAssistantWithMeta,
			check: func(t *testing.T, parts []codersdk.ChatMessagePart) {
				require.Len(t, parts, 1)
				assert.Equal(t, codersdk.ChatMessagePartTypeText, parts[0].Type)
				assert.Equal(t, "here is my answer", parts[0].Text)
				assert.JSONEq(t, string(sdkMetadata), string(parts[0].ProviderMetadata))
			},
		},
		{
			name: "assistant/sdk_tool_call",
			role: codersdk.ChatMessageRoleAssistant,
			raw:  newAssistantToolCall,
			check: func(t *testing.T, parts []codersdk.ChatMessagePart) {
				require.Len(t, parts, 1)
				assert.Equal(t, codersdk.ChatMessagePartTypeToolCall, parts[0].Type)
				assert.Equal(t, "call_456", parts[0].ToolCallID)
				assert.Equal(t, "execute", parts[0].ToolName)
				assert.JSONEq(t, `{"cmd":"ls"}`, string(parts[0].Args))
			},
		},
		{
			name: "tool/sdk_tool_result",
			role: codersdk.ChatMessageRoleTool,
			raw:  newToolResult,
			check: func(t *testing.T, parts []codersdk.ChatMessagePart) {
				require.Len(t, parts, 1)
				assert.Equal(t, codersdk.ChatMessagePartTypeToolResult, parts[0].Type)
				assert.Equal(t, "call_456", parts[0].ToolCallID)
				assert.Equal(t, "execute", parts[0].ToolName)
				assert.JSONEq(t, `{"output":"file1.go"}`, string(parts[0].Result))
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			parts, err := chatprompt.ParseContent(testMsg(tc.role, tc.raw))
			require.NoError(t, err)
			tc.check(t, parts)
		})
	}
}

func TestParseContent_V1(t *testing.T) {
	t.Parallel()

	t.Run("system", func(t *testing.T) {
		t.Parallel()
		raw, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
			codersdk.ChatMessageText("You are helpful."),
		})
		require.NoError(t, err)

		parts, err := chatprompt.ParseContent(testMsgV1(codersdk.ChatMessageRoleSystem, raw))
		require.NoError(t, err)
		require.Len(t, parts, 1)
		assert.Equal(t, codersdk.ChatMessagePartTypeText, parts[0].Type)
		assert.Equal(t, "You are helpful.", parts[0].Text)
	})

	t.Run("system_bare_string_errors", func(t *testing.T) {
		t.Parallel()
		// A bare JSON string is not valid V1 content.
		_, err := chatprompt.ParseContent(testMsgV1(
			codersdk.ChatMessageRoleSystem,
			nullRaw(json.RawMessage(`"You are helpful."`)),
		))
		require.Error(t, err)
	})

	t.Run("unknown_version_errors", func(t *testing.T) {
		t.Parallel()
		msg := testMsgV1(codersdk.ChatMessageRoleUser, nullRaw(json.RawMessage(`[{"type":"text","text":"hi"}]`)))
		msg.ContentVersion = 99
		_, err := chatprompt.ParseContent(msg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported content version")
	})
}

// TestProviderMetadataRoundTrip verifies that Anthropic cache
// control hints survive the full path: legacy fantasy DB row →
// ParseContent → SDK part (ProviderMetadata) → partsToMessageParts
// → fantasy.MessagePart (ProviderOptions).
func TestProviderMetadataRoundTrip(t *testing.T) {
	t.Parallel()

	legacyContent, err := chatprompt.MarshalContent([]fantasy.Content{
		fantasy.TextContent{
			Text: "cached response",
			ProviderMetadata: fantasy.ProviderMetadata{
				"anthropic": &fantasyanthropic.ProviderCacheControlOptions{
					CacheControl: fantasyanthropic.CacheControl{Type: "ephemeral"},
				},
			},
		},
	}, nil)
	require.NoError(t, err)

	// Step 1: ParseContent preserves metadata on the SDK part.
	parts, err := chatprompt.ParseContent(testMsg(codersdk.ChatMessageRoleAssistant, legacyContent))
	require.NoError(t, err)
	require.Len(t, parts, 1)
	require.NotNil(t, parts[0].ProviderMetadata,
		"ProviderMetadata must survive ParseContent")

	// Step 2: ConvertMessagesWithFiles reconstructs typed
	// ProviderOptions on the fantasy part.
	prompt, err := chatprompt.ConvertMessagesWithFiles(
		context.Background(),
		[]database.ChatMessage{{
			Role:       database.ChatMessageRoleAssistant,
			Visibility: database.ChatMessageVisibilityBoth,
			Content:    legacyContent,
		}},
		nil,
		slogtest.Make(t, nil),
	)
	require.NoError(t, err)
	require.Len(t, prompt, 1)
	require.Len(t, prompt[0].Content, 1)

	textPart, ok := fantasy.AsMessagePart[fantasy.TextPart](prompt[0].Content[0])
	require.True(t, ok, "expected TextPart")
	require.Equal(t, "cached response", textPart.Text)

	cc := fantasyanthropic.GetCacheControl(textPart.ProviderOptions)
	require.NotNil(t, cc, "Anthropic cache control must survive round-trip")
	require.Equal(t, "ephemeral", cc.Type)
}

// TestFileReferencePreservation verifies file-reference parts
// survive the storage round-trip and convert to text for LLMs.
func TestFileReferencePreservation(t *testing.T) {
	t.Parallel()

	raw, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{{
		Type:      codersdk.ChatMessagePartTypeFileReference,
		FileName:  "main.go",
		StartLine: 10,
		EndLine:   20,
		Content:   "func main() {}",
	}})
	require.NoError(t, err)

	// Storage round-trip: all fields intact.
	parts, err := chatprompt.ParseContent(testMsg(codersdk.ChatMessageRoleUser, raw))
	require.NoError(t, err)
	require.Len(t, parts, 1)
	assert.Equal(t, codersdk.ChatMessagePartTypeFileReference, parts[0].Type)
	assert.Equal(t, "main.go", parts[0].FileName)
	assert.Equal(t, 10, parts[0].StartLine)
	assert.Equal(t, 20, parts[0].EndLine)
	assert.Equal(t, "func main() {}", parts[0].Content)

	// LLM dispatch: file-reference becomes a TextPart.
	prompt, err := chatprompt.ConvertMessagesWithFiles(
		context.Background(),
		[]database.ChatMessage{{
			Role:       database.ChatMessageRoleUser,
			Visibility: database.ChatMessageVisibilityBoth,
			Content:    raw,
		}},
		nil,
		slogtest.Make(t, nil),
	)
	require.NoError(t, err)
	require.Len(t, prompt, 1)
	require.Len(t, prompt[0].Content, 1)

	textPart, ok := fantasy.AsMessagePart[fantasy.TextPart](prompt[0].Content[0])
	require.True(t, ok, "file-reference should become TextPart for LLM")
	assert.Contains(t, textPart.Text, "[file-reference]")
	assert.Contains(t, textPart.Text, "main.go")
	assert.Contains(t, textPart.Text, "10-20")
	assert.Contains(t, textPart.Text, "func main() {}")
}

// TestAssistantWriteRoundTrip verifies the Stage 4 write path:
// fantasy.Content (with ProviderMetadata) → PartFromContent →
// MarshalParts → DB → ParseContent (SDK path) →
// ConvertMessagesWithFiles → fantasy part with ProviderOptions.
func TestAssistantWriteRoundTrip(t *testing.T) {
	t.Parallel()

	original := fantasy.TextContent{
		Text: "response with cache hints",
		ProviderMetadata: fantasy.ProviderMetadata{
			"anthropic": &fantasyanthropic.ProviderCacheControlOptions{
				CacheControl: fantasyanthropic.CacheControl{Type: "ephemeral"},
			},
		},
	}

	// Simulate persistStep: PartFromContent → MarshalParts.
	sdkPart := chatprompt.PartFromContent(original)
	require.Equal(t, codersdk.ChatMessagePartTypeText, sdkPart.Type)
	require.NotNil(t, sdkPart.ProviderMetadata)

	raw, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{sdkPart})
	require.NoError(t, err)

	// Read back via ParseContent (takes the new SDK path, not
	// the legacy fallback, because the stored format is flat).
	parts, err := chatprompt.ParseContent(testMsg(codersdk.ChatMessageRoleAssistant, raw))
	require.NoError(t, err)
	require.Len(t, parts, 1)
	assert.Equal(t, "response with cache hints", parts[0].Text)
	assert.JSONEq(t, string(sdkPart.ProviderMetadata), string(parts[0].ProviderMetadata))

	// Full LLM dispatch: metadata reconstructed as typed options.
	prompt, err := chatprompt.ConvertMessagesWithFiles(
		context.Background(),
		[]database.ChatMessage{{
			Role:       database.ChatMessageRoleAssistant,
			Visibility: database.ChatMessageVisibilityBoth,
			Content:    raw,
		}},
		nil,
		slogtest.Make(t, nil),
	)
	require.NoError(t, err)
	require.Len(t, prompt, 1)
	require.Len(t, prompt[0].Content, 1)

	textPart, ok := fantasy.AsMessagePart[fantasy.TextPart](prompt[0].Content[0])
	require.True(t, ok)
	require.Equal(t, "response with cache hints", textPart.Text)

	cc := fantasyanthropic.GetCacheControl(textPart.ProviderOptions)
	require.NotNil(t, cc, "cache control must survive new write → new read round-trip")
	require.Equal(t, "ephemeral", cc.Type)
}

// TestMixedFormatConversation verifies ConvertMessagesWithFiles
// handles a realistic post-deploy conversation where legacy and new
// storage formats coexist.
func TestMixedFormatConversation(t *testing.T) {
	t.Parallel()

	fileID := uuid.New()
	resolvedFileData := []byte("resolved-png-bytes")

	resolver := func(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]chatprompt.FileData, error) {
		out := make(map[uuid.UUID]chatprompt.FileData)
		for _, id := range ids {
			if id == fileID {
				out[id] = chatprompt.FileData{Data: resolvedFileData, MediaType: "image/png"}
			}
		}
		return out, nil
	}

	// 1. System (JSON string).
	systemRaw, err := json.Marshal("You are helpful.")
	require.NoError(t, err)

	// 2. Old user (fantasy envelope: text + file with file_id).
	oldUserRaw := mustJSON(t, []json.RawMessage{
		mustJSON(t, map[string]any{
			"type": "text",
			"data": map[string]any{"text": "Look at this image."},
		}),
		mustJSON(t, map[string]any{
			"type": "file",
			"data": map[string]any{
				"media_type": "image/png",
				"file_id":    fileID.String(),
			},
		}),
	})

	// 3. Old assistant (fantasy envelope: tool-call).
	oldAssistantRaw, err := chatprompt.MarshalContent([]fantasy.Content{
		fantasy.ToolCallContent{
			ToolCallID: "call_1",
			ToolName:   "analyze_image",
			Input:      `{"detail":"high"}`,
		},
	}, nil)
	require.NoError(t, err)

	// 4. Old tool (legacy result rows).
	oldToolRaw, err := chatprompt.MarshalToolResult(
		"call_1", "analyze_image",
		json.RawMessage(`{"description":"a cat"}`), false, false,
		false, nil,
	)
	require.NoError(t, err)

	// 5. New user (SDK parts: text + file-reference).
	newUserRaw, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		{Type: codersdk.ChatMessagePartTypeText, Text: "Check this diff."},
		{Type: codersdk.ChatMessagePartTypeFileReference, FileName: "main.go", StartLine: 5, EndLine: 15, Content: "func main() {}"},
	})
	require.NoError(t, err)

	// 6. New assistant (SDK parts: text with metadata).
	newAssistantMeta := json.RawMessage(`{"anthropic":{"type":"anthropic.cache_control_options","data":{"cache_control":{"type":"ephemeral"}}}}`)
	newAssistantRaw, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		{Type: codersdk.ChatMessagePartTypeText, Text: "Here is my analysis.", ProviderMetadata: newAssistantMeta},
	})
	require.NoError(t, err)

	messages := []database.ChatMessage{
		{Role: database.ChatMessageRoleSystem, Visibility: database.ChatMessageVisibilityModel, Content: pqtype.NullRawMessage{RawMessage: systemRaw, Valid: true}},
		{Role: database.ChatMessageRoleUser, Visibility: database.ChatMessageVisibilityBoth, Content: pqtype.NullRawMessage{RawMessage: oldUserRaw, Valid: true}},
		{Role: database.ChatMessageRoleAssistant, Visibility: database.ChatMessageVisibilityBoth, Content: oldAssistantRaw},
		{Role: database.ChatMessageRoleTool, Visibility: database.ChatMessageVisibilityBoth, Content: oldToolRaw},
		{Role: database.ChatMessageRoleUser, Visibility: database.ChatMessageVisibilityBoth, Content: newUserRaw},
		{Role: database.ChatMessageRoleAssistant, Visibility: database.ChatMessageVisibilityBoth, Content: newAssistantRaw},
	}

	prompt, err := chatprompt.ConvertMessagesWithFiles(
		context.Background(), messages, resolver, slogtest.Make(t, nil),
	)
	require.NoError(t, err)
	require.Len(t, prompt, 6, "all 6 messages should produce prompt entries")

	// 1. System.
	require.Equal(t, fantasy.MessageRoleSystem, prompt[0].Role)
	systemText, ok := fantasy.AsMessagePart[fantasy.TextPart](prompt[0].Content[0])
	require.True(t, ok)
	assert.Equal(t, "You are helpful.", systemText.Text)

	// 2. Old user: text + file with resolved data.
	require.Equal(t, fantasy.MessageRoleUser, prompt[1].Role)
	require.Len(t, prompt[1].Content, 2)
	userText, ok := fantasy.AsMessagePart[fantasy.TextPart](prompt[1].Content[0])
	require.True(t, ok)
	assert.Equal(t, "Look at this image.", userText.Text)
	filePart, ok := fantasy.AsMessagePart[fantasy.FilePart](prompt[1].Content[1])
	require.True(t, ok)
	assert.Equal(t, resolvedFileData, filePart.Data)
	assert.Equal(t, "image/png", filePart.MediaType)

	// 3. Old assistant: tool-call with normalized input.
	require.Equal(t, fantasy.MessageRoleAssistant, prompt[2].Role)
	toolCalls := chatprompt.ExtractToolCalls(prompt[2].Content)
	require.Len(t, toolCalls, 1)
	assert.Equal(t, "call_1", toolCalls[0].ToolCallID)
	assert.Equal(t, "analyze_image", toolCalls[0].ToolName)
	assert.JSONEq(t, `{"detail":"high"}`, toolCalls[0].Input)

	// 4. Old tool: result paired with call_1.
	require.Equal(t, fantasy.MessageRoleTool, prompt[3].Role)
	require.Len(t, prompt[3].Content, 1)
	toolResult, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](prompt[3].Content[0])
	require.True(t, ok)
	assert.Equal(t, "call_1", toolResult.ToolCallID)

	// 5. New user: text + file-reference (converted to TextPart).
	require.Equal(t, fantasy.MessageRoleUser, prompt[4].Role)
	require.Len(t, prompt[4].Content, 2)
	newUserText, ok := fantasy.AsMessagePart[fantasy.TextPart](prompt[4].Content[0])
	require.True(t, ok)
	assert.Equal(t, "Check this diff.", newUserText.Text)
	refText, ok := fantasy.AsMessagePart[fantasy.TextPart](prompt[4].Content[1])
	require.True(t, ok)
	assert.Contains(t, refText.Text, "[file-reference]")
	assert.Contains(t, refText.Text, "main.go")

	// 6. New assistant: text with ProviderMetadata → ProviderOptions.
	require.Equal(t, fantasy.MessageRoleAssistant, prompt[5].Role)
	require.Len(t, prompt[5].Content, 1)
	newAssistantText, ok := fantasy.AsMessagePart[fantasy.TextPart](prompt[5].Content[0])
	require.True(t, ok)
	assert.Equal(t, "Here is my analysis.", newAssistantText.Text)
	cc := fantasyanthropic.GetCacheControl(newAssistantText.ProviderOptions)
	require.NotNil(t, cc, "ProviderMetadata must survive on new-format assistant messages")
	assert.Equal(t, "ephemeral", cc.Type)
}

// TestQueuedMessageRoundTrip verifies that a user message with
// file-reference parts survives the queue → promote cycle. The
// queued path stores MarshalParts output as raw JSON in
// chat_queued_messages, db2sdk.ChatQueuedMessage parses it for
// display while queued, then PromoteQueued copies the same raw
// bytes into chat_messages where ParseContent reads them.
func TestQueuedMessageRoundTrip(t *testing.T) {
	t.Parallel()

	// Simulate the write path: user sends a message with text +
	// file-reference, which gets queued.
	parts := []codersdk.ChatMessagePart{
		{Type: codersdk.ChatMessagePartTypeText, Text: "Review this change."},
		{Type: codersdk.ChatMessagePartTypeFileReference, FileName: "api.go", StartLine: 42, EndLine: 58, Content: "func handleRequest() {}"},
	}
	raw, err := chatprompt.MarshalParts(parts)
	require.NoError(t, err)

	// Step 1: While queued, db2sdk.ChatQueuedMessage parses the
	// content for display. Verify it produces correct parts
	// (with internal fields stripped).
	queuedMsg := db2sdk.ChatQueuedMessage(database.ChatQueuedMessage{
		ID:      1,
		ChatID:  uuid.New(),
		Content: raw.RawMessage,
	})
	require.Len(t, queuedMsg.Content, 2)
	assert.Equal(t, codersdk.ChatMessagePartTypeText, queuedMsg.Content[0].Type)
	assert.Equal(t, "Review this change.", queuedMsg.Content[0].Text)
	assert.Equal(t, codersdk.ChatMessagePartTypeFileReference, queuedMsg.Content[1].Type)
	assert.Equal(t, "api.go", queuedMsg.Content[1].FileName)
	assert.Equal(t, 42, queuedMsg.Content[1].StartLine)
	assert.Equal(t, 58, queuedMsg.Content[1].EndLine)
	assert.Equal(t, "func handleRequest() {}", queuedMsg.Content[1].Content)

	// Step 2: PromoteQueued copies the raw bytes into
	// chat_messages. ParseContent must handle them identically.
	promoted, err := chatprompt.ParseContent(testMsg(codersdk.ChatMessageRoleUser, pqtype.NullRawMessage{
		RawMessage: raw.RawMessage,
		Valid:      true,
	}))
	require.NoError(t, err)
	require.Len(t, promoted, 2)
	assert.Equal(t, codersdk.ChatMessagePartTypeText, promoted[0].Type)
	assert.Equal(t, "Review this change.", promoted[0].Text)
	assert.Equal(t, codersdk.ChatMessagePartTypeFileReference, promoted[1].Type)
	assert.Equal(t, "api.go", promoted[1].FileName)
	assert.Equal(t, 42, promoted[1].StartLine)
	assert.Equal(t, 58, promoted[1].EndLine)
	assert.Equal(t, "func handleRequest() {}", promoted[1].Content)

	// Step 3: The promoted message is used for LLM dispatch.
	// File-reference becomes a TextPart.
	prompt, err := chatprompt.ConvertMessagesWithFiles(
		context.Background(),
		[]database.ChatMessage{{
			Role:       database.ChatMessageRoleUser,
			Visibility: database.ChatMessageVisibilityBoth,
			Content:    pqtype.NullRawMessage{RawMessage: raw.RawMessage, Valid: true},
		}},
		nil,
		slogtest.Make(t, nil),
	)
	require.NoError(t, err)
	require.Len(t, prompt, 1)
	require.Len(t, prompt[0].Content, 2)

	textPart, ok := fantasy.AsMessagePart[fantasy.TextPart](prompt[0].Content[0])
	require.True(t, ok)
	assert.Equal(t, "Review this change.", textPart.Text)

	refPart, ok := fantasy.AsMessagePart[fantasy.TextPart](prompt[0].Content[1])
	require.True(t, ok)
	assert.Contains(t, refPart.Text, "[file-reference]")
	assert.Contains(t, refPart.Text, "api.go")
}

func TestParseContent_ErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("null_content_returns_nil", func(t *testing.T) {
		t.Parallel()
		parts, err := chatprompt.ParseContent(testMsg(codersdk.ChatMessageRoleUser, pqtype.NullRawMessage{}))
		require.NoError(t, err)
		assert.Nil(t, parts)
	})

	t.Run("empty_content_returns_nil", func(t *testing.T) {
		t.Parallel()
		parts, err := chatprompt.ParseContent(testMsg(codersdk.ChatMessageRoleAssistant, pqtype.NullRawMessage{
			RawMessage: []byte{},
			Valid:      true,
		}))
		require.NoError(t, err)
		assert.Nil(t, parts)
	})

	t.Run("unknown_role", func(t *testing.T) {
		t.Parallel()
		_, err := chatprompt.ParseContent(testMsg(codersdk.ChatMessageRole("banana"), nullRaw(json.RawMessage(`"hello"`))))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported chat message role")
	})

	t.Run("system/malformed_json", func(t *testing.T) {
		t.Parallel()
		_, err := chatprompt.ParseContent(testMsg(codersdk.ChatMessageRoleSystem, nullRaw(json.RawMessage(`not json`))))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse system content")
	})

	t.Run("user/malformed_json", func(t *testing.T) {
		t.Parallel()
		_, err := chatprompt.ParseContent(testMsg(codersdk.ChatMessageRoleUser, nullRaw(json.RawMessage(`{not json`))))
		require.Error(t, err)
	})

	t.Run("assistant/malformed_json", func(t *testing.T) {
		t.Parallel()
		_, err := chatprompt.ParseContent(testMsg(codersdk.ChatMessageRoleAssistant, nullRaw(json.RawMessage(`{not json`))))
		require.Error(t, err)
	})

	t.Run("tool/malformed_json", func(t *testing.T) {
		t.Parallel()
		_, err := chatprompt.ParseContent(testMsg(codersdk.ChatMessageRoleTool, nullRaw(json.RawMessage(`{not json`))))
		require.Error(t, err)
	})
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return data
}

func mustMarshalContent(t *testing.T, content []fantasy.Content) pqtype.NullRawMessage {
	t.Helper()
	result, err := chatprompt.MarshalContent(content, nil)
	require.NoError(t, err)
	return result
}

func mustMarshalToolResult(t *testing.T, toolCallID, toolName string, result json.RawMessage, isError, isMedia, providerExecuted bool) pqtype.NullRawMessage {
	t.Helper()
	raw, err := chatprompt.MarshalToolResult(toolCallID, toolName, result, isError, isMedia, providerExecuted, nil)
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

func TestNulEscapeRoundTrip(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitShort)

	// Seed minimal dependencies for the DB round-trip path:
	// user, provider, model config, chat.
	user := dbgen.User(t, db, database.User{})

	_, err := db.InsertChatProvider(ctx, database.InsertChatProviderParams{
		Provider:             "openai",
		DisplayName:          "openai",
		APIKey:               "test-key",
		CreatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		Enabled:              true,
		CentralApiKeyEnabled: true,
	})
	require.NoError(t, err)

	model, err := db.InsertChatModelConfig(ctx, database.InsertChatModelConfigParams{
		Provider:             "openai",
		Model:                "gpt-4o-mini",
		DisplayName:          "Test Model",
		CreatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		Enabled:              true,
		IsDefault:            true,
		ContextLimit:         128000,
		CompressionThreshold: 70,
		Options:              json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	org := dbgen.Organization(t, db, database.Organization{})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})

	chat, err := db.InsertChat(ctx, database.InsertChatParams{
		OrganizationID:    org.ID,
		Status:            database.ChatStatusWaiting,
		ClientType:        database.ChatClientTypeUi,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
		Title:             "nul-roundtrip-test",
	})
	require.NoError(t, err)

	textTests := []struct {
		name   string
		input  string
		hasNul bool // Whether the input contains actual NUL bytes.
	}{
		// --- basic ---
		{"NoNul", "hello world", false},
		{"SingleNul", "a\x00b", true},
		{"MultipleNuls", "a\x00b\x00c", true},
		{"ConsecutiveNuls", "\x00\x00\x00", true},

		// --- boundaries ---
		{"EmptyString", "", false},
		{"NulOnly", "\x00", true},
		{"NulAtStart", "\x00hello", true},
		{"NulAtEnd", "hello\x00", true},

		// --- sentinel / marker in original data ---
		// U+E000 is the sentinel character. The encoder must
		// double it so it round-trips without being mistaken
		// for an encoded NUL.
		{"SentinelInOriginal", "a\uE000b", false},
		{"ConsecutiveSentinels", "\uE000\uE000\uE000", false},
		// U+E001 is the marker character used in the NUL pair.
		{"MarkerCharInOriginal", "a\uE001b", false},
		// U+E000 followed by U+E001 looks exactly like an
		// encoded NUL in the encoded form, so the encoder must
		// double the U+E000 to avoid confusion.
		{"SentinelThenMarkerChar", "\uE000\uE001", false},
		{"NulAndSentinel", "a\x00b\uE000c", true},
		// Both orders: sentinel adjacent to NUL.
		{"SentinelThenNul", "\uE000\x00", true},
		{"NulThenSentinel", "\x00\uE000", true},
		{"AlternatingSentinelNul", "\x00\uE000\x00\uE000", true},

		// --- strings containing backslashes ---
		// Backslashes are normal characters at the Go string
		// level; no special handling needed (unlike the old
		// JSON-byte approach).
		{"BackslashU0000Text", "\\u0000", false},
		{"BackslashThenNul", "\\\x00", true},

		// --- literal text that looks like escape patterns ---
		{"LiteralTextU0000", "the value is u0000 here", false},
		{"LiteralTextUE000", "sentinel uE000 text", false},

		// --- other control characters mixed with NUL ---
		{"ControlCharsMixedWithNul", "\x01\x00\x02\x00\x1f", true},

		// --- long / stress ---
		{"LongNulRun", "\x00\x00\x00\x00\x00\x00\x00\x00", true},
		// Simulated find -print0 output.
		{"FindPrint0", "/usr/bin/ls\x00/usr/bin/cat\x00/usr/bin/grep\x00", true},
	}

	for _, tc := range textTests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			parts := []codersdk.ChatMessagePart{
				codersdk.ChatMessageText(tc.input),
			}

			encoded, err := chatprompt.MarshalParts(parts)
			require.NoError(t, err)

			// When the input has real NUL bytes, the stored JSON
			// must not contain the \u0000 escape sequence.
			if tc.hasNul {
				require.NotContains(t, string(encoded.RawMessage), `\u0000`,
					"encoded JSON must not contain \\u0000")
			}

			// In-memory round-trip through ParseContent.
			msg := testMsgV1(codersdk.ChatMessageRoleAssistant, encoded)
			decoded, err := chatprompt.ParseContent(msg)
			require.NoError(t, err)

			require.Len(t, decoded, 1)
			require.Equal(t, tc.input, decoded[0].Text)

			// Full DB round-trip: write to PostgreSQL jsonb, read
			// back, and verify the value survives storage.
			ctx := testutil.Context(t, testutil.WaitShort)
			dbMsgs, err := db.InsertChatMessages(ctx, database.InsertChatMessagesParams{
				ChatID:              chat.ID,
				CreatedBy:           []uuid.UUID{user.ID},
				ModelConfigID:       []uuid.UUID{model.ID},
				Role:                []database.ChatMessageRole{database.ChatMessageRoleAssistant},
				Content:             []string{string(encoded.RawMessage)},
				ContentVersion:      []int16{chatprompt.CurrentContentVersion},
				Visibility:          []database.ChatMessageVisibility{database.ChatMessageVisibilityBoth},
				InputTokens:         []int64{0},
				OutputTokens:        []int64{0},
				TotalTokens:         []int64{0},
				ReasoningTokens:     []int64{0},
				CacheCreationTokens: []int64{0},
				CacheReadTokens:     []int64{0},
				ContextLimit:        []int64{0},
				Compressed:          []bool{false},
				TotalCostMicros:     []int64{0},
				RuntimeMs:           []int64{0},
			})
			require.NoError(t, err)
			require.Len(t, dbMsgs, 1)

			readBack, err := db.GetChatMessageByID(ctx, dbMsgs[0].ID)
			require.NoError(t, err)

			dbDecoded, err := chatprompt.ParseContent(readBack)
			require.NoError(t, err)
			require.Len(t, dbDecoded, 1)
			require.Equal(t, tc.input, dbDecoded[0].Text)
		})
	}

	// Tool result with NUL in the result JSON value.
	t.Run("ToolResultWithNul", func(t *testing.T) {
		t.Parallel()

		resultJSON := json.RawMessage(`"output:\u0000done"`)
		parts := []codersdk.ChatMessagePart{
			codersdk.ChatMessageToolResult("call-1", "my_tool", resultJSON, false, false),
		}

		encoded, err := chatprompt.MarshalParts(parts)
		require.NoError(t, err)
		require.NotContains(t, string(encoded.RawMessage), `\u0000`,
			"encoded JSON must not contain \\u0000")

		msg := testMsgV1(codersdk.ChatMessageRoleTool, encoded)
		decoded, err := chatprompt.ParseContent(msg)
		require.NoError(t, err)
		require.Len(t, decoded, 1)
		// JSON re-serialization may reformat, so compare
		// semantically.
		assert.JSONEq(t, string(resultJSON), string(decoded[0].Result))
	})

	// Multiple parts in one message: one with NUL, one without.
	t.Run("MultiPartMixed", func(t *testing.T) {
		t.Parallel()

		parts := []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("clean text"),
			codersdk.ChatMessageText("has\x00nul"),
		}

		encoded, err := chatprompt.MarshalParts(parts)
		require.NoError(t, err)
		require.NotContains(t, string(encoded.RawMessage), `\u0000`,
			"encoded JSON must not contain \\u0000")

		msg := testMsgV1(codersdk.ChatMessageRoleAssistant, encoded)
		decoded, err := chatprompt.ParseContent(msg)
		require.NoError(t, err)
		require.Len(t, decoded, 2)
		require.Equal(t, "clean text", decoded[0].Text)
		require.Equal(t, "has\x00nul", decoded[1].Text)
	})
}

func TestConvertMessagesWithFiles_FiltersEmptyTextAndReasoningParts(t *testing.T) {
	t.Parallel()

	// Helper to build a DB message from SDK parts.
	makeMsg := func(t *testing.T, role database.ChatMessageRole, parts []codersdk.ChatMessagePart) database.ChatMessage {
		t.Helper()
		encoded, err := chatprompt.MarshalParts(parts)
		require.NoError(t, err)
		return database.ChatMessage{
			Role:           role,
			Visibility:     database.ChatMessageVisibilityBoth,
			Content:        encoded,
			ContentVersion: chatprompt.CurrentContentVersion,
		}
	}

	t.Run("UserRole", func(t *testing.T) {
		t.Parallel()

		parts := []codersdk.ChatMessagePart{
			codersdk.ChatMessageText(""),                     // empty — filtered
			codersdk.ChatMessageText("   \t\n "),             // whitespace — filtered
			codersdk.ChatMessageReasoning(""),                // empty — filtered
			codersdk.ChatMessageReasoning("  \n"),            // whitespace — filtered
			codersdk.ChatMessageText("hello"),                // kept
			codersdk.ChatMessageText("  hello  "),            // kept with original whitespace
			codersdk.ChatMessageReasoning("thinking deeply"), // kept
			codersdk.ChatMessageToolCall("call-1", "my_tool", json.RawMessage(`{"x":1}`)),
			codersdk.ChatMessageToolResult("call-1", "my_tool", json.RawMessage(`{"ok":true}`), false, false),
		}

		prompt, err := chatprompt.ConvertMessagesWithFiles(
			context.Background(),
			[]database.ChatMessage{makeMsg(t, database.ChatMessageRoleUser, parts)},
			nil,
			slogtest.Make(t, nil),
		)
		require.NoError(t, err)
		require.Len(t, prompt, 1)

		resultParts := prompt[0].Content
		require.Len(t, resultParts, 5, "expected 5 parts after filtering empty text/reasoning")

		textPart, ok := fantasy.AsMessagePart[fantasy.TextPart](resultParts[0])
		require.True(t, ok, "expected TextPart at index 0")
		require.Equal(t, "hello", textPart.Text)

		// Leading/trailing whitespace is preserved — only
		// all-whitespace parts are dropped.
		paddedPart, ok := fantasy.AsMessagePart[fantasy.TextPart](resultParts[1])
		require.True(t, ok, "expected TextPart at index 1")
		require.Equal(t, "  hello  ", paddedPart.Text)

		reasoningPart, ok := fantasy.AsMessagePart[fantasy.ReasoningPart](resultParts[2])
		require.True(t, ok, "expected ReasoningPart at index 2")
		require.Equal(t, "thinking deeply", reasoningPart.Text)

		toolCallPart, ok := fantasy.AsMessagePart[fantasy.ToolCallPart](resultParts[3])
		require.True(t, ok, "expected ToolCallPart at index 3")
		require.Equal(t, "call-1", toolCallPart.ToolCallID)

		toolResultPart, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](resultParts[4])
		require.True(t, ok, "expected ToolResultPart at index 4")
		require.Equal(t, "call-1", toolResultPart.ToolCallID)
	})

	t.Run("AssistantRole", func(t *testing.T) {
		t.Parallel()

		parts := []codersdk.ChatMessagePart{
			codersdk.ChatMessageText(""),          // empty — filtered
			codersdk.ChatMessageText(" "),         // whitespace — filtered
			codersdk.ChatMessageReasoning(""),     // empty — filtered
			codersdk.ChatMessageText("  reply  "), // kept with whitespace
			codersdk.ChatMessageToolCall("tc-1", "read_file", json.RawMessage(`{"path":"x"}`)),
		}

		prompt, err := chatprompt.ConvertMessagesWithFiles(
			context.Background(),
			[]database.ChatMessage{makeMsg(t, database.ChatMessageRoleAssistant, parts)},
			nil,
			slogtest.Make(t, nil),
		)
		require.NoError(t, err)
		// 2 messages: assistant + synthetic tool result injected
		// by injectMissingToolResults for the unmatched tool call.
		require.Len(t, prompt, 2)

		resultParts := prompt[0].Content
		require.Len(t, resultParts, 2, "expected text + tool-call after filtering")

		textPart, ok := fantasy.AsMessagePart[fantasy.TextPart](resultParts[0])
		require.True(t, ok, "expected TextPart")
		require.Equal(t, "  reply  ", textPart.Text)

		tcPart, ok := fantasy.AsMessagePart[fantasy.ToolCallPart](resultParts[1])
		require.True(t, ok, "expected ToolCallPart")
		require.Equal(t, "tc-1", tcPart.ToolCallID)
	})

	t.Run("AllEmptyDropsMessage", func(t *testing.T) {
		t.Parallel()

		// When every part is filtered, the message itself should
		// be dropped rather than appending an empty-content message.
		parts := []codersdk.ChatMessagePart{
			codersdk.ChatMessageText(""),
			codersdk.ChatMessageText("   "),
			codersdk.ChatMessageReasoning(""),
		}

		prompt, err := chatprompt.ConvertMessagesWithFiles(
			context.Background(),
			[]database.ChatMessage{makeMsg(t, database.ChatMessageRoleAssistant, parts)},
			nil,
			slogtest.Make(t, nil),
		)
		require.NoError(t, err)
		require.Empty(t, prompt, "all-empty message should be dropped entirely")
	})
}

func TestConvertMessagesWithFiles_PasteTextBecomesTextPart(t *testing.T) {
	t.Parallel()

	fileID := uuid.New()
	prompt := convertSingleResolvedFileMessage(t, fileID, chatprompt.FileData{
		Name:      "pasted-text-2025-01-01-12-00-00.txt",
		Data:      []byte("hello world"),
		MediaType: "text/plain",
	})

	require.Len(t, prompt, 1)
	require.Len(t, prompt[0].Content, 1)

	textPart, ok := fantasy.AsMessagePart[fantasy.TextPart](prompt[0].Content[0])
	require.True(t, ok, "expected TextPart")

	_, isFilePart := fantasy.AsMessagePart[fantasy.FilePart](prompt[0].Content[0])
	require.False(t, isFilePart, "synthetic pasted text should not remain a FilePart")
	require.Contains(t, textPart.Text, "The user pasted text into the chat UI")
	require.Contains(t, textPart.Text, "hello world")
}

func TestConvertMessagesWithFiles_PasteTextTruncatesAtBudget(t *testing.T) {
	t.Parallel()

	fileID := uuid.New()
	body := bytes.Repeat([]byte("x"), 200000)
	prompt := convertSingleResolvedFileMessage(t, fileID, chatprompt.FileData{
		Name:      "pasted-text-2025-01-01-12-00-00.txt",
		Data:      body,
		MediaType: "text/plain",
	})

	require.Len(t, prompt, 1)
	require.Len(t, prompt[0].Content, 1)

	textPart, ok := fantasy.AsMessagePart[fantasy.TextPart](prompt[0].Content[0])
	require.True(t, ok, "expected TextPart")
	require.Contains(t, textPart.Text, "The pasted text was truncated to 131072 bytes")

	const attachmentHeader = "Synthetic attachment name: pasted-text-2025-01-01-12-00-00.txt\n\n"
	bodyStart := strings.Index(textPart.Text, attachmentHeader)
	require.NotEqual(t, -1, bodyStart, "expected synthetic attachment header")
	bodyStart += len(attachmentHeader)

	warningIndex := strings.Index(textPart.Text, "\n\n[pasted-text] The pasted text was truncated to 131072 bytes before sending to the model.")
	require.NotEqual(t, -1, warningIndex, "expected truncation warning")
	require.Equal(t, string(body[:128*1024]), textPart.Text[bodyStart:warningIndex])
}

func TestConvertMessagesWithFiles_BinaryPasteNameStillStaysFilePart(t *testing.T) {
	t.Parallel()

	fileID := uuid.New()
	prompt := convertSingleResolvedFileMessage(t, fileID, chatprompt.FileData{
		Name:      "pasted-text-2025-01-01-12-00-00.txt",
		Data:      []byte("not-really-a-png"),
		MediaType: "image/png",
	})

	require.Len(t, prompt, 1)
	require.Len(t, prompt[0].Content, 1)

	filePart, ok := fantasy.AsMessagePart[fantasy.FilePart](prompt[0].Content[0])
	require.True(t, ok, "expected FilePart")

	_, isTextPart := fantasy.AsMessagePart[fantasy.TextPart](prompt[0].Content[0])
	require.False(t, isTextPart, "binary media should stay a FilePart")
	require.Equal(t, "image/png", filePart.MediaType)
}

func TestConvertMessagesWithFiles_NonPasteTextFileStillStaysFilePart(t *testing.T) {
	t.Parallel()

	fileID := uuid.New()
	prompt := convertSingleResolvedFileMessage(t, fileID, chatprompt.FileData{
		Name:      "report.txt",
		Data:      []byte("plain text report"),
		MediaType: "text/plain",
	})

	require.Len(t, prompt, 1)
	require.Len(t, prompt[0].Content, 1)

	filePart, ok := fantasy.AsMessagePart[fantasy.FilePart](prompt[0].Content[0])
	require.True(t, ok, "expected FilePart")

	_, isTextPart := fantasy.AsMessagePart[fantasy.TextPart](prompt[0].Content[0])
	require.False(t, isTextPart, "non-synthetic text files should stay FilePart attachments")
	require.Equal(t, []byte("plain text report"), filePart.Data)
}

func TestConvertMessagesWithFiles_IsSyntheticPaste(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		fileName  string
		mediaType string
		want      bool
	}{
		{name: "plain text", fileName: "pasted-text-2025-01-01-12-00-00.txt", mediaType: "text/plain", want: true},
		{name: "markdown", fileName: "pasted-text-2025-01-01-12-00-00.txt", mediaType: "text/markdown", want: true},
		{name: "json", fileName: "pasted-text-2025-01-01-12-00-00.txt", mediaType: "application/json", want: true},
		{name: "binary mime", fileName: "pasted-text-2025-01-01-12-00-00.txt", mediaType: "image/png", want: false},
		{name: "non synthetic name", fileName: "report.txt", mediaType: "text/plain", want: false},
		{name: "malformed timestamp", fileName: "pasted-text-2025-01-01.txt", mediaType: "text/plain", want: false},
		{name: "wrong extension", fileName: "pasted-text-2025-01-01-12-00-00.md", mediaType: "text/plain", want: false},
		{name: "empty name", fileName: "", mediaType: "text/plain", want: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, chatprompt.IsSyntheticPasteForTest(tt.fileName, tt.mediaType))
		})
	}
}

func convertSingleResolvedFileMessage(t *testing.T, fileID uuid.UUID, fileData chatprompt.FileData) []fantasy.Message {
	t.Helper()

	rawContent := mustJSON(t, []json.RawMessage{
		mustJSON(t, map[string]any{
			"type": "file",
			"data": map[string]any{
				"media_type": fileData.MediaType,
				"file_id":    fileID.String(),
			},
		}),
	})

	resolver := func(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]chatprompt.FileData, error) {
		result := make(map[uuid.UUID]chatprompt.FileData)
		for _, id := range ids {
			if id == fileID {
				result[id] = fileData
			}
		}
		return result, nil
	}

	prompt, err := chatprompt.ConvertMessagesWithFiles(
		context.Background(),
		[]database.ChatMessage{{
			Role:       database.ChatMessageRoleUser,
			Visibility: database.ChatMessageVisibilityBoth,
			Content:    pqtype.NullRawMessage{RawMessage: rawContent, Valid: true},
		}},
		resolver,
		slogtest.Make(t, nil),
	)
	require.NoError(t, err)
	return prompt
}

func TestMediaToolResultRoundTrip(t *testing.T) {
	t.Parallel()

	// Full DB round-trip test: insert messages into PostgreSQL,
	// load them back via GetChatMessagesForPromptByChatID, and
	// verify the fantasy message parts are identical after the
	// round-trip.
	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitShort)

	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})

	_, err := db.InsertChatProvider(ctx, database.InsertChatProviderParams{
		Provider:             "anthropic",
		DisplayName:          "anthropic",
		APIKey:               "test-key",
		CreatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		Enabled:              true,
		CentralApiKeyEnabled: true,
	})
	require.NoError(t, err)

	model, err := db.InsertChatModelConfig(ctx, database.InsertChatModelConfigParams{
		Provider:             "anthropic",
		Model:                "test-model",
		DisplayName:          "Test Model",
		CreatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		Enabled:              true,
		IsDefault:            true,
		ContextLimit:         200000,
		CompressionThreshold: 70,
		Options:              json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	// Small base64 payload standing in for a real screenshot.
	const imageData = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAAC0lEQVQI12NgAAIABQAB"

	// insertPair writes an assistant tool-call message and a
	// tool-result message into the database, returning the chat
	// they belong to.
	insertPair := func(
		t *testing.T,
		callID, toolName string,
		resultParts []codersdk.ChatMessagePart,
	) database.Chat {
		t.Helper()

		chat, chatErr := db.InsertChat(ctx, database.InsertChatParams{
			OrganizationID:    org.ID,
			Status:            database.ChatStatusWaiting,
			ClientType:        database.ChatClientTypeUi,
			OwnerID:           user.ID,
			LastModelConfigID: model.ID,
			Title:             "media-roundtrip-" + callID,
		})
		require.NoError(t, chatErr)

		// Assistant message with the tool call.
		callPart := codersdk.ChatMessageToolCall(callID, toolName, json.RawMessage(`{}`))
		assistantEncoded, encErr := chatprompt.MarshalParts([]codersdk.ChatMessagePart{callPart})
		require.NoError(t, encErr)

		// Tool result message.
		resultEncoded, encErr := chatprompt.MarshalParts(resultParts)
		require.NoError(t, encErr)

		_, insertErr := db.InsertChatMessages(ctx, database.InsertChatMessagesParams{
			ChatID:              chat.ID,
			CreatedBy:           []uuid.UUID{user.ID, user.ID},
			ModelConfigID:       []uuid.UUID{model.ID, model.ID},
			Role:                []database.ChatMessageRole{database.ChatMessageRoleAssistant, database.ChatMessageRoleTool},
			Content:             []string{string(assistantEncoded.RawMessage), string(resultEncoded.RawMessage)},
			ContentVersion:      []int16{chatprompt.CurrentContentVersion, chatprompt.CurrentContentVersion},
			Visibility:          []database.ChatMessageVisibility{database.ChatMessageVisibilityBoth, database.ChatMessageVisibilityBoth},
			InputTokens:         []int64{0, 0},
			OutputTokens:        []int64{0, 0},
			TotalTokens:         []int64{0, 0},
			ReasoningTokens:     []int64{0, 0},
			CacheCreationTokens: []int64{0, 0},
			CacheReadTokens:     []int64{0, 0},
			ContextLimit:        []int64{0, 0},
			Compressed:          []bool{false, false},
			TotalCostMicros:     []int64{0, 0},
			RuntimeMs:           []int64{0, 0},
		})
		require.NoError(t, insertErr)
		return chat
	}

	// loadPrompt reads messages back from the DB via the same
	// path used by runChat, and converts them to fantasy messages.
	loadPrompt := func(t *testing.T, chat database.Chat) []fantasy.Message {
		t.Helper()
		dbMsgs, loadErr := db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
		require.NoError(t, loadErr)
		prompt, convErr := chatprompt.ConvertMessagesWithFiles(
			ctx, dbMsgs, nil, slogtest.Make(t, nil),
		)
		require.NoError(t, convErr)
		return prompt
	}

	t.Run("MediaResultRoundTripsAsMedia", func(t *testing.T) {
		t.Parallel()

		const callID = "call-screenshot-1"
		const toolName = "computer"
		const mimeType = "image/png"

		// Use PartFromContent (the production write path) to
		// produce the SDK part, rather than hand-crafting JSON.
		// Computer use is a provider-defined tool, but Coder executes it
		// locally via chatloop.ProviderTool.Runner, so screenshot results
		// persist as tool-role messages with ProviderExecuted=false.
		sdkPart := chatprompt.PartFromContent(fantasy.ToolResultContent{
			ToolCallID: callID,
			ToolName:   toolName,
			Result: fantasy.ToolResultOutputContentMedia{
				Data:      imageData,
				MediaType: mimeType,
			},
		})

		chat := insertPair(t, callID, toolName, []codersdk.ChatMessagePart{sdkPart})

		prompt := loadPrompt(t, chat)
		// assistant + tool
		require.Len(t, prompt, 2)

		toolMsg := prompt[1]
		require.Equal(t, fantasy.MessageRoleTool, toolMsg.Role)
		require.Len(t, toolMsg.Content, 1)

		resultPart, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](toolMsg.Content[0])
		require.True(t, ok, "expected ToolResultPart")
		require.Equal(t, callID, resultPart.ToolCallID)
		require.False(t, resultPart.ProviderExecuted)

		mediaOutput, ok := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentMedia](resultPart.Output)
		require.True(t, ok, "expected ToolResultOutputContentMedia, got %T", resultPart.Output)
		require.Equal(t, imageData, mediaOutput.Data)
		require.Equal(t, mimeType, mediaOutput.MediaType)
	})

	t.Run("MediaResultWithText", func(t *testing.T) {
		t.Parallel()

		const callID = "call-screenshot-2"
		const toolName = "computer"
		const mimeType = "image/png"

		sdkPart := chatprompt.PartFromContent(fantasy.ToolResultContent{
			ToolCallID: callID,
			ToolName:   toolName,
			Result: fantasy.ToolResultOutputContentMedia{
				Data:      imageData,
				MediaType: mimeType,
				Text:      "screenshot after click",
			},
		})

		chat := insertPair(t, callID, toolName, []codersdk.ChatMessagePart{sdkPart})

		prompt := loadPrompt(t, chat)
		require.Len(t, prompt, 2)

		resultPart, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](prompt[1].Content[0])
		require.True(t, ok)
		require.False(t, resultPart.ProviderExecuted)

		mediaOutput, ok := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentMedia](resultPart.Output)
		require.True(t, ok, "expected media output")
		require.Equal(t, imageData, mediaOutput.Data)
		require.Equal(t, mimeType, mediaOutput.MediaType)
		require.Equal(t, "screenshot after click", mediaOutput.Text)
	})

	t.Run("TextResultStaysText", func(t *testing.T) {
		t.Parallel()

		const callID = "call-text-1"
		const toolName = "read_file"

		textResult := json.RawMessage(`{"output":"file contents here"}`)

		chat := insertPair(t, callID, toolName, []codersdk.ChatMessagePart{
			codersdk.ChatMessageToolResult(callID, toolName, textResult, false, false),
		})

		prompt := loadPrompt(t, chat)
		require.Len(t, prompt, 2)

		resultPart, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](prompt[1].Content[0])
		require.True(t, ok)

		_, isMedia := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentMedia](resultPart.Output)
		require.False(t, isMedia, "text result should not be detected as media")

		textOutput, ok := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentText](resultPart.Output)
		require.True(t, ok, "expected ToolResultOutputContentText")
		require.JSONEq(t, string(textResult), textOutput.Text)
	})

	t.Run("MissingMimeTypeStaysText", func(t *testing.T) {
		t.Parallel()

		const callID = "call-no-mime"
		const toolName = "computer"

		noMimeJSON := json.RawMessage(`{"data":"some_base64","text":""}`)

		chat := insertPair(t, callID, toolName, []codersdk.ChatMessagePart{
			codersdk.ChatMessageToolResult(callID, toolName, noMimeJSON, false, false),
		})

		prompt := loadPrompt(t, chat)
		require.Len(t, prompt, 2)

		resultPart, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](prompt[1].Content[0])
		require.True(t, ok)

		_, isMedia := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentMedia](resultPart.Output)
		require.False(t, isMedia, "missing mime_type should not produce media")
	})

	t.Run("MissingDataStaysText", func(t *testing.T) {
		t.Parallel()

		const callID = "call-no-data"
		const toolName = "computer"

		noDataJSON := json.RawMessage(`{"mime_type":"image/png","text":""}`)

		chat := insertPair(t, callID, toolName, []codersdk.ChatMessagePart{
			codersdk.ChatMessageToolResult(callID, toolName, noDataJSON, false, false),
		})

		prompt := loadPrompt(t, chat)
		require.Len(t, prompt, 2)

		resultPart, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](prompt[1].Content[0])
		require.True(t, ok)

		_, isMedia := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentMedia](resultPart.Output)
		require.False(t, isMedia, "missing data should not produce media")
	})

	t.Run("ErrorResultStaysError", func(t *testing.T) {
		t.Parallel()

		const callID = "call-err"
		const toolName = "computer"

		// Use PartFromContent to go through the production
		// write path for error results.
		sdkPart := chatprompt.PartFromContent(fantasy.ToolResultContent{
			ToolCallID: callID,
			ToolName:   toolName,
			Result: fantasy.ToolResultOutputContentError{
				Error: xerrors.New("screenshot failed"),
			},
		})

		chat := insertPair(t, callID, toolName, []codersdk.ChatMessagePart{sdkPart})

		prompt := loadPrompt(t, chat)
		require.Len(t, prompt, 2)

		resultPart, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](prompt[1].Content[0])
		require.True(t, ok)

		errOutput, isError := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentError](resultPart.Output)
		require.True(t, isError, "error result should remain error")
		require.Contains(t, errOutput.Error.Error(), "screenshot failed")
	})

	t.Run("NonMediaResultTypeStaysText", func(t *testing.T) {
		t.Parallel()

		// A text tool result that happens to contain "data" and
		// "mime_type" fields must NOT be misidentified as media
		// when IsMedia is false. The protection is entirely the
		// IsMedia boolean flag on the ChatMessagePart.
		const callID = "call-not-media"
		const toolName = "list_files"

		textJSON, jsonErr := json.Marshal(map[string]any{
			"result_type": "listing",
			"data":        "file1.txt",
			"mime_type":   "text/csv",
		})
		require.NoError(t, jsonErr)

		chat := insertPair(t, callID, toolName, []codersdk.ChatMessagePart{
			codersdk.ChatMessageToolResult(callID, toolName, textJSON, false, false),
		})

		prompt := loadPrompt(t, chat)
		require.Len(t, prompt, 2)

		resultPart, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](prompt[1].Content[0])
		require.True(t, ok)

		_, isMedia := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentMedia](resultPart.Output)
		require.False(t, isMedia, "non-media result_type must not be detected as media")

		textOutput, ok := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentText](resultPart.Output)
		require.True(t, ok, "expected ToolResultOutputContentText")
		require.JSONEq(t, string(textJSON), textOutput.Text)
	})

	t.Run("IsMediaTrueButMissingMimeType", func(t *testing.T) {
		t.Parallel()

		// IsMedia is true but the JSON payload has no mime_type
		// field. The media reconstruction guard should fail and
		// the result should fall through to text.
		const callID = "call-media-no-mime"
		const toolName = "computer"

		noMimeJSON := json.RawMessage(`{"data":"some_base64","text":""}`)

		chat := insertPair(t, callID, toolName, []codersdk.ChatMessagePart{
			codersdk.ChatMessageToolResult(callID, toolName, noMimeJSON, false, true),
		})

		prompt := loadPrompt(t, chat)
		require.Len(t, prompt, 2)

		resultPart, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](prompt[1].Content[0])
		require.True(t, ok)

		_, isMedia := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentMedia](resultPart.Output)
		require.False(t, isMedia, "IsMedia=true with missing mime_type should fall through to text")

		_, isText := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentText](resultPart.Output)
		require.True(t, isText, "expected ToolResultOutputContentText")
	})

	t.Run("IsMediaTrueButMissingData", func(t *testing.T) {
		t.Parallel()

		// IsMedia is true but the JSON payload has no data field.
		// The media reconstruction guard should fail and the result
		// should fall through to text.
		const callID = "call-media-no-data"
		const toolName = "computer"

		noDataJSON := json.RawMessage(`{"mime_type":"image/png","text":""}`)

		chat := insertPair(t, callID, toolName, []codersdk.ChatMessagePart{
			codersdk.ChatMessageToolResult(callID, toolName, noDataJSON, false, true),
		})

		prompt := loadPrompt(t, chat)
		require.Len(t, prompt, 2)

		resultPart, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](prompt[1].Content[0])
		require.True(t, ok)

		_, isMedia := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentMedia](resultPart.Output)
		require.False(t, isMedia, "IsMedia=true with missing data should fall through to text")

		_, isText := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentText](resultPart.Output)
		require.True(t, isText, "expected ToolResultOutputContentText")
	})

	t.Run("IsMediaTrueButGarbageJSON", func(t *testing.T) {
		t.Parallel()

		// IsMedia is true but the result is a JSON string, not
		// an object. Unmarshal into persistedMediaResult fails
		// and the result should fall through to text. Truly
		// invalid JSON cannot reach the read path because both
		// MarshalParts and PostgreSQL jsonb reject it, so a
		// non-object JSON value is the realistic edge case.
		const callID = "call-media-garbage"
		const toolName = "computer"

		garbageJSON := json.RawMessage(`"not a json object"`)

		chat := insertPair(t, callID, toolName, []codersdk.ChatMessagePart{
			codersdk.ChatMessageToolResult(callID, toolName, garbageJSON, false, true),
		})

		prompt := loadPrompt(t, chat)
		require.Len(t, prompt, 2)

		resultPart, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](prompt[1].Content[0])
		require.True(t, ok)

		_, isMedia := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentMedia](resultPart.Output)
		require.False(t, isMedia, "IsMedia=true with garbage JSON should fall through to text")

		_, isText := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentText](resultPart.Output)
		require.True(t, isText, "expected ToolResultOutputContentText")
	})
}

func TestPartFromContent_CreatedAtNotStamped(t *testing.T) {
	t.Parallel()

	// PartFromContent must NOT stamp CreatedAt itself.
	// The chatloop layer records timestamps separately and
	// the persistence layer applies them. PartFromContent
	// is called in multiple contexts (SSE publishing,
	// persistence) so stamping inside it would produce
	// inaccurate durations.

	t.Run("ToolCallHasNilCreatedAt", func(t *testing.T) {
		t.Parallel()
		part := chatprompt.PartFromContent(fantasy.ToolCallContent{
			ToolCallID: "tc-1",
			ToolName:   "execute",
		})
		assert.Nil(t, part.CreatedAt)
	})

	t.Run("ToolCallPointerHasNilCreatedAt", func(t *testing.T) {
		t.Parallel()
		part := chatprompt.PartFromContent(&fantasy.ToolCallContent{
			ToolCallID: "tc-1",
			ToolName:   "execute",
		})
		assert.Nil(t, part.CreatedAt)
	})

	t.Run("ToolResultHasNilCreatedAt", func(t *testing.T) {
		t.Parallel()
		part := chatprompt.PartFromContent(fantasy.ToolResultContent{
			ToolCallID: "tc-1",
			ToolName:   "execute",
			Result:     fantasy.ToolResultOutputContentText{Text: "{}"},
		})
		assert.Nil(t, part.CreatedAt)
	})

	t.Run("TextHasNilCreatedAt", func(t *testing.T) {
		t.Parallel()
		part := chatprompt.PartFromContent(fantasy.TextContent{Text: "hello"})
		assert.Nil(t, part.CreatedAt)
	})
}
