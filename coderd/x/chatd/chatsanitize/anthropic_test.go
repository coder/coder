package chatsanitize_test

import (
	"testing"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chatsanitize"
)

type testSourceMessagePart struct {
	id string
}

func (testSourceMessagePart) GetType() fantasy.ContentType {
	return fantasy.ContentTypeSource
}

func (testSourceMessagePart) Options() fantasy.ProviderOptions {
	return nil
}

type testToolResultOutput struct {
	Value string `json:"value"`
}

func (testToolResultOutput) GetType() fantasy.ToolResultContentType {
	return "test"
}

func validWebSearchProviderOptionsForTest() fantasy.ProviderOptions {
	return fantasy.ProviderOptions{
		fantasyanthropic.Name: &fantasyanthropic.WebSearchResultMetadata{
			Results: []fantasyanthropic.WebSearchResultItem{
				{
					URL:              "https://example.com",
					Title:            "Example",
					EncryptedContent: "encrypted",
				},
			},
		},
	}
}

func TestSanitizeAnthropicProviderToolHistory(t *testing.T) {
	t.Parallel()

	textPart := fantasy.TextPart{Text: "Here is a summary."}
	sourcePart := testSourceMessagePart{id: "source-1"}
	reasoningPart := fantasy.ReasoningPart{Text: "Need to search first."}
	filePart := fantasy.FilePart{Data: []byte("notes"), MediaType: "text/plain"}
	providerCall := func(id string) fantasy.ToolCallPart {
		return fantasy.ToolCallPart{
			ToolCallID:       id,
			ToolName:         "web_search",
			Input:            `{"query":"coder"}`,
			ProviderExecuted: true,
		}
	}
	providerResult := func(id string) fantasy.ToolResultPart {
		return fantasy.ToolResultPart{
			ToolCallID:       id,
			Output:           fantasy.ToolResultOutputContentText{Text: `{"ok":true}`},
			ProviderExecuted: true,
			ProviderOptions:  validWebSearchProviderOptionsForTest(),
		}
	}
	resultText := fantasy.TextPart{Text: `{"ok":true}`}
	localCall := fantasy.ToolCallPart{
		ToolCallID: "srvtoolu_local",
		ToolName:   "read_file",
		Input:      `{"path":"main.go"}`,
	}
	localResult := fantasy.ToolResultPart{
		ToolCallID: "srvtoolu_local",
		Output:     fantasy.ToolResultOutputContentText{Text: `{"ok":true}`},
	}
	disableParallelToolUse := true
	providerOptions := fantasy.ProviderOptions{
		fantasyanthropic.Name: &fantasyanthropic.ProviderOptions{
			DisableParallelToolUse: &disableParallelToolUse,
		},
	}
	enableParallelToolUse := false
	providerOptionsAllowParallel := fantasy.ProviderOptions{
		fantasyanthropic.Name: &fantasyanthropic.ProviderOptions{
			DisableParallelToolUse: &enableParallelToolUse,
		},
	}
	pointerCall := providerCall("srvtoolu_pointer")
	pointerResult := providerResult("srvtoolu_pointer")

	testCases := []struct {
		name               string
		provider           string
		messages           []fantasy.Message
		want               []fantasy.Message
		wantRemovedCalls   int
		wantRemovedResults int
		wantDropped        int
	}{
		{
			name:     "removes unpaired call and keeps text",
			provider: fantasyanthropic.Name,
			messages: []fantasy.Message{{
				Role: fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{
					textPart,
					providerCall("srvtoolu_orphan_call"),
				},
			}},
			want: []fantasy.Message{{
				Role:    fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{textPart},
			}},
			wantRemovedCalls: 1,
		},
		{
			name:     "textifies result-only assistant message",
			provider: fantasyanthropic.Name,
			messages: []fantasy.Message{{
				Role:    fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{providerResult("srvtoolu_orphan_result")},
			}},
			want: []fantasy.Message{{
				Role:    fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{resultText},
			}},
			wantRemovedResults: 1,
		},
		{
			name:     "textifies orphan result and keeps text",
			provider: fantasyanthropic.Name,
			messages: []fantasy.Message{{
				Role: fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{
					textPart,
					providerResult("srvtoolu_orphan_result"),
				},
			}},
			want: []fantasy.Message{{
				Role: fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{
					textPart,
					resultText,
				},
			}},
			wantRemovedResults: 1,
		},
		{
			name:     "textifies result before matching call",
			provider: fantasyanthropic.Name,
			messages: []fantasy.Message{{
				Role: fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{
					textPart,
					providerResult("srvtoolu_search"),
					providerCall("srvtoolu_search"),
				},
			}},
			want: []fantasy.Message{{
				Role: fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{
					textPart,
					resultText,
				},
			}},
			wantRemovedCalls:   1,
			wantRemovedResults: 1,
		},
		{
			name:     "keeps valid web search call and result",
			provider: fantasyanthropic.Name,
			messages: []fantasy.Message{{
				Role: fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{
					providerCall("srvtoolu_search"),
					providerResult("srvtoolu_search"),
				},
			}},
			want: []fantasy.Message{{
				Role: fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{
					providerCall("srvtoolu_search"),
					providerResult("srvtoolu_search"),
				},
			}},
		},
		{
			name:     "keeps valid pair and textifies orphan result",
			provider: fantasyanthropic.Name,
			messages: []fantasy.Message{{
				Role: fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{
					providerCall("srvtoolu_search"),
					providerResult("srvtoolu_search"),
					providerResult("srvtoolu_orphan_result"),
				},
			}},
			want: []fantasy.Message{{
				Role: fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{
					providerCall("srvtoolu_search"),
					providerResult("srvtoolu_search"),
					resultText,
				},
			}},
			wantRemovedResults: 1,
		},
		{
			name:     "removes invalid json call and dependent result",
			provider: fantasyanthropic.Name,
			messages: []fantasy.Message{{
				Role: fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{
					textPart,
					fantasy.ToolCallPart{
						ToolCallID:       "srvtoolu_bad_json",
						ToolName:         "web_search",
						Input:            `{"query":`,
						ProviderExecuted: true,
					},
					providerResult("srvtoolu_bad_json"),
				},
			}},
			want: []fantasy.Message{{
				Role: fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{
					textPart,
					resultText,
				},
			}},
			wantRemovedCalls:   1,
			wantRemovedResults: 1,
		},
		{
			name:     "textifies result with missing provider metadata",
			provider: fantasyanthropic.Name,
			messages: []fantasy.Message{{
				Role: fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{
					textPart,
					providerCall("srvtoolu_missing_meta"),
					fantasy.ToolResultPart{
						ToolCallID:       "srvtoolu_missing_meta",
						Output:           fantasy.ToolResultOutputContentText{Text: `{"ok":true}`},
						ProviderExecuted: true,
					},
				},
			}},
			want: []fantasy.Message{{
				Role: fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{
					textPart,
					resultText,
				},
			}},
			wantRemovedCalls:   1,
			wantRemovedResults: 1,
		},
		{
			name:     "removes empty call ID and dependent result",
			provider: fantasyanthropic.Name,
			messages: []fantasy.Message{{
				Role: fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{
					textPart,
					providerCall(""),
					providerResult(""),
				},
			}},
			want: []fantasy.Message{{
				Role: fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{
					textPart,
					resultText,
				},
			}},
			wantRemovedCalls:   1,
			wantRemovedResults: 1,
		},
		{
			name:     "removes empty tool name and dependent result",
			provider: fantasyanthropic.Name,
			messages: []fantasy.Message{{
				Role: fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{
					textPart,
					fantasy.ToolCallPart{
						ToolCallID:       "srvtoolu_empty_name",
						Input:            `{"query":"coder"}`,
						ProviderExecuted: true,
					},
					providerResult("srvtoolu_empty_name"),
				},
			}},
			want: []fantasy.Message{{
				Role: fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{
					textPart,
					resultText,
				},
			}},
			wantRemovedCalls:   1,
			wantRemovedResults: 1,
		},
		{
			name:     "removes unsupported provider tool and result",
			provider: fantasyanthropic.Name,
			messages: []fantasy.Message{{
				Role: fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{
					textPart,
					fantasy.ToolCallPart{
						ToolCallID:       "srvtoolu_code",
						ToolName:         "code_execution",
						Input:            `{"code":"print(1)"}`,
						ProviderExecuted: true,
					},
					providerResult("srvtoolu_code"),
				},
			}},
			want: []fantasy.Message{{
				Role: fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{
					textPart,
					resultText,
				},
			}},
			wantRemovedCalls:   1,
			wantRemovedResults: 1,
		},
		{
			name:     "removes duplicate ID with two calls and one result",
			provider: fantasyanthropic.Name,
			messages: []fantasy.Message{{
				Role: fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{
					textPart,
					providerCall("srvtoolu_duplicate"),
					providerCall("srvtoolu_duplicate"),
					providerResult("srvtoolu_duplicate"),
				},
			}},
			want: []fantasy.Message{{
				Role: fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{
					textPart,
					resultText,
				},
			}},
			wantRemovedCalls:   2,
			wantRemovedResults: 1,
		},
		{
			name:     "removes duplicate ID with one call and two results",
			provider: fantasyanthropic.Name,
			messages: []fantasy.Message{{
				Role: fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{
					textPart,
					providerCall("srvtoolu_duplicate"),
					providerResult("srvtoolu_duplicate"),
					providerResult("srvtoolu_duplicate"),
				},
			}},
			want: []fantasy.Message{{
				Role: fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{
					textPart,
					resultText,
					resultText,
				},
			}},
			wantRemovedCalls:   1,
			wantRemovedResults: 2,
		},
		{
			name:     "textifies repeated valid-looking pairs",
			provider: fantasyanthropic.Name,
			messages: []fantasy.Message{{
				Role: fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{
					providerCall("srvtoolu_duplicate"),
					providerResult("srvtoolu_duplicate"),
					providerCall("srvtoolu_duplicate"),
					providerResult("srvtoolu_duplicate"),
				},
			}},
			want: []fantasy.Message{{
				Role: fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{
					resultText,
					resultText,
				},
			}},
			wantRemovedCalls:   2,
			wantRemovedResults: 2,
		},
		{
			name:     "provider call plus local result removes provider call only",
			provider: fantasyanthropic.Name,
			messages: []fantasy.Message{{
				Role: fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{
					providerCall("srvtoolu_mismatch"),
					fantasy.ToolResultPart{
						ToolCallID: "srvtoolu_mismatch",
						Output:     fantasy.ToolResultOutputContentText{Text: `{"ok":true}`},
					},
				},
			}},
			want: []fantasy.Message{{
				Role: fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{
					fantasy.ToolResultPart{
						ToolCallID: "srvtoolu_mismatch",
						Output:     fantasy.ToolResultOutputContentText{Text: `{"ok":true}`},
					},
				},
			}},
			wantRemovedCalls: 1,
		},
		{
			name:     "local call plus provider result textifies provider result",
			provider: fantasyanthropic.Name,
			messages: []fantasy.Message{{
				Role: fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{
					fantasy.ToolCallPart{
						ToolCallID: "srvtoolu_mismatch",
						ToolName:   "read_file",
						Input:      `{"path":"main.go"}`,
					},
					providerResult("srvtoolu_mismatch"),
				},
			}},
			want: []fantasy.Message{{
				Role: fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{
					fantasy.ToolCallPart{
						ToolCallID: "srvtoolu_mismatch",
						ToolName:   "read_file",
						Input:      `{"path":"main.go"}`,
					},
					resultText,
				},
			}},
			wantRemovedResults: 1,
		},
		{
			name:     "textifies provider results outside assistant",
			provider: fantasyanthropic.Name,
			messages: []fantasy.Message{
				{
					Role: fantasy.MessageRoleUser,
					Content: []fantasy.MessagePart{
						fantasy.TextPart{Text: "Please summarize."},
						providerCall("srvtoolu_user_call"),
						providerResult("srvtoolu_user_result"),
						localResult,
					},
				},
				{
					Role: fantasy.MessageRoleTool,
					Content: []fantasy.MessagePart{
						providerResult("srvtoolu_tool"),
						fantasy.TextPart{Text: "local text"},
					},
				},
			},
			want: []fantasy.Message{
				{
					Role: fantasy.MessageRoleUser,
					Content: []fantasy.MessagePart{
						fantasy.TextPart{Text: "Please summarize."},
						resultText,
						localResult,
					},
				},
				{
					Role: fantasy.MessageRoleTool,
					Content: []fantasy.MessagePart{
						resultText,
						fantasy.TextPart{Text: "local text"},
					},
				},
			},
			wantRemovedCalls:   1,
			wantRemovedResults: 2,
		},
		{
			name:     "textifies non-assistant provider result message",
			provider: fantasyanthropic.Name,
			messages: []fantasy.Message{{
				Role:    fantasy.MessageRoleTool,
				Content: []fantasy.MessagePart{providerResult("srvtoolu_tool")},
			}},
			want: []fantasy.Message{{
				Role:    fantasy.MessageRoleTool,
				Content: []fantasy.MessagePart{resultText},
			}},
			wantRemovedResults: 1,
		},
		{
			name:     "handles pointer tool parts",
			provider: fantasyanthropic.Name,
			messages: []fantasy.Message{{
				Role: fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{
					&pointerCall,
					&pointerResult,
				},
			}},
			want: []fantasy.Message{{
				Role: fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{
					&pointerCall,
					&pointerResult,
				},
			}},
		},
		{
			name:     "preserves surrounding source text reasoning and file parts",
			provider: fantasyanthropic.Name,
			messages: []fantasy.Message{{
				Role: fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{
					textPart,
					sourcePart,
					reasoningPart,
					providerCall("srvtoolu_search"),
					providerResult("srvtoolu_search"),
					filePart,
				},
			}},
			want: []fantasy.Message{{
				Role: fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{
					textPart,
					sourcePart,
					reasoningPart,
					providerCall("srvtoolu_search"),
					providerResult("srvtoolu_search"),
					filePart,
				},
			}},
		},
		{
			name:     "textified orphan prevents duplicate coalescing",
			provider: fantasyanthropic.Name,
			messages: []fantasy.Message{
				{
					Role: fantasy.MessageRoleAssistant,
					Content: []fantasy.MessagePart{
						providerCall("srvtoolu_search"),
						providerResult("srvtoolu_search"),
					},
				},
				{
					Role:    fantasy.MessageRoleUser,
					Content: []fantasy.MessagePart{providerResult("srvtoolu_orphan")},
				},
				{
					Role: fantasy.MessageRoleAssistant,
					Content: []fantasy.MessagePart{
						providerCall("srvtoolu_search"),
						providerResult("srvtoolu_search"),
					},
				},
			},
			want: []fantasy.Message{
				{
					Role: fantasy.MessageRoleAssistant,
					Content: []fantasy.MessagePart{
						providerCall("srvtoolu_search"),
						providerResult("srvtoolu_search"),
					},
				},
				{
					Role:    fantasy.MessageRoleUser,
					Content: []fantasy.MessagePart{resultText},
				},
				{
					Role: fantasy.MessageRoleAssistant,
					Content: []fantasy.MessagePart{
						providerCall("srvtoolu_search"),
						providerResult("srvtoolu_search"),
					},
				},
			},
			wantRemovedResults: 1,
		},
		{
			name:     "keeps local srvtoolu-like IDs untouched",
			provider: fantasyanthropic.Name,
			messages: []fantasy.Message{{
				Role: fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{
					localCall,
					localResult,
				},
			}},
			want: []fantasy.Message{{
				Role: fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{
					localCall,
					localResult,
				},
			}},
		},
		{
			name:     "coalesces adjacent roles after dropping empty message",
			provider: fantasyanthropic.Name,
			messages: []fantasy.Message{
				{
					Role: fantasy.MessageRoleUser,
					Content: []fantasy.MessagePart{
						fantasy.TextPart{Text: "search for coder"},
					},
				},
				{
					Role:    fantasy.MessageRoleAssistant,
					Content: []fantasy.MessagePart{providerCall("srvtoolu_orphan_call")},
				},
				{
					Role: fantasy.MessageRoleUser,
					Content: []fantasy.MessagePart{
						fantasy.TextPart{Text: "now summarize"},
					},
					ProviderOptions: providerOptions,
				},
			},
			want: []fantasy.Message{{
				Role: fantasy.MessageRoleUser,
				Content: []fantasy.MessagePart{
					fantasy.TextPart{Text: "search for coder"},
					fantasy.TextPart{
						Text:            "now summarize",
						ProviderOptions: providerOptions,
					},
				},
			}},
			wantRemovedCalls: 1,
			wantDropped:      1,
		},
		{
			name:     "coalesces adjacent provider options without flattening boundaries",
			provider: fantasyanthropic.Name,
			messages: []fantasy.Message{
				{
					Role: fantasy.MessageRoleUser,
					Content: []fantasy.MessagePart{
						fantasy.TextPart{Text: "search for coder"},
					},
					ProviderOptions: providerOptionsAllowParallel,
				},
				{
					Role:    fantasy.MessageRoleAssistant,
					Content: []fantasy.MessagePart{providerCall("srvtoolu_orphan_call")},
				},
				{
					Role: fantasy.MessageRoleUser,
					Content: []fantasy.MessagePart{
						fantasy.TextPart{Text: "now summarize"},
					},
					ProviderOptions: providerOptions,
				},
			},
			want: []fantasy.Message{{
				Role: fantasy.MessageRoleUser,
				Content: []fantasy.MessagePart{
					fantasy.TextPart{
						Text:            "search for coder",
						ProviderOptions: providerOptionsAllowParallel,
					},
					fantasy.TextPart{
						Text:            "now summarize",
						ProviderOptions: providerOptions,
					},
				},
			}},
			wantRemovedCalls: 1,
			wantDropped:      1,
		},
		{
			name:     "leaves other providers unchanged",
			provider: "fake",
			messages: []fantasy.Message{{
				Role:    fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{providerResult("srvtoolu_orphan_result")},
			}},
			want: []fantasy.Message{{
				Role:    fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{providerResult("srvtoolu_orphan_result")},
			}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			sanitized, stats := chatsanitize.SanitizeAnthropicProviderToolHistory(
				tc.provider,
				tc.messages,
			)
			require.Equal(t, tc.wantRemovedCalls, stats.RemovedToolCalls)
			require.Equal(t, tc.wantRemovedResults, stats.RemovedToolResults)
			require.Equal(t, tc.wantDropped, stats.DroppedMessages)
			require.Equal(t, tc.want, sanitized)
			if tc.provider == fantasyanthropic.Name {
				require.Empty(t, chatsanitize.ValidateAnthropicProviderToolHistory(sanitized))
			}
		})
	}
}

