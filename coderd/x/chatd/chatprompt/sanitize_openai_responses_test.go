package chatprompt_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"charm.land/fantasy"
	fantasyopenai "charm.land/fantasy/providers/openai"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
)

func TestSanitizeOpenAIResponsesMessages(t *testing.T) {
	t.Parallel()

	result := fantasy.ToolResultOutputContentText{Text: `{"ok":true}`}
	textPart := fantasy.TextPart{Text: "keep text"}
	validCall := fantasy.ToolCallPart{
		ToolCallID: "call-valid",
		ToolName:   "read_file",
		Input:      `{"path":"main.go"}`,
	}
	validResult := fantasy.ToolResultPart{
		ToolCallID: "call-valid",
		Output:     result,
	}
	providerCall := fantasy.ToolCallPart{
		ToolCallID:       "call-provider",
		ToolName:         "code_interpreter",
		Input:            `{"code":"print(1)"}`,
		ProviderExecuted: true,
	}
	providerResult := fantasy.ToolResultPart{
		ToolCallID:       "call-provider",
		Output:           result,
		ProviderExecuted: true,
	}
	orphanCall := fantasy.ToolCallPart{
		ToolCallID: "call-orphan",
		ToolName:   "read_file",
		Input:      `{"path":"missing.go"}`,
	}
	orphanResult := fantasy.ToolResultPart{
		ToolCallID: "call-orphan-result",
		Output:     result,
	}
	webSearchCall := fantasy.ToolCallPart{
		ToolCallID:       "call-web",
		ToolName:         "web_search",
		Input:            `{"query":"coder"}`,
		ProviderExecuted: true,
	}
	webSearchResult := fantasy.ToolResultPart{
		ToolCallID:       "call-web",
		Output:           result,
		ProviderExecuted: true,
	}
	webSearchPreviewCall := fantasy.ToolCallPart{
		ToolCallID:       "call-web-preview",
		ToolName:         "web_search_preview",
		Input:            `{"query":"coder"}`,
		ProviderExecuted: true,
	}
	webSearchPreviewResult := fantasy.ToolResultPart{
		ToolCallID:       "call-web-preview",
		Output:           result,
		ProviderExecuted: true,
	}

	providerUser := "user-1"
	partOptions := fantasy.ProviderOptions{
		fantasyopenai.Name: &fantasyopenai.ResponsesProviderOptions{User: &providerUser},
	}
	reasoningPart := fantasy.ReasoningPart{
		Text:            "keep reasoning",
		ProviderOptions: partOptions,
	}
	filePart := fantasy.FilePart{
		Filename:        "keep.txt",
		Data:            []byte("keep file"),
		MediaType:       "text/plain",
		ProviderOptions: partOptions,
	}

	providerUserA := "user-a"
	providerUserB := "user-b"
	messageOptionsA := fantasy.ProviderOptions{
		fantasyopenai.Name: &fantasyopenai.ResponsesProviderOptions{User: &providerUserA},
	}
	messageOptionsB := fantasy.ProviderOptions{
		fantasyopenai.Name: &fantasyopenai.ResponsesProviderOptions{User: &providerUserB},
	}

	testCases := []struct {
		name          string
		messages      []fantasy.Message
		want          []fantasy.Message
		wantStats     chatprompt.OpenAIResponsesSanitizationStats
		wantSameInput bool
	}{
		{
			name: "KeepsValidLocalPair",
			messages: []fantasy.Message{
				{
					Role:    fantasy.MessageRoleAssistant,
					Content: []fantasy.MessagePart{validCall},
				},
				{
					Role:    fantasy.MessageRoleTool,
					Content: []fantasy.MessagePart{validResult},
				},
			},
			want: []fantasy.Message{
				{
					Role:    fantasy.MessageRoleAssistant,
					Content: []fantasy.MessagePart{validCall},
				},
				{
					Role:    fantasy.MessageRoleTool,
					Content: []fantasy.MessagePart{validResult},
				},
			},
			wantSameInput: true,
		},
		{
			name: "KeepsValidPairInSameAssistantMessage",
			messages: []fantasy.Message{{
				Role:    fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{validCall, validResult},
			}},
			want: []fantasy.Message{{
				Role:    fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{validCall, validResult},
			}},
			wantSameInput: true,
		},
		{
			name: "KeepsProviderExecutedNonWebPair",
			messages: []fantasy.Message{{
				Role:    fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{providerCall, providerResult},
			}},
			want: []fantasy.Message{{
				Role:    fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{providerCall, providerResult},
			}},
			wantSameInput: true,
		},
		{
			name: "DropsOrphanAssistantToolCall",
			messages: []fantasy.Message{{
				Role:    fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{orphanCall},
			}},
			want: []fantasy.Message{},
			wantStats: chatprompt.OpenAIResponsesSanitizationStats{
				RemovedToolCalls:   1,
				DroppedMessages:    1,
				UnsafeForChainMode: true,
			},
		},
		{
			name: "DropsOrphanToolResult",
			messages: []fantasy.Message{{
				Role:    fantasy.MessageRoleTool,
				Content: []fantasy.MessagePart{orphanResult},
			}},
			want: []fantasy.Message{},
			wantStats: chatprompt.OpenAIResponsesSanitizationStats{
				RemovedToolResults: 1,
				DroppedMessages:    1,
				UnsafeForChainMode: true,
			},
		},
		{
			name: "RemovesEmptyToolCallIDParts",
			messages: []fantasy.Message{{
				Role: fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{
					fantasy.ToolCallPart{ToolName: "read_file", Input: `{}`},
					fantasy.ToolResultPart{Output: result},
				},
			}},
			want: []fantasy.Message{},
			wantStats: chatprompt.OpenAIResponsesSanitizationStats{
				RemovedToolCalls:   1,
				RemovedToolResults: 1,
				DroppedMessages:    1,
				UnsafeForChainMode: true,
			},
		},
		{
			name: "PreservesSurroundingContentAndOptions",
			messages: []fantasy.Message{
				{
					Role: fantasy.MessageRoleAssistant,
					Content: []fantasy.MessagePart{
						textPart,
						orphanCall,
						reasoningPart,
						filePart,
						validCall,
					},
				},
				{
					Role:    fantasy.MessageRoleTool,
					Content: []fantasy.MessagePart{validResult},
				},
			},
			want: []fantasy.Message{
				{
					Role: fantasy.MessageRoleAssistant,
					Content: []fantasy.MessagePart{
						textPart,
						reasoningPart,
						filePart,
						validCall,
					},
				},
				{
					Role:    fantasy.MessageRoleTool,
					Content: []fantasy.MessagePart{validResult},
				},
			},
			wantStats: chatprompt.OpenAIResponsesSanitizationStats{
				RemovedToolCalls:   1,
				UnsafeForChainMode: true,
			},
		},
		{
			name: "RemovesOnlyInvalidIDFromMixedMessage",
			messages: []fantasy.Message{{
				Role: fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{
					validCall,
					orphanCall,
					validResult,
				},
			}},
			want: []fantasy.Message{{
				Role: fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{
					validCall,
					validResult,
				},
			}},
			wantStats: chatprompt.OpenAIResponsesSanitizationStats{
				RemovedToolCalls:   1,
				UnsafeForChainMode: true,
			},
		},
		{
			name: "RemovesResultBeforeCall",
			messages: []fantasy.Message{
				{
					Role:    fantasy.MessageRoleTool,
					Content: []fantasy.MessagePart{validResult},
				},
				{
					Role:    fantasy.MessageRoleAssistant,
					Content: []fantasy.MessagePart{validCall},
				},
			},
			want: []fantasy.Message{},
			wantStats: chatprompt.OpenAIResponsesSanitizationStats{
				RemovedToolCalls:   1,
				RemovedToolResults: 1,
				DroppedMessages:    2,
				UnsafeForChainMode: true,
			},
		},
		{
			name: "DuplicateCallsRemoveEveryOccurrence",
			messages: []fantasy.Message{{
				Role: fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{
					validCall,
					validCall,
					validResult,
				},
			}},
			want: []fantasy.Message{},
			wantStats: chatprompt.OpenAIResponsesSanitizationStats{
				RemovedToolCalls:   2,
				RemovedToolResults: 1,
				DroppedMessages:    1,
				UnsafeForChainMode: true,
			},
		},
		{
			name: "DuplicateResultsRemoveEveryOccurrence",
			messages: []fantasy.Message{{
				Role: fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{
					validCall,
					validResult,
					validResult,
				},
			}},
			want: []fantasy.Message{},
			wantStats: chatprompt.OpenAIResponsesSanitizationStats{
				RemovedToolCalls:   1,
				RemovedToolResults: 2,
				DroppedMessages:    1,
				UnsafeForChainMode: true,
			},
		},
		{
			name: "RemovesWebSearchPair",
			messages: []fantasy.Message{{
				Role:    fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{webSearchCall, webSearchResult},
			}},
			want: []fantasy.Message{},
			wantStats: chatprompt.OpenAIResponsesSanitizationStats{
				RemovedToolCalls:        1,
				RemovedToolResults:      1,
				RemovedWebSearchCalls:   1,
				RemovedWebSearchResults: 1,
				DroppedMessages:         1,
				UnsafeForChainMode:      true,
			},
		},
		{
			name: "RemovesWebSearchPreviewPair",
			messages: []fantasy.Message{{
				Role:    fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{webSearchPreviewCall, webSearchPreviewResult},
			}},
			want: []fantasy.Message{},
			wantStats: chatprompt.OpenAIResponsesSanitizationStats{
				RemovedToolCalls:        1,
				RemovedToolResults:      1,
				RemovedWebSearchCalls:   1,
				RemovedWebSearchResults: 1,
				DroppedMessages:         1,
				UnsafeForChainMode:      true,
			},
		},
		{
			name: "CoalescesAdjacentSameRoleAfterDrop",
			messages: []fantasy.Message{
				{
					Role:            fantasy.MessageRoleUser,
					Content:         []fantasy.MessagePart{fantasy.TextPart{Text: "first"}},
					ProviderOptions: messageOptionsA,
				},
				{
					Role:    fantasy.MessageRoleAssistant,
					Content: []fantasy.MessagePart{orphanCall},
				},
				{
					Role:            fantasy.MessageRoleUser,
					Content:         []fantasy.MessagePart{fantasy.TextPart{Text: "second"}},
					ProviderOptions: messageOptionsB,
				},
			},
			want: []fantasy.Message{{
				Role: fantasy.MessageRoleUser,
				Content: []fantasy.MessagePart{
					fantasy.TextPart{Text: "first", ProviderOptions: messageOptionsA},
					fantasy.TextPart{Text: "second", ProviderOptions: messageOptionsB},
				},
			}},
			wantStats: chatprompt.OpenAIResponsesSanitizationStats{
				RemovedToolCalls:   1,
				DroppedMessages:    1,
				UnsafeForChainMode: true,
			},
		},
		{
			name: "RemovesPointerFormToolCall",
			messages: []fantasy.Message{{
				Role: fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{
					textPart,
					&fantasy.ToolCallPart{
						ToolCallID: "call-pointer",
						ToolName:   "read_file",
						Input:      `{}`,
					},
				},
			}},
			want: []fantasy.Message{{
				Role:    fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{textPart},
			}},
			wantStats: chatprompt.OpenAIResponsesSanitizationStats{
				RemovedToolCalls:   1,
				UnsafeForChainMode: true,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, stats := chatprompt.SanitizeOpenAIResponsesMessages(tc.messages)

			require.Equal(t, tc.wantStats, stats)
			require.Equal(t, tc.want, got)
			if tc.wantSameInput {
				require.NotEmpty(t, got)
				require.True(t, &got[0] == &tc.messages[0])
			}
		})
	}
}

