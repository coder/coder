package responses_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/extract"
	"github.com/coder/coder/v2/aibridge/extract/responses"
)

func TestRequestExtractor(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		body    string
		want    extract.RequestFacts
		wantErr bool
	}{
		{
			name: "string_input",
			body: `{"model":"gpt-5","stream":true,"input":"tell me a joke"}`,
			want: extract.RequestFacts{Model: "gpt-5", Streaming: true, Prompt: "tell me a joke"},
		},
		{
			name: "input_text_parts",
			body: `{"model":"gpt-5","input":[{"role":"assistant","content":"hi"},{"role":"user","content":[` +
				`{"type":"input_text","text":"first"},{"type":"input_image","image_url":"x"},{"type":"input_text","text":"second"}]}]}`,
			want: extract.RequestFacts{Model: "gpt-5", Prompt: "first\nsecond"},
		},
		{
			name: "string_content",
			body: `{"input":[{"role":"user","content":"hello"}]}`,
			want: extract.RequestFacts{Prompt: "hello"},
		},
		{
			// An agentic loop iteration answers a tool call: no prompt, but
			// the call ID correlates it with the previous interception.
			name: "function_call_output",
			body: `{"model":"gpt-5","input":[{"role":"user","content":"hi"},{"type":"function_call","call_id":"call_1"},` +
				`{"type":"function_call_output","call_id":"call_1","output":"8"}]}`,
			want: extract.RequestFacts{Model: "gpt-5", CorrelatingToolCallID: "call_1"},
		},
		{
			// A trailing function_call is not an answer to a tool call.
			name: "trailing_function_call",
			body: `{"input":[{"role":"user","content":"hi"},{"type":"function_call","call_id":"call_1"}]}`,
			want: extract.RequestFacts{},
		},
		{
			name:    "unexpected_input_type",
			body:    `{"model":"gpt-5","input":42}`,
			want:    extract.RequestFacts{Model: "gpt-5"},
			wantErr: true,
		},
		{
			name:    "invalid_json",
			body:    `{"model":`,
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := responses.RequestExtractor{}.ExtractRequest([]byte(tc.body))
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.want, got)
		})
	}
}