func TestAnthropicProviderToolPartsToRemove(t *testing.T) {
	t.Parallel()

	providerCall := func(id string) fantasy.ToolCallPart {
		return fantasy.ToolCallPart{
			ToolCallID:       id,
			ToolName:         "web_search",
			Input:            `{"query":"coder"}`,
			ProviderExecuted: true,
		}
	}
	providerResult := func(id string) fantasy.ToolResultPart {
		return fantasy.ToolResultPart{
			ToolCallID:       id,
			Output:           fantasy.ToolResultOutputContentText{Text: `{"ok":true}`},
			ProviderExecuted: true,
			ProviderOptions:  validWebSearchProviderOptionsForTest(),
		}
	}

	testCases := []struct {
		name           string
		provider       string
		parts          []fantasy.MessagePart
		wantRemove     []int
		wantViolations []chatsanitize.AnthropicProviderToolHistoryViolation
	}{
		{
			name:           "empty input",
			provider:       fantasyanthropic.Name,
			wantRemove:     []int{},
			wantViolations: []chatsanitize.AnthropicProviderToolHistoryViolation{},
		},
		{
			name:     "valid provider call and result",
			provider: fantasyanthropic.Name,
			parts: []fantasy.MessagePart{
				providerCall("srvtoolu_search"),
				providerResult("srvtoolu_search"),
			},
			wantRemove:     []int{},
			wantViolations: []chatsanitize.AnthropicProviderToolHistoryViolation{},
		},
		{
			name:     "orphan provider call",
			provider: fantasyanthropic.Name,
			parts: []fantasy.MessagePart{
				fantasy.TextPart{Text: "keep"},
				providerCall("srvtoolu_orphan_call"),
			},
			wantRemove: []int{1},
			wantViolations: []chatsanitize.AnthropicProviderToolHistoryViolation{{
				MessageIndex: 0,
				PartIndex:    1,
				ID:           "srvtoolu_orphan_call",
				Reason:       "provider_executed_call_without_result",
			}},
		},
		{
			name:     "orphan provider result",
			provider: fantasyanthropic.Name,
			parts: []fantasy.MessagePart{
				fantasy.TextPart{Text: "keep"},
				providerResult("srvtoolu_orphan_result"),
			},
			wantRemove: []int{1},
			wantViolations: []chatsanitize.AnthropicProviderToolHistoryViolation{{
				MessageIndex: 0,
				PartIndex:    1,
				ID:           "srvtoolu_orphan_result",
				Reason:       "provider_executed_result_without_call",
			}},
		},
		{
			name:     "provider result before call",
			provider: fantasyanthropic.Name,
			parts: []fantasy.MessagePart{
				providerResult("srvtoolu_search"),
				providerCall("srvtoolu_search"),
			},
			wantRemove: []int{0, 1},
			wantViolations: []chatsanitize.AnthropicProviderToolHistoryViolation{
				{
					MessageIndex: 0,
					PartIndex:    0,
					ID:           "srvtoolu_search",
					Reason:       "provider_executed_result_before_call",
				},
				{
					MessageIndex: 0,
					PartIndex:    1,
					ID:           "srvtoolu_search",
					Reason:       "provider_executed_result_before_call",
				},
			},
		},
		{
			name:     "duplicate provider IDs",
			provider: fantasyanthropic.Name,
			parts: []fantasy.MessagePart{
				providerCall("srvtoolu_duplicate"),
				providerResult("srvtoolu_duplicate"),
				providerResult("srvtoolu_duplicate"),
			},
			wantRemove: []int{0, 1, 2},
			wantViolations: []chatsanitize.AnthropicProviderToolHistoryViolation{
				{
					MessageIndex: 0,
					PartIndex:    0,
					ID:           "srvtoolu_duplicate",
					Reason:       "duplicate_provider_executed_id",
				},
				{
					MessageIndex: 0,
					PartIndex:    1,
					ID:           "srvtoolu_duplicate",
					Reason:       "duplicate_provider_executed_id",
				},
				{
					MessageIndex: 0,
					PartIndex:    2,
					ID:           "srvtoolu_duplicate",
					Reason:       "duplicate_provider_executed_id",
				},
			},
		},
		{
			name:     "non Anthropic provider",
			provider: "fake",
			parts: []fantasy.MessagePart{
				providerResult("srvtoolu_orphan_result"),
			},
			wantRemove:     []int{},
			wantViolations: []chatsanitize.AnthropicProviderToolHistoryViolation{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			remove, violations := chatsanitize.AnthropicProviderToolPartsToRemove(
				tc.provider,
				tc.parts,
			)
			require.NotNil(t, remove)

			gotRemove := make([]int, 0, len(remove))
			for partIndex := range remove {
				gotRemove = append(gotRemove, partIndex)
			}
			require.ElementsMatch(t, tc.wantRemove, gotRemove)
			require.ElementsMatch(t, tc.wantViolations, violations)
		})
	}
}

