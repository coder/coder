package chatstructured

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"charm.land/fantasy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// canonicalJSON re-encodes a raw document with sorted keys and exact numbers.
func canonicalJSON(t *testing.T, raw string) string {
	t.Helper()
	doc, err := ParseFinalizerArguments([]byte(raw))
	require.NoError(t, err)
	out, err := json.Marshal(doc)
	require.NoError(t, err)
	return string(out)
}

func TestFinalizerDefinition(t *testing.T) {
	t.Parallel()
	// Annotations, nested booleans and exact numbers survive unchanged.
	body := `"title":"Report","description":"d","examples":[{"score":0.3}],"type":"object",` +
		`"properties":{"score":{"type":"number","multipleOf":0.1,"maximum":9007199254740993},"yes":true,"no":false},"required":["score"]`
	s := mustCompile(t, `{"$schema":"`+draft07+`",`+body+`}`)
	def := s.FinalizerDefinition()
	require.Equal(t, "coder_structured_output", def.Name)
	require.Equal(t, FinalizerToolName, def.GetName())
	require.NotEmpty(t, def.Description)
	got, err := json.Marshal(def.InputSchema)
	require.NoError(t, err)
	want := canonicalJSON(t, `{"type":"object","properties":{"output":{`+body+`}},"required":["output"],"additionalProperties":false}`)
	require.Equal(t, want, string(got))

	// Mutating a returned definition changes neither later definitions
	// nor validation.
	output := def.InputSchema["properties"].(map[string]any)["output"].(map[string]any)
	output["type"] = "string"
	output["required"].([]any)[0] = "missing"
	delete(output["properties"].(map[string]any), "score")
	def.InputSchema["required"].([]string)[0] = "other"
	again, err := json.Marshal(s.FinalizerDefinition().InputSchema)
	require.NoError(t, err)
	require.Equal(t, want, string(again))
	_, err = s.CheckFinalizerArguments([]byte(`{"output":{"score":0.3}}`))
	require.NoError(t, err)
	_, err = s.CheckFinalizerArguments([]byte(`{"output":{"score":0.35}}`))
	require.ErrorIs(t, err, ErrValueMismatch)
}

func TestCheckFinalizerArguments(t *testing.T) {
	t.Parallel()
	num := mustCompile(t, `{"type":"object","properties":{"n":{"multipleOf":0.1,"maximum":9007199254740992}},"required":["n"]}`)
	open := mustCompile(t, `{}`)
	null := mustCompile(t, `{"type":"null"}`)
	// Raw U+2028 is 3 bytes in the output but 6 once re-marshaled.
	separators := strings.Repeat("\u2028", 21844)
	remarshaled, err := json.Marshal(separators)
	require.NoError(t, err)
	require.Greater(t, len(remarshaled), 64<<10)
	for _, tt := range []struct {
		s    *Schema
		args string
		want error
	}{
		{num, `{"output":{"n":0.3}}`, nil},
		{num, `{"output":{"n":9007199254740992}}`, nil},
		{num, `{"output":{"n":9007199254740993}}`, ErrValueMismatch},
		{num, `{"output":{"n":0.35}}`, ErrValueMismatch},
		{num, `{"output":[1]}`, ErrValueMismatch},
		{open, `{"output":[1,"a"]}`, nil},
		{open, `{"output":"s"}`, nil},
		{open, `{"output":1.5}`, nil},
		{open, `{"output":true}`, nil},
		{open, ` { "output" : null } `, nil},
		{null, `{"output":null}`, nil},
		{null, `{"output":0}`, ErrValueMismatch},
		{open, `{"output":"` + separators + `"}`, nil},
		{open, `{}`, ErrInvalidFinalizerArguments},
		{open, `{"output":1,"x":2}`, ErrInvalidFinalizerArguments},
		{open, `{"OUTPUT":1}`, ErrInvalidFinalizerArguments},
		{open, `[{"output":1}]`, ErrInvalidFinalizerArguments},
		{open, `"output"`, ErrInvalidFinalizerArguments},
		{open, `null`, ErrInvalidFinalizerArguments},
		{open, `{"output":1,"output":2}`, ErrDuplicateKey},
		{open, `{"output":1} {}`, ErrTrailingData},
		{open, `{"output":` + strings.Repeat("[", 33) + strings.Repeat("]", 33) + `}`, ErrTooDeep},
		{open, `{"output":` + string(objectValue(4096)) + `}`, ErrTooManyNodes},
		{open, `{"output":"` + strings.Repeat("a", 80<<10+1-13) + `"}`, ErrTooLarge},
		{open, `{"output":"` + strings.Repeat("a", 64<<10) + `"}`, ErrTooLarge},
	} {
		got, err := tt.s.CheckFinalizerArguments([]byte(tt.args))
		if tt.want != nil {
			require.ErrorIs(t, err, tt.want, tt.args)
			continue
		}
		require.NoError(t, err, tt.args)
		envelope, err := ParseFinalizerArguments([]byte(tt.args))
		require.NoError(t, err)
		require.Equal(t, envelope.(map[string]any)["output"], got, tt.args)
	}
}

