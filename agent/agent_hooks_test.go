package agent_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agenthooks"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

// TestAgent_WorkspaceHookDeniesExecute drives the chat execute tool
// against a real agent whose workspace declares a pre_tool_use hook.
// PROTOTYPE (CODAGT-1083): this is the end-to-end deny path minus
// coderd's tool loop.
func TestAgent_WorkspaceHookDeniesExecute(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("hooks run under sh")
	}

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".coder"), 0o755))
	hooks, err := json.Marshal(agenthooks.File{
		Version: 1,
		Hooks: []agenthooks.Hook{{
			Name:    "block-no-verify",
			Event:   "pre_tool_use",
			Matcher: "execute",
			Command: "./block-no-verify.sh",
		}},
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".coder", "hooks.json"), hooks, 0o600))
	//nolint:gosec // hook scripts must be executable
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".coder", "block-no-verify.sh"), []byte(`#!/bin/sh
input=$(cat)
if printf '%s' "$input" | grep -q -- '--no-verify'; then
  printf '%s' '{"permission":{"decision":"deny","reason":"git hooks are mandatory in this repository"}}'
fi
`), 0o700))

	//nolint:dogsled
	conn, _, _, _, _ := setupAgent(t, agentsdk.Manifest{Directory: dir}, 0,
		func(_ *agenttest.Client, o *agent.Options) {
			// The hooks file and script live on disk, not in the
			// in-memory filesystem the test harness defaults to.
			o.Filesystem = afero.NewOsFs()
			o.WorkspaceHooksFile = agenthooks.DefaultFileName
		})
	conn.SetExtraHeaders(http.Header{
		workspacesdk.CoderChatIDHeader: {uuid.New().String()},
	})

	tool := chattool.Execute(chattool.ExecuteOptions{
		GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) { return conn, nil },
	})
	run := func(command string) chattool.ExecuteResult {
		input, err := json.Marshal(map[string]any{"command": command})
		require.NoError(t, err)
		ctx := testutil.Context(t, testutil.WaitLong)
		resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "call-1", Name: "execute", Input: string(input)})
		require.NoError(t, err)
		var result chattool.ExecuteResult
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result), resp.Content)
		return result
	}

	denied := run("git commit --no-verify -m x")
	require.False(t, denied.Success)
	require.Contains(t, denied.Error, `blocked by a workspace hook ("block-no-verify")`)
	require.Contains(t, denied.Error, "git hooks are mandatory")
	require.Len(t, denied.Hooks, 1)
	require.Equal(t, "deny", denied.Hooks[0].Decision)
	t.Logf("model sees: %s", denied.Error)

	allowed := run("echo ok")
	require.True(t, allowed.Success, allowed.Error)
	require.Contains(t, allowed.Output, "ok")
	require.Len(t, allowed.Hooks, 1)
	require.Equal(t, "allow", allowed.Hooks[0].Decision)
	t.Logf("hook overhead on allowed call: %dms of %dms wall", allowed.Hooks[0].DurationMs, allowed.WallDurationMs)
}
