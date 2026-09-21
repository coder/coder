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

// hookedAgent starts a real agent whose workspace directory declares
// the given hooks and returns the chat tools wired to it.
// PROTOTYPE (CODAGT-1083): this is the end-to-end path minus coderd's
// tool loop.
func hookedAgent(t *testing.T, hooks []agenthooks.Hook, scripts map[string]string) (dir string, conn workspacesdk.AgentConn) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("hooks run under sh")
	}
	dir = t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".coder"), 0o755))
	file, err := json.Marshal(agenthooks.File{Version: 1, Hooks: hooks})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".coder", "hooks.json"), file, 0o600))
	for name, body := range scripts {
		//nolint:gosec // hook scripts must be executable
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".coder", name), []byte(body), 0o700))
	}

	//nolint:dogsled
	agentConn, _, _, _, _ := setupAgent(t, agentsdk.Manifest{Directory: dir}, 0,
		func(_ *agenttest.Client, o *agent.Options) {
			// The hooks file and script live on disk, not in the
			// in-memory filesystem the test harness defaults to.
			o.Filesystem = afero.NewOsFs()
			o.WorkspaceHooksFile = agenthooks.DefaultFileName
		})
	agentConn.SetExtraHeaders(http.Header{
		workspacesdk.CoderChatIDHeader: {uuid.New().String()},
	})
	return dir, agentConn
}

func runTool(t *testing.T, tool fantasy.AgentTool, input map[string]any) fantasy.ToolResponse {
	t.Helper()
	data, err := json.Marshal(input)
	require.NoError(t, err)
	ctx := testutil.Context(t, testutil.WaitLong)
	resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "toolu_" + uuid.NewString(), Name: tool.Info().Name, Input: string(data)})
	require.NoError(t, err)
	return resp
}

func hooksOf(t *testing.T, resp fantasy.ToolResponse) []workspacesdk.HookDecision {
	t.Helper()
	hooks, err := chattool.WorkspaceHooksFromMetadata(resp.Metadata)
	require.NoError(t, err)
	return hooks
}

func TestAgent_WorkspaceHookDeniesExecute(t *testing.T) {
	t.Parallel()
	_, conn := hookedAgent(t, []agenthooks.Hook{{
		Name: "block-no-verify", Event: "pre_tool_use", Matcher: "execute", Command: "./block-no-verify.sh",
	}}, map[string]string{"block-no-verify.sh": `#!/bin/sh
input=$(cat)
if printf '%s' "$input" | grep -q -- '--no-verify'; then
  printf '%s' '{"permission":{"decision":"deny","reason":"git hooks are mandatory in this repository"},"model_context":"Fix the failing hook instead of bypassing it."}'
fi
`})
	tool := chattool.Execute(chattool.ExecuteOptions{
		GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) { return conn, nil },
	})
	run := func(command string) (chattool.ExecuteResult, fantasy.ToolResponse) {
		resp := runTool(t, tool, map[string]any{"command": command})
		var result chattool.ExecuteResult
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result), resp.Content)
		return result, resp
	}

	denied, resp := run("git push --no-verify")
	require.False(t, denied.Success)
	require.Contains(t, denied.Error, `blocked by a workspace hook ("block-no-verify")`)
	require.Contains(t, denied.Error, "git hooks are mandatory")
	require.NotContains(t, resp.Content, "Fix the failing hook", "model_context must not ride in model-visible text")
	hooks := hooksOf(t, resp)
	require.Len(t, hooks, 1)
	require.Equal(t, "deny", hooks[0].Decision)
	require.Equal(t, "Fix the failing hook instead of bypassing it.", hooks[0].ModelContext)
	t.Logf("model sees: %s", denied.Error)

	allowed, resp := run("echo ok")
	require.True(t, allowed.Success, allowed.Error)
	require.Contains(t, allowed.Output, "ok")
	hooks = hooksOf(t, resp)
	require.Len(t, hooks, 1)
	require.Equal(t, "allow", hooks[0].Decision)
	t.Logf("hook overhead on allowed call: %dms of %dms wall", hooks[0].DurationMs, allowed.WallDurationMs)
}

func TestAgent_WorkspaceHookRewritesExecute(t *testing.T) {
	t.Parallel()
	_, conn := hookedAgent(t, []agenthooks.Hook{{
		Name: "prefer-pnpm", Event: "pre_tool_use", Matcher: "execute", Command: "./prefer-pnpm.sh",
	}}, map[string]string{"prefer-pnpm.sh": `#!/bin/sh
input=$(cat)
case "$input" in
  *'npm install'*) printf '%s' '{"permission":{"decision":"allow","input_override":{"command":"echo pnpm install"}},"user_message":"Rewrote npm install to pnpm install."}' ;;
esac
`})
	tool := chattool.Execute(chattool.ExecuteOptions{
		GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) { return conn, nil },
	})
	resp := runTool(t, tool, map[string]any{"command": "echo npm install"})
	var result chattool.ExecuteResult
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result), resp.Content)
	require.True(t, result.Success, result.Error)
	require.Equal(t, "pnpm install\n", result.Output, "the rewritten command must run")
	hooks := hooksOf(t, resp)
	require.Len(t, hooks, 1)
	require.JSONEq(t, `{"command":"echo pnpm install"}`, string(hooks[0].InputOverride))
	require.Equal(t, "Rewrote npm install to pnpm install.", hooks[0].UserMessage)
}

func TestAgent_WorkspaceHookInjectsContext(t *testing.T) {
	t.Parallel()
	dir, conn := hookedAgent(t, []agenthooks.Hook{{
		Name: "sql-reminder", Event: "pre_tool_use", Matcher: "edit_files", Command: "./sql-reminder.sh",
	}}, map[string]string{"sql-reminder.sh": `#!/bin/sh
input=$(cat)
case "$input" in
  *'.sql'*) printf '%s' '{"model_context":"Repository note: run make gen after editing SQL queries."}' ;;
esac
`})
	target := filepath.Join(dir, "query.sql")
	require.NoError(t, os.WriteFile(target, []byte("SELECT 1;\n"), 0o600))
	tool := chattool.EditFiles(chattool.EditFilesOptions{
		GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) { return conn, nil },
	})
	resp := runTool(t, tool, map[string]any{"files": []map[string]any{{
		"path":  target,
		"edits": []map[string]any{{"old_text": "SELECT 1;", "new_text": "SELECT 2;"}},
	}}})
	require.False(t, resp.IsError, resp.Content)
	data, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "SELECT 2;\n", string(data))
	require.NotContains(t, resp.Content, "Repository note", "model_context must not ride in model-visible text")
	hooks := hooksOf(t, resp)
	require.Len(t, hooks, 1)
	require.Equal(t, "Repository note: run make gen after editing SQL queries.", hooks[0].ModelContext)
}
