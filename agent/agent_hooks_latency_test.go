package agent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"slices"
	"strings"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agenthooks"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

// TestAgent_WorkspaceHookLatency measures the execute tool's wall time
// through a real agent with no hooks file, one allow hook, and three
// sequential allow hooks, and reports the ratios. It runs only when
// CODER_TEST_HOOK_LATENCY is set because it is a measurement, not a
// correctness check. PROTOTYPE (CODAGT-1083).
//
//	CODER_TEST_HOOK_LATENCY=1 go test ./agent/ -run TestAgent_WorkspaceHookLatency -count=1 -v
func TestAgent_WorkspaceHookLatency(t *testing.T) {
	t.Parallel()
	if os.Getenv("CODER_TEST_HOOK_LATENCY") == "" {
		t.Skip("set CODER_TEST_HOOK_LATENCY=1 to run the latency measurement")
	}
	const iterations = 40

	// allowHook is the cheapest realistic hook: read stdin, emit an
	// allow decision. Anything a user writes will be at least this.
	const allowHook = "#!/bin/sh\ncat >/dev/null\nprintf '%s' '{\"permission\":{\"decision\":\"allow\"}}'\n"

	measure := func(t *testing.T, hooks []agenthooks.Hook, scripts map[string]string) (wall []time.Duration, hook []time.Duration) {
		_, conn := hookedAgent(t, hooks, scripts)
		tool := chattool.Execute(chattool.ExecuteOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) { return conn, nil },
		})
		// Warm up the agent, shell, and connection once before timing.
		runTool(t, tool, map[string]any{"command": "true"})
		for i := 0; i < iterations; i++ {
			start := time.Now()
			resp := runTool(t, tool, map[string]any{"command": "true"})
			wall = append(wall, time.Since(start))
			var result chattool.ExecuteResult
			require.NoError(t, json.Unmarshal([]byte(resp.Content), &result), resp.Content)
			require.True(t, result.Success, result.Error)
			var sum time.Duration
			for _, d := range hooksOf(t, resp) {
				sum += time.Duration(d.DurationMs) * time.Millisecond
			}
			hook = append(hook, sum)
		}
		return wall, hook
	}

	var (
		oneHookFile   = []agenthooks.Hook{{Name: "allow-1", Event: "pre_tool_use", Matcher: "execute", Command: "./allow.sh"}}
		threeHookFile = []agenthooks.Hook{
			{Name: "allow-1", Event: "pre_tool_use", Matcher: "execute", Command: "./allow.sh"},
			{Name: "allow-2", Event: "pre_tool_use", Matcher: "execute", Command: "./allow.sh"},
			{Name: "allow-3", Event: "pre_tool_use", Matcher: "execute", Command: "./allow.sh"},
		}
		scripts = map[string]string{"allow.sh": allowHook}
	)

	// A hooks file that declares nothing separates "hooks enabled and
	// file read" from "hook process spawned".
	baseWall, _ := measure(t, nil, nil)
	emptyWall, _ := measure(t, []agenthooks.Hook{}, nil)
	oneWall, oneHook := measure(t, oneHookFile, scripts)
	threeWall, threeHook := measure(t, threeHookFile, scripts)

	t.Log(latencyReport(fmt.Sprintf("execute(\"true\") through a real agent, %d iterations each", iterations), baseWall, emptyWall, oneWall, threeWall, oneHook, threeHook))
}

type summary struct{ p50, p90, max time.Duration }

func summarize(samples []time.Duration) summary {
	s := slices.Clone(samples)
	slices.Sort(s)
	return summary{
		p50: s[len(s)/2],
		p90: s[len(s)*9/10],
		max: s[len(s)-1],
	}
}

func ms(d time.Duration) string { return fmt.Sprintf("%.1fms", float64(d)/float64(time.Millisecond)) }

func ratio(a, b time.Duration) float64 { return float64(a) / float64(b) }

// latencyReport renders the four variants as a table with p50 ratios
// against the no-hooks baseline.
func latencyReport(title string, baseWall, emptyWall, oneWall, threeWall, oneHook, threeHook []time.Duration) string {
	base := summarize(baseWall)
	empty := summarize(emptyWall)
	one := summarize(oneWall)
	three := summarize(threeWall)
	var sb strings.Builder
	line := func(format string, args ...any) { _, _ = fmt.Fprintf(&sb, format, args...) }
	line("\n%s\n", title)
	line("%-22s %8s %8s %8s %10s\n", "variant", "p50", "p90", "max", "p50 ratio")
	line("%-22s %8s %8s %8s %10s\n", "no hooks file", ms(base.p50), ms(base.p90), ms(base.max), "1.00x")
	line("%-22s %8s %8s %8s %9.2fx\n", "empty hooks file", ms(empty.p50), ms(empty.p90), ms(empty.max), ratio(empty.p50, base.p50))
	line("%-22s %8s %8s %8s %9.2fx\n", "1 allow hook", ms(one.p50), ms(one.p90), ms(one.max), ratio(one.p50, base.p50))
	line("%-22s %8s %8s %8s %9.2fx\n", "3 sequential hooks", ms(three.p50), ms(three.p90), ms(three.max), ratio(three.p50, base.p50))
	line("hook process time reported by the agent (p50): 1 hook %s, 3 hooks %s\n", ms(summarize(oneHook).p50), ms(summarize(threeHook).p50))
	line("added wall time per hook (p50): %s\n", ms((three.p50-one.p50)/2))
	return sb.String()
}

