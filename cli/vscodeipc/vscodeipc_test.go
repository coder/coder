package vscodeipc_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"testing"

	"github.com/google/uuid"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"nhooyr.io/websocket"

	"github.com/coder/coder/agent"
	"github.com/coder/coder/cli/vscodeipc"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/tailnet"
	"github.com/coder/coder/tailnet/tailnettest"
	"github.com/coder/coder/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestVSCodeIPC(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	id := uuid.New()
	derpMap := tailnettest.RunDERPAndSTUN(t)
	coordinator := tailnet.NewCoordinator()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case fmt.Sprintf("/api/v2/workspaceagents/%s/connection", id):
			httpapi.Write(ctx, w, http.StatusOK, codersdk.WorkspaceAgentConnectionInfo{
				DERPMap: derpMap,
			})
			return
		case fmt.Sprintf("/api/v2/workspaceagents/%s/coordinate", id):
			ws, err := websocket.Accept(w, r, nil)
			require.NoError(t, err)
			conn := websocket.NetConn(ctx, ws, websocket.MessageBinary)
			_ = coordinator.ServeClient(conn, uuid.New(), id)
			return
		case "/api/v2/workspaceagents/me/version":
			w.WriteHeader(http.StatusOK)
			return
		case "/api/v2/workspaceagents/me/metadata":
			httpapi.Write(ctx, w, http.StatusOK, codersdk.WorkspaceAgentMetadata{
				DERPMap: derpMap,
			})
			return
		case "/api/v2/workspaceagents/me/coordinate":
			ws, err := websocket.Accept(w, r, nil)
			require.NoError(t, err)
			conn := websocket.NetConn(ctx, ws, websocket.MessageBinary)
			_ = coordinator.ServeAgent(conn, id)
			return
		case "/api/v2/workspaceagents/me/report-stats":
			w.WriteHeader(http.StatusOK)
			return
		case "/":
			w.WriteHeader(http.StatusOK)
			return
		default:
			t.Fatalf("unexpected request %s", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	srvURL, _ := url.Parse(srv.URL)

	client := codersdk.New(srvURL)
	agentConn := agent.New(agent.Options{
		Client:     client,
		Filesystem: afero.NewMemMapFs(),
		TempDir:    t.TempDir(),
	})
	t.Cleanup(func() {
		_ = agentConn.Close()
	})

	handler, closer, err := vscodeipc.New(ctx, client, id, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = closer.Close()
	})

	// Ensure that we're actually connected!
	require.Eventually(t, func() bool {
		res := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/network", nil)
		handler.ServeHTTP(res, req)
		network := &vscodeipc.NetworkResponse{}
		err = json.NewDecoder(res.Body).Decode(&network)
		require.NoError(t, err)
		return network.Latency != 0
	}, testutil.WaitLong, testutil.IntervalFast)

	_, port, err := net.SplitHostPort(srvURL.Host)
	require.NoError(t, err)

	t.Run("Port", func(t *testing.T) {
		// Tests that the port endpoint can be used for forward traffic.
		// For this test, we simply use the already listening httptest server.
		t.Parallel()
		input, output := net.Pipe()
		defer input.Close()
		defer output.Close()
		res := &hijackable{httptest.NewRecorder(), output}
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/port/%s", port), nil)
		go handler.ServeHTTP(res, req)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1/", nil)
		require.NoError(t, err)
		client := http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					return input, nil
				},
			},
		}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("Execute", func(t *testing.T) {
		t.Parallel()
		if runtime.GOOS == "windows" {
			t.Skip("Execute isn't supported on Windows yet!")
			return
		}

		res := httptest.NewRecorder()
		data, _ := json.Marshal(vscodeipc.ExecuteRequest{
			Command: "echo test",
		})
		req := httptest.NewRequest(http.MethodPost, "/execute", bytes.NewReader(data))
		handler.ServeHTTP(res, req)

		decoder := json.NewDecoder(res.Body)
		var msg vscodeipc.ExecuteResponse
		err = decoder.Decode(&msg)
		require.NoError(t, err)
		require.Equal(t, "test\n", msg.Data)
		err = decoder.Decode(&msg)
		require.NoError(t, err)
		require.Equal(t, 0, *msg.ExitCode)
	})
}

type hijackable struct {
	*httptest.ResponseRecorder
	conn net.Conn
}

func (h *hijackable) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return h.conn, bufio.NewReadWriter(bufio.NewReader(h.conn), bufio.NewWriter(h.conn)), nil
}
