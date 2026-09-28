package chatloop

import (
	"context"
	"sync"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestExecuteLocalToolsToolCallIdentity(t *testing.T) {
	t.Parallel()

	type seen struct {
		identity chattool.ToolCallIdentity
		ok       bool
	}
	// runBatch runs two parallel calls and one serial call and returns
	// the identity each read from its context, by tool call ID.
	runBatch := func(t *testing.T, batch chattool.ToolCallIdentity) map[string]seen {
		ctx := testutil.Context(t, testutil.WaitShort)
		var (
			mu  sync.Mutex
			got = map[string]seen{}
		)
		record := func(ctx context.Context, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			identity, ok := chattool.ToolCallIdentityFromContext(ctx)
			mu.Lock()
			got[call.ID] = seen{identity: identity, ok: ok}
			mu.Unlock()
			return fantasy.NewTextResponse("ok"), nil
		}
		parallel := fantasy.NewAgentTool("parallel_probe", "records its identity",
			func(ctx context.Context, _ struct{}, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
				return record(ctx, call)
			})
		serial := serialMarkerTool{fantasy.NewAgentTool("serial_probe", "records its identity",
			func(ctx context.Context, _ struct{}, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
				return record(ctx, call)
			})}
		_, err := ExecuteLocalTools(ctx, ExecuteLocalToolsOptions{
			Tools:       []fantasy.AgentTool{parallel, serial},
			ActiveTools: []string{"parallel_probe", "serial_probe"},
			ToolCalls: []fantasy.ToolCallContent{
				{ToolCallID: "call-1", ToolName: "parallel_probe", Input: "{}"},
				{ToolCallID: "call-2", ToolName: "parallel_probe", Input: "{}"},
				{ToolCallID: "call-3", ToolName: "serial_probe", Input: "{}"},
			},
			ToolCallIdentity: batch,
			Clock:            quartz.NewReal(),
		})
		require.NoError(t, err)
		return got
	}

	t.Run("EachCallSeesItsOwnIdentity", func(t *testing.T) {
		t.Parallel()

		chatID := uuid.New()
		got := runBatch(t, chattool.ToolCallIdentity{ChatID: chatID, MessageID: 42})
		identity := func(toolCallID, toolName string) seen {
			return seen{ok: true, identity: chattool.ToolCallIdentity{
				ChatID: chatID, MessageID: 42, ToolCallID: toolCallID, ToolName: toolName,
			}}
		}
		assert.Equal(t, map[string]seen{
			"call-1": identity("call-1", "parallel_probe"),
			"call-2": identity("call-2", "parallel_probe"),
			"call-3": identity("call-3", "serial_probe"),
		}, got)
	})

	t.Run("NoChatIDNoIdentity", func(t *testing.T) {
		t.Parallel()

		got := runBatch(t, chattool.ToolCallIdentity{})
		assert.Equal(t, map[string]seen{"call-1": {}, "call-2": {}, "call-3": {}}, got)
	})
}