// TestAgent_WorkspaceHookLatencyDeployment runs the same measurement
// against a workspace on a running deployment, so the baseline includes
// the coderd to agent network path a chat tool call really pays. The
// workspace agent must run with CODER_AGENT_EXP_WORKSPACE_HOOKS_FILE
// set. The test writes and removes the hooks file inside the workspace
// through the agent API. PROTOTYPE (CODAGT-1083).
//
//	CODER_TEST_HOOK_LATENCY_URL=http://127.0.0.1:3000 \
//	CODER_TEST_HOOK_LATENCY_TOKEN=$(cat ~/.config/coderv2/session) \
//	CODER_TEST_HOOK_LATENCY_WORKSPACE=owner/name \
//	go test ./agent/ -run TestAgent_WorkspaceHookLatencyDeployment -count=1 -v
func TestAgent_WorkspaceHookLatencyDeployment(t *testing.T) {
	t.Parallel()
	accessURL := os.Getenv("CODER_TEST_HOOK_LATENCY_URL")
	token := os.Getenv("CODER_TEST_HOOK_LATENCY_TOKEN")
	workspace := os.Getenv("CODER_TEST_HOOK_LATENCY_WORKSPACE")
	if accessURL == "" || token == "" || workspace == "" {
		t.Skip("set CODER_TEST_HOOK_LATENCY_URL, _TOKEN, and _WORKSPACE (owner/name) to run against a deployment")
	}
	const iterations = 40
	ctx := testutil.Context(t, 10*time.Minute)

	parsed, err := url.Parse(accessURL)
	require.NoError(t, err)
	client := codersdk.New(parsed)
	client.SetSessionToken(token)
	t.Cleanup(client.HTTPClient.CloseIdleConnections)
	owner, name, ok := strings.Cut(workspace, "/")
	require.True(t, ok, "workspace must be owner/name")
	ws, err := client.WorkspaceByOwnerAndName(ctx, owner, name, codersdk.WorkspaceOptions{})
	require.NoError(t, err)
	require.Len(t, ws.LatestBuild.Resources, 1, "expected one resource")
	agentID := ws.LatestBuild.Resources[0].Agents[0].ID
	workDir := ws.LatestBuild.Resources[0].Agents[0].ExpandedDirectory
	require.NotEmpty(t, workDir, "agent has no expanded directory")

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelWarn)
	conn, err := workspacesdk.New(client).DialAgent(ctx, agentID, &workspacesdk.DialAgentOptions{Logger: logger})
	require.NoError(t, err)
	// Registered before the hooks file cleanup so it runs after it.
	t.Cleanup(func() { _ = conn.Close() })
	conn.SetExtraHeaders(http.Header{workspacesdk.CoderChatIDHeader: {uuid.New().String()}})
	tool := chattool.Execute(chattool.ExecuteOptions{
		GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) { return conn, nil },
	})

	hooksDir := path.Join(workDir, ".coder")
	writeFile := func(name, content string) {
		_, err := conn.WriteFile(ctx, path.Join(hooksDir, name), strings.NewReader(content))
		require.NoError(t, err)
	}
	runExecute := func(command string) (chattool.ExecuteResult, fantasy.ToolResponse) {
		resp := runTool(t, tool, map[string]any{"command": command})
		var result chattool.ExecuteResult
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result), resp.Content)
		return result, resp
	}
	removeHooksFile := func() {
		result, _ := runExecute("rm -f " + path.Join(hooksDir, "hooks.json"))
		require.True(t, result.Success, result.Error)
	}
	removeHooksFile()
	t.Cleanup(removeHooksFile)

	const allowHook = "#!/bin/sh\ncat >/dev/null\nprintf '%s' '{\"permission\":{\"decision\":\"allow\"}}'\n"
	writeFile("allow.sh", allowHook)
	result, _ := runExecute("chmod +x " + path.Join(hooksDir, "allow.sh"))
	require.True(t, result.Success, result.Error)

	measure := func(expectHooks int) (wall []time.Duration, hook []time.Duration) {
		runExecute("true")
		for i := 0; i < iterations; i++ {
			start := time.Now()
			result, resp := runExecute("true")
			wall = append(wall, time.Since(start))
			require.True(t, result.Success, result.Error)
			decisions := hooksOf(t, resp)
			require.Len(t, decisions, expectHooks, "agent must run with CODER_AGENT_EXP_WORKSPACE_HOOKS_FILE set")
			var sum time.Duration
			for _, d := range decisions {
				sum += time.Duration(d.DurationMs) * time.Millisecond
			}
			hook = append(hook, sum)
		}
		return wall, hook
	}
	hooksFile := func(n int) string {
		file := agenthooks.File{Version: 1}
		for i := 0; i < n; i++ {
			file.Hooks = append(file.Hooks, agenthooks.Hook{
				Name: fmt.Sprintf("allow-%d", i+1), Event: "pre_tool_use", Matcher: "execute", Command: "./allow.sh",
			})
		}
		data, err := json.Marshal(file)
		require.NoError(t, err)
		return string(data)
	}

	baseWall, _ := measure(0)
	writeFile("hooks.json", hooksFile(0))
	emptyWall, _ := measure(0)
	writeFile("hooks.json", hooksFile(1))
	oneWall, oneHook := measure(1)
	writeFile("hooks.json", hooksFile(3))
	threeWall, threeHook := measure(3)

	t.Log(latencyReport(fmt.Sprintf("execute(\"true\") against %s on %s, %d iterations each", workspace, accessURL, iterations), baseWall, emptyWall, oneWall, threeWall, oneHook, threeHook))
}
