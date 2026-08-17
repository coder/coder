package chatd

import (
	"context"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
)

type deferredTestAgentTool struct {
	info fantasy.ToolInfo
}

func (t deferredTestAgentTool) Info() fantasy.ToolInfo                   { return t.info }
func (deferredTestAgentTool) ProviderOptions() fantasy.ProviderOptions   { return nil }
func (deferredTestAgentTool) SetProviderOptions(fantasy.ProviderOptions) {}
func (deferredTestAgentTool) Run(context.Context, fantasy.ToolCall) (fantasy.ToolResponse, error) {
	return fantasy.NewTextResponse("ok"), nil
}

func testDeferredTool(name, description string, parameters map[string]any) deferredMCPTool {
	return deferredMCPTool{tool: deferredTestAgentTool{info: fantasy.ToolInfo{
		Name: name, Description: description, Parameters: parameters,
	}}}
}

func TestDecideMCPToolSearch(t *testing.T) {
	t.Parallel()
	small := []deferredMCPTool{testDeferredTool("server__small", "small", map[string]any{"value": map[string]any{"type": "string"}})}
	large := []deferredMCPTool{testDeferredTool("server__large", strings.Repeat("large ", 2000), map[string]any{"value": map[string]any{"type": "string"}})}

	tests := []struct {
		name       string
		experiment bool
		force      bool
		window     int64
		candidates []deferredMCPTool
		want       bool
	}{
		{name: "below", experiment: true, window: 100_000, candidates: small},
		{name: "above", experiment: true, window: 10_000, candidates: large, want: true},
		{name: "forced", experiment: true, force: true, window: 100_000, candidates: small, want: true},
		{name: "experiment off", force: true, window: 10, candidates: large},
		{name: "empty", experiment: true, force: true},
		{name: "collision", experiment: true, force: true, candidates: []deferredMCPTool{testDeferredTool(chattool.FindToolsName, "collision", nil)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, decideMCPToolSearch(mcpToolSearchInput{
				experimentEnabled: tt.experiment,
				forceDefer:        tt.force,
				contextWindow:     tt.window,
				candidates:        tt.candidates,
			}).apply)
		})
	}
}

func TestDeriveDeferredMCPActivations(t *testing.T) {
	t.Parallel()
	candidates := []deferredMCPTool{
		testDeferredTool("server__first", "first", nil),
		testDeferredTool("server__second", "second", nil),
	}
	findResult, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageToolResult("call-1", chattool.FindToolsName, []byte(`{"activated":["server__second","disconnected"]}`), false, false),
	})
	require.NoError(t, err)
	directCall, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageToolCall("call-2", "server__first", []byte(`{}`)),
	})
	require.NoError(t, err)
	malformed, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageToolResult("call-3", chattool.FindToolsName, []byte(`"not-json"`), false, false),
	})
	require.NoError(t, err)
	rows := []database.ChatMessage{
		{Role: database.ChatMessageRoleTool, Content: findResult, ContentVersion: chatprompt.CurrentContentVersion},
		{Role: database.ChatMessageRoleAssistant, Content: directCall, ContentVersion: chatprompt.CurrentContentVersion},
		{Role: database.ChatMessageRoleTool, Content: malformed, ContentVersion: chatprompt.CurrentContentVersion},
	}
	require.Equal(t, []string{"server__second", "server__first"}, deriveDeferredMCPActivations(rows, candidates))
}

func TestFlattenMCPParameterText(t *testing.T) {
	t.Parallel()
	text := flattenMCPParameterText(map[string]any{
		"repository": map[string]any{"type": "string", "description": "Repository name"},
	})
	require.Contains(t, text, "repository")
	require.Contains(t, text, "Repository name")
}
