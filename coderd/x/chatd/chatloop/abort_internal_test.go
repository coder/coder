package chatloop

import (
	"context"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

func TestExecuteLocalToolsAbortError(t *testing.T) {
	t.Parallel()

	abortTool := fantasy.NewAgentTool(
		"aborter",
		"fails with an abort-class error",
		func(_ context.Context, _ struct{}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			return fantasy.ToolResponse{}, &chattool.AbortToolExecutionError{Err: xerrors.New("ledger unavailable")}
		},
	)
	okTool := fantasy.NewAgentTool(
		"sibling",
		"succeeds",
		func(_ context.Context, _ struct{}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			return fantasy.NewTextResponse("ok"), nil
		},
	)

	var published []codersdk.ChatMessagePart
	outcome, err := ExecuteLocalTools(context.Background(), ExecuteLocalToolsOptions{
		Tools:       []fantasy.AgentTool{abortTool, okTool},
		ActiveTools: []string{"aborter", "sibling"},
		ToolCalls: []fantasy.ToolCallContent{
			{ToolCallID: "call-abort", ToolName: "aborter", Input: "{}"},
			{ToolCallID: "call-ok", ToolName: "sibling", Input: "{}"},
		},
		PublishMessagePart: func(_ codersdk.ChatMessageRole, part codersdk.ChatMessagePart) {
			published = append(published, part)
		},
		Clock: quartz.NewReal(),
	})
	require.Error(t, err)
	var abortErr *chattool.AbortToolExecutionError
	require.ErrorAs(t, err, &abortErr)
	// The aborted call's synthetic result is dropped so it is
	// never committed, while the completed sibling's result
	// survives for the caller to persist: its side effect already
	// happened, and the retry must re-run only the aborted call.
	require.Len(t, outcome.Step.Content, 1)
	tr, ok := outcome.Step.Content[0].(fantasy.ToolResultContent)
	require.True(t, ok)
	require.Equal(t, "call-ok", tr.ToolCallID)
	require.NotEmpty(t, published)
	for _, part := range published {
		require.NotEqual(t, "call-abort", part.ToolCallID)
	}
}
