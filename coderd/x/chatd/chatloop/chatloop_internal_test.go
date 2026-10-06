package chatloop

import (
	"iter"
	"testing"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	fantasyopenai "charm.land/fantasy/providers/openai"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

func TestProcessStepStreamPreservesReasoningMetadataAcrossNilDelta(t *testing.T) {
	t.Parallel()

	stream := iter.Seq[fantasy.StreamPart](func(yield func(fantasy.StreamPart) bool) {
		yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeReasoningStart, ID: "0"})
		yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeReasoningDelta, ID: "0", Delta: "thinking"})
		yield(fantasy.StreamPart{
			Type: fantasy.StreamPartTypeReasoningDelta,
			ID:   "0",
			ProviderMetadata: fantasy.ProviderMetadata{
				fantasyanthropic.Name: &fantasyanthropic.ReasoningOptionMetadata{
					Signature: "sig",
				},
			},
		})
		yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeReasoningDelta, ID: "0", ProviderMetadata: fantasy.ProviderMetadata{}})
		yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeReasoningDelta, ID: "0"})
		yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeReasoningEnd, ID: "0", ProviderMetadata: fantasy.ProviderMetadata{}})
		yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop})
	})

	result, err := processStepStream(stream, quartz.NewMock(t), func(codersdk.ChatMessageRole, codersdk.ChatMessagePart) {})
	require.NoError(t, err)
	require.Len(t, result.content, 1)
	reasoning, ok := fantasy.AsContentType[fantasy.ReasoningContent](result.content[0])
	require.True(t, ok)
	require.Equal(t, "thinking", reasoning.Text)
	metadata := fantasyanthropic.GetReasoningMetadata(fantasy.ProviderOptions(reasoning.ProviderMetadata))
	require.NotNil(t, metadata)
	require.Equal(t, "sig", metadata.Signature)
}

func TestProcessStepStreamPersistsRedactedThinkingOnEnd(t *testing.T) {
	t.Parallel()

	stream := iter.Seq[fantasy.StreamPart](func(yield func(fantasy.StreamPart) bool) {
		reasoningMetadata := fantasy.ProviderMetadata{
			fantasyanthropic.Name: &fantasyanthropic.ReasoningOptionMetadata{
				RedactedData: "redacted-payload",
			},
		}
		yield(fantasy.StreamPart{
			Type:             fantasy.StreamPartTypeReasoningStart,
			ID:               "0",
			ProviderMetadata: reasoningMetadata,
		})
		yield(fantasy.StreamPart{
			Type:             fantasy.StreamPartTypeReasoningEnd,
			ID:               "0",
			ProviderMetadata: reasoningMetadata,
		})
		yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextStart, ID: "1"})
		yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextDelta, ID: "1", Delta: "done"})
		yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextEnd, ID: "1"})
		yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop})
	})

	result, err := processStepStream(stream, quartz.NewMock(t), func(codersdk.ChatMessageRole, codersdk.ChatMessagePart) {})
	require.NoError(t, err)
	require.Len(t, result.content, 2)
	reasoning, ok := fantasy.AsContentType[fantasy.ReasoningContent](result.content[0])
	require.True(t, ok)
	require.Empty(t, reasoning.Text)
	metadata := fantasyanthropic.GetReasoningMetadata(fantasy.ProviderOptions(reasoning.ProviderMetadata))
	require.NotNil(t, metadata)
	require.Equal(t, "redacted-payload", metadata.RedactedData)
}

func TestProcessStepStreamProviderWebSearch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		metadata  fantasy.ProviderMetadata
		wantError string
	}{
		{
			name: "AnthropicResults",
			metadata: fantasy.ProviderMetadata{
				fantasyanthropic.Name: &fantasyanthropic.WebSearchResultMetadata{
					Results: []fantasyanthropic.WebSearchResultItem{{URL: "https://go.dev/doc/go1.27", Title: "Go 1.27"}},
				},
			},
		},
		{
			name: "AnthropicErrorCode",
			metadata: fantasy.ProviderMetadata{
				fantasyanthropic.Name: &fantasyanthropic.WebSearchResultMetadata{ErrorCode: "max_uses_exceeded"},
			},
			wantError: "web search failed: max_uses_exceeded",
		},
		{
			name: "OpenAIFailedStatus",
			metadata: fantasy.ProviderMetadata{
				fantasyopenai.Name: &fantasyopenai.WebSearchCallMetadata{ItemID: "ws_1", Status: "failed"},
			},
			wantError: "web search failed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			stream := iter.Seq[fantasy.StreamPart](func(yield func(fantasy.StreamPart) bool) {
				yield(fantasy.StreamPart{
					Type:             fantasy.StreamPartTypeSource,
					ID:               "https://go.dev/doc/go1.27",
					SourceType:       fantasy.SourceTypeURL,
					URL:              "https://go.dev/doc/go1.27",
					Title:            "Go 1.27",
					SourceToolCallID: "srvtoolu_1",
				})
				yield(fantasy.StreamPart{
					Type:             fantasy.StreamPartTypeToolResult,
					ID:               "srvtoolu_1",
					ToolCallName:     "web_search",
					ProviderExecuted: true,
					ProviderMetadata: tt.metadata,
				})
				yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop})
			})

			var published []codersdk.ChatMessagePart
			result, err := processStepStream(stream, quartz.NewMock(t), func(_ codersdk.ChatMessageRole, part codersdk.ChatMessagePart) {
				published = append(published, part)
			})
			require.NoError(t, err)
			require.Len(t, result.content, 2)

			source, ok := fantasy.AsContentType[fantasy.SourceContent](result.content[0])
			require.True(t, ok)
			require.Equal(t, "srvtoolu_1", source.ToolCallID)
			toolResult, ok := fantasy.AsContentType[fantasy.ToolResultContent](result.content[1])
			require.True(t, ok)
			if tt.wantError == "" {
				require.Nil(t, toolResult.Result)
			} else {
				errorOutput, ok := toolResult.Result.(fantasy.ToolResultOutputContentError)
				require.True(t, ok, "result should be an error output, got %T", toolResult.Result)
				require.EqualError(t, errorOutput.Error, tt.wantError)
			}

			require.Len(t, published, 2)
			require.Equal(t, codersdk.ChatMessagePartTypeSource, published[0].Type)
			require.Equal(t, "srvtoolu_1", published[0].ToolCallID)
			require.Equal(t, codersdk.ChatMessagePartTypeToolResult, published[1].Type)
			require.Equal(t, tt.wantError != "", published[1].IsError)
		})
	}
}
