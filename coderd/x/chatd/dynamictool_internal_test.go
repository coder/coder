package chatd

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
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
		full, ok := result[0].(*dynamicTool)
		require.True(t, ok)
		require.Nil(t, full.FullInputSchema())
	})

	t.Run("SchemaKeptWhole", func(t *testing.T) {
		t.Parallel()
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
		// 9007199254740993 has no exact float64 form.
		schema := `{"type":"object","properties":{"filter":{"$ref":"#/$defs/Filter"}},"required":["filter"],"additionalProperties":false,"$defs":{"Filter":{"type":"string"},"ID":{"const":9007199254740993}}}`
		tools := []codersdk.DynamicTool{
			{
				Name:        "ref_tool",
				Description: "Schema with $defs",
				InputSchema: json.RawMessage(schema),
			},
		}
		result := dynamicToolsFromSDK(logger, tools)
		require.Len(t, result, 1)

		full, ok := result[0].(*dynamicTool)
		require.True(t, ok)
		raw, err := json.Marshal(full.FullInputSchema())
		require.NoError(t, err)
		require.JSONEq(t, schema, string(raw))
		require.Contains(t, string(raw), `{"const":9007199254740993}`)

		// Info() keeps carrying the flattened pair.
		info := result[0].Info()
		require.Contains(t, info.Parameters, "filter")
		require.Equal(t, []string{"filter"}, info.Required)
	})

	t.Run("SchemaNumbersReachProvidersAsNumbers", func(t *testing.T) {
		t.Parallel()
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
		schema := `{"type":"object","properties":{"q":{"type":"string","minLength":3},"id":{"const":9007199254740993},"e":{"enum":[12345678901234567891,1.5]}},"required":["q"]}`
		tools := dynamicToolsFromSDK(logger, []codersdk.DynamicTool{{Name: "numbers", InputSchema: json.RawMessage(schema)}})
		call := fantasy.Call{
			Prompt: []fantasy.Message{{
				Role:    fantasy.MessageRoleUser,
				Content: []fantasy.MessagePart{fantasy.TextPart{Text: "hi"}},
			}},
			Tools: chatloop.BuildToolDefinitions(tools, nil, nil),
		}
		// The provider SDKs encode tool schemas with their own JSON
		// encoders, which differ from encoding/json.
		requireNumbersSent := func(t *testing.T, provider, model string, serverURL string, sent <-chan string) {
			ctx := testutil.Context(t, testutil.WaitShort)
			keys := chatprovider.ProviderAPIKeys{
				ByProvider:        map[string]string{provider: "test-key"},
				BaseURLByProvider: map[string]string{provider: serverURL},
			}
			languageModel, err := chatprovider.ModelFromConfig(provider, model, keys, chatprovider.UserAgent(), nil, nil, nil)
			require.NoError(t, err)
			_, err = languageModel.LanguageModel().Generate(ctx, call)
			require.NoError(t, err)
			raw := testutil.TryReceive(ctx, t, sent)
			for _, want := range []string{`"minLength":3`, `"const":9007199254740993`, `"enum":[12345678901234567891,1.5]`} {
				require.Contains(t, raw, want)
			}
		}

		t.Run("Anthropic", func(t *testing.T) {
			t.Parallel()
			sent := make(chan string, 1)
			serverURL := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
				var schemas []string
				for _, tool := range req.Tools {
					schemas = append(schemas, string(tool.InputSchema))
				}
				sent <- strings.Join(schemas, ",")
				return chattest.AnthropicNonStreamingResponse("ok")
			})
			requireNumbersSent(t, "anthropic", "claude-sonnet-4-20250514", serverURL, sent)
		})

		t.Run("OpenAI", func(t *testing.T) {
			t.Parallel()
			sent := make(chan string, 1)
			serverURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
				sent <- string(req.RawBody)
				return chattest.OpenAINonStreamingResponse("ok")
			})
			requireNumbersSent(t, "openai", "gpt-4", serverURL, sent)
		})
	})

	t.Run("OversizedSchemasFallBack", func(t *testing.T) {
		t.Parallel()
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
		schema := func(padding int) json.RawMessage {
			return json.RawMessage(fmt.Sprintf(
				`{"type":"object","properties":{"q":{"type":"string"}},"required":["q"],"$defs":{"P":{"description":%q}}}`,
				strings.Repeat("a", padding),
			))
		}
		// One schema is over the 64 KiB per-tool cap. Of the five under it,
		// the fifth would take the total past the 256 KiB cap.
		tools := []codersdk.DynamicTool{{Name: "huge", InputSchema: schema(64 << 10)}}
		for i := range 5 {
			tools = append(tools, codersdk.DynamicTool{
				Name:        fmt.Sprintf("big_%d", i),
				InputSchema: schema(55_000),
			})
		}
		result := dynamicToolsFromSDK(logger, tools)
		require.Len(t, result, len(tools))

		var kept []string
		for _, tool := range result {
			full, ok := tool.(*dynamicTool)
			require.True(t, ok)
			if full.FullInputSchema() != nil {
				kept = append(kept, tool.Info().Name)
			}
			// A tool past a cap keeps the flattened pair.
			info := tool.Info()
			require.Contains(t, info.Parameters, "q")
			require.Equal(t, []string{"q"}, info.Required)
		}
		require.Equal(t, []string{"big_0", "big_1", "big_2", "big_3"}, kept)
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
