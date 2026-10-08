package chatloop

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

func TestProjectedResultTruncationAndBilling(t *testing.T) {
	t.Parallel()
	clock := quartz.NewMock(t)
	userText := strings.Repeat("reasoning", 10000)
	tool := fantasy.NewAgentTool("acp_wait_agent", "", func(ctx context.Context, _ struct{}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
		clock.Advance(2 * time.Second).MustWait(ctx)
		return chattool.WithUserResult(fantasy.NewTextResponse(strings.Repeat("answer", 10000)), map[string]string{"output": userText}), nil
	})
	var user json.RawMessage
	count := 0
	result, err := ExecuteLocalTools(context.Background(), ExecuteLocalToolsOptions{Clock: clock, ContextLimit: 1000, Tools: []fantasy.AgentTool{tool}, ActiveTools: []string{"acp_wait_agent"}, ToolCalls: []fantasy.ToolCallContent{{ToolCallID: "wait", ToolName: "acp_wait_agent", Input: "{}"}}, PublishMessagePart: func(_ codersdk.ChatMessageRole, part codersdk.ChatMessagePart) {
		user = part.Result
		count++
	}})
	require.NoError(t, err)
	require.Equal(t, 1, count)
	require.Contains(t, string(user), userText)
	require.Len(t, result.Content, 1)
	model := result.Content[0].(fantasy.ToolResultContent).Result.(fantasy.ToolResultOutputContentText).Text
	require.Less(t, len(model), 60000)
	require.Equal(t, 2*time.Second, result.BatchRuntime)
	require.Equal(t, 1, result.BatchBilledCalls)
}
