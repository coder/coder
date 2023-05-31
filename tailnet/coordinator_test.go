package tailnet_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"nhooyr.io/websocket"
	"tailscale.com/tailcfg"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/tailnet"
	"github.com/coder/coder/testutil"
)

func TestCoordinator(t *testing.T) {
	t.Parallel()
	t.Run("ClientWithoutAgent", func(t *testing.T) {
		t.Parallel()
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		coordinator := tailnet.NewCoordinator(logger, emptyDerpMapFn)
		client, server := net.Pipe()
		sendNode, errChan := tailnet.ServeCoordinator(client, func(_ tailnet.CoordinatorNodeUpdate) error {
			return nil
		})
		id := uuid.New()
		closeChan := make(chan struct{})
		go func() {
			err := coordinator.ServeClient(server, id, uuid.New())
			assert.NoError(t, err)
			close(closeChan)
		}()
		sendNode(&tailnet.Node{})
		require.Eventually(t, func() bool {
			return coordinator.Node(id) != nil
		}, testutil.WaitShort, testutil.IntervalFast)
		require.NoError(t, client.Close())
		require.NoError(t, server.Close())
		<-errChan
		<-closeChan
	})

	t.Run("AgentWithoutClients", func(t *testing.T) {
		t.Parallel()
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		coordinator := tailnet.NewCoordinator(logger, emptyDerpMapFn)
		client, server := net.Pipe()
		sendNode, errChan := tailnet.ServeCoordinator(client, func(_ tailnet.CoordinatorNodeUpdate) error {
			return nil
		})
		id := uuid.New()
		closeChan := make(chan struct{})
		go func() {
			err := coordinator.ServeAgent(server, id, "")
			assert.NoError(t, err)
			close(closeChan)
		}()
		sendNode(&tailnet.Node{})
		require.Eventually(t, func() bool {
			return coordinator.Node(id) != nil
		}, testutil.WaitShort, testutil.IntervalFast)
		err := client.Close()
		require.NoError(t, err)
		<-errChan
		<-closeChan
	})

	t.Run("AgentWithClient", func(t *testing.T) {
		t.Parallel()
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		coordinator := tailnet.NewCoordinator(logger, emptyDerpMapFn)

		// in this test we use real websockets to test use of deadlines
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitSuperLong)
		defer cancel()
		agentWS, agentServerWS := websocketConn(ctx, t)
		defer agentWS.Close()
		agentNodeChan := make(chan []*tailnet.Node)
		sendAgentNode, agentErrChan := tailnet.ServeCoordinator(agentWS, func(update tailnet.CoordinatorNodeUpdate) error {
			agentNodeChan <- update.Nodes
			return nil
		})
		agentID := uuid.New()
		closeAgentChan := make(chan struct{})
		go func() {
			err := coordinator.ServeAgent(agentServerWS, agentID, "")
			assert.NoError(t, err)
			close(closeAgentChan)
		}()
		sendAgentNode(&tailnet.Node{})
		require.Eventually(t, func() bool {
			return coordinator.Node(agentID) != nil
		}, testutil.WaitShort, testutil.IntervalFast)

		clientWS, clientServerWS := websocketConn(ctx, t)
		defer clientWS.Close()
		defer clientServerWS.Close()
		clientNodeChan := make(chan []*tailnet.Node)
		sendClientNode, clientErrChan := tailnet.ServeCoordinator(clientWS, func(update tailnet.CoordinatorNodeUpdate) error {
			clientNodeChan <- update.Nodes
			return nil
		})
		clientID := uuid.New()
		closeClientChan := make(chan struct{})
		go func() {
			err := coordinator.ServeClient(clientServerWS, clientID, agentID)
			assert.NoError(t, err)
			close(closeClientChan)
		}()
		select {
		case agentNodes := <-clientNodeChan:
			require.Len(t, agentNodes, 1)
		case <-ctx.Done():
			t.Fatal("timed out")
		}
		sendClientNode(&tailnet.Node{})
		clientNodes := <-agentNodeChan
		require.Len(t, clientNodes, 1)

		// wait longer than the internal wait timeout.
		// this tests for regression of https://github.com/coder/coder/issues/7428
		time.Sleep(tailnet.WriteTimeout * 3 / 2)

		// Ensure an update to the agent node reaches the client!
		sendAgentNode(&tailnet.Node{})
		select {
		case agentNodes := <-clientNodeChan:
			require.Len(t, agentNodes, 1)
		case <-ctx.Done():
			t.Fatal("timed out")
		}

		// Close the agent WebSocket so a new one can connect.
		err := agentWS.Close()
		require.NoError(t, err)
		<-agentErrChan
		<-closeAgentChan

		// Create a new agent connection. This is to simulate a reconnect!
		agentWS, agentServerWS = net.Pipe()
		defer agentWS.Close()
		agentNodeChan = make(chan []*tailnet.Node)
		_, agentErrChan = tailnet.ServeCoordinator(agentWS, func(update tailnet.CoordinatorNodeUpdate) error {
			agentNodeChan <- update.Nodes
			return nil
		})
		closeAgentChan = make(chan struct{})
		go func() {
			err := coordinator.ServeAgent(agentServerWS, agentID, "")
			assert.NoError(t, err)
			close(closeAgentChan)
		}()
		// Ensure the existing listening client sends it's node immediately!
		clientNodes = <-agentNodeChan
		require.Len(t, clientNodes, 1)

		err = agentWS.Close()
		require.NoError(t, err)
		<-agentErrChan
		<-closeAgentChan

		err = clientWS.Close()
		require.NoError(t, err)
		<-clientErrChan
		<-closeClientChan
	})

	t.Run("AgentDoubleConnect", func(t *testing.T) {
		t.Parallel()
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		coordinator := tailnet.NewCoordinator(logger, emptyDerpMapFn)

		agentWS1, agentServerWS1 := net.Pipe()
		defer agentWS1.Close()
		agentNodeChan1 := make(chan []*tailnet.Node)
		sendAgentNode1, agentErrChan1 := tailnet.ServeCoordinator(agentWS1, func(update tailnet.CoordinatorNodeUpdate) error {
			agentNodeChan1 <- update.Nodes
			return nil
		})
		agentID := uuid.New()
		closeAgentChan1 := make(chan struct{})
		go func() {
			err := coordinator.ServeAgent(agentServerWS1, agentID, "")
			assert.NoError(t, err)
			close(closeAgentChan1)
		}()
		sendAgentNode1(&tailnet.Node{})
		require.Eventually(t, func() bool {
			return coordinator.Node(agentID) != nil
		}, testutil.WaitShort, testutil.IntervalFast)

		clientWS, clientServerWS := net.Pipe()
		defer clientWS.Close()
		defer clientServerWS.Close()
		clientNodeChan := make(chan []*tailnet.Node)
		sendClientNode, clientErrChan := tailnet.ServeCoordinator(clientWS, func(update tailnet.CoordinatorNodeUpdate) error {
			clientNodeChan <- update.Nodes
			return nil
		})
		clientID := uuid.New()
		closeClientChan := make(chan struct{})
		go func() {
			err := coordinator.ServeClient(clientServerWS, clientID, agentID)
			assert.NoError(t, err)
			close(closeClientChan)
		}()
		agentNodes := <-clientNodeChan
		require.Len(t, agentNodes, 1)
		sendClientNode(&tailnet.Node{})
		clientNodes := <-agentNodeChan1
		require.Len(t, clientNodes, 1)

		// Ensure an update to the agent node reaches the client!
		sendAgentNode1(&tailnet.Node{})
		agentNodes = <-clientNodeChan
		require.Len(t, agentNodes, 1)

		// Create a new agent connection without disconnecting the old one.
		agentWS2, agentServerWS2 := net.Pipe()
		defer agentWS2.Close()
		agentNodeChan2 := make(chan []*tailnet.Node)
		_, agentErrChan2 := tailnet.ServeCoordinator(agentWS2, func(update tailnet.CoordinatorNodeUpdate) error {
			agentNodeChan2 <- update.Nodes
			return nil
		})
		closeAgentChan2 := make(chan struct{})
		go func() {
			err := coordinator.ServeAgent(agentServerWS2, agentID, "")
			assert.NoError(t, err)
			close(closeAgentChan2)
		}()

		// Ensure the existing listening client sends it's node immediately!
		clientNodes = <-agentNodeChan2
		require.Len(t, clientNodes, 1)

		counts, ok := coordinator.(interface {
			NodeCount() int
			AgentCount() int
		})
		if !ok {
			t.Fatal("coordinator should have NodeCount() and AgentCount()")
		}

		assert.Equal(t, 2, counts.NodeCount())
		assert.Equal(t, 1, counts.AgentCount())

		err := agentWS2.Close()
		require.NoError(t, err)
		<-agentErrChan2
		<-closeAgentChan2

		err = clientWS.Close()
		require.NoError(t, err)
		<-clientErrChan
		<-closeClientChan

		// This original agent websocket should've been closed forcefully.
		<-agentErrChan1
		<-closeAgentChan1
	})

	t.Run("SendsDERPMap", func(t *testing.T) {
		t.Parallel()
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)

		derpMapFn := func() *tailcfg.DERPMap {
			return &tailcfg.DERPMap{
				Regions: map[int]*tailcfg.DERPRegion{
					1: {
						RegionID: 1,
						Nodes: []*tailcfg.DERPNode{
							{
								Name:     "derp1",
								RegionID: 1,
								HostName: "derp1.example.com",
								// blah
							},
						},
					},
				},
			}
		}

		coordinator := tailnet.NewCoordinator(logger, derpMapFn)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitSuperLong)
		defer cancel()
		agentWS, agentServerWS := websocketConn(ctx, t)
		defer agentWS.Close()
		agentUpdateChan := make(chan tailnet.CoordinatorNodeUpdate)
		sendAgentNode, agentErrChan := tailnet.ServeCoordinator(agentWS, func(update tailnet.CoordinatorNodeUpdate) error {
			agentUpdateChan <- update
			return nil
		})
		agentID := uuid.New()
		closeAgentChan := make(chan struct{})
		go func() {
			err := coordinator.ServeAgent(agentServerWS, agentID, "")
			assert.NoError(t, err)
			close(closeAgentChan)
		}()
		sendAgentNode(&tailnet.Node{})
		require.Eventually(t, func() bool {
			return coordinator.Node(agentID) != nil
		}, testutil.WaitShort, testutil.IntervalFast)

		clientWS, clientServerWS := websocketConn(ctx, t)
		defer clientWS.Close()
		defer clientServerWS.Close()
		clientUpdateChan := make(chan tailnet.CoordinatorNodeUpdate)
		sendClientNode, clientErrChan := tailnet.ServeCoordinator(clientWS, func(update tailnet.CoordinatorNodeUpdate) error {
			clientUpdateChan <- update
			return nil
		})
		clientID := uuid.New()
		closeClientChan := make(chan struct{})
		go func() {
			err := coordinator.ServeClient(clientServerWS, clientID, agentID)
			assert.NoError(t, err)
			close(closeClientChan)
		}()
		select {
		case clientUpdate := <-clientUpdateChan:
			require.Equal(t, derpMapFn(), clientUpdate.DERPMap)
			require.Len(t, clientUpdate.Nodes, 1)
		case <-ctx.Done():
			t.Fatal("timed out")
		}
		sendClientNode(&tailnet.Node{})
		agentUpdate := <-agentUpdateChan
		require.Equal(t, derpMapFn(), agentUpdate.DERPMap)
		require.Len(t, agentUpdate.Nodes, 1)

		// Ensure an update to the agent node reaches the client!
		sendAgentNode(&tailnet.Node{})
		select {
		case clientUpdate := <-clientUpdateChan:
			require.Equal(t, derpMapFn(), clientUpdate.DERPMap)
			require.Len(t, clientUpdate.Nodes, 1)
		case <-ctx.Done():
			t.Fatal("timed out")
		}

		err := agentWS.Close()
		require.NoError(t, err)
		<-agentErrChan
		<-closeAgentChan

		err = clientWS.Close()
		require.NoError(t, err)
		<-clientErrChan
		<-closeClientChan
	})
}

