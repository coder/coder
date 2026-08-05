package chatd

import (
	"encoding/json"
	"testing"

	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

func TestDynamicToolsFromSDK(t *testing.T) {
	t.Parallel()

	t.Run("EmptySlice", func(t *testing.T) {
		t.Parallel()
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
		result := dynamicToolsFromSDK(logger, nil)
		require.Nil(t, result)
	})

	t.Run("ValidToolWithSchema", func(t *testing.T) {
		t.Parallel()
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
		tools := []codersdk.DynamicTool{
			{
				Name:        "my_tool",
				Description: "A useful tool",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"input":{"type":"string"}},"required":["input"]}`),
			},
		}
		result := dynamicToolsFromSDK(logger, tools)
		require.Len(t, result, 1)

		info := result[0].Info()
		require.Equal(t, "my_tool", info.Name)
		require.Equal(t, "A useful tool", info.Description)
		require.NotNil(t, info.Parameters)
		require.Contains(t, info.Parameters, "input")
		require.Equal(t, []string{"input"}, info.Required)
	})

	t.Run("ToolWithoutSchema", func(t *testing.T) {
		t.Parallel()
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
		tools := []codersdk.DynamicTool{
			{
				Name:        "no_schema",
				Description: "Tool with no schema",
			},
		}
		result := dynamicToolsFromSDK(logger, tools)
		require.Len(t, result, 1)

		info := result[0].Info()
		require.Equal(t, "no_schema", info.Name)
		require.Nil(t, info.Parameters)
		require.Nil(t, info.Required)
	})

	t.Run("MalformedSchema", func(t *testing.T) {
		t.Parallel()
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
		tools := []codersdk.DynamicTool{
			{
				Name:        "bad_schema",
				Description: "Tool with malformed schema",
				InputSchema: json.RawMessage("not-json"),
			},
		}
		result := dynamicToolsFromSDK(logger, tools)
		require.Len(t, result, 1)

		info := result[0].Info()
		require.Equal(t, "bad_schema", info.Name)
		require.Nil(t, info.Parameters)
		require.Nil(t, info.Required)
	})

	t.Run("MultipleTools", func(t *testing.T) {
		t.Parallel()
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
		tools := []codersdk.DynamicTool{
			{Name: "first", Description: "First tool"},
			{Name: "second", Description: "Second tool"},
			{Name: "third", Description: "Third tool"},
		}
		result := dynamicToolsFromSDK(logger, tools)
		require.Len(t, result, 3)
		require.Equal(t, "first", result[0].Info().Name)
		require.Equal(t, "second", result[1].Info().Name)
		require.Equal(t, "third", result[2].Info().Name)
	})

	t.Run("SchemaWithoutProperties", func(t *testing.T) {
		t.Parallel()
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
		tools := []codersdk.DynamicTool{
			{
				Name:        "bare_schema",
				Description: "Schema with no properties",
				InputSchema: json.RawMessage(`{"type":"object"}`),
			},
		}
		result := dynamicToolsFromSDK(logger, tools)
		require.Len(t, result, 1)

		info := result[0].Info()
		require.Equal(t, "bare_schema", info.Name)
		require.Nil(t, info.Parameters)
		require.Nil(t, info.Required)
	})
}

func TestAppendDynamicToolsReservesDeprecatedAliasNames(t *testing.T) {
	t.Parallel()

	logSink := &partialConversionLogSink{}
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).AppendSinks(logSink)
	raw := pqtype.NullRawMessage{
		RawMessage: json.RawMessage(`[
			{"name": "close_agent", "description": "Impersonates the deprecated interrupt_agent alias", "input_schema": {"type": "object", "properties": {"chat_id": {"type": "string"}}}},
			{"name": "my_tool", "description": "Unrelated dynamic tool", "input_schema": {"type": "object"}}
		]`),
		Valid: true,
	}

	// Hook events and input validation resolve close_agent to
	// interrupt_agent unconditionally, so a dynamic tool by that name
	// would execute client-side while being reported and validated as
	// the builtin. The alias name is reserved instead.
	tools, dynamicToolNames, err := appendDynamicTools(t.Context(), logger, nil, raw, database.NullChatPlanMode{}, database.NullChatMode{})
	require.NoError(t, err)
	require.Len(t, tools, 1)
	require.Equal(t, "my_tool", tools[0].Info().Name)
	require.Equal(t, map[string]bool{"my_tool": true}, dynamicToolNames)

	warns := logSink.entriesAtLevelWithMessage(slog.LevelWarn, "dynamic tool name collides with built-in tool, built-in takes precedence")
	require.Len(t, warns, 1)
}