func TestValidateAnthropicProviderToolHistory(t *testing.T) {
	t.Parallel()

	providerCall := func(id string) fantasy.ToolCallPart {
		return fantasy.ToolCallPart{
			ToolCallID:       id,
			ToolName:         "web_search",
			Input:            `{"query":"coder"}`,
			ProviderExecuted: true,
		}
	}
	providerResult := func(id string) fantasy.ToolResultPart {
		return fantasy.ToolResultPart{
			ToolCallID:       id,
			Output:           fantasy.ToolResultOutputContentText{Text: `{"ok":true}`},
			ProviderExecuted: true,
			ProviderOptions:  validWebSearchProviderOptionsForTest(),
		}
	}

	testCases := []struct {
		name     string
		messages []fantasy.Message
		want     []chatsanitize.AnthropicProviderToolHistoryViolation
	}{
		{
			name: "orphan result",
			messages: []fantasy.Message{{
				Role: fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{
					fantasy.TextPart{Text: "summary"},
					providerResult("srvtoolu_orphan"),
				},
			}},
			want: []chatsanitize.AnthropicProviderToolHistoryViolation{{
				MessageIndex: 0,
				PartIndex:    1,
				ID:           "srvtoolu_orphan",
				Reason:       "provider_executed_result_without_call",
			}},
		},
		{
			name: "result before call",
			messages: []fantasy.Message{{
				Role: fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{
					providerResult("srvtoolu_search"),
					providerCall("srvtoolu_search"),
				},
			}},
			want: []chatsanitize.AnthropicProviderToolHistoryViolation{
				{
					MessageIndex: 0,
					PartIndex:    0,
					ID:           "srvtoolu_search",
					Reason:       "provider_executed_result_before_call",
				},
				{
					MessageIndex: 0,
					PartIndex:    1,
					ID:           "srvtoolu_search",
					Reason:       "provider_executed_result_before_call",
				},
			},
		},
		{
			name: "duplicate ID",
			messages: []fantasy.Message{{
				Role: fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{
					providerCall("srvtoolu_duplicate"),
					providerResult("srvtoolu_duplicate"),
					providerResult("srvtoolu_duplicate"),
				},
			}},
			want: []chatsanitize.AnthropicProviderToolHistoryViolation{
				{
					MessageIndex: 0,
					PartIndex:    0,
					ID:           "srvtoolu_duplicate",
					Reason:       "duplicate_provider_executed_id",
				},
				{
					MessageIndex: 0,
					PartIndex:    1,
					ID:           "srvtoolu_duplicate",
					Reason:       "duplicate_provider_executed_id",
				},
				{
					MessageIndex: 0,
					PartIndex:    2,
					ID:           "srvtoolu_duplicate",
					Reason:       "duplicate_provider_executed_id",
				},
			},
		},
		{
			name: "invalid call structure",
			messages: []fantasy.Message{{
				Role: fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{
					fantasy.ToolCallPart{
						ToolCallID:       "srvtoolu_bad_json",
						ToolName:         "web_search",
						Input:            `{"query":`,
						ProviderExecuted: true,
					},
					providerResult("srvtoolu_bad_json"),
				},
			}},
			want: []chatsanitize.AnthropicProviderToolHistoryViolation{
				{
					MessageIndex: 0,
					PartIndex:    0,
					ID:           "srvtoolu_bad_json",
					Reason:       "invalid_provider_executed_tool_call",
				},
				{
					MessageIndex: 0,
					PartIndex:    1,
					ID:           "srvtoolu_bad_json",
					Reason:       "invalid_provider_executed_tool_call",
				},
			},
		},
		{
			name: "mismatched provider flags",
			messages: []fantasy.Message{
				{
					Role: fantasy.MessageRoleAssistant,
					Content: []fantasy.MessagePart{
						providerCall("srvtoolu_provider_call"),
						fantasy.ToolResultPart{
							ToolCallID: "srvtoolu_provider_call",
							Output:     fantasy.ToolResultOutputContentText{Text: `{"ok":true}`},
						},
					},
				},
				{
					Role: fantasy.MessageRoleAssistant,
					Content: []fantasy.MessagePart{
						fantasy.ToolCallPart{
							ToolCallID: "srvtoolu_provider_result",
							ToolName:   "read_file",
							Input:      `{"path":"main.go"}`,
						},
						providerResult("srvtoolu_provider_result"),
					},
				},
			},
			want: []chatsanitize.AnthropicProviderToolHistoryViolation{
				{
					MessageIndex: 0,
					PartIndex:    0,
					ID:           "srvtoolu_provider_call",
					Reason:       "provider_executed_call_without_result",
				},
				{
					MessageIndex: 1,
					PartIndex:    1,
					ID:           "srvtoolu_provider_result",
					Reason:       "provider_executed_result_without_call",
				},
			},
		},
		{
			name: "provider blocks outside assistant",
			messages: []fantasy.Message{
				{
					Role: fantasy.MessageRoleUser,
					Content: []fantasy.MessagePart{
						fantasy.TextPart{Text: "search"},
						providerCall("srvtoolu_user"),
					},
				},
				{
					Role: fantasy.MessageRoleTool,
					Content: []fantasy.MessagePart{
						providerResult("srvtoolu_tool"),
					},
				},
			},
			want: []chatsanitize.AnthropicProviderToolHistoryViolation{
				{
					MessageIndex: 0,
					PartIndex:    1,
					ID:           "srvtoolu_user",
					Reason:       "provider_executed_block_outside_assistant",
				},
				{
					MessageIndex: 1,
					PartIndex:    0,
					ID:           "srvtoolu_tool",
					Reason:       "provider_executed_block_outside_assistant",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			violations := chatsanitize.ValidateAnthropicProviderToolHistory(tc.messages)
			require.ElementsMatch(t, tc.want, violations)
		})
	}
}

func TestAnthropicProviderToolSerializationHelpers(t *testing.T) {
	t.Parallel()

	validCall := func() fantasy.ToolCallPart {
		return fantasy.ToolCallPart{
			ToolCallID:       "srvtoolu_search",
			ToolName:         "web_search",
			Input:            `{"query":"coder"}`,
			ProviderExecuted: true,
		}
	}
	validResult := func() fantasy.ToolResultPart {
		return fantasy.ToolResultPart{
			ToolCallID:       "srvtoolu_search",
			Output:           fantasy.ToolResultOutputContentText{Text: `{"ok":true}`},
			ProviderExecuted: true,
			ProviderOptions:  validWebSearchProviderOptionsForTest(),
		}
	}

	require.True(t, chatsanitize.IsAllowedAnthropicProviderToolName("web_search"))
	require.False(t, chatsanitize.IsAllowedAnthropicProviderToolName("code_execution"))

	callPointer := validCall()
	var nilCall *fantasy.ToolCallPart
	callTests := []struct {
		name string
		part fantasy.MessagePart
		want bool
	}{
		{
			name: "valid value",
			part: validCall(),
			want: true,
		},
		{
			name: "valid pointer",
			part: &callPointer,
			want: true,
		},
		{
			name: "nil typed pointer",
			part: nilCall,
		},
		{
			name: "unrelated concrete message part",
			part: testSourceMessagePart{id: "source-1"},
		},
		{
			name: "provider executed false",
			part: func() fantasy.ToolCallPart {
				call := validCall()
				call.ProviderExecuted = false
				return call
			}(),
		},
		{
			name: "empty ID",
			part: func() fantasy.ToolCallPart {
				call := validCall()
				call.ToolCallID = ""
				return call
			}(),
		},
		{
			name: "whitespace ID",
			part: func() fantasy.ToolCallPart {
				call := validCall()
				call.ToolCallID = "   "
				return call
			}(),
		},
		{
			name: "empty tool name",
			part: func() fantasy.ToolCallPart {
				call := validCall()
				call.ToolName = ""
				return call
			}(),
		},
		{
			name: "unsupported tool name",
			part: func() fantasy.ToolCallPart {
				call := validCall()
				call.ToolName = "code_execution"
				return call
			}(),
		},
		{
			name: "invalid JSON input",
			part: func() fantasy.ToolCallPart {
				call := validCall()
				call.Input = `{"query":`
				return call
			}(),
		},
	}
	for _, tc := range callTests {
		t.Run("call "+tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, chatsanitize.IsSerializableAnthropicProviderToolCall(tc.part))
		})
	}

	resultPointer := validResult()
	var nilResult *fantasy.ToolResultPart
	resultTests := []struct {
		name        string
		part        fantasy.MessagePart
		matchedCall fantasy.MessagePart
		want        bool
	}{
		{
			name:        "valid value",
			part:        validResult(),
			matchedCall: validCall(),
			want:        true,
		},
		{
			name:        "valid pointer",
			part:        &resultPointer,
			matchedCall: &callPointer,
			want:        true,
		},
		{
			name:        "nil typed pointer",
			part:        nilResult,
			matchedCall: validCall(),
		},
		{
			name:        "unrelated concrete message part",
			part:        testSourceMessagePart{id: "source-1"},
			matchedCall: validCall(),
		},
		{
			name: "provider executed false",
			part: func() fantasy.ToolResultPart {
				result := validResult()
				result.ProviderExecuted = false
				return result
			}(),
			matchedCall: validCall(),
		},
		{
			name: "empty result ID",
			part: func() fantasy.ToolResultPart {
				result := validResult()
				result.ToolCallID = ""
				return result
			}(),
			matchedCall: validCall(),
		},
		{
			name: "mismatched result ID",
			part: func() fantasy.ToolResultPart {
				result := validResult()
				result.ToolCallID = "srvtoolu_other"
				return result
			}(),
			matchedCall: validCall(),
		},
		{
			name: "nil output with metadata",
			part: func() fantasy.ToolResultPart {
				result := validResult()
				result.Output = nil
				return result
			}(),
			matchedCall: validCall(),
			want:        true,
		},
		{
			name: "empty text output with metadata",
			part: func() fantasy.ToolResultPart {
				result := validResult()
				result.Output = fantasy.ToolResultOutputContentText{}
				return result
			}(),
			matchedCall: validCall(),
			want:        true,
		},
		{
			name: "missing metadata",
			part: func() fantasy.ToolResultPart {
				result := validResult()
				result.ProviderOptions = nil
				return result
			}(),
			matchedCall: validCall(),
		},
		{
			name: "nil metadata",
			part: func() fantasy.ToolResultPart {
				result := validResult()
				result.ProviderOptions = fantasy.ProviderOptions{
					fantasyanthropic.Name: nil,
				}
				return result
			}(),
			matchedCall: validCall(),
		},
		{
			name: "wrong metadata type",
			part: func() fantasy.ToolResultPart {
				result := validResult()
				result.ProviderOptions = fantasy.ProviderOptions{
					fantasyanthropic.Name: &fantasyanthropic.ProviderOptions{},
				}
				return result
			}(),
			matchedCall: validCall(),
		},
		{
			name: "matched call is not serializable",
			part: validResult(),
			matchedCall: func() fantasy.ToolCallPart {
				call := validCall()
				call.Input = `{"query":`
				return call
			}(),
		},
		{
			name:        "matched call is unrelated part",
			part:        validResult(),
			matchedCall: testSourceMessagePart{id: "source-1"},
		},
	}
	for _, tc := range resultTests {
		t.Run("result "+tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, chatsanitize.IsSerializableAnthropicProviderToolResult(tc.part, tc.matchedCall))
		})
	}
}

