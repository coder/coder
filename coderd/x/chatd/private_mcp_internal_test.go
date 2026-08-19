package chatd

import (
	"context"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
)

func TestAppendPrivateMCPToolsExistingToolsWinCollisions(t *testing.T) {
	t.Parallel()

	tool := func(name string) fantasy.AgentTool {
		return fantasy.NewAgentTool(name, name, func(context.Context, struct{}, fantasy.ToolCall) (fantasy.ToolResponse, error) {
			return fantasy.NewTextResponse("ok"), nil
		})
	}
	existing := tool("shared__echo")
	got := appendPrivateMCPTools(
		t.Context(),
		slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		[]fantasy.AgentTool{existing},
		[]fantasy.AgentTool{tool("shared__echo"), tool("private__echo")},
	)

	require.Len(t, got, 2)
	require.Same(t, existing, got[0])
	require.Equal(t, "private__echo", got[1].Info().Name)
}
