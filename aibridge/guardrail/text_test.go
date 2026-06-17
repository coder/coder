package guardrail_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/guardrail"
)

func TestUserPromptTexts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want []guardrail.TextRef
	}{
		{
			name: "anthropic/openai last user string content",
			body: `{"messages":[{"role":"user","content":"first"},{"role":"assistant","content":"reply"},{"role":"user","content":"yo"}]}`,
			want: []guardrail.TextRef{{Pointer: "messages.2.content", Role: guardrail.RoleUser, Value: "yo"}},
		},
		{
			name: "last user content array uses last text block",
			body: `{"messages":[{"role":"user","content":[{"type":"text","text":"context"},{"type":"text","text":"prompt"}]}]}`,
			want: []guardrail.TextRef{{Pointer: "messages.0.content.1.text", Role: guardrail.RoleUser, Value: "prompt"}},
		},
		{
			name: "trailing system message is skipped to the user prompt",
			body: `{"messages":[{"role":"user","content":"my email is a@b.com"},{"role":"system","content":"# MCP instructions"}]}`,
			want: []guardrail.TextRef{{Pointer: "messages.0.content", Role: guardrail.RoleUser, Value: "my email is a@b.com"}},
		},
		{
			name: "trailing assistant message is skipped to the user prompt",
			body: `{"messages":[{"role":"user","content":"my email is a@b.com"},{"role":"assistant","content":"ok"}]}`,
			want: []guardrail.TextRef{{Pointer: "messages.0.content", Role: guardrail.RoleUser, Value: "my email is a@b.com"}},
		},
		{
			name: "scanning stops at most recent user turn",
			body: `{"messages":[{"role":"user","content":"a@b.com"},{"role":"assistant","content":"ok"},{"role":"user","content":"yo"}]}`,
			want: []guardrail.TextRef{{Pointer: "messages.2.content", Role: guardrail.RoleUser, Value: "yo"}},
		},
		{
			name: "text-less tool-result user turn falls back to prior user prompt",
			body: `{"messages":[{"role":"user","content":"a@b.com"},{"role":"user","content":[{"type":"tool_result","tool_use_id":"x"}]}]}`,
			want: []guardrail.TextRef{{Pointer: "messages.0.content", Role: guardrail.RoleUser, Value: "a@b.com"}},
		},
		{
			name: "anthropic system is ignored",
			body: `{"system":"contact a@b.com","messages":[{"role":"user","content":"hi"}]}`,
			want: []guardrail.TextRef{{Pointer: "messages.0.content", Role: guardrail.RoleUser, Value: "hi"}},
		},
		{
			name: "openai responses input string",
			body: `{"input":"my email is a@b.com"}`,
			want: []guardrail.TextRef{{Pointer: "input", Role: guardrail.RoleUser, Value: "my email is a@b.com"}},
		},
		{
			name: "openai responses input array last user item",
			body: `{"input":[{"role":"user","content":[{"type":"input_text","text":"prompt"}]}]}`,
			want: []guardrail.TextRef{{Pointer: "input.0.content.0.text", Role: guardrail.RoleUser, Value: "prompt"}},
		},
		{
			name: "openai responses skips trailing non-user input item",
			body: `{"input":[{"role":"user","content":"a@b.com"},{"role":"assistant","content":"ok"}]}`,
			want: []guardrail.TextRef{{Pointer: "input.0.content", Role: guardrail.RoleUser, Value: "a@b.com"}},
		},
		{
			name: "no messages",
			body: `{"model":"gpt-4"}`,
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, guardrail.UserPromptTexts([]byte(tt.body)))
		})
	}
}

func TestConversationTexts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want []guardrail.TextRef
	}{
		{
			name: "all roles in order including anthropic system",
			body: `{"system":"sys","messages":[{"role":"user","content":"hi"},{"role":"assistant","content":"yo"},{"role":"user","content":"bye"}]}`,
			want: []guardrail.TextRef{
				{Pointer: "system", Role: guardrail.RoleSystem, Value: "sys"},
				{Pointer: "messages.0.content", Role: guardrail.RoleUser, Value: "hi"},
				{Pointer: "messages.1.content", Role: guardrail.RoleAssistant, Value: "yo"},
				{Pointer: "messages.2.content", Role: guardrail.RoleUser, Value: "bye"},
			},
		},
		{
			name: "content array text blocks expanded",
			body: `{"messages":[{"role":"user","content":[{"type":"text","text":"a"},{"type":"text","text":"b"}]}]}`,
			want: []guardrail.TextRef{
				{Pointer: "messages.0.content.0.text", Role: guardrail.RoleUser, Value: "a"},
				{Pointer: "messages.0.content.1.text", Role: guardrail.RoleUser, Value: "b"},
			},
		},
		{
			name: "openai responses input string",
			body: `{"input":"hello"}`,
			want: []guardrail.TextRef{{Pointer: "input", Role: guardrail.RoleUser, Value: "hello"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, guardrail.ConversationTexts([]byte(tt.body)))
		})
	}
}