func TestFinalizerRunner(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	raw := `{"type":"object","properties":{"a":{"type":"string"}},"required":["a"],"additionalProperties":false}`
	s := mustCompile(t, raw)
	r := s.FinalizerRunner()
	schema, err := ParseSchemaDocument([]byte(raw))
	require.NoError(t, err)
	info := r.Info()
	require.Equal(t, fantasy.ToolInfo{
		Name: FinalizerToolName, Description: s.FinalizerDefinition().Description,
		Parameters: map[string]any{"output": schema}, Required: []string{"output"},
	}, info)
	info.Parameters["output"].(map[string]any)["type"] = "string"
	require.Equal(t, schema, r.Info().Parameters["output"])

	resp, err := r.Run(ctx, fantasy.ToolCall{Input: `{"output":{"a":"x"}}`})
	require.NoError(t, err)
	require.Equal(t, fantasy.NewTextResponse(finalizerAcknowledgment), resp)
	resp, err = r.Run(ctx, fantasy.ToolCall{Input: `{"output":{"a":987654321}}`})
	require.NoError(t, err)
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "json value does not match the schema: a invalid_type")

	// Every failure class is model-visible, bounded and never echoes values.
	const canary = "CANARY-7f3c"
	for _, input := range []string{
		`{"output":{"a":"` + canary + `","b":"` + canary + `"}}`, `{"output":{"a":987654321}}`, `{"x":"` + canary + `"}`,
		`{"output":"` + canary + `"`, `{"output":"` + canary + `","output":1}`, `{"output":"` + canary + `"} ` + canary,
		`{"output":"` + strings.Repeat(canary, 7000) + `"}`, strings.Repeat(canary, 8000), "\xff" + canary,
	} {
		resp, err := r.Run(ctx, fantasy.ToolCall{Input: input})
		require.NoError(t, err)
		require.True(t, resp.IsError)
		require.LessOrEqual(t, len(resp.Content), 1024)
		require.True(t, utf8.ValidString(resp.Content))
		require.NotContains(t, resp.Content, canary)
		require.NotContains(t, resp.Content, "987654321")
	}

	opts := fantasy.ProviderOptions{"openai": nil}
	r.SetProviderOptions(opts)
	require.Equal(t, opts, r.ProviderOptions())
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			resp, err := r.Run(ctx, fantasy.ToolCall{Input: `{"output":{"a":"x"}}`})
			assert.NoError(t, err)
			assert.False(t, resp.IsError)
			resp, err = r.Run(ctx, fantasy.ToolCall{Input: `{"output":{}}`})
			assert.NoError(t, err)
			assert.True(t, resp.IsError)
		})
	}
	wg.Wait()
}

func FuzzCheckFinalizerArguments(f *testing.F) {
	s := mustCompile(f, `{"type":["object","array","string","null"],"patternProperties":{"^k":{"type":"integer"}},"items":{"maxLength":3}}`)
	for _, seed := range []string{
		`{"output":null}`, `{"output":{"k1":1,"x":"y"}}`, `{"output":["abc","abcd"]}`, `{"output":"\u2028"}`,
		`{"output":1}`, `{"OUTPUT":1}`, `{"output":1,"output":2}`, `[]`, `{"output":{"k":1.5}}`,
	} {
		f.Add([]byte(seed))
	}
	r := s.FinalizerRunner()
	f.Fuzz(func(t *testing.T, raw []byte) {
		got, err := s.CheckFinalizerArguments(raw)
		resp, runErr := r.Run(context.Background(), fantasy.ToolCall{Input: string(raw)})
		require.NoError(t, runErr)
		require.Equal(t, err != nil, resp.IsError)
		require.LessOrEqual(t, len(resp.Content), 1024)
		if err != nil {
			return
		}
		var args struct {
			Output json.RawMessage `json:"output"`
		}
		require.NoError(t, json.Unmarshal(raw, &args))
		again, err := s.Validate(args.Output)
		require.NoError(t, err)
		require.Equal(t, got, again)
	})
}
