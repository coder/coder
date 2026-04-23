package responses //nolint:testpackage // tests unexported internals

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/fixtures"
	"github.com/coder/coder/v2/aibridge/utils"
)

func TestNewRequestPayload(t *testing.T) {
	t.Parallel()

	payloadWithWrongTypes := []byte(`{"model":123,"stream":"yes","input":42,"background":"nope"}`)
	tests := []struct {
		name       string
		raw        []byte
		want       []byte
		model      string
		stream     bool
		background bool
		err        string
	}{
		{
			name: "empty payload",
			raw:  nil,
			want: nil,
			err:  "empty request body",
		},
		{
			name: "invalid json",
			raw:  []byte(`{broken`),
			want: nil,
			err:  "invalid JSON payload",
		},
		{
			// RequestPayload just checks for JSON validity,
			// schema errors are not surfaced here and
			// the original body is preserved for upstream handling
			// similar to how reverse proxy would behave.
			name:       "wrong field types still wrap",
			raw:        payloadWithWrongTypes,
			want:       payloadWithWrongTypes,
			model:      "123",
			stream:     false,
			background: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			payload, err := NewRequestPayload(tc.raw)

			if tc.err != "" {
				require.ErrorContains(t, err, tc.err)
				assert.Nil(t, payload)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, payload)
			assert.EqualValues(t, tc.want, payload)
			assert.Equal(t, tc.model, payload.model())
			assert.Equal(t, tc.stream, payload.Stream())
			assert.Equal(t, tc.background, payload.background())
		})
	}
}

func TestCorrelatingToolCallID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		payload  []byte
		wantCall *string
	}{
		{
			name:    "no input items",
			payload: []byte(`{"model":"gpt-4o"}`),
		},
		{
			name:    "empty input array",
			payload: []byte(`{"model":"gpt-4o","input":[]}`),
		},
		{
			name:    "no function_call_output items",
			payload: []byte(`{"model":"gpt-4o","input":[{"role":"user","content":"hi"}]}`),
		},
		{
			name:     "single function_call_output",
			payload:  []byte(`{"model":"gpt-4o","input":[{"role":"user","content":"hi"},{"type":"function_call_output","call_id":"call_abc","output":"result"}]}`),
			wantCall: utils.PtrTo("call_abc"),
		},
		{
			name:     "multiple function_call_outputs returns last",
			payload:  []byte(`{"model":"gpt-4o","input":[{"type":"function_call_output","call_id":"call_first","output":"r1"},{"role":"user","content":"hi"},{"type":"function_call_output","call_id":"call_second","output":"r2"}]}`),
			wantCall: utils.PtrTo("call_second"),
		},
		{
			name:    "last input is not a tool result",
			payload: []byte(`{"model":"gpt-4o","input":[{"type":"function_call_output","call_id":"call_first","output":"r1"},{"role":"user","content":"hi"}]}`),
		},
		{
			name:    "missing call id",
			payload: []byte(`{"input":[{"type":"function_call_output","output":"ok"}]}`),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			callID := mustPayload(t, tc.payload).correlatingToolCallID()
			assert.Equal(t, tc.wantCall, callID)
		})
	}
}