func TestAnthropicToolResultOutputText(t *testing.T) {
	t.Parallel()

	textPointer := fantasy.ToolResultOutputContentText{Text: "pointer text"}
	errorPointer := fantasy.ToolResultOutputContentError{Error: xerrors.New("pointer error")}
	mediaPointer := fantasy.ToolResultOutputContentMedia{Text: "pointer media"}
	var nilTextPointer *fantasy.ToolResultOutputContentText
	var nilErrorPointer *fantasy.ToolResultOutputContentError
	var nilMediaPointer *fantasy.ToolResultOutputContentMedia

	testCases := []struct {
		name   string
		output fantasy.ToolResultOutputContent
		want   string
	}{
		{
			name:   "text value",
			output: fantasy.ToolResultOutputContentText{Text: "text value"},
			want:   "text value",
		},
		{
			name:   "text pointer",
			output: &textPointer,
			want:   "pointer text",
		},
		{
			name:   "nil text pointer",
			output: nilTextPointer,
		},
		{
			name:   "error value",
			output: fantasy.ToolResultOutputContentError{Error: xerrors.New("error value")},
			want:   "error value",
		},
		{
			name:   "error pointer",
			output: &errorPointer,
			want:   "pointer error",
		},
		{
			name:   "nil error pointer",
			output: nilErrorPointer,
		},
		{
			name: "error value with nil error",
			output: fantasy.ToolResultOutputContentError{
				Error: nil,
			},
		},
		{
			name:   "media value",
			output: fantasy.ToolResultOutputContentMedia{Text: "media value"},
			want:   "media value",
		},
		{
			name:   "media pointer",
			output: &mediaPointer,
			want:   "pointer media",
		},
		{
			name:   "nil media pointer",
			output: nilMediaPointer,
		},
		{
			name: "media value without text",
			output: fantasy.ToolResultOutputContentMedia{
				Data:      "base64",
				MediaType: "image/png",
			},
		},
		{
			name:   "nil output",
			output: nil,
		},
		{
			name:   "json fallback",
			output: testToolResultOutput{Value: "custom"},
			want:   `{"value":"custom"}`,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, chatsanitize.AnthropicToolResultOutputText(tc.output))
		})
	}
}
