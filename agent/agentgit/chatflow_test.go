package agentgit_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentchat"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/agent/agentfiles"
	"github.com/coder/coder/v2/agent/agentgit"
	"github.com/coder/coder/v2/agent/agentproc"
	"github.com/coder/coder/v2/agent/usershell"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/wsjson"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/websocket"
)

type chatHomeEnvInfo struct {
	usershell.SystemEnvInfo
	home string
}

func (e chatHomeEnvInfo) HomeDir() (string, error) { return e.home, nil }

// TestChatToolsUnbornRepoFlow drives the sequence a chat produces when
// it creates a repository: execute without a workdir runs git init,
// write_file creates files before the first commit, and a later
// execute commits. It uses the agent's chat middleware, process API,
// files API, and git watch websocket, as chatd does. The repo must be
// reported while HEAD is unborn and again with its new HEAD after the
// commit.
func TestChatToolsUnbornRepoFlow(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}

	home := testutil.TempDirResolved(t)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	ps := agentgit.NewPathStore()
	fs := afero.NewOsFs()
	envInfo := chatHomeEnvInfo{home: home}
	procAPI := agentproc.NewAPI(logger, agentexec.DefaultExecer, fs, ps, envInfo, nil, func() string { return "" })
	t.Cleanup(func() { _ = procAPI.Close() })
	r := chi.NewRouter()
	r.Use(agentchat.Middleware)
	r.Mount("/api/v0", agentfiles.NewAPI(logger, fs, ps, agentfiles.WithEnvInfo(envInfo)).Routes())
	r.Mount("/api/v0/git", agentgit.NewAPI(logger, ps).Routes())
	r.Mount("/api/v0/processes", procAPI.Routes())
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	chatID := uuid.New()
	ctx := testutil.Context(t, testutil.WaitLong)
	do := func(method, path string, body []byte) []byte {
		t.Helper()
		req, err := http.NewRequestWithContext(ctx, method, srv.URL+path, bytes.NewReader(body))
		require.NoError(t, err)
		req.Header.Set(workspacesdk.CoderChatIDHeader, chatID.String())
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		var buf bytes.Buffer
		_, err = buf.ReadFrom(resp.Body)
		require.NoError(t, err)
		require.Less(t, resp.StatusCode, 300, "%s %s: %s", method, path, buf.String())
		return buf.Bytes()
	}
	execute := func(command string) {
		t.Helper()
		body, err := json.Marshal(workspacesdk.StartProcessRequest{Command: command})
		require.NoError(t, err)
		var started workspacesdk.StartProcessResponse
		require.NoError(t, json.Unmarshal(do(http.MethodPost, "/api/v0/processes/start", body), &started))
		var out workspacesdk.ProcessOutputResponse
		require.NoError(t, json.Unmarshal(do(http.MethodGet, "/api/v0/processes/"+started.ID+"/output?wait=true", nil), &out))
		require.NotNil(t, out.ExitCode, "process still running: %s", out.Output)
		require.Zero(t, *out.ExitCode, "%s: %s", command, out.Output)
	}

	// The chat is open, so the watcher is connected throughout.
	wsURL := "ws" + srv.URL[len("http"):] + "/api/v0/git/watch?chat_id=" + chatID.String()
	conn, resp, err := websocket.Dial(ctx, wsURL, nil)
	require.NoError(t, err)
	if resp.Body != nil {
		_ = resp.Body.Close()
	}
	t.Cleanup(func() { _ = conn.Close(websocket.StatusNormalClosure, "") })
	stream := wsjson.NewStream[codersdk.WorkspaceAgentGitServerMessage, codersdk.WorkspaceAgentGitClientMessage](
		conn, websocket.MessageText, websocket.MessageText, logger)
	msgs := stream.Chan()
	demo := filepath.Join(home, "demo")
	waitForRepo := func(match func(codersdk.WorkspaceAgentRepoChanges) bool) codersdk.WorkspaceAgentRepoChanges {
		t.Helper()
		for {
			for _, repo := range recvMsg(ctx, t, msgs).Repositories {
				if repo.RepoRoot == demo && match(repo) {
					return repo
				}
			}
		}
	}

	execute(fmt.Sprintf("mkdir -p %q && cd %q && git init -q", demo, demo))
	for rel, content := range map[string]string{
		".gitignore":    "__pycache__/\n",
		"main.py":       "print('hi')\n",
		"docs/logo.svg": "<svg/>\n",
		"README.md":     "# demo\n",
	} {
		do(http.MethodPost, "/api/v0/write-file?path="+url.QueryEscape(filepath.Join(demo, rel)), []byte(content))
	}

	unborn := waitForRepo(func(repo codersdk.WorkspaceAgentRepoChanges) bool {
		return repo.UnifiedDiff != ""
	})
	require.False(t, unborn.Removed)
	require.Empty(t, unborn.HeadCommit)
	require.Contains(t, unborn.UnifiedDiff, "+++ b/main.py")
	require.Contains(t, unborn.UnifiedDiff, "+++ b/docs/logo.svg")

	// The commit runs in the default directory, as the chat did, so
	// it reaches the watcher through a notification only.
	execute(fmt.Sprintf("cd %q && git checkout -q -b initial-setup && git add . && "+
		"git -c user.name=Test -c user.email=test@test.com commit -qm init", demo))
	committed := waitForRepo(func(repo codersdk.WorkspaceAgentRepoChanges) bool {
		return repo.HeadCommit != ""
	})
	require.Equal(t, "initial-setup", committed.Branch)
	require.Equal(t, gitOutput(t, demo, "rev-parse", "HEAD"), committed.HeadCommit)
	require.Empty(t, committed.UnifiedDiff)
	_, err = os.Stat(filepath.Join(demo, ".git"))
	require.NoError(t, err)
}