func TestLastUserPrompt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		reqPayload []byte
		expect     string
		found      bool
		expectErr  string
	}{
		{
			name:       "no input",
			reqPayload: []byte(`{}`),
			found:      false,
		},
		{
			name:       "input null",
			reqPayload: []byte(`{"input": null}`),
			found:      false,
		},
		{
			name:       "empty input array",
			reqPayload: []byte(`{"input": []}`),
			found:      false,
		},
		{
			name:       "input empty string",
			reqPayload: []byte(`{"input": ""}`),
			expect:     "",
			found:      true,
		},
		{
			name:       "input array content empty string",
			reqPayload: []byte(`{"input": [{"role": "user", "content": ""}]}`),
			expect:     "",
			found:      true,
		},
		{
			name:       "input array content array empty string",
			reqPayload: []byte(`{"input": [ { "role": "user", "content": [{"type": "input_text", "text": ""}] } ] }`),
			expect:     "",
			found:      true,
		},
		{
			name:       "input array content array multiple inputs",
			reqPayload: []byte(`{"input": [ { "role": "user", "content": [{"type": "input_text", "text": "a"}, {"type": "input_text", "text": "b"}] } ] }`),
			expect:     "a\nb",
			found:      true,
		},
		{
			name:       "simple string input",
			reqPayload: fixtures.Request(t, fixtures.OaiResponsesBlockingSimple),
			expect:     "tell me a joke",
			found:      true,
		},
		{
			name:       "array single input string",
			reqPayload: fixtures.Request(t, fixtures.OaiResponsesBlockingSingleBuiltinTool),
			expect:     "Is 3 + 5 a prime number? Use the add function to calculate the sum.",
			found:      true,
		},
		{
			name:       "array multiple items content objects",
			reqPayload: fixtures.Request(t, fixtures.OaiResponsesStreamingCodex),
			expect:     "hello",
			found:      true,
		},
		{
			name:       "input integer",
			reqPayload: []byte(`{"input": 123}`),
			expectErr:  "unexpected input type",
		},
		{
			name:       "no user role",
			reqPayload: []byte(`{"input": [{"role": "assistant", "content": "hello"}]}`),
			found:      false,
		},
		{
			name:       "user with empty content array",
			reqPayload: []byte(`{"input": [{"role": "user", "content": []}]}`),
			found:      false,
		},
		{
			name:       "user content missing",
			reqPayload: []byte(`{"input": [{"role": "user"}]}`),
			found:      false,
		},
		{
			name:       "user content null",
			reqPayload: []byte(`{"input": [{"role": "user", "content": null}]}`),
			found:      false,
		},
		{
			name:       "input array integer",
			reqPayload: []byte(`{"input": [{"role": "user", "content": 123}]}`),
			expectErr:  "unexpected input content type",
		},
		{
			name:       "user with non input_text content",
			reqPayload: []byte(`{"input": [{"role": "user", "content": [{"type": "input_image", "url": "http://example.com/img.png"}]}]}`),
			found:      false,
		},
		{
			name:       "user content not last",
			reqPayload: []byte(`{"input": [ {"role": "user", "content":"input"}, {"role": "assistant", "content": "hello"} ]}`),
			found:      false,
		},
		{
			name:       "input array content array integer",
			reqPayload: []byte(`{"input": [ { "role": "user", "content": [{"type": "input_text", "text": 123}] } ] }`),
			found:      false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			prompt, promptFound, err := mustPayload(t, tc.reqPayload).lastUserPrompt(t.Context(), slog.Make())
			if tc.expectErr != "" {
				require.ErrorContains(t, err, tc.expectErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expect, prompt)
			require.Equal(t, tc.found, promptFound)
		})
	}
}