func TestAnalyzeOpenAIResponsesMessages(t *testing.T) {
	t.Parallel()

	result := fantasy.ToolResultOutputContentText{Text: `{"ok":true}`}
	cleanMessages := []fantasy.Message{{
		Role: fantasy.MessageRoleAssistant,
		Content: []fantasy.MessagePart{
			fantasy.ToolCallPart{ToolCallID: "call-clean", ToolName: "read_file", Input: `{}`},
			fantasy.ToolResultPart{ToolCallID: "call-clean", Output: result},
		},
	}}

	t.Run("CleanPromptReturnsZeroStats", func(t *testing.T) {
		t.Parallel()

		before := cloneOpenAIResponsesMessages(cleanMessages)
		stats := chatprompt.AnalyzeOpenAIResponsesMessages(cleanMessages)

		require.Equal(t, chatprompt.OpenAIResponsesSanitizationStats{}, stats)
		require.Equal(t, before, cleanMessages)
	})

	t.Run("InvalidPairsAreUnsafeWithoutDroppedMessages", func(t *testing.T) {
		t.Parallel()

		messages := []fantasy.Message{{
			Role: fantasy.MessageRoleAssistant,
			Content: []fantasy.MessagePart{
				fantasy.ToolCallPart{ToolCallID: "call-orphan", ToolName: "read_file", Input: `{}`},
				fantasy.ToolResultPart{Output: result},
			},
		}}
		before := cloneOpenAIResponsesMessages(messages)

		stats := chatprompt.AnalyzeOpenAIResponsesMessages(messages)

		require.Equal(t, chatprompt.OpenAIResponsesSanitizationStats{
			RemovedToolCalls:   1,
			RemovedToolResults: 1,
			UnsafeForChainMode: true,
		}, stats)
		require.Equal(t, before, messages)
	})

	t.Run("WebSearchIsUnsafeWithoutDroppedMessages", func(t *testing.T) {
		t.Parallel()

		messages := []fantasy.Message{{
			Role: fantasy.MessageRoleAssistant,
			Content: []fantasy.MessagePart{
				fantasy.ToolCallPart{
					ToolCallID:       "call-web",
					ToolName:         "web_search_preview",
					Input:            `{}`,
					ProviderExecuted: true,
				},
				fantasy.ToolResultPart{
					ToolCallID:       "call-web",
					Output:           result,
					ProviderExecuted: true,
				},
			},
		}}
		before := cloneOpenAIResponsesMessages(messages)

		stats := chatprompt.AnalyzeOpenAIResponsesMessages(messages)

		require.Equal(t, chatprompt.OpenAIResponsesSanitizationStats{
			RemovedToolCalls:        1,
			RemovedToolResults:      1,
			RemovedWebSearchCalls:   1,
			RemovedWebSearchResults: 1,
			UnsafeForChainMode:      true,
		}, stats)
		require.Equal(t, before, messages)
	})
}

