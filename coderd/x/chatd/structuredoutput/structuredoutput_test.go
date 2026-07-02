package structuredoutput_test

import (
	"encoding/json"
	"strings"
	"testing"

	"charm.land/fantasy"
	fantasyschema "charm.land/fantasy/schema"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/x/chatd/structuredoutput"
	"github.com/coder/coder/v2/codersdk"
)

const validSchema = `{
	"type": "object",
	"properties": {
		"title": {"type": "string"},
		"tags": {"type": "array", "items": {"type": "string"}},
		"score": {"type": "integer", "minimum": 0}
	},
	"required": ["title", "score"],
	"additionalProperties": false
}`

func jsonSchemaFormat(schema string) *codersdk.ChatResponseFormat {
	return &codersdk.ChatResponseFormat{
		Type: codersdk.ChatResponseFormatTypeJSONSchema,
		JSONSchema: &codersdk.ChatResponseFormatJSONSchema{
			Name:   "test_output",
			Schema: json.RawMessage(schema),
		},
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()

	t.Run("NilFormat", func(t *testing.T) {
		t.Parallel()
		require.Nil(t, structuredoutput.Validate(nil))
	})

	t.Run("TextWithoutSchema", func(t *testing.T) {
		t.Parallel()
		require.Nil(t, structuredoutput.Validate(&codersdk.ChatResponseFormat{
			Type: codersdk.ChatResponseFormatTypeText,
		}))
	})

	t.Run("TextWithSchemaRejected", func(t *testing.T) {
		t.Parallel()
		err := structuredoutput.Validate(&codersdk.ChatResponseFormat{
			Type:       codersdk.ChatResponseFormatTypeText,
			JSONSchema: &codersdk.ChatResponseFormatJSONSchema{Name: "x", Schema: json.RawMessage(`{"type":"object"}`)},
		})
		require.NotNil(t, err)
		require.Equal(t, "response_format.json_schema", err.Field)
	})

	t.Run("MissingType", func(t *testing.T) {
		t.Parallel()
		err := structuredoutput.Validate(&codersdk.ChatResponseFormat{})
		require.NotNil(t, err)
		require.Equal(t, "response_format.type", err.Field)
	})

	t.Run("UnknownType", func(t *testing.T) {
		t.Parallel()
		err := structuredoutput.Validate(&codersdk.ChatResponseFormat{Type: "yaml"})
		require.NotNil(t, err)
		require.Equal(t, "response_format.type", err.Field)
	})

	t.Run("JSONSchemaMissingPayload", func(t *testing.T) {
		t.Parallel()
		err := structuredoutput.Validate(&codersdk.ChatResponseFormat{
			Type: codersdk.ChatResponseFormatTypeJSONSchema,
		})
		require.NotNil(t, err)
		require.Equal(t, "response_format.json_schema", err.Field)
	})

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		require.Nil(t, structuredoutput.Validate(jsonSchemaFormat(validSchema)))
	})

	t.Run("Name", func(t *testing.T) {
		t.Parallel()
		for _, name := range []string{"", "has space", "über", strings.Repeat("a", 65)} {
			format := jsonSchemaFormat(validSchema)
			format.JSONSchema.Name = name
			err := structuredoutput.Validate(format)
			require.NotNil(t, err, "name %q should be rejected", name)
			require.Equal(t, "response_format.json_schema.name", err.Field)
		}
		for _, name := range []string{"a", "snake_case", "kebab-case", "MiXeD123", strings.Repeat("a", 64)} {
			format := jsonSchemaFormat(validSchema)
			format.JSONSchema.Name = name
			require.Nil(t, structuredoutput.Validate(format), "name %q should be accepted", name)
		}
	})

	t.Run("StrictDefaulting", func(t *testing.T) {
		t.Parallel()
		// Omitted and explicit true are accepted.
		require.Nil(t, structuredoutput.Validate(jsonSchemaFormat(validSchema)))
		format := jsonSchemaFormat(validSchema)
		format.JSONSchema.Strict = ptr.Ref(true)
		require.Nil(t, structuredoutput.Validate(format))
		// Explicit false is rejected rather than silently ignored.
		format.JSONSchema.Strict = ptr.Ref(false)
		err := structuredoutput.Validate(format)
		require.NotNil(t, err)
		require.Equal(t, "response_format.json_schema.strict", err.Field)
	})

	t.Run("SchemaMissing", func(t *testing.T) {
		t.Parallel()
		format := jsonSchemaFormat(validSchema)
		format.JSONSchema.Schema = nil
		err := structuredoutput.Validate(format)
		require.NotNil(t, err)
		require.Equal(t, "response_format.json_schema.schema", err.Field)
	})

	t.Run("SchemaTooLarge", func(t *testing.T) {
		t.Parallel()
		huge := `{"type":"object","description":"` + strings.Repeat("x", structuredoutput.MaxSchemaBytes) + `"}`
		err := structuredoutput.Validate(jsonSchemaFormat(huge))
		require.NotNil(t, err)
		require.Equal(t, "response_format.json_schema.schema", err.Field)
		require.Contains(t, err.Detail, "maximum size")
	})

	t.Run("SchemaNotJSON", func(t *testing.T) {
		t.Parallel()
		err := structuredoutput.Validate(jsonSchemaFormat(`{"type": "object"`))
		require.NotNil(t, err)
		require.Equal(t, "response_format.json_schema.schema", err.Field)
	})

	t.Run("RootNotObject", func(t *testing.T) {
		t.Parallel()
		for _, schema := range []string{
			`{"type":"array","items":{"type":"string"}}`,
			`{"type":"string"}`,
			`{"properties":{"a":{"type":"string"}}}`,
			`true`,
		} {
			err := structuredoutput.Validate(jsonSchemaFormat(schema))
			require.NotNil(t, err, "schema %s should be rejected", schema)
			require.Equal(t, "response_format.json_schema.schema", err.Field)
		}
	})

	t.Run("FragmentLocalRefs", func(t *testing.T) {
		t.Parallel()
		valid := `{
			"type": "object",
			"properties": {"node": {"$ref": "#/$defs/node"}},
			"$defs": {"node": {"type": "object", "properties": {"name": {"type": "string"}}}}
		}`
		require.Nil(t, structuredoutput.Validate(jsonSchemaFormat(valid)))

		for _, schema := range []string{
			`{"type":"object","properties":{"a":{"$ref":"https://example.com/schema.json"}}}`,
			`{"type":"object","properties":{"a":{"$ref":"file:///etc/passwd"}}}`,
			`{"type":"object","properties":{"a":{"$dynamicRef":"https://example.com/x"}}}`,
			`{"type":"object","allOf":[{"$ref":"other.json#/foo"}]}`,
		} {
			err := structuredoutput.Validate(jsonSchemaFormat(schema))
			require.NotNil(t, err, "schema %s should be rejected", schema)
			require.Equal(t, "response_format.json_schema.schema", err.Field)
			require.Contains(t, err.Detail, "fragment-local")
		}
	})
}