func TestInjectTools(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		raw       []byte
		injected  []responses.ToolUnionParam
		wantNames []string
		wantErr   string
		wantSame  bool
	}{
		{
			name:      "appends to existing tools",
			raw:       []byte(`{"model":"gpt-4o","input":"hello","tools":[{"type":"function","name":"existing"}]}`),
			injected:  []responses.ToolUnionParam{injectedFunctionTool("injected")},
			wantNames: []string{"existing", "injected"},
		},
		{
			name:      "adds tools when none exist",
			raw:       []byte(`{"model":"gpt-4o","input":"hello"}`),
			injected:  []responses.ToolUnionParam{injectedFunctionTool("injected")},
			wantNames: []string{"injected"},
		},
		{
			name:      "adds to empty tools array",
			raw:       []byte(`{"model":"gpt-4o","input":"hello","tools":[]}`),
			injected:  []responses.ToolUnionParam{injectedFunctionTool("injected")},
			wantNames: []string{"injected"},
		},
		{
			name: "appends multiple injected tools",
			raw:  []byte(`{"model":"gpt-4o","input":"hello","tools":[{"type":"function","name":"existing"}]}`),
			injected: []responses.ToolUnionParam{
				injectedFunctionTool("injected-one"),
				injectedFunctionTool("injected-two"),
			},
			wantNames: []string{"existing", "injected-one", "injected-two"},
		},
		{
			name:     "empty injected tools is no op",
			raw:      []byte(`{"model":"gpt-4o","input":"hello","tools":[{"type":"function","name":"existing"}]}`),
			wantSame: true,
		},
		{
			name:     "errors on unsupported tools shape",
			raw:      []byte(`{"model":"gpt-4o","input":"hello","tools":"bad"}`),
			injected: []responses.ToolUnionParam{injectedFunctionTool("injected")},
			wantErr:  "failed to get existing tools: unsupported 'tools' type: String",
			wantSame: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p := mustPayload(t, tc.raw)
			updated, err := p.injectTools(tc.injected)
			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}

			if tc.wantSame {
				require.EqualValues(t, tc.raw, updated)
			}
			for i, wantName := range tc.wantNames {
				path := fmt.Sprintf("tools.%d.name", i) // name of the i-th element in tools array
				require.Equal(t, wantName, gjson.GetBytes(updated, path).String())
			}
		})
	}
}

func TestDisableParallelToolCalls(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  []byte
	}{
		{
			name: "sets flag when not present",
			raw:  []byte(`{"model":"gpt-4o"}`),
		},
		{
			name: "overrides when already true",
			raw:  []byte(`{"model":"gpt-4o","parallel_tool_calls":true}`),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p := mustPayload(t, tc.raw)
			updated, err := p.disableParallelToolCalls()
			require.NoError(t, err)
			assert.False(t, gjson.GetBytes(updated, "parallel_tool_calls").Bool())
		})
	}
}