func TestSanitizeOpenAIResponsesMessages_ConvertedHistory(t *testing.T) {
	t.Parallel()

	t.Run("V1LocalToolPairStaysClean", func(t *testing.T) {
		t.Parallel()

		assistantContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
			codersdk.ChatMessageToolCall(
				"call-v1",
				"execute",
				json.RawMessage(`{"command":"echo ok"}`),
			),
		})
		require.NoError(t, err)
		toolContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
			codersdk.ChatMessageToolResult(
				"call-v1",
				"execute",
				json.RawMessage(`{"ok":true}`),
				false,
				false,
			),
		})
		require.NoError(t, err)
		prompt := convertMessagesWithoutFiles(t, []database.ChatMessage{
			testMsgV1(codersdk.ChatMessageRoleAssistant, assistantContent),
			testMsgV1(codersdk.ChatMessageRoleTool, toolContent),
		})

		got, stats := chatprompt.SanitizeOpenAIResponsesMessages(prompt)

		require.Equal(t, chatprompt.OpenAIResponsesSanitizationStats{}, stats)
		require.Equal(t, prompt, got)
		require.NotEmpty(t, got)
		require.True(t, &got[0] == &prompt[0])
	})

	t.Run("LegacyV0Skipped", func(t *testing.T) {
		t.Parallel()
		t.Skip("legacy V0 cannot cleanly represent provider executed OpenAI web search history")
	})
}

