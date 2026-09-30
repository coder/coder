package agentproc

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

func TestTrackedProcessReapedTerminal(t *testing.T) {
	t.Parallel()
	for _, exitCode := range []int{0, 23} {
		t.Run(fmt.Sprint(exitCode), func(t *testing.T) {
			t.Parallel()
			api, server, clock := trackedAPI(t, nil)
			chat := uuid.NewString()
			marker := filepath.Join(t.TempDir(), "count")
			req := trackedRequest(api, fmt.Sprintf("printf x >> \"$RESULT\"; printf actual-output; exit %d", exitCode))
			req.Env = map[string]string{"RESULT": marker}
			req.InputDigest = workspacesdk.StartProcessDigest(req)
			require.Equal(t, http.StatusOK, processRequest(t, server, "/start-tracked", req, nil, chat))
			proc, ok := api.manager.get(uuid.MustParse(chat), req.ProcessID.String())
			require.True(t, ok)
			waitProcessDone(t, proc.done)
			actual := proc.info()
			url := fmt.Sprintf("/%s/output-tracked?agent_instance_id=%s&input_digest=%s", req.ProcessID, req.AgentInstanceID, req.InputDigest)
			var output workspacesdk.ProcessOutputResponse
			require.Equal(t, http.StatusOK, processRequest(t, server, url, nil, &output, chat))
			require.Equal(t, "actual-output", output.Output)
			clock.Advance(exitedProcessReapAge + time.Second).MustWait(testutil.Context(t, testutil.WaitLong))
			require.Empty(t, api.manager.list(uuid.Nil))
			_, ok = api.manager.get(uuid.MustParse(chat), req.ProcessID.String())
			require.False(t, ok, "buffer-bearing process must actually be removed")

			request, err := http.NewRequestWithContext(testutil.Context(t, testutil.WaitLong), http.MethodGet, server.URL+url, nil)
			require.NoError(t, err)
			request.Header.Set(workspacesdk.CoderChatIDHeader, chat)
			response, err := server.Client().Do(request)
			require.NoError(t, err)
			defer response.Body.Close()
			require.Equal(t, http.StatusGone, response.StatusCode)
			var expired workspacesdk.ProcessOutputUnavailableError
			require.NoError(t, json.NewDecoder(response.Body).Decode(&expired))
			require.True(t, expired.OutputUnavailable)
			require.Equal(t, req.AgentInstanceID, expired.AgentInstanceID)
			require.Equal(t, req.InputDigest, expired.InputDigest)
			actual.Command, actual.WorkDir = "", ""
			require.Equal(t, actual, expired.Process)
			require.Equal(t, exitCode, *expired.Process.ExitCode)
			for _, otherChat := range []string{"", uuid.NewString()} {
				require.Equal(t, http.StatusNotFound, processRequest(t, server, url, nil, nil, otherChat))
				require.Equal(t, http.StatusConflict, processRequest(t, server, "/cancel-tracked", cancelRequest(req), nil, otherChat))
			}
			withoutDigest := fmt.Sprintf("/%s/output-tracked?agent_instance_id=%s", req.ProcessID, req.AgentInstanceID)
			require.Equal(t, http.StatusNotFound, processRequest(t, server, withoutDigest, nil, nil, chat))
			require.Equal(t, http.StatusConflict, processRequest(t, server, withoutDigest+"&input_digest=wrong", nil, nil, chat))
			var canceled workspacesdk.CancelProcessResponse
			require.Equal(t, http.StatusOK, processRequest(t, server, "/cancel-tracked", cancelRequest(req), &canceled, chat))
			require.False(t, canceled.Fenced)
			require.Equal(t, &actual, canceled.Process)
			require.Equal(t, http.StatusGone, processRequest(t, server, "/start-tracked", req, nil, chat))
			data, err := os.ReadFile(marker)
			require.NoError(t, err)
			require.Equal(t, "x", string(data), "terminal recovery must never reexecute")

			restarted, restartServer, _ := trackedAPI(t, nil)
			require.Equal(t, http.StatusConflict, processRequest(t, restartServer, url, nil, nil, chat))
			require.Equal(t, http.StatusConflict, processRequest(t, restartServer, "/start-tracked", req, nil, chat))
			currentURL := fmt.Sprintf("/%s/output-tracked?agent_instance_id=%s&input_digest=%s", req.ProcessID, restarted.manager.instanceID, req.InputDigest)
			require.Equal(t, http.StatusNotFound, processRequest(t, restartServer, currentURL, nil, nil, chat))
			require.Empty(t, restarted.manager.receipts, "observation must not fence or invent terminal state")
		})
	}
}