// TestCoordinator_AgentUpdateWhileClientConnects tests for regression on
// https://github.com/coder/coder/issues/7295
func TestCoordinator_AgentUpdateWhileClientConnects(t *testing.T) {
	t.Parallel()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	coordinator := tailnet.NewCoordinator(logger, emptyDerpMapFn)
	agentWS, agentServerWS := net.Pipe()
	defer agentWS.Close()

	agentID := uuid.New()
	go func() {
		err := coordinator.ServeAgent(agentServerWS, agentID, "")
		assert.NoError(t, err)
	}()

	// send an agent update before the client connects so that there is
	// node data available to send right away.
	aNode := tailnet.Node{PreferredDERP: 0}
	aData, err := json.Marshal(&aNode)
	require.NoError(t, err)
	err = agentWS.SetWriteDeadline(time.Now().Add(testutil.WaitShort))
	require.NoError(t, err)
	_, err = agentWS.Write(aData)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return coordinator.Node(agentID) != nil
	}, testutil.WaitShort, testutil.IntervalFast)

	// Connect from the client
	clientWS, clientServerWS := net.Pipe()
	defer clientWS.Close()
	clientID := uuid.New()
	go func() {
		err := coordinator.ServeClient(clientServerWS, clientID, agentID)
		assert.NoError(t, err)
	}()

	// peek one byte from the node update, so we know the coordinator is
	// trying to write to the client.
	buf := make([]byte, 2048)
	err = clientWS.SetReadDeadline(time.Now().Add(testutil.WaitShort))
	require.NoError(t, err)
	n, err := clientWS.Read(buf[:1])
	require.NoError(t, err)
	require.Equal(t, 1, n)

	// send a second update
	aNode.PreferredDERP = 1
	require.NoError(t, err)
	aData, err = json.Marshal(&aNode)
	require.NoError(t, err)
	err = agentWS.SetWriteDeadline(time.Now().Add(testutil.WaitShort))
	require.NoError(t, err)
	_, err = agentWS.Write(aData)
	require.NoError(t, err)

	// read the rest of the update from the client, should be initial node.
	err = clientWS.SetReadDeadline(time.Now().Add(testutil.WaitShort))
	require.NoError(t, err)
	n, err = clientWS.Read(buf[1:])
	require.NoError(t, err)
	var cUpdate tailnet.CoordinatorNodeUpdate
	err = json.Unmarshal(buf[:n+1], &cUpdate)
	require.NoError(t, err)
	require.Len(t, cUpdate.Nodes, 1)
	require.Equal(t, 0, cUpdate.Nodes[0].PreferredDERP)

	// read second update
	// without a fix for https://github.com/coder/coder/issues/7295 our
	// read would time out here.
	err = clientWS.SetReadDeadline(time.Now().Add(testutil.WaitShort))
	require.NoError(t, err)
	buf = make([]byte, 2048)
	n, err = clientWS.Read(buf)
	require.NoError(t, err)
	err = json.Unmarshal(buf[:n], &cUpdate)
	require.NoError(t, err)
	require.Len(t, cUpdate.Nodes, 1)
	require.Equal(t, 1, cUpdate.Nodes[0].PreferredDERP)
}

func websocketConn(ctx context.Context, t *testing.T) (client net.Conn, server net.Conn) {
	t.Helper()
	sc := make(chan net.Conn, 1)
	s := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		wss, err := websocket.Accept(rw, r, nil)
		require.NoError(t, err)
		conn := websocket.NetConn(r.Context(), wss, websocket.MessageBinary)
		sc <- conn
		close(sc) // there can be only one

		// hold open until context canceled
		<-ctx.Done()
	}))
	t.Cleanup(s.Close)
	// nolint: bodyclose
	wsc, _, err := websocket.Dial(ctx, s.URL, nil)
	require.NoError(t, err)
	client = websocket.NetConn(ctx, wsc, websocket.MessageBinary)
	server, ok := <-sc
	require.True(t, ok)
	return client, server
}

func emptyDerpMapFn() *tailcfg.DERPMap {
	return &tailcfg.DERPMap{}
}
