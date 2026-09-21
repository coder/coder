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
	"github.com/coder/coder/v2/agent/agentfiles"
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

// rewriteHook replaces the command with one that prints the tool_use_id
// it was given, proving both the override and the ID plumbing.
const rewriteHook = `#!/bin/sh
input=$(cat)
id=$(printf '%s' "$input" | sed -n 's/.*"tool_use_id":"\([^"]*\)".*/\1/p')
printf '{"permission":{"decision":"allow","input_override":{"command":"echo rewritten %s"}}}' "$id"
`

// contextHook allows and attaches model_context and user_message.
const contextHook = `#!/bin/sh
cat >/dev/null
printf '%s' '{"model_context":"Repository note: run make gen after editing SQL.","user_message":"Hooks reminded the agent about make gen."}'
`

// recordHook allows and echoes the tool input it saw into a file, so a
// test can check what a later hook receives.
const recordHook = `#!/bin/sh
cat > "$CODER_HOOK_ROOT/seen-$1.json"
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

type handlers struct {
	proc  http.Handler
	files http.Handler
}

func newHandlers(t *testing.T, dir string) handlers {
	t.Helper()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	workingDir := func() string { return dir }
	fs := afero.NewOsFs()
	runner := agenthooks.New(logger, agentexec.DefaultExecer, fs, nil, workingDir, agenthooks.DefaultFileName)
	proc := agentproc.NewAPI(logger, agentexec.DefaultExecer, fs, nil, nil, nil, workingDir)
	t.Cleanup(func() { _ = proc.Close() })
	proc.SetPreToolHook(runner.PreToolUse)
	files := agentfiles.NewAPI(logger, fs, nil, agentfiles.WithPreToolHook(runner.PreToolUse))
	return handlers{
		proc:  agentchat.Middleware(proc.Routes()),
		files: agentchat.Middleware(files.Routes()),
	}
}

// testChatID is fixed so process output lookups match the chat that
// started the process.
var testChatID = uuid.MustParse("0f1e2d3c-4b5a-6978-8796-a5b4c3d2e1f0")

func do(t *testing.T, handler http.Handler, method, target string, body any) *httptest.ResponseRecorder {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	var reader *bytes.Reader
	switch b := body.(type) {
	case nil:
		reader = bytes.NewReader(nil)
	case string:
		reader = bytes.NewReader([]byte(b))
	default:
		data, err := json.Marshal(b)
		require.NoError(t, err)
		reader = bytes.NewReader(data)
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(ctx, method, target, reader)
	r.Header.Set(workspacesdk.CoderChatIDHeader, testChatID.String())
	r.Header.Set(workspacesdk.CoderToolCallIDHeader, "toolu_test")
	handler.ServeHTTP(w, r)
	return w
}

func decodeDenied(t *testing.T, w *httptest.ResponseRecorder) workspacesdk.HookDecision {
	t.Helper()
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	require.Equal(t, workspacesdk.HookDecisionDeny, w.Header().Get(workspacesdk.CoderWorkspaceHookHeader))
	var resp workspacesdk.HookDeniedResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotEmpty(t, resp.Hooks)
	return (&workspacesdk.HookDeniedError{Hooks: resp.Hooks}).Denied()
}

func startProcess(t *testing.T, h handlers, command string) *httptest.ResponseRecorder {
	t.Helper()
	return do(t, h.proc, http.MethodPost, "/start", workspacesdk.StartProcessRequest{Command: command})
}

func decodeStarted(t *testing.T, w *httptest.ResponseRecorder) workspacesdk.StartProcessResponse {
	t.Helper()
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp workspacesdk.StartProcessResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Started)
	require.NotEmpty(t, resp.ID)
	return resp
}

// waitOutput drains the process output until it exits.
func waitOutput(t *testing.T, h handlers, id string) string {
	t.Helper()
	w := do(t, h.proc, http.MethodGet, "/"+id+"/output?wait=true", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var out workspacesdk.ProcessOutputResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	require.False(t, out.Running, "process should have exited")
	return out.Output
}

func hookFile(hooks ...agenthooks.Hook) agenthooks.File {
	return agenthooks.File{Version: 1, Hooks: hooks}
}

func TestPreToolUseDeny(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeHooks(t, dir, hookFile(agenthooks.Hook{
		Name: "block-no-verify", Event: "pre_tool_use", Matcher: "execute", Command: "./block-no-verify.sh",
	}), map[string]string{"block-no-verify.sh": blockNoVerifyHook})
	h := newHandlers(t, dir)

	denied := decodeDenied(t, startProcess(t, h, "git push --no-verify"))
	require.Equal(t, "block-no-verify", denied.Hook)
	require.Contains(t, denied.Reason, "without --no-verify")
	require.Equal(t, "Fix the failing hook instead of bypassing it.", denied.ModelContext)

	allowed := decodeStarted(t, startProcess(t, h, "echo ok"))
	require.Len(t, allowed.Hooks, 1)
	require.Equal(t, "allow", allowed.Hooks[0].Decision)
}

func TestPreToolUseRewrite(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeHooks(t, dir, hookFile(
		agenthooks.Hook{Name: "rewrite", Event: "pre_tool_use", Matcher: "execute", Command: "./rewrite.sh"},
		agenthooks.Hook{Name: "record", Event: "pre_tool_use", Matcher: "execute", Command: "./record.sh after"},
	), map[string]string{"rewrite.sh": rewriteHook, "record.sh": recordHook})
	h := newHandlers(t, dir)

	resp := decodeStarted(t, startProcess(t, h, "echo original"))
	require.Len(t, resp.Hooks, 2)
	require.JSONEq(t, `{"command":"echo rewritten toolu_test"}`, string(resp.Hooks[0].InputOverride))
	require.Equal(t, "rewritten toolu_test\n", waitOutput(t, h, resp.ID), "the rewritten command must run, not the original")

	// The hook after the rewrite sees the rewritten input.
	seen, err := os.ReadFile(filepath.Join(dir, ".coder", "seen-after.json"))
	require.NoError(t, err)
	require.Contains(t, string(seen), `echo rewritten toolu_test`)
	require.NotContains(t, string(seen), "echo original")
}

func TestPreToolUseContext(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeHooks(t, dir, hookFile(agenthooks.Hook{
		Name: "context", Event: "pre_tool_use", Matcher: "*", Command: "./context.sh",
	}), map[string]string{"context.sh": contextHook})
	h := newHandlers(t, dir)

	resp := decodeStarted(t, startProcess(t, h, "echo ok"))
	require.Len(t, resp.Hooks, 1)
	require.Equal(t, "allow", resp.Hooks[0].Decision)
	require.Equal(t, "Repository note: run make gen after editing SQL.", resp.Hooks[0].ModelContext)
	require.Equal(t, "Hooks reminded the agent about make gen.", resp.Hooks[0].UserMessage)
}

func TestPreToolUseFileTools(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := filepath.Join(dir, "protected.txt")
	require.NoError(t, os.WriteFile(target, []byte("line one\nline two\n"), 0o600))
	// Denies any tool whose input mentions protected.txt.
	writeHooks(t, dir, hookFile(agenthooks.Hook{
		Name: "protect", Event: "pre_tool_use", Matcher: "*", Command: "./protect.sh",
	}), map[string]string{"protect.sh": `#!/bin/sh
