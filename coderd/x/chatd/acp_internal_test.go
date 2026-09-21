package chatd

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestACPToolSchemas(t *testing.T) {
	t.Parallel()
	tools := (&Server{}).acpTools(rootChatToolsOptions{})
	require.Len(t, tools, 5)
	for _, tool := range tools {
		info := tool.Info()
		require.Contains(t, []string{"acp_spawn_agent", "acp_message_agent", "acp_interrupt_agent", "acp_wait_agent", "acp_list_agents"}, info.Name)
		if info.Name == "acp_spawn_agent" {
			agent, ok := info.Parameters["agent"].(map[string]any)
			require.True(t, ok)
			require.Equal(t, []any{"claude_code", "codex"}, agent["enum"])
			require.Contains(t, info.Required, "agent")
			require.NotContains(t, info.Parameters, "type")
		} else if info.Name != "acp_list_agents" {
			require.Contains(t, info.Required, "session_id")
			require.Contains(t, info.Required, "workspace_agent_id")
		}
	}
}

func TestACPToolResponseRetainsCurrentTurnTools(t *testing.T) {
	t.Parallel()
	entries := []codersdk.ACPEntry{
		{ID: "old-prompt", Role: "user", Kind: "text", Text: "Previous task"},
		{ID: "old-tool", Role: "assistant", Kind: "tool", Title: "Old tool", Status: "completed"},
		{ID: "prompt", Role: "user", Kind: "text", Text: "Run tests"},
		{ID: "text", Role: "assistant", Kind: "text", Text: "Checking tests."},
		{ID: "tool", Role: "assistant", Kind: "tool", Title: "Run tests", Status: "completed", Input: map[string]any{"command": "go test ./..."}, Output: "PASS"},
		{ID: "done", Role: "assistant", Kind: "text", Text: "All tests pass."},
	}
	raw, err := json.Marshal(codersdk.ACPSession{Entries: entries})
	require.NoError(t, err)
	response := acpToolResponse(raw, uuid.New(), nil)
	require.False(t, response.IsError)
	var output struct {
		Entries []codersdk.ACPEntry `json:"entries"`
		Report  string              `json:"report"`
	}
	require.NoError(t, json.Unmarshal([]byte(response.Content), &output))
	require.Equal(t, entries[3:], output.Entries)
	require.Equal(t, "Checking tests.All tests pass.", output.Report)
}