func TestAppendInputItems(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		raw       []byte
		items     []responses.ResponseInputItemUnionParam
		wantErr   string
		wantSame  bool
		wantPaths map[string]string
	}{
		{
			name:  "string input becomes user message",
			raw:   []byte(`{"model":"gpt-4o","input":"hello"}`),
			items: []responses.ResponseInputItemUnionParam{responses.ResponseInputItemParamOfFunctionCallOutput("call_123", "done")},
			wantPaths: map[string]string{
				"input.0.role":    "user",
				"input.0.content": "hello",
				"input.1.type":    "function_call_output",
				"input.1.call_id": "call_123",
			},
		},
		{
			name:  "array input is preserved and appended",
			raw:   []byte(`{"model":"gpt-4o","input":[{"role":"user","content":"hello"}]}`),
			items: []responses.ResponseInputItemUnionParam{responses.ResponseInputItemParamOfFunctionCallOutput("call_123", "done")},
			wantPaths: map[string]string{
				"input.0.content": "hello",
				"input.1.call_id": "call_123",
			},
		},
		{
			name:     "unsupported input shape errors during rewrite",
			raw:      []byte(`{"model":"gpt-4o","input":123}`),
			items:    []responses.ResponseInputItemUnionParam{responses.ResponseInputItemParamOfFunctionCallOutput("call_123", "done")},
			wantErr:  "failed to get existing 'input' items: unsupported 'input' type: Number",
			wantSame: true,
		},
		{
			name:  "missing input creates appended input",
			raw:   []byte(`{"model":"gpt-4o"}`),
			items: []responses.ResponseInputItemUnionParam{responses.ResponseInputItemParamOfFunctionCallOutput("call_123", "done")},
			wantPaths: map[string]string{
				"input.0.type":    "function_call_output",
				"input.0.call_id": "call_123",
			},
		},
		{
			name:  "null input creates appended input",
			raw:   []byte(`{"model":"gpt-4o","input":null}`),
			items: []responses.ResponseInputItemUnionParam{responses.ResponseInputItemParamOfFunctionCallOutput("call_123", "done")},
			wantPaths: map[string]string{
				"input.0.type":    "function_call_output",
				"input.0.call_id": "call_123",
			},
		},
		{
			name: "multiple output item types are appended in order",
			raw:  []byte(`{"model":"gpt-4o","input":[{"role":"user","content":"hello"}]}`),
			items: []responses.ResponseInputItemUnionParam{
				responses.ResponseInputItemParamOfCompaction("encrypted-content"),
				responses.ResponseInputItemParamOfOutputMessage([]responses.ResponseOutputMessageContentUnionParam{
					{
						OfOutputText: &responses.ResponseOutputTextParam{
							Annotations: []responses.ResponseOutputTextAnnotationUnionParam{},
							Text:        "assistant text",
						},
					},
				}, "msg_123", responses.ResponseOutputMessageStatusCompleted),
				responses.ResponseInputItemParamOfFileSearchCall("fs_123", []string{"hello"}, "completed"),
				responses.ResponseInputItemParamOfImageGenerationCall("img_123", "base64-image", "completed"),
			},
			wantPaths: map[string]string{
				"input.0.content":        "hello",
				"input.1.type":           "compaction",
				"input.2.type":           "message",
				"input.2.id":             "msg_123",
				"input.2.content.0.type": "output_text",
				"input.2.content.0.text": "assistant text",
				"input.3.type":           "file_search_call",
				"input.3.id":             "fs_123",
				"input.4.type":           "image_generation_call",
				"input.4.id":             "img_123",
			},
		},
		{
			name:     "empty appended items is no op",
			raw:      []byte(`{"model":"gpt-4o","input":"hello"}`),
			wantSame: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p := mustPayload(t, tc.raw)
			updated, err := p.appendInputItems(tc.items)

			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}

			if tc.wantSame {
				require.EqualValues(t, tc.raw, updated)
			}

			for path, want := range tc.wantPaths {
				require.Equal(t, want, gjson.GetBytes(updated, path).String())
			}
		})
	}
}

func TestChainedRewritesProduceValidJSON(t *testing.T) {
	t.Parallel()

	p := mustPayload(t, []byte(`{"model":"gpt-4o","input":"hello"}`))
	p, err := p.injectTools([]responses.ToolUnionParam{{
		OfFunction: &responses.FunctionToolParam{
			Name:        "tool_a",
			Description: openai.String("tool"),
			Strict:      openai.Bool(false),
			Parameters: map[string]any{
				"type": "object",
			},
		},
	}})
	require.NoError(t, err)
	p, err = p.disableParallelToolCalls()
	require.NoError(t, err)
	p, err = p.appendInputItems([]responses.ResponseInputItemUnionParam{
		responses.ResponseInputItemParamOfFunctionCallOutput("call_123", "done"),
	})
	require.NoError(t, err)

	assert.True(t, json.Valid(p), "chained rewrites should produce valid JSON")
	assert.Equal(t, "tool_a", gjson.GetBytes(p, "tools.0.name").String())
	assert.Equal(t, "call_123", gjson.GetBytes(p, "input.1.call_id").String())
	assert.False(t, gjson.GetBytes(p, "parallel_tool_calls").Bool())
}

func injectedFunctionTool(name string) responses.ToolUnionParam {
	return responses.ToolUnionParam{
		OfFunction: &responses.FunctionToolParam{
			Name:        name,
			Description: openai.String("tool"),
			Strict:      openai.Bool(false),
			Parameters: map[string]any{
				"type": "object",
			},
		},
	}
}

func mustPayload(t *testing.T, raw []byte) RequestPayload {
	t.Helper()

	payload, err := NewRequestPayload(raw)
	require.NoError(t, err)
	return payload
}
