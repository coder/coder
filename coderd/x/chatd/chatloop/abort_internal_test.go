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

	var published int
	_, err := ExecuteLocalTools(context.Background(), ExecuteLocalToolsOptions{
		Tools:       []fantasy.AgentTool{abortTool, okTool},
		ActiveTools: []string{"aborter", "sibling"},
		ToolCalls: []fantasy.ToolCallContent{
			{ToolCallID: "call-abort", ToolName: "aborter", Input: "{}"},
			{ToolCallID: "call-ok", ToolName: "sibling", Input: "{}"},
		},
		PublishMessagePart: func(codersdk.ChatMessageRole, codersdk.ChatMessagePart) {
			published++
		},
		Clock: quartz.NewReal(),
	})
	require.Error(t, err)
	var abortErr *chattool.AbortToolExecutionError
	require.ErrorAs(t, err, &abortErr)
	// The batch is poisoned: no results are published, including
	// the successful sibling's, so the retry re-runs the batch.
	require.Zero(t, published)
}
