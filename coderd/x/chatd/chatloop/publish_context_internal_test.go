package chatloop

import (
	"context"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

func TestMessagePartPublisherContextRoundTrip(t *testing.T) {
	t.Parallel()

	require.Nil(t, MessagePartPublisherFromContext(context.Background()))

	var published []codersdk.ChatMessagePart
	publish := func(_ codersdk.ChatMessageRole, part codersdk.ChatMessagePart) {
		published = append(published, part)
	}
	ctx := WithMessagePartPublisher(context.Background(), publish)
	got := MessagePartPublisherFromContext(ctx)
	require.NotNil(t, got)
	got(codersdk.ChatMessageRoleTool, codersdk.ChatMessagePart{ToolCallID: "call-1"})
	require.Len(t, published, 1)
	require.Equal(t, "call-1", published[0].ToolCallID)

	// A nil publisher must not be stored.
	require.Nil(t, MessagePartPublisherFromContext(WithMessagePartPublisher(context.Background(), nil)))
}

func TestExecuteLocalToolsInjectsMessagePartPublisher(t *testing.T) {
	t.Parallel()

	var toolSawPublisher bool
	tool := fantasy.NewAgentTool(
		"probe",
		"reports whether the execution context carries a publisher",
		func(ctx context.Context, _ struct{}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			toolSawPublisher = MessagePartPublisherFromContext(ctx) != nil
			return fantasy.NewTextResponse("ok"), nil
		},
	)

	_, err := ExecuteLocalTools(context.Background(), ExecuteLocalToolsOptions{
		Tools:       []fantasy.AgentTool{tool},
		ActiveTools: []string{"probe"},
		ToolCalls: []fantasy.ToolCallContent{{
			ToolCallID: "call-1",
			ToolName:   "probe",
			Input:      "{}",
		}},
		PublishMessagePart: func(codersdk.ChatMessageRole, codersdk.ChatMessagePart) {},
		Clock:              quartz.NewReal(),
	})
	require.NoError(t, err)
	require.True(t, toolSawPublisher)
}
