package tailnet_test

import (
	"net"
	"testing"

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
		coordinator := tailnet.NewCoordinator()
		client, server := net.Pipe()
		sendNode, errChan := tailnet.ServeCoordinator(client, func(node []*tailnet.Node) error {
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
		coordinator := tailnet.NewCoordinator()
		client, server := net.Pipe()
		sendNode, errChan := tailnet.ServeCoordinator(client, func(node []*tailnet.Node) error {
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
		coordinator := tailnet.NewCoordinator()

		agentWS, agentServerWS := net.Pipe()
		defer agentWS.Close()
		agentNodeChan := make(chan []*tailnet.Node)
		sendAgentNode, agentErrChan := tailnet.ServeCoordinator(agentWS, func(nodes []*tailnet.Node) error {
			agentNodeChan <- nodes
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

		clientWS, clientServerWS := net.Pipe()
		defer clientWS.Close()
		defer clientServerWS.Close()
		clientNodeChan := make(chan []*tailnet.Node)
		sendClientNode, clientErrChan := tailnet.ServeCoordinator(clientWS, func(nodes []*tailnet.Node) error {
			clientNodeChan <- nodes
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
		clientNodes := <-agentNodeChan
		require.Len(t, clientNodes, 1)

		// Ensure an update to the agent node reaches the client!
		sendAgentNode(&tailnet.Node{})
		agentNodes = <-clientNodeChan
		require.Len(t, agentNodes, 1)

		// Close the agent WebSocket so a new one can connect.
		err := agentWS.Close()
		require.NoError(t, err)
		<-agentErrChan
		<-closeAgentChan

		// Create a new agent connection. This is to simulate a reconnect!
		agentWS, agentServerWS = net.Pipe()
		defer agentWS.Close()
		agentNodeChan = make(chan []*tailnet.Node)
		_, agentErrChan = tailnet.ServeCoordinator(agentWS, func(nodes []*tailnet.Node) error {
			agentNodeChan <- nodes
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
		coordinator := tailnet.NewCoordinator()

		agentWS1, agentServerWS1 := net.Pipe()
		defer agentWS1.Close()
		agentNodeChan1 := make(chan []*tailnet.Node)
		sendAgentNode1, agentErrChan1 := tailnet.ServeCoordinator(agentWS1, func(nodes []*tailnet.Node) error {
			agentNodeChan1 <- nodes
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
		sendClientNode, clientErrChan := tailnet.ServeCoordinator(clientWS, func(nodes []*tailnet.Node) error {
			clientNodeChan <- nodes
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
		_, agentErrChan2 := tailnet.ServeCoordinator(agentWS2, func(nodes []*tailnet.Node) error {
			agentNodeChan2 <- nodes
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
}
