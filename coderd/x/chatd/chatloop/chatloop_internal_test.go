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

func TestProcessStepStreamLabelsNarration(t *testing.T) {
	t.Parallel()

	phase := func(itemID, phase string) fantasy.ProviderMetadata {
		return fantasy.ProviderMetadata{
			fantasyopenai.Name: &fantasyopenai.ResponsesTextMetadata{ItemID: itemID, Phase: phase},
		}
	}
	stream := iter.Seq[fantasy.StreamPart](func(yield func(fantasy.StreamPart) bool) {
		// The narration's end carries no metadata, so the start's label
		// must persist.
		yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextStart, ID: "msg_1", ProviderMetadata: phase("msg_1", "commentary")})
		yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextDelta, ID: "msg_1", Delta: "Reading"})
		yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextDelta, ID: "msg_1", Delta: " the file."})
		yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextEnd, ID: "msg_1", ProviderMetadata: fantasy.ProviderMetadata{}})
		yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextStart, ID: "msg_2", ProviderMetadata: phase("msg_2", "final_answer")})
		yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextDelta, ID: "msg_2", Delta: "It is flaky."})
		yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextEnd, ID: "msg_2", ProviderMetadata: phase("msg_2", "final_answer")})
		yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop})
	})

	var published []codersdk.ChatMessagePart
	result, err := processStepStream(stream, quartz.NewMock(t), func(_ codersdk.ChatMessageRole, part codersdk.ChatMessagePart) {
		published = append(published, part)
	})
	require.NoError(t, err)

	narration := codersdk.ChatMessageText("Reading")
	narration.Narration = true
	rest := codersdk.ChatMessageText(" the file.")
	rest.Narration = true
	require.Equal(t, []codersdk.ChatMessagePart{
		narration,
		rest,
		codersdk.ChatMessageText("It is flaky."),
	}, published)

	require.Equal(t, []fantasy.Content{
		fantasy.TextContent{Text: "Reading the file.", ProviderMetadata: phase("msg_1", "commentary")},
		fantasy.TextContent{Text: "It is flaky.", ProviderMetadata: phase("msg_2", "final_answer")},
	}, result.content)
}
