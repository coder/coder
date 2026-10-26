//go:build linux

package agentnative

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/quartz"
)

func TestCodexManagerShutdownRestoresCompletedJob(t *testing.T) {
	t.Parallel()
	ctx := codexTestContext(t)
	dir := filepath.Join(t.TempDir(), "state")
	first := newCodexFixture(t)
	second := newCodexFixture(t)
	newManager := func(f codexTestFixture) *Manager {
		m, err := New(Options{StateDir: dir, Clock: f.clock, Factories: map[string]func(BackendOptions) (Backend, error){
			"codex": func(o BackendOptions) (Backend, error) {
				f.codex.options.Host = o.Host
				f.codex.options.StateDir = o.StateDir
				return f.codex, nil
			},
		}})
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, m.Close()) })
		return m
	}
	base := first.rpc.handler
	turnID := uuid.NewString()
	first.rpc.handler = func(method string, params map[string]any) (any, error) {
		if method == "turn/start" {
			turn := codexTestTurn(turnID, "completed", codexTestUser(params["clientUserMessageId"].(string)))
			first.rpc.emit("turn/completed", map[string]any{"threadId": first.session.NativeID, "turn": turn})
			return map[string]any{"turn": turn}, nil
		}
		return base(method, params)
	}
	m := newManager(first)
	jobSpec := JobSpec{Harness: "codex", Cwd: first.session.Cwd, Prompt: "synthetic unit-test input"}
	job, err := m.RunJob(ctx, jobSpec)
	require.NoError(t, err)
	require.Equal(t, "completed", job.Status)
	before, err := m.Session(job.SessionID)
	require.NoError(t, err)
	require.Empty(t, before.Fault)
	m.mu.Lock()
	requests := m.state.clone().Sessions[job.SessionID].Requests
	m.mu.Unlock()

	// Only a local test child is signaled. The RPC peer remains a test double.
	first.codex.options.Command = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sleep", "3600")
	}
	first.codex.clock = quartz.NewReal()
	require.NoError(t, first.codex.startServer())
	t.Cleanup(func() { _ = first.codex.killOwnedGroup() })
	require.NoError(t, m.Shutdown(ctx))
	after, err := m.Session(job.SessionID)
	require.NoError(t, err)
	require.Empty(t, after.Fault, "intentional shutdown must not poison a completed session")
	require.JSONEq(t, string(before.State), string(after.State))
	require.NoError(t, m.Close())

	// Restore returns native history only as a snapshot, never live events.
	base = second.rpc.handler
	second.rpc.handler = func(method string, params map[string]any) (any, error) {
		switch method {
		case "thread/read":
			return map[string]any{"thread": codexTestThread(before, "idle")}, nil
		case "thread/resume":
			return map[string]any{"thread": codexTestThread(before, "idle"), "initialTurnsPage": map[string]any{"data": []any{codexTestTurn(turnID, "completed", codexTestUser("historic"))}}}, nil
		default:
			return base(method, params)
		}
	}
	reopened := newManager(second)
	require.NoError(t, reopened.Restore(ctx))
	restored, err := reopened.Session(job.SessionID)
	require.NoError(t, err)
	require.Empty(t, restored.Fault)
	require.Equal(t, "automation", restored.Mode)
	require.Equal(t, before.NativeID, restored.NativeID)
	require.JSONEq(t, string(before.State), string(restored.State))
	reopened.mu.Lock()
	restoredState := reopened.state.clone()
	reopened.mu.Unlock()
	require.Equal(t, requests, restoredState.Sessions[job.SessionID].Requests)
	require.Equal(t, job, *restoredState.Job)
	for _, event := range restoredState.Sessions[job.SessionID].Events {
		if event.Seq > before.Cursor {
			require.Equal(t, "mode", event.Type, "restoration must neither fault nor replay native history")
		}
	}
	again, err := reopened.RunJob(ctx, jobSpec)
	require.NoError(t, err)
	require.Equal(t, job, again)
	require.Equal(t, []string{"initialize", "thread/read", "thread/resume"}, second.rpc.methods())
}
