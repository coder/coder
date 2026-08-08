package chatloop

import (
	"context"
	"encoding/json"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nilParamsTool is an AgentTool whose Info reports a nil parameter map,
// mirroring a zero-argument MCP tool.
type nilParamsTool struct {
	name string
}

func (t nilParamsTool) Info() fantasy.ToolInfo {
	return fantasy.ToolInfo{
		Name:       t.name,
		Parameters: nil,
		Required:   nil,
	}
}

func (nilParamsTool) Run(context.Context, fantasy.ToolCall) (fantasy.ToolResponse, error) {
	return fantasy.NewTextResponse("ok"), nil
}

func (nilParamsTool) ProviderOptions() fantasy.ProviderOptions { return nil }

func (nilParamsTool) SetProviderOptions(fantasy.ProviderOptions) {}

// TestBuildToolDefinitions_NilParametersBecomesEmptyObject verifies that
// a tool with no parameters produces "properties": {} rather than null.
// OpenAI rejects a null "properties" value with "None is not of type
// 'object'".
func TestBuildToolDefinitions_NilParametersBecomesEmptyObject(t *testing.T) {
	t.Parallel()

	tools := buildToolDefinitions(
		[]fantasy.AgentTool{nilParamsTool{name: "no_args"}},
		[]string{"no_args"},
		nil,
	)
	require.Len(t, tools, 1)

	fn, ok := tools[0].(fantasy.FunctionTool)
	require.True(t, ok, "expected a FunctionTool")

	properties, ok := fn.InputSchema["properties"]
	require.True(t, ok, "input schema should have a properties field")
	require.NotNil(t, properties, "properties should never be nil")

	// The whole schema must serialize with "properties":{}, not null.
	bs, err := json.Marshal(fn.InputSchema)
	require.NoError(t, err)
	assert.Contains(t, string(bs), `"properties":{}`)
	assert.NotContains(t, string(bs), `"properties":null`)
}
