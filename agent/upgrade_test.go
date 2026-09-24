package agent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

func TestAgent_SSHUpgrade_Error(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		port    int
		portStr string
		upgrade string
		error   string
		status  int
	}{
		{
			name:   "MissingPort",
			status: http.StatusNotFound,
		},
		{
			name:    "InvalidPort",
			portStr: "invalid",
			status:  http.StatusBadRequest,
			error:   "Invalid port",
		},
		{
			name:   "UnsupportedPort",
			port:   9999,
			status: http.StatusBadRequest,
			error:  "Unsupported port",
		},
		{
			name:   "MissingUpgrade",
			port:   workspacesdk.AgentStandardSSHPort,
			status: http.StatusBadRequest,
			error:  "Missing upgrade header",
		},
		{
			name:    "InvalidUpgrade",
			upgrade: "invalid",
			port:    workspacesdk.AgentStandardSSHPort,
			status:  http.StatusBadRequest,
			error:   "Invalid upgrade header",
		},
	}

	//nolint:dogsled
	agentConn, _, _, _, _ := setupAgent(t, agentsdk.Manifest{}, 0)
	client := &http.Client{
		Transport: &http.Transport{
			DisableKeepAlives: true,
			DialContext:       agentConn.DialContext,
		},
	}
	t.Cleanup(client.CloseIdleConnections)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			port := tc.portStr
			if port == "" && tc.port != 0 {
				port = strconv.Itoa(tc.port)
			}
			url := fmt.Sprintf("http://127.0.0.1:%d/api/v0/tcp/%s",
				workspacesdk.AgentHTTPAPIServerPort, port)
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			require.NoError(t, err)
			if tc.upgrade != "" {
				req.Header.Set("Connection", "Upgrade")
				req.Header.Set("Upgrade", tc.upgrade)
			}

			require.True(t, agentConn.AwaitReachable(ctx))
			res, err := client.Do(req)
			require.NoError(t, err)
			defer res.Body.Close()

			require.Equal(t, tc.status, res.StatusCode)
			if tc.error != "" {
				var apiErr codersdk.Response
				require.NoError(t, json.NewDecoder(res.Body).Decode(&apiErr))
				require.Contains(t, apiErr.Message, tc.error)
			}
		})
	}
}

func TestAgent_SSHUpgrade(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		id     string
		expect string
	}{
		{
			name: "EmptyClientSessionID",
		},
		{
			name: "InvalidClientSessionID",
			id:   "invalid",
		},
		{
			name:   "ValidClientSessionID",
			id:     "0123456789abcdef0123456789abcdef",
			expect: "0123456789abcdef0123456789abcdef",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			//nolint:dogsled
			conn, client, stats, _, _ := setupAgent(t, agentsdk.Manifest{}, 0)
			require.True(t, conn.AwaitReachable(ctx))

			if tc.id != "" {
				conn.SetExtraHeaders(http.Header{
					"baggage": []string{tracing.SessionIDBaggageKey + "=" + tc.id},
				})
			}

			sshClient, err := conn.SSHClient(ctx)
			require.NoError(t, err)
			defer sshClient.Close()
			session, err := sshClient.NewSession()
			require.NoError(t, err)
			defer session.Close()
			stdin, err := session.StdinPipe()
			require.NoError(t, err)
			err = session.Shell()
			require.NoError(t, err)

			// Generate SSH traffic so the connstats window sees the session.
			_, err = stdin.Write([]byte("echo test\n"))
			require.NoError(t, err)

			assertSSHStats(t, stats)
			_, err = stdin.Write([]byte("exit 0\n"))
			require.NoError(t, err, "writing exit to stdin")
			_ = stdin.Close()
			err = session.Wait()
			require.NoError(t, err, "waiting for session to exit")

			assertConnectionReport(t, client, connectionReport{
				connectionType:  proto.Connection_SSH,
				clientSessionID: tc.expect,
			})
		})
	}
}
