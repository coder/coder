package extractors

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/recorder"
)

func TestMessagesExtractRequest(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name           string
		body           string
		wantErr        string
		wantModel      string
		wantToolCallID *string
		wantPrompt     *string
		wantStreaming  bool
	}{
		{
			name:           "AllFields",
			body:           `{"model":"claude-test","stream":true,"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"ok"},{"type":"text","text":"hi"}]}]}`,
			wantModel:      "claude-test",
			wantToolCallID: new("toolu_1"),
			wantPrompt:     new("hi"),
			wantStreaming:  true,
		},
		{
			name:      "NoPrompt",
			body:      `{"model":"claude-test","messages":[]}`,
			wantModel: "claude-test",
		},
		{
			name:       "EmptyPrompt",
			body:       `{"model":"claude-test","messages":[{"role":"user","content":""}]}`,
			wantModel:  "claude-test",
			wantPrompt: new(""),
		},
		{
			name:      "PromptError",
			body:      `{"model":"claude-test","messages":{}}`,
			wantErr:   "read last user prompt",
			wantModel: "claude-test",
		},
		{
			name:    "InvalidBody",
			body:    `not JSON`,
			wantErr: "parse messages request",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			start := &recorder.InterceptionRecord{}
			e, ok := NewMessages().(*messagesExtractor)
			require.True(t, ok)

			err := e.ExtractRequest([]byte(tc.body), start)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.wantModel, start.Model)
			require.Equal(t, tc.wantToolCallID, start.CorrelatingToolCallID)
			require.Equal(t, tc.wantPrompt, e.prompt)
			require.Equal(t, tc.wantStreaming, e.streaming)
		})
	}
}
