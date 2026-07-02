package fakellm_test

import (
	"context"
	"encoding/json"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/fakellm"
)

// TestModel_ToolCallLoop drives the exact scripted conversation from the
// fakellm design discussion end-to-end through Model, without any real
// tool implementation: the test itself plays "the tool executor" by
// reading the scripted result back out via ResultFor and feeding it into
// the next Call's prompt — proving the whole thing is deterministic with
// zero real tool dispatch and zero wall-clock delay.
func TestModel_ToolCallLoop(t *testing.T) {
	t.Parallel()

	script := fakellm.MustParseString(`
		{"text": "let me check that"}
		{"think": "I need to check if the file exists. I'll use the execute tool"}
		{"tool_call": {"name": "execute", "args": {"command": "ls -l"}, "result": {"success": false, "output": "no such file or directory", "exit_code": 2}}}
		{"text": "nope it's not there. should I create it?"}
		{"tool_call": {"name": "user_choice", "args": {"options": ["yes", "no"]}, "result": {"choice": "yes"}}}
	`)
	model := fakellm.NewModel(script)
	ctx := context.Background()

	var prompt fantasy.Prompt
	prompt = append(prompt, fantasy.NewUserMessage("does /tmp/foo exist?"))

	// --- Turn 1 ---
	resp, err := model.Generate(ctx, fantasy.Call{Prompt: prompt})
	require.NoError(t, err)
	require.Equal(t, "let me check that", resp.Content.Text())
	require.Equal(t, "I need to check if the file exists. I'll use the execute tool", resp.Content.ReasoningText())
	require.Equal(t, fantasy.FinishReasonToolCalls, resp.FinishReason)

	toolCalls := resp.Content.ToolCalls()
	require.Len(t, toolCalls, 1)
	require.Equal(t, "execute", toolCalls[0].ToolName)
	require.JSONEq(t, `{"command":"ls -l"}`, toolCalls[0].Input)

	// The test plays "the tool" here: no real command runs. We just pull
	// the scripted result straight out of the model.
	result, ok := model.ResultFor(toolCalls[0].ToolCallID)
	require.True(t, ok)
	require.JSONEq(t, `{"success":false,"output":"no such file or directory","exit_code":2}`, string(result))

	prompt = append(prompt,
		fantasy.Message{Role: fantasy.MessageRoleAssistant, Content: nil}, // real code would append the actual assistant message; omitted for brevity
		toolResultMessage(t, toolCalls[0].ToolCallID, result),
	)

	// --- Turn 2 ---
	resp, err = model.Generate(ctx, fantasy.Call{Prompt: prompt})
	require.NoError(t, err)
	require.Equal(t, "nope it's not there. should I create it?", resp.Content.Text())
	require.Equal(t, fantasy.FinishReasonToolCalls, resp.FinishReason)

	toolCalls = resp.Content.ToolCalls()
	require.Len(t, toolCalls, 1)
	require.Equal(t, "user_choice", toolCalls[0].ToolName)

	result, ok = model.ResultFor(toolCalls[0].ToolCallID)
	require.True(t, ok)
	require.JSONEq(t, `{"choice":"yes"}`, string(result))

	// --- Turn 3: script exhausted, must fail loudly, not silently repeat. ---
	_, err = model.Generate(ctx, fantasy.Call{Prompt: prompt})
	require.ErrorContains(t, err, "script exhausted")
}

func TestModel_ScriptedError(t *testing.T) {
	t.Parallel()

	script := fakellm.MustParseString(`{"error": {"message": "rate limited"}}`)
	model := fakellm.NewModel(script)

	_, err := model.Generate(context.Background(), fantasy.Call{})
	require.ErrorContains(t, err, "rate limited")
}

func TestModel_Stream(t *testing.T) {
	t.Parallel()

	script := fakellm.MustParseString(`
		{"text": "hi"}
		{"tool_call": {"name": "noop", "args": {}, "result": {"ok": true}}}
	`)
	model := fakellm.NewModel(script)

	stream, err := model.Stream(context.Background(), fantasy.Call{})
	require.NoError(t, err)

	var types []fantasy.StreamPartType
	for part := range stream {
		types = append(types, part.Type)
	}
	require.Contains(t, types, fantasy.StreamPartTypeTextDelta)
	require.Contains(t, types, fantasy.StreamPartTypeToolCall)
	require.Equal(t, fantasy.StreamPartTypeFinish, types[len(types)-1])
}

func toolResultMessage(t *testing.T, toolCallID string, result json.RawMessage) fantasy.Message {
	t.Helper()
	return fantasy.Message{
		Role: fantasy.MessageRoleTool,
		Content: []fantasy.MessagePart{
			fantasy.ToolResultPart{
				ToolCallID: toolCallID,
				Output:     fantasy.ToolResultOutputContentText{Text: string(result)},
			},
		},
	}
}
