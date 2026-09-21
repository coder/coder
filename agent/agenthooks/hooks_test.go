package agenthooks_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentchat"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/agent/agenthooks"
	"github.com/coder/coder/v2/agent/agentproc"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

// blockNoVerifyHook denies any execute whose command contains
// "--no-verify" and lets everything else through. It reads the
// pre_tool_use request from stdin and writes an agenthooks.Response.
const blockNoVerifyHook = `#!/bin/sh
input=$(cat)
if printf '%s' "$input" | grep -q -- '--no-verify'; then
  printf '%s' '{"permission":{"decision":"deny","reason":"git hooks are mandatory in this repository; run the command without --no-verify"},"model_context":"Fix the failing hook instead of bypassing it."}'
fi
`

func writeHooks(t *testing.T, dir string, file agenthooks.File, scripts map[string]string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".coder"), 0o755))
	data, err := json.Marshal(file)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".coder", "hooks.json"), data, 0o600))
	for name, body := range scripts {
		//nolint:gosec // hook scripts must be executable
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".coder", name), []byte(body), 0o700))
	}
}

func newHandler(t *testing.T, dir string) http.Handler {
	t.Helper()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	workingDir := func() string { return dir }
	api := agentproc.NewAPI(logger, agentexec.DefaultExecer, afero.NewOsFs(), nil, nil, nil, workingDir)
	t.Cleanup(func() { _ = api.Close() })
	runner := agenthooks.New(logger, agentexec.DefaultExecer, afero.NewOsFs(), nil, workingDir, agenthooks.DefaultFileName)
	api.SetPreToolHook(runner.PreToolUse)
	return agentchat.Middleware(api.Routes())
}

func postStart(t *testing.T, handler http.Handler, command string) workspacesdk.StartProcessResponse {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	body, err := json.Marshal(workspacesdk.StartProcessRequest{Command: command})
	require.NoError(t, err)
	w := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(ctx, http.MethodPost, "/start", bytes.NewReader(body))
	r.Header.Set(workspacesdk.CoderChatIDHeader, uuid.New().String())
	handler.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp workspacesdk.StartProcessResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp
}

func TestPreToolUseDeny(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeHooks(t, dir, agenthooks.File{
		Version: 1,
		Hooks: []agenthooks.Hook{{
			Name:    "block-no-verify",
			Event:   "pre_tool_use",
			Matcher: "execute",
			Command: "./block-no-verify.sh",
		}},
	}, map[string]string{"block-no-verify.sh": blockNoVerifyHook})
	handler := newHandler(t, dir)

	denied := postStart(t, handler, "git commit --no-verify -m x")
	require.False(t, denied.Started)
	require.Empty(t, denied.ID)
	require.Len(t, denied.Hooks, 1)
	require.Equal(t, "block-no-verify", denied.Hooks[0].Hook)
	require.Equal(t, "deny", denied.Hooks[0].Decision)
	require.Contains(t, denied.Hooks[0].Reason, "without --no-verify")
	require.Equal(t, "Fix the failing hook instead of bypassing it.", denied.Hooks[0].ModelContext)

	allowed := postStart(t, handler, "echo ok")
	require.True(t, allowed.Started)
	require.NotEmpty(t, allowed.ID)
	require.Len(t, allowed.Hooks, 1)
	require.Equal(t, "allow", allowed.Hooks[0].Decision)
}

func TestPreToolUseFailsClosed(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"crash":     "#!/bin/sh\necho boom >&2\nexit 3\n",
		"malformed": "#!/bin/sh\necho 'not json'\n",
		"unknown":   "#!/bin/sh\nprintf '%s' '{\"permission\":{\"decision\":\"maybe\"}}'\n",
		"missing":   "", // script file not written
	}
	for name, script := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			scripts := map[string]string{}
			if script != "" {
				scripts["hook.sh"] = script
			}
			writeHooks(t, dir, agenthooks.File{
				Version: 1,
				Hooks:   []agenthooks.Hook{{Name: name, Event: "pre_tool_use", Matcher: "execute", Command: "./hook.sh"}},
			}, scripts)
			resp := postStart(t, newHandler(t, dir), "echo ok")
			require.False(t, resp.Started, "a broken guard must not let the command through")
			require.Len(t, resp.Hooks, 1)
			require.Equal(t, "deny", resp.Hooks[0].Decision)
			require.NotEmpty(t, resp.Hooks[0].Reason)
		})
	}
}

func TestNoHooksFile(t *testing.T) {
	t.Parallel()
	resp := postStart(t, newHandler(t, t.TempDir()), "echo ok")
	require.True(t, resp.Started)
	require.Empty(t, resp.Hooks)
}
