package chatloop

import (
	"context"
	"encoding/json"
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

// staticParametersTool returns a fixed ToolInfo, letting a test control
// Parameters directly. fantasy.NewAgentTool always generates a non-nil
// Parameters map, so it cannot reproduce the nil Parameters that MCP tool
// wrappers report for a schema with an empty "properties" object.
type staticParametersTool struct {
	fantasy.AgentTool
	info fantasy.ToolInfo
}

func (t staticParametersTool) Info() fantasy.ToolInfo { return t.info }

func (t staticParametersTool) ProviderOptions() fantasy.ProviderOptions { return nil }

// TestBuildToolDefinitionsNilPropertiesBecomesEmptyObject verifies that a
// tool whose input schema has no properties (for example an MCP tool
// reporting {"type": "object", "properties": {}}) still serializes
// "properties" as an empty JSON object. A nil Parameters map would serialize
// to null, which OpenAI rejects with "Invalid schema for function ... None is
// not of type 'object'".
func TestBuildToolDefinitionsNilPropertiesBecomesEmptyObject(t *testing.T) {
	t.Parallel()

	tool := staticParametersTool{
		info: fantasy.ToolInfo{
			Name:        "document_graphql_schema",
			Description: "Run a GraphQL query",
			Parameters:  nil,
		},
	}

	defs := buildToolDefinitions([]fantasy.AgentTool{tool}, nil, nil)
	require.Len(t, defs, 1)

	ft, ok := defs[0].(fantasy.FunctionTool)
	require.True(t, ok, "expected a fantasy.FunctionTool")

	properties, ok := ft.InputSchema["properties"].(map[string]any)
	require.True(t, ok, "properties must be a map, got %T", ft.InputSchema["properties"])
	require.NotNil(t, properties, "properties must not be nil")

	// Verify it serializes to {} not null.
	bs, err := json.Marshal(ft.InputSchema)
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"object","properties":{}}`, string(bs))
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
