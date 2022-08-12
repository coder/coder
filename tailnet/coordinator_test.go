package tailnet_test

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"nhooyr.io/websocket"

	"github.com/coder/coder/tailnet"
	"github.com/coder/coder/testutil"
)

func TestCoordinator(t *testing.T) {
	t.Parallel()
	t.Run("ClientWithoutAgent", func(t *testing.T) {
		t.Parallel()
		coordinator := tailnet.NewCoordinator()
		client, server := pipeWS(t)
		sendNode, errChan := tailnet.ServeCoordinator(context.Background(), client, func(node []*tailnet.Node) {})
		id := uuid.New()
		closeChan := make(chan struct{})
		go func() {
			err := coordinator.ServeClient(context.Background(), server, id, uuid.New())
			assert.NoError(t, err)
			close(closeChan)
		}()
		sendNode(&tailnet.Node{})
		require.Eventually(t, func() bool {
			return coordinator.Node(id) != nil
		}, testutil.WaitShort, testutil.IntervalFast)
		err := client.Close(websocket.StatusNormalClosure, "")
		require.NoError(t, err)
		<-errChan
		<-closeChan
	})

	t.Run("AgentWithoutClients", func(t *testing.T) {
		t.Parallel()
		coordinator := tailnet.NewCoordinator()
		client, server := pipeWS(t)
		sendNode, errChan := tailnet.ServeCoordinator(context.Background(), client, func(node []*tailnet.Node) {})
		id := uuid.New()
		closeChan := make(chan struct{})
		go func() {
			err := coordinator.ServeAgent(context.Background(), server, id)
			assert.NoError(t, err)
			close(closeChan)
		}()
		sendNode(&tailnet.Node{})
		require.Eventually(t, func() bool {
			return coordinator.Node(id) != nil
		}, testutil.WaitShort, testutil.IntervalFast)
		err := client.Close(websocket.StatusNormalClosure, "")
		require.NoError(t, err)
		<-errChan
		<-closeChan
	})

	t.Run("AgentWithClient", func(t *testing.T) {
		t.Parallel()
		coordinator := tailnet.NewCoordinator()

		agentWS, agentServerWS := pipeWS(t)
		defer agentWS.Close(websocket.StatusNormalClosure, "")
		agentNodeChan := make(chan []*tailnet.Node)
		sendAgentNode, agentErrChan := tailnet.ServeCoordinator(context.Background(), agentWS, func(nodes []*tailnet.Node) {
			agentNodeChan <- nodes
		})
		agentID := uuid.New()
		closeAgentChan := make(chan struct{})
		go func() {
			err := coordinator.ServeAgent(context.Background(), agentServerWS, agentID)
			assert.NoError(t, err)
			close(closeAgentChan)
		}()
		sendAgentNode(&tailnet.Node{})
		require.Eventually(t, func() bool {
			return coordinator.Node(agentID) != nil
		}, testutil.WaitShort, testutil.IntervalFast)

		clientWS, clientServerWS := pipeWS(t)
		defer clientWS.Close(websocket.StatusNormalClosure, "")
		defer clientServerWS.Close(websocket.StatusNormalClosure, "")
		clientNodeChan := make(chan []*tailnet.Node)
		sendClientNode, clientErrChan := tailnet.ServeCoordinator(context.Background(), clientWS, func(nodes []*tailnet.Node) {
			clientNodeChan <- nodes
		})
		clientID := uuid.New()
		closeClientChan := make(chan struct{})
		go func() {
			err := coordinator.ServeClient(context.Background(), clientServerWS, clientID, agentID)
			assert.NoError(t, err)
			close(closeClientChan)
		}()
		agentNodes := <-clientNodeChan
		require.Len(t, agentNodes, 1)
		sendClientNode(&tailnet.Node{})
		clientNodes := <-agentNodeChan
		require.Len(t, clientNodes, 1)

		// Ensure an update to the agent node reaches the client!
		sendAgentNode(&tailnet.Node{})
		agentNodes = <-clientNodeChan
		require.Len(t, agentNodes, 1)

		// Close the agent WebSocket so a new one can connect.
		err := agentWS.Close(websocket.StatusNormalClosure, "")
		require.NoError(t, err)
		<-agentErrChan
		<-closeAgentChan

		// Create a new agent connection. This is to simulate a reconnect!
		agentWS, agentServerWS = pipeWS(t)
		defer agentWS.Close(websocket.StatusNormalClosure, "")
		agentNodeChan = make(chan []*tailnet.Node)
		_, agentErrChan = tailnet.ServeCoordinator(context.Background(), agentWS, func(nodes []*tailnet.Node) {
			agentNodeChan <- nodes
		})
		closeAgentChan = make(chan struct{})
		go func() {
			err := coordinator.ServeAgent(context.Background(), agentServerWS, agentID)
			assert.NoError(t, err)
			close(closeAgentChan)
		}()
		// Ensure the existing listening client sends it's node immediately!
		clientNodes = <-agentNodeChan
		require.Len(t, clientNodes, 1)

		err = agentWS.Close(websocket.StatusNormalClosure, "")
		require.NoError(t, err)
		<-agentErrChan
		<-closeAgentChan

		err = clientWS.Close(websocket.StatusNormalClosure, "")
		require.NoError(t, err)
		<-clientErrChan
		<-closeClientChan
	})
}

// pipeWS creates a new piped WebSocket pair.
func pipeWS(t *testing.T) (clientConn, serverConn *websocket.Conn) {
	t.Helper()
	// nolint:bodyclose
	clientConn, _, _ = websocket.Dial(context.Background(), "ws://example.com", &websocket.DialOptions{
		HTTPClient: &http.Client{
			Transport: fakeTransport{
				h: func(w http.ResponseWriter, r *http.Request) {
					serverConn, _ = websocket.Accept(w, r, nil)
				},
			},
		},
	})
	t.Cleanup(func() {
		_ = serverConn.Close(websocket.StatusInternalError, "")
		_ = clientConn.Close(websocket.StatusInternalError, "")
	})
	return clientConn, serverConn
}

type fakeTransport struct {
	h http.HandlerFunc
}

func (t fakeTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	clientConn, serverConn := net.Pipe()
	hj := testHijacker{
		ResponseRecorder: httptest.NewRecorder(),
		serverConn:       serverConn,
	}
	t.h.ServeHTTP(hj, r)
	resp := hj.ResponseRecorder.Result()
	if resp.StatusCode == http.StatusSwitchingProtocols {
		resp.Body = clientConn
	}
	return resp, nil
}

type testHijacker struct {
	*httptest.ResponseRecorder
	serverConn net.Conn
}

var _ http.Hijacker = testHijacker{}

func (hj testHijacker) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return hj.serverConn, bufio.NewReadWriter(bufio.NewReader(hj.serverConn), bufio.NewWriter(hj.serverConn)), nil
}
