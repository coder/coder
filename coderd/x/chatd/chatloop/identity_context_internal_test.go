package chatloop

import (
	"context"
	"sync"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/quartz"
)

func TestExecuteLocalToolsInjectsToolCallIdentity(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	var (
		mu   sync.Mutex
		seen = map[string]chattool.ToolCallIdentity{}
	)
	probe := fantasy.NewAgentTool(
		"probe",
		"records the identity the dispatcher supplied",
		func(ctx context.Context, _ struct{}, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			// Calls run on their own goroutines, so record and assert
			// on the test goroutine instead.
			identity, _ := chattool.ToolCallIdentityFromContext(ctx)
			mu.Lock()
			seen[call.ID] = identity
			mu.Unlock()
			return fantasy.NewTextResponse("ok"), nil
		},
	)

	_, err := ExecuteLocalTools(context.Background(), ExecuteLocalToolsOptions{
		Tools:       []fantasy.AgentTool{probe},
		ActiveTools: []string{"probe"},
		ToolCalls: []fantasy.ToolCallContent{
			{ToolCallID: "call-1", ToolName: "probe", Input: "{}"},
			{ToolCallID: "call-2", ToolName: "probe", Input: "{}"},
		},
		ChatID:             chatID,
		AssistantMessageID: 42,
		Clock:              quartz.NewReal(),
	})
	require.NoError(t, err)

	// Each concurrently dispatched call must see its own ID, not a
	// sibling's, or two calls would derive one idempotency key.
	require.Equal(t, map[string]chattool.ToolCallIdentity{
		"call-1": {ChatID: chatID, AssistantMessageID: 42, ToolCallID: "call-1"},
		"call-2": {ChatID: chatID, AssistantMessageID: 42, ToolCallID: "call-2"},
	}, seen)
}

func TestExecuteLocalToolsWithoutDispatchIdentity(t *testing.T) {
	t.Parallel()

	identified := make(chan bool, 1)
	probe := fantasy.NewAgentTool(
		"probe",
		"reports whether the execution context identifies the call",
		func(ctx context.Context, _ struct{}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			_, ok := chattool.ToolCallIdentityFromContext(ctx)
			identified <- ok
			return fantasy.NewTextResponse("ok"), nil
		},
	)

	_, err := ExecuteLocalTools(context.Background(), ExecuteLocalToolsOptions{
		Tools:       []fantasy.AgentTool{probe},
		ActiveTools: []string{"probe"},
		ToolCalls:   []fantasy.ToolCallContent{{ToolCallID: "call-1", ToolName: "probe", Input: "{}"}},
		Clock:       quartz.NewReal(),
	})
	require.NoError(t, err)

	// A partial identity must not resolve: it would key distinct calls
	// from different chats onto the same reservation.
	require.False(t, <-identified)
}