func TestLogOpenAIResponsesSanitization(t *testing.T) {
	t.Parallel()

	t.Run("ZeroStatsDoesNotLog", func(t *testing.T) {
		t.Parallel()

		sink := &openAIResponsesLogSink{}
		logger := slog.Make(sink)

		chatprompt.LogOpenAIResponsesSanitization(
			context.Background(),
			logger,
			"prepare",
			"openai",
			"gpt-4o-mini",
			chatprompt.OpenAIResponsesSanitizationStats{},
		)

		require.Empty(t, sink.snapshotEntries())
	})

	t.Run("UnsafeOnlyDoesNotLog", func(t *testing.T) {
		t.Parallel()

		sink := &openAIResponsesLogSink{}
		logger := slog.Make(sink)

		chatprompt.LogOpenAIResponsesSanitization(
			context.Background(),
			logger,
			"prepare",
			"openai",
			"gpt-4o-mini",
			chatprompt.OpenAIResponsesSanitizationStats{UnsafeForChainMode: true},
		)

		require.Empty(t, sink.snapshotEntries())
	})

	t.Run("RemovalLogsApprovedFields", func(t *testing.T) {
		t.Parallel()

		sink := &openAIResponsesLogSink{}
		logger := slog.Make(sink)
		stats := chatprompt.OpenAIResponsesSanitizationStats{
			RemovedToolCalls:        2,
			RemovedToolResults:      1,
			RemovedWebSearchCalls:   1,
			RemovedWebSearchResults: 1,
			DroppedMessages:         1,
			UnsafeForChainMode:      true,
		}

		chatprompt.LogOpenAIResponsesSanitization(
			context.Background(),
			logger,
			"prepare",
			"openai",
			"gpt-4o-mini",
			stats,
			slog.F("extra_allowed", true),
		)

		entries := sink.snapshotEntries()
		require.Len(t, entries, 1)
		require.Equal(t, slog.LevelWarn, entries[0].Level)
		require.Equal(t, "sanitized OpenAI Responses prompt history", entries[0].Message)
		require.Equal(t, map[string]any{
			"phase":                      "prepare",
			"provider":                   "openai",
			"model":                      "gpt-4o-mini",
			"removed_tool_calls":         2,
			"removed_tool_results":       1,
			"removed_web_search_calls":   1,
			"removed_web_search_results": 1,
			"dropped_messages":           1,
			"disabled_chain_mode":        false,
			"unsafe_for_chain_mode":      true,
			"extra_allowed":              true,
		}, openAIResponsesFieldsMap(entries[0].Fields))
	})

	t.Run("DisabledChainModeLogsWithoutRemoval", func(t *testing.T) {
		t.Parallel()

		sink := &openAIResponsesLogSink{}
		logger := slog.Make(sink)

		chatprompt.LogOpenAIResponsesSanitization(
			context.Background(),
			logger,
			"prepare",
			"openai",
			"gpt-4o-mini",
			chatprompt.OpenAIResponsesSanitizationStats{DisabledChainMode: true},
		)

		require.Len(t, sink.snapshotEntries(), 1)
	})
}

func cloneOpenAIResponsesMessages(messages []fantasy.Message) []fantasy.Message {
	clone := make([]fantasy.Message, len(messages))
	copy(clone, messages)
	for i := range clone {
		clone[i].Content = append([]fantasy.MessagePart(nil), messages[i].Content...)
	}
	return clone
}

type openAIResponsesLogSink struct {
	mu      sync.Mutex
	entries []slog.SinkEntry
}

func (s *openAIResponsesLogSink) LogEntry(_ context.Context, entry slog.SinkEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, entry)
}

func (*openAIResponsesLogSink) Sync() {}

func (s *openAIResponsesLogSink) snapshotEntries() []slog.SinkEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]slog.SinkEntry(nil), s.entries...)
}

func openAIResponsesFieldsMap(fields []slog.Field) map[string]any {
	result := make(map[string]any, len(fields))
	for _, field := range fields {
		result[field.Name] = field.Value
	}
	return result
}
