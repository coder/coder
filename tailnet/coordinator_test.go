package tailnet_test

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"nhooyr.io/websocket"

	"github.com/coder/coder/tailnet"
)

func TestCoordinator(t *testing.T) {
	t.Parallel()
	t.Run("Nodes", func(t *testing.T) {
		t.Parallel()
		agentID := uuid.New()
		coordinator := tailnet.NewCoordinator()
		ctx, cancelFunc := context.WithCancel(context.Background())
		client1, client2 := pipeWS(t)
		clientNode := make(chan []*tailnet.Node)
		sendClientNode, clientErrChan := tailnet.Coordinate(ctx, client1, func(node []*tailnet.Node) {
			clientNode <- node
		})
		go coordinator.Client(ctx, agentID, client2)

		agent1, agent2 := pipeWS(t)
		agentNode := make(chan []*tailnet.Node)
		sendAgentNode, agentErrChan := tailnet.Coordinate(ctx, agent1, func(node []*tailnet.Node) {
			agentNode <- node
		})
		go coordinator.Agent(ctx, agentID, agent2)

		sendAgentNode(&tailnet.Node{})
		sendClientNode(&tailnet.Node{})

		<-clientNode
		<-agentNode

		cancelFunc()
		<-clientErrChan
		<-agentErrChan
	})
}

// pipeWS creates a new piped WebSocket pair.
func pipeWS(t *testing.T) (clientConn, serverConn *websocket.Conn) {
	t.Helper()
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
		_ = clientConn.Close(websocket.StatusGoingAway, "")
		_ = serverConn.Close(websocket.StatusGoingAway, "")
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