input=$(cat)
if printf '%s' "$input" | grep -q 'protected.txt'; then
  printf '%s' '{"permission":{"decision":"deny","reason":"protected.txt is read-only for agents"}}'
fi
`})
	h := newHandlers(t, dir)
	other := filepath.Join(dir, "other.txt")

	t.Run("read_file", func(t *testing.T) {
		t.Parallel()
		denied := decodeDenied(t, do(t, h.files, http.MethodGet, "/read-file-lines?path="+target, nil))
		require.Equal(t, "protect", denied.Hook)

		w := do(t, h.files, http.MethodGet, "/read-file-lines?path="+other, nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp workspacesdk.ReadFileLinesResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.Len(t, resp.Hooks, 1)
		require.Equal(t, "allow", resp.Hooks[0].Decision)
	})

	t.Run("write_file", func(t *testing.T) {
		t.Parallel()
		denied := decodeDenied(t, do(t, h.files, http.MethodPost, "/write-file?path="+target, "overwritten"))
		require.Equal(t, "protect", denied.Hook)
		data, err := os.ReadFile(target)
		require.NoError(t, err)
		require.Equal(t, "line one\nline two\n", string(data), "denied write must not touch the file")

		w := do(t, h.files, http.MethodPost, "/write-file?path="+other, "hello")
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp workspacesdk.WriteFileResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.Len(t, resp.Hooks, 1)
		data, err = os.ReadFile(other)
		require.NoError(t, err)
		require.Equal(t, "hello", string(data))
	})

	t.Run("edit_files", func(t *testing.T) {
		t.Parallel()
		denied := decodeDenied(t, do(t, h.files, http.MethodPost, "/edit-files", workspacesdk.FileEditRequest{
			Files: []workspacesdk.FileEdits{{Path: target, Edits: []workspacesdk.FileEdit{{OldText: "line one", NewText: "LINE ONE"}}}},
		}))
		require.Equal(t, "protect", denied.Hook)
		data, err := os.ReadFile(target)
		require.NoError(t, err)
		require.Equal(t, "line one\nline two\n", string(data), "denied edit must not touch the file")
	})
}

func TestPreToolUseFailsClosed(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"crash":            "#!/bin/sh\necho boom >&2\nexit 3\n",
		"malformed":        "#!/bin/sh\necho 'not json'\n",
		"unknown":          "#!/bin/sh\nprintf '%s' '{\"permission\":{\"decision\":\"maybe\"}}'\n",
		"invalid-override": "#!/bin/sh\nprintf '%s' '{\"permission\":{\"decision\":\"allow\",\"input_override\":\"not an object\"}}'\n",
		"missing":          "", // script file not written
	}
	for name, script := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			scripts := map[string]string{}
			if script != "" {
				scripts["hook.sh"] = script
			}
			writeHooks(t, dir, hookFile(agenthooks.Hook{Name: name, Event: "pre_tool_use", Matcher: "execute", Command: "./hook.sh"}), scripts)
			w := startProcess(t, newHandlers(t, dir), "echo ok")
			if name == "invalid-override" {
				// A string is valid JSON, so the runner accepts it and the
				// execute handler rejects the shape.
				require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
				return
			}
			denied := decodeDenied(t, w)
			require.NotEmpty(t, denied.Reason, "a broken guard must not let the command through")
		})
	}
}

func TestNoHooksFile(t *testing.T) {
	t.Parallel()
	resp := decodeStarted(t, startProcess(t, newHandlers(t, t.TempDir()), "echo ok"))
	require.Empty(t, resp.Hooks)
}
