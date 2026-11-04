package agent_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/testutil"
)

func TestWorkloadHandlerAuthenticatesEveryRequest(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	//nolint:dogsled // Only the agent connection is needed for HTTP requests.
	conn, _, _, _, _ := setupAgent(t, agentsdk.Manifest{AgentID: id}, 0, func(_ *agenttest.Client, o *agent.Options) {
		o.WorkloadHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer test-supervisor-token" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = io.WriteString(w, r.URL.Path)
		})
	})
	ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
	defer cancel()
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return conn.DialContext(ctx, "tcp", net.JoinHostPort(tailnet.TailscaleServicePrefix.AddrFromUUID(id).String(), strconv.Itoa(workspacesdk.AgentHTTPAPIServerPort)))
	}}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport}
	chatID, toolCallID := uuid.NewString(), uuid.NewString()
	for _, authorized := range []bool{true, false, true} {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://agent/api/v0/workloads/info", nil)
		require.NoError(t, err)
		req.Header.Set(workspacesdk.CoderChatIDHeader, chatID)
		req.Header.Set(workspacesdk.CoderToolCallIDHeader, toolCallID)
		if authorized {
			req.Header.Set("Authorization", "Bearer test-supervisor-token")
		}
		res, err := client.Do(req)
		require.NoError(t, err)
		body, err := io.ReadAll(res.Body)
		_ = res.Body.Close()
		require.NoError(t, err)
		if authorized {
			require.Equal(t, http.StatusOK, res.StatusCode)
			require.Equal(t, "/info", string(body))
		} else {
			require.Equal(t, http.StatusUnauthorized, res.StatusCode)
		}
	}
}
