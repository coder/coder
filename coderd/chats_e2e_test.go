package coderd_test

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd/chats"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/toolsdk"
	"github.com/coder/coder/v2/testutil"
)

const (
	chatsE2EEnvVar = "CODER_CHATS_E2E"
	chatWait       = 2 * time.Minute
)

// TestChats_WorkspaceBash_E2E tests the full chat flow with workspace bash tool
// using an in-process agent. This validates:
// - Chat creation with workspace binding
// - LLM tool invocation (coder_workspace_bash)
// - Agent command execution
// - Tool result propagation
func TestChats_WorkspaceBash_E2E(t *testing.T) {
	if os.Getenv(chatsE2EEnvVar) == "" {
		t.Skipf("Skipping test as %s is not set", chatsE2EEnvVar)
	}
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		t.Skip("Skipping test as ANTHROPIC_API_KEY not set")
	}

	model := os.Getenv("ANTHROPIC_MODEL")
	if model == "" {
		model = string(anthropic.ModelClaudeOpus4_5)
		t.Logf("ANTHROPIC_MODEL not set; defaulting to %s", model)
	}

	ctx := testutil.Context(t, testutil.WaitLong)

	// Create the test server with API access.
	client, _, api := coderdtest.NewWithAPI(t, nil)
	user := coderdtest.CreateFirstUser(t, client)

	// Set up the chat runner with the workspace bash tool.
	api.ChatRunner = chats.NewRunner(chats.RunnerOptions{
		DB:         api.Database,
		Logger:     api.Logger,
		AccessURL:  api.AccessURL,
		HTTPClient: api.HTTPClient,
		Tools:      []toolsdk.GenericTool{toolsdk.WorkspaceBash.Generic()},
	})

	// Create a workspace with an agent using dbfake (no real provisioner needed).
	r := dbfake.WorkspaceBuild(t, api.Database, database.WorkspaceTable{
		OrganizationID: user.OrganizationID,
		OwnerID:        user.UserID,
	}).WithAgent().Do()

	// Start an in-process agent that connects to the test server.
	_ = agenttest.New(t, client.URL, r.AgentToken, func(o *agent.Options) {
		// Agent options can be customized here if needed.
	})

	// Wait for the agent to be ready.
	resources := coderdtest.NewWorkspaceAgentWaiter(t, client, r.Workspace.ID).Wait()
	require.Len(t, resources, 1, "expected one resource")
	require.Len(t, resources[0].Agents, 1, "expected one agent")

	// Create a chat bound to the workspace.
	chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
		Provider:    "anthropic",
		Model:       model,
		WorkspaceID: &r.Workspace.ID,
	})
	require.NoError(t, err)

	// Send a prompt asking the LLM to run a command in the workspace.
	prompt := fmt.Sprintf("Use the coder_workspace_bash tool to run 'echo pong' in the workspace named %q (use workspace=%s). Respond with only the command output.", r.Workspace.Name, r.Workspace.Name)
	resp, err := client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{Content: prompt})
	require.NoError(t, err)
	require.NotEmpty(t, resp.RunID)

	// Wait for the tool result to appear.
	require.Eventually(t, func() bool {
		msgs, err := client.ChatMessages(ctx, chat.ID)
		if err != nil {
			return false
		}
		for _, msg := range msgs {
			if msg.Role != "tool_result" {
				continue
			}
			var env chats.ToolResultEnvelope
			if err := json.Unmarshal(msg.Content, &env); err != nil {
				continue
			}
			if env.RunID != resp.RunID || env.ToolName != toolsdk.ToolNameWorkspaceBash {
				continue
			}
			if env.Error != "" {
				t.Logf("tool error: %s", env.Error)
				return false
			}
			var result toolsdk.WorkspaceBashResult
			if err := json.Unmarshal(env.Result, &result); err != nil {
				return false
			}
			t.Logf("tool result: exit_code=%d output=%q", result.ExitCode, result.Output)
			return result.ExitCode == 0 && strings.Contains(strings.ToLower(result.Output), "pong")
		}
		return false
	}, chatWait, testutil.IntervalSlow)
}
