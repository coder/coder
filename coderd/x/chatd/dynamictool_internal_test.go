package chatd

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
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
