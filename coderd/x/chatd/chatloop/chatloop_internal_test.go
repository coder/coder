package chatloop

import (
	"context"
	"iter"
	"testing"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
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

	result, err := processStepStream(context.Background(), stream, quartz.NewMock(t), func(codersdk.ChatMessageRole, codersdk.ChatMessagePart) {})
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

	result, err := processStepStream(context.Background(), stream, quartz.NewMock(t), func(codersdk.ChatMessageRole, codersdk.ChatMessagePart) {})
	require.NoError(t, err)
	require.Len(t, result.content, 2)
	reasoning, ok := fantasy.AsContentType[fantasy.ReasoningContent](result.content[0])
	require.True(t, ok)
	require.Empty(t, reasoning.Text)
	metadata := fantasyanthropic.GetReasoningMetadata(fantasy.ProviderOptions(reasoning.ProviderMetadata))
	require.NotNil(t, metadata)
	require.Equal(t, "redacted-payload", metadata.RedactedData)
}

func TestFlushActiveStatePreservesEmptySignedReasoning(t *testing.T) {
	t.Parallel()

	result := &stepResult{}
	flushActiveState(
		result,
		quartz.NewMock(t),
		map[string]string{},
		map[string]reasoningState{
			"signed": {
				options: fantasy.ProviderMetadata{
					fantasyanthropic.Name: &fantasyanthropic.ReasoningOptionMetadata{
						RedactedData: "redacted-payload",
					},
				},
			},
			"empty": {},
		},
		map[string]*fantasy.ToolCallContent{},
		map[string]string{},
	)

	require.Len(t, result.content, 1)
	reasoning, ok := fantasy.AsContentType[fantasy.ReasoningContent](result.content[0])
	require.True(t, ok)
	require.Empty(t, reasoning.Text)
	metadata := fantasyanthropic.GetReasoningMetadata(fantasy.ProviderOptions(reasoning.ProviderMetadata))
	require.NotNil(t, metadata)
	require.Equal(t, "redacted-payload", metadata.RedactedData)
}
