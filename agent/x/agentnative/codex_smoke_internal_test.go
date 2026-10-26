//go:build linux

package agentnative

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

type codexSmokeRPC struct {
	codexRPC
	t *testing.T
}

func (r codexSmokeRPC) call(ctx context.Context, method string, params, result any) error {
	if method == "turn/start" {
		r.t.Error("smoke test must not submit prompts")
		return ErrUnsupported
	}
	err := r.codexRPC.call(ctx, method, params, result)
	r.t.Logf("native method %s: %v", method, err)
	return err
}

func codexSmokeOptions(t *testing.T) (BackendOptions, *codexTestHost) {
	t.Helper()
	binary := os.Getenv("CODEX_NATIVE_SMOKE_BINARY")
	if binary == "" {
		t.Skip("set CODEX_NATIVE_SMOKE_BINARY to a pinned Codex executable")
	}
	require.True(t, filepath.IsAbs(binary))
	// Keep Unix socket names below the platform length limit.
	home, err := os.MkdirTemp("", "codex-smoke-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(home) })
	bin := filepath.Join(home, "bin")
	require.NoError(t, os.Mkdir(bin, 0o700))
	require.NoError(t, os.Symlink(binary, filepath.Join(bin, "codex")))
	host := &codexTestHost{sessions: map[string]Session{}, events: map[string][]Event{}}
	options := BackendOptions{Host: host, StateDir: filepath.Join(home, "state"), Environment: []string{"HOME=" + home, "CODEX_HOME=" + filepath.Join(home, "native"), "PATH=" + bin + ":/usr/bin:/bin", "TMPDIR=" + home}}
	require.NoError(t, os.Mkdir(filepath.Join(home, "native"), 0o700))
	return options, host
}

// Opt in with an installed 0.164.0 binary. The isolated HOME/CODEX_HOME and
// explicit environment exclude provider credentials. No prompt is submitted.
func TestCodexInstalledCLISmoke(t *testing.T) {
	t.Parallel()
	options, host := codexSmokeOptions(t)
	home := filepath.Dir(options.StateDir)
	c, err := NewCodex(options)
	require.NoError(t, err)
	factory := c.dial
	c.dial = func(ctx context.Context, notify func(codexMessage), disconnect func()) (codexRPC, error) {
		rpc, err := factory(ctx, notify, disconnect)
		if err != nil {
			return nil, err
		}
		return codexSmokeRPC{codexRPC: rpc, t: t}, nil
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		_ = c.Shutdown(ctx)
	})
	ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
	defer cancel()
	session, err := c.Create(ctx, home)
	require.NoError(t, err)
	host.register(session)
	require.NoError(t, c.Resume(ctx, session))
	command, err := c.Attach(ctx, session)
	require.NoError(t, err)
	require.Equal(t, session.NativeID, command.Args[len(command.Args)-1])
	require.NoError(t, c.Close())
	reopened, err := NewCodex(options)
	require.NoError(t, err)
	require.NoError(t, reopened.Shutdown(ctx), "fresh observer must stop the durable native owner")
	record, err := reopened.readServerRecord()
	require.NoError(t, err)
	require.NotNil(t, record)
	processes, err := codexOwnedProcesses(*record)
	require.NoError(t, err)
	defer codexCloseProcesses(processes)
	require.Empty(t, processes)
	for _, event := range host.list(session.ID) {
		require.Equal(t, "mode", event.Type, "clean closure must not fault the saved session")
	}
}

func TestCodexInstalledCLIColdContainerRestore(t *testing.T) {
	t.Parallel()
	options, host := codexSmokeOptions(t)
	home := filepath.Dir(options.StateDir)
	id := uuid.NewString()
	state, err := json.Marshal(codexState{Version: codexVersion, Requests: map[string]*codexReceipt{}})
	require.NoError(t, err)
	session := Session{ID: "codex:" + id, NativeID: id, Harness: "codex", Cwd: home, State: state, Mode: "human"}
	host.register(session)
	// Use the pinned upstream create_fake_rollout fixture format. This is
	// preexisting native history, not a prompt or an imported resume parameter.
	// No provider is contacted to materialize the test's native rollout.
	rollout := filepath.Join(home, "native", "sessions", "2026", "01", "01", "rollout-2026-01-01T12-00-00-"+id+".jsonl")
	require.NoError(t, os.MkdirAll(filepath.Dir(rollout), 0o700))
	preview := "native history retained across container replacement"
	var lines []string
	for _, entry := range []any{
		map[string]any{"type": "session_meta", "timestamp": "2026-01-01T12:00:00Z", "payload": map[string]any{"id": id, "session_id": id, "timestamp": "2026-01-01T12:00:00Z", "cwd": home, "originator": "codex", "cli_version": codexVersion, "source": "cli", "model_provider": "openai"}},
		map[string]any{"type": "response_item", "timestamp": "2026-01-01T12:00:00Z", "payload": map[string]any{"type": "message", "role": "user", "content": []any{map[string]any{"type": "input_text", "text": preview}}}},
		map[string]any{"type": "event_msg", "timestamp": "2026-01-01T12:00:00Z", "payload": map[string]any{"type": "user_message", "message": preview, "kind": "plain"}},
	} {
		encoded, err := json.Marshal(entry)
		require.NoError(t, err)
		lines = append(lines, string(encoded))
	}
	require.NoError(t, os.WriteFile(rollout, []byte(strings.Join(lines, "\n")+"\n"), 0o600))
	newObserver := func() *Codex {
		c, err := NewCodex(options)
		require.NoError(t, err)
		dial := c.dial
		c.dial = func(ctx context.Context, notify func(codexMessage), disconnect func()) (codexRPC, error) {
			rpc, err := dial(ctx, notify, disconnect)
			if err != nil {
				return nil, err
			}
			return codexSmokeRPC{codexRPC: rpc, t: t}, nil
		}
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()
			_ = c.Shutdown(ctx)
		})
		return c
	}
	ctx := codexTestContext(t)
	first := newObserver()
	require.NoError(t, first.Resume(ctx, session))
	require.NoError(t, first.Shutdown(ctx))
	record, err := first.readServerRecord()
	require.NoError(t, err)
	oldMarker := record.Marker
	// Docker recreation preserves the host boot and native data volume but
	// replaces the PID namespace. Do not reinterpret old PIDs in the new one.
	record.Namespace += "-retired-container"
	require.NoError(t, first.writeServerRecord(*record))
	second := newObserver()
	require.NoError(t, second.Resume(ctx, session))
	_, err = second.Attach(ctx, session)
	require.NoError(t, err)
	current, err := second.readServerRecord()
	require.NoError(t, err)
	require.Equal(t, record.Boot, current.Boot)
	require.NotEqual(t, record.Namespace, current.Namespace)
	require.NotEqual(t, oldMarker, current.Marker)
	data, err := os.ReadFile(rollout)
	require.NoError(t, err)
	require.Contains(t, string(data), preview)
	require.NoError(t, second.Shutdown(ctx))
	for _, event := range host.list(session.ID) {
		require.Equal(t, "mode", event.Type, "restoration must neither fault nor replay native history")
	}
}