func TestNewRequest(t *testing.T) {
	t.Parallel()

	t.Run("NilForNoRequest", func(t *testing.T) {
		t.Parallel()
		req, verr := structuredoutput.NewRequest(nil)
		require.Nil(t, verr)
		require.Nil(t, req)

		req, verr = structuredoutput.NewRequest(&codersdk.ChatResponseFormat{
			Type: codersdk.ChatResponseFormatTypeText,
		})
		require.Nil(t, verr)
		require.Nil(t, req)
	})

	t.Run("CarriesMetadata", func(t *testing.T) {
		t.Parallel()
		format := jsonSchemaFormat(validSchema)
		format.JSONSchema.Description = "a report"
		req, verr := structuredoutput.NewRequest(format)
		require.Nil(t, verr)
		require.Equal(t, "test_output", req.Name)
		require.Equal(t, "a report", req.Description)
		require.JSONEq(t, validSchema, string(req.Schema))
	})
}

func TestTool(t *testing.T) {
	t.Parallel()

	newRequest := func(t *testing.T, schema string) *structuredoutput.Request {
		t.Helper()
		req, verr := structuredoutput.NewRequest(jsonSchemaFormat(schema))
		require.Nil(t, verr)
		require.NotNil(t, req)
		return req
	}

	t.Run("Info", func(t *testing.T) {
		t.Parallel()
		tool := structuredoutput.Tool(newRequest(t, validSchema))
		info := tool.Info()
		require.Equal(t, structuredoutput.ToolName, info.Name)
		require.Equal(t, []string{"output"}, info.Required)
		require.False(t, info.Parallel)

		// The caller schema is wrapped under the "output" property.
		outputSchema, ok := info.Parameters["output"].(map[string]any)
		require.True(t, ok, "output parameter should be the caller schema")
		require.Equal(t, "object", outputSchema["type"])
		var want map[string]any
		require.NoError(t, json.Unmarshal([]byte(validSchema), &want))
		require.Equal(t, want, outputSchema)

		// Each Info call returns a deep copy: mutations by consumers
		// (e.g. chatloop's schema.Normalize) must not leak into
		// later calls.
		outputSchema["type"] = "mutated"
		fresh, ok := tool.Info().Parameters["output"].(map[string]any)
		require.True(t, ok)
		require.Equal(t, "object", fresh["type"])
	})

	t.Run("InfoIncludesDescription", func(t *testing.T) {
		t.Parallel()
		format := jsonSchemaFormat(validSchema)
		format.JSONSchema.Description = "a quarterly report"
		req, verr := structuredoutput.NewRequest(format)
		require.Nil(t, verr)
		info := structuredoutput.Tool(req).Info()
		require.Contains(t, info.Description, "Output description: a quarterly report")
	})

	t.Run("RunValidCanonicalizes", func(t *testing.T) {
		t.Parallel()
		tool := structuredoutput.Tool(newRequest(t, validSchema))
		resp, err := tool.Run(t.Context(), fantasy.ToolCall{
			ID:   "call_1",
			Name: structuredoutput.ToolName,
			// Non-canonical spacing and key order.
			Input: "{\"output\": {\n\t\"score\": 3,   \"title\": \"hi\"\n}}",
		})
		require.NoError(t, err)
		require.False(t, resp.IsError)
		require.JSONEq(t, `{"score":3,"title":"hi"}`, resp.Content)
		// Canonical encoding is compact.
		require.NotContains(t, resp.Content, "\n")
	})

	t.Run("RunInvalidJSONArgs", func(t *testing.T) {
		t.Parallel()
		tool := structuredoutput.Tool(newRequest(t, validSchema))
		resp, err := tool.Run(t.Context(), fantasy.ToolCall{Input: `not json`})
		require.NoError(t, err)
		require.True(t, resp.IsError)
		require.Contains(t, resp.Content, "invalid arguments")
	})

	t.Run("RunMissingOutput", func(t *testing.T) {
		t.Parallel()
		tool := structuredoutput.Tool(newRequest(t, validSchema))
		resp, err := tool.Run(t.Context(), fantasy.ToolCall{Input: `{"result": {}}`})
		require.NoError(t, err)
		require.True(t, resp.IsError)
		require.Contains(t, resp.Content, `missing required "output" argument`)
	})

	t.Run("RunSchemaMismatch", func(t *testing.T) {
		t.Parallel()
		tool := structuredoutput.Tool(newRequest(t, validSchema))
		for _, input := range []string{
			`{"output": {"title": "hi"}}`,                            // missing required score
			`{"output": {"title": "hi", "score": -1}}`,               // minimum violation
			`{"output": {"title": "hi", "score": 1, "extra": true}}`, // additionalProperties
			`{"output": {"title": 42, "score": 1}}`,                  // wrong type
			`{"output": ["not", "an", "object"]}`,                    // wrong root kind
		} {
			resp, err := tool.Run(t.Context(), fantasy.ToolCall{Input: input})
			require.NoError(t, err)
			require.True(t, resp.IsError, "input %s should fail validation", input)
			require.Contains(t, resp.Content, "does not satisfy the required schema")
			require.Contains(t, resp.Content, structuredoutput.ToolName)
		}
	})

	// buildToolDefinitions in chatloop runs schema.Normalize on the
	// wrapped tool schema before sending it to the provider. Guard
	// that normalizing a nested caller schema keeps its semantics
	// (type arrays become anyOf, bare arrays gain items) and that
	// the in-place mutation does not corrupt the tool's validation.
	t.Run("SchemaNormalizeRoundTrip", func(t *testing.T) {
		t.Parallel()
		nested := `{
			"type": "object",
			"properties": {
				"name": {"type": ["string", "null"]},
				"list": {"type": "array"},
				"child": {
					"type": "object",
					"properties": {"deep": {"type": ["integer", "string"]}},
					"additionalProperties": false
				}
			},
			"required": ["name"]
		}`
		tool := structuredoutput.Tool(newRequest(t, nested))
		info := tool.Info()

		// Mirror the wrapping and normalization the chatloop applies
		// before sending the tool definition to the provider.
		inputSchema := map[string]any{
			"type":       "object",
			"properties": info.Parameters,
			"required":   info.Required,
		}
		fantasyschema.Normalize(inputSchema)

		// The normalized tool-input schema validates the whole
		// {"output": ...} argument object. ValidateOutput extracts
		// the "output" property before validating, so nest the tool
		// args under "output" once to reuse it for the check.
		normalized, err := json.Marshal(inputSchema)
		require.NoError(t, err)
		envelope, verr := structuredoutput.NewRequest(&codersdk.ChatResponseFormat{
			Type: codersdk.ChatResponseFormatTypeJSONSchema,
			JSONSchema: &codersdk.ChatResponseFormatJSONSchema{
				Name:   "normalized_envelope",
				Schema: normalized,
			},
		})
		require.Nil(t, verr)

		validArgs := `{"output": {"name": null, "list": [1, "x"], "child": {"deep": "y"}}}`
		invalidArgs := `{"output": {"child": {"deep": true}}}`
		_, err = envelope.ValidateOutput([]byte(`{"output": ` + validArgs + `}`))
		require.NoError(t, err, "normalized schema must accept what the original accepts")
		_, err = envelope.ValidateOutput([]byte(`{"output": ` + invalidArgs + `}`))
		require.Error(t, err, "normalized schema must reject what the original rejects")

		// The Normalize mutation above must not corrupt the tool's
		// own validation: Run validates against the compiled
		// schema, and each Info call deep-copies the parameter map.
		resp, runErr := tool.Run(t.Context(), fantasy.ToolCall{Input: validArgs})
		require.NoError(t, runErr)
		require.False(t, resp.IsError)
		resp, runErr = tool.Run(t.Context(), fantasy.ToolCall{Input: invalidArgs})
		require.NoError(t, runErr)
		require.True(t, resp.IsError)
	})
}
